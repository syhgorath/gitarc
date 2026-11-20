package gitengine

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Merge kuyruğu planı (1. aşama, SALT OKUNUR).
//
// Soru: "Bu PR'ları sırayla `main`'e birleştirsem sanki tek tek yapılmış gibi sorunsuz biter mi?" Cevap, dalları SIRAYLA
// birleştirilmiş gibi bellekte simüle ederek bulunur: her adımda `git merge-tree --write-tree <şimdiki> <dal>`; temizse
// sonuç ağacından `git commit-tree` ile yalnızca bir nesne (ref'siz) yazılır ve sıradaki dal onun üzerinde denenir.
// Çalışma ağacına, indekse ve hiçbir ref'e dokunulmaz (AGENTS Zero-Touch kuralı).
//
// Sınır (dürüst olmak gerekirse): bu METİN düzeyindeki çakışmayı gösterir; anlamsal uyumsuzluğu (derlenmeyen birleşim,
// kırılan test) ancak güncellenmiş dalın CI'ı yakalar. CI'ın durumu bilinmiyorsa yeşil varsayılmaz.

var reTreeOID = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

const queuePlanTimeout = 60 * time.Second

// QueuePlan: seçilen dalların (boşsa tüm açık PR'ların, PR numarasına göre) kuyruk planı. Sıra verilen sıradır.
func (m *Manager) QueuePlan(branchIDs []string) (model.QueuePlan, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.queuePlanLive(live, branchIDs)
	}
	if m.NoProject() {
		return model.QueuePlan{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	branches := append([]model.BranchArc(nil), m.simRepo.branches...)
	strict := false
	if m.simRepo.protection != nil {
		strict = m.simRepo.protection.StrictUpToDate
	}
	return QueuePlanFromSnapshotData("simulation", m.simRepo.repo.DefaultBranch, branches, branchIDs, &strict)
}

// selectQueueBranches: kuyruğa girecek dalları sırayla seçer. Boşsa tüm açık PR'lar (PR numarasına göre).
func selectQueueBranches(branches []model.BranchArc, ids []string) ([]model.BranchArc, error) {
	open := func(b model.BranchArc) bool { return b.State == "open" && !b.HistoryOnly }
	var out []model.BranchArc
	if len(ids) == 0 {
		for _, b := range branches {
			if open(b) && b.PR != nil && b.PR.Number > 0 {
				out = append(out, b)
			}
		}
		sort.SliceStable(out, func(i, j int) bool { return out[i].PR.Number < out[j].PR.Number })
		return out, nil
	}
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		var found *model.BranchArc
		for i := range branches {
			if branches[i].ID == id || branches[i].Name == id {
				if found == nil || (open(branches[i]) && !open(*found)) {
					found = &branches[i]
				}
			}
		}
		if found == nil {
			return nil, fmt.Errorf("kuyruğa eklenemeyen dal: %q bulunamadı", id)
		}
		if !open(*found) {
			return nil, fmt.Errorf("kuyruğa eklenemeyen dal: %q açık değil (birleşmiş ya da silinmiş)", found.Name)
		}
		out = append(out, *found)
	}
	return out, nil
}

func newQueueItem(i int, b model.BranchArc) model.QueueItem {
	it := model.QueueItem{Position: i + 1, BranchID: b.ID, Branch: b.Name, Behind: b.BehindMain}
	if b.PR != nil {
		it.PR, it.Title, it.URL, it.Review = b.PR.Number, b.PR.Title, b.PR.URL, b.PR.ReviewState
		if b.PR.CI != nil {
			it.CI = b.PR.CI.State
		}
	}
	return it
}

func fileSet(files []string) map[string]bool {
	s := make(map[string]bool, len(files))
	for _, f := range files {
		s[f] = true
	}
	return s
}

func intersect(a []string, b map[string]bool) []string {
	var out []string
	for _, f := range a {
		if b[f] {
			out = append(out, f)
		}
	}
	return out
}

// QueuePlanFromSnapshotData: git'e gerek duymadan, anlık görüntüdeki çakışma bayraklarından plan (simülasyon ve tarayıcı
// yedeği ile AYNI mantık; fallback.js bunun birebir karşılığıdır). Kuyrukta önceki, temiz birleşmiş dallarla ortak dosya
// değiştiren dal "çakışır" sayılır.
func QueuePlanFromSnapshotData(mode, baseBranch string, branches []model.BranchArc, ids []string, strict *bool) (model.QueuePlan, error) {
	sel, err := selectQueueBranches(branches, ids)
	if err != nil {
		return model.QueuePlan{}, err
	}
	items := make([]model.QueueItem, 0, len(sel))
	var merged []model.BranchArc // kuyrukta şimdiye dek temiz birleşmiş dallar
	for i, b := range sel {
		it := newQueueItem(i, b)
		switch {
		case b.Conflict != nil && b.Conflict.HasConflict:
			it.Status, it.ConflictsOnMain, it.ConflictFiles = "conflict", true, append([]string(nil), b.Conflict.Files...)
			it.Reason = fmt.Sprintf("kuyruğa girmeden önce de %s ile çakışıyor", baseBranch)
		default:
			var with []model.BranchArc
			var shared []string
			for _, p := range merged {
				if common := intersect(b.ChangedFiles, fileSet(p.ChangedFiles)); len(common) > 0 {
					with = append(with, p)
					shared = append(shared, common...)
				}
			}
			if len(with) > 0 {
				it.Status, it.ConflictFiles = "conflict", uniqueSorted(shared)
				for _, p := range with {
					it.ConflictsWithBranches = append(it.ConflictsWithBranches, p.Name)
					if p.PR != nil && p.PR.Number > 0 {
						it.ConflictsWith = append(it.ConflictsWith, p.PR.Number)
					}
				}
				it.Reason = "kuyrukta önceki " + describeQueueRefs(with) + " ile aynı dosyaları değiştiriyor"
			} else {
				it.Status, it.Reason = "clean", "önceki kuyruk birleşmiş gibi temiz birleşir"
				merged = append(merged, b)
			}
		}
		items = append(items, it)
	}
	return assembleQueuePlan(mode, baseBranch, items, strict), nil
}

func describeQueueRefs(bs []model.BranchArc) string {
	var parts []string
	for _, b := range bs {
		if b.PR != nil && b.PR.Number > 0 {
			parts = append(parts, fmt.Sprintf("#%d", b.PR.Number))
		} else {
			parts = append(parts, b.Name)
		}
	}
	return strings.Join(parts, ", ")
}

func uniqueSorted(in []string) []string {
	set := fileSet(in)
	out := make([]string, 0, len(set))
	for f := range set {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}

// assembleQueuePlan: üç motorun ortak son adımı: engeli bulur, sayıları ve notları üretir.
func assembleQueuePlan(mode, baseBranch string, items []model.QueueItem, strict *bool) model.QueuePlan {
	plan := model.QueuePlan{Mode: mode, BaseBranch: baseBranch, Items: items, StrictUpToDate: strict}
	if plan.Items == nil {
		plan.Items = []model.QueueItem{}
	}
	for i := range plan.Items {
		if plan.StopsAt == 0 && plan.Items[i].Status != "clean" {
			plan.StopsAt = plan.Items[i].Position
		} else if plan.StopsAt != 0 {
			plan.Items[i].AfterBlocker = true
		}
	}
	plan.Mergeable = len(plan.Items)
	if plan.StopsAt != 0 {
		plan.Mergeable = plan.StopsAt - 1
	}
	plan.CIRounds = 1
	if strict != nil && *strict {
		plan.CIRounds = len(plan.Items)
	}
	if len(plan.Items) == 0 {
		plan.Notes = append(plan.Notes, "Kuyruğa girecek açık PR/dal yok.")
		return plan
	}
	if plan.StopsAt != 0 {
		it := plan.Items[plan.StopsAt-1]
		plan.Notes = append(plan.Notes, fmt.Sprintf("Kuyruk %d. sırada (%s) durur: %s. Ondan sonrakiler, engel aşılana kadar bekler.", plan.StopsAt, queueLabel(it), it.Reason))
	}
	if strict != nil && *strict && len(plan.Items) > 1 {
		plan.Notes = append(plan.Notes, fmt.Sprintf("Korumada “dal güncel olmalı” açık: her birleşmeden sonra sıradaki dal main'e göre güncellenip CI yeniden koşar (≈ %d ardışık CI turu).", plan.CIRounds))
	}
	if strict == nil {
		plan.Notes = append(plan.Notes, "Korumanın “dal güncel olmalı” kuralı okunamadı (yetki yok ya da gh yok); CI tur sayısı tahminidir.")
	}
	for _, it := range plan.Items {
		switch it.CI {
		case "FAILURE":
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: CI kırmızı; birleşmeden önce düzeltilmeli.", queueLabel(it)))
		case "RUNNING":
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: CI sürüyor.", queueLabel(it)))
		case "":
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: CI durumu bilinmiyor (yeşil varsayılmaz).", queueLabel(it)))
		}
		if it.Behind > 0 {
			plan.Notes = append(plan.Notes, fmt.Sprintf("%s: %s'in %d commit gerisinde.", queueLabel(it), baseBranch, it.Behind))
		}
	}
	plan.Notes = append(plan.Notes, "Plan metin düzeyindeki çakışmayı gösterir; anlamsal uyumsuzluğu güncellenmiş dalın CI'ı yakalar. Plan hiçbir şey değiştirmedi.")
	return plan
}

func queueLabel(it model.QueueItem) string {
	if it.PR > 0 {
		return fmt.Sprintf("#%d %s", it.PR, it.Branch)
	}
	return it.Branch
}

// ---- Canlı: sıralı bellek-içi birleşme simülasyonu ----

func (m *Manager) queuePlanLive(live *repoEntry, ids []string) (model.QueuePlan, error) {
	snap, err := m.Snapshot()
	if err != nil {
		return model.QueuePlan{}, err
	}
	sel, err := selectQueueBranches(snap.Branches, ids)
	if err != nil {
		return model.QueuePlan{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), queuePlanTimeout)
	defer cancel()

	var strict *bool
	if read, state := readLiveProtection(ctx, live.path, snap.Repo.DefaultBranch); state == "active" {
		v := read.settings.StrictUpToDate
		strict = &v
	} else if state == "none" {
		f := false
		strict = &f
	}

	items := make([]model.QueueItem, 0, len(sel))
	if len(sel) == 0 {
		return assembleQueuePlan("live", snap.Repo.DefaultBranch, items, strict), nil
	}

	baseRef, _, _, err := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, sel[0])
	if err != nil {
		return model.QueuePlan{}, err
	}
	baseSHA, err := revParse(ctx, live.path, baseRef)
	if err != nil {
		return model.QueuePlan{}, fmt.Errorf("%s çözümlenemedi: %v", baseRef, err)
	}
	identity := buildGitIdentityArgs(ctx, live.path)

	cur := baseSHA
	var merged []model.BranchArc
	for i, b := range sel {
		it := newQueueItem(i, b)
		if err := validateBranchName(b.Name); err != nil {
			it.Status, it.Reason = "unknown", err.Error()
			items = append(items, it)
			continue
		}
		_, branchRef, _, rerr := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, b)
		var branchSHA string
		if rerr == nil {
			branchSHA, rerr = revParse(ctx, live.path, branchRef)
		}
		if rerr != nil {
			it.Status, it.Reason = "unknown", "dalın ucu çözümlenemedi: "+rerr.Error()
			items = append(items, it)
			continue
		}

		out, code, _ := runCmd(ctx, live.path, "git", "merge-tree", "--write-tree", cur, branchSHA)
		switch {
		case ctx.Err() != nil:
			it.Status, it.Reason = "unknown", "simülasyon zaman aşımına uğradı"
		case code == 0:
			tree := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
			if !reTreeOID.MatchString(tree) {
				it.Status, it.Reason = "unknown", "birleşim ağacı okunamadı"
				break
			}
			args := append(append([]string{}, identity...), "commit-tree", tree, "-p", cur, "-p", branchSHA, "-m", "gitarc: merge kuyruğu simülasyonu")
			newSHA, _, cerr := runCmd(ctx, live.path, "git", args...)
			newSHA = strings.TrimSpace(newSHA)
			if cerr != nil || !reTreeOID.MatchString(newSHA) {
				it.Status, it.Reason = "unknown", "simülasyon commit'i yazılamadı"
				break
			}
			cur = newSHA
			it.Status, it.Reason = "clean", "önceki kuyruk birleşmiş gibi temiz birleşir"
			merged = append(merged, b)
		case code == 1:
			c := ParseMergeTreeOutput(code, out)
			it.Status = "conflict"
			if c != nil {
				it.ConflictFiles = append([]string(nil), c.Files...)
			}
			// Kuyruğa girmeden, yalnızca main ile de çakışıyor mu?
			if _, mcode, _ := runCmd(ctx, live.path, "git", "merge-tree", "--write-tree", baseSHA, branchSHA); mcode == 1 {
				it.ConflictsOnMain = true
				it.Reason = fmt.Sprintf("kuyruğa girmeden önce de %s ile çakışıyor", snap.Repo.DefaultBranch)
				break
			}
			var with []model.BranchArc
			for _, p := range merged {
				if len(intersect(it.ConflictFiles, fileSet(p.ChangedFiles))) > 0 {
					with = append(with, p)
				}
			}
			if len(with) == 0 {
				with = merged // dosya eşleşmesi bulunamadı: tüm önceki kuyruk sebep olabilir
			}
			for _, p := range with {
				it.ConflictsWithBranches = append(it.ConflictsWithBranches, p.Name)
				if p.PR != nil && p.PR.Number > 0 {
					it.ConflictsWith = append(it.ConflictsWith, p.PR.Number)
				}
			}
			it.Reason = "kuyrukta önceki " + describeQueueRefs(with) + " birleşince çakışıyor"
		default:
			it.Status, it.Reason = "unknown", "simülasyon başarısız (git merge-tree hata kodu "+fmt.Sprint(code)+")"
		}
		items = append(items, it)
	}
	return assembleQueuePlan("live", snap.Repo.DefaultBranch, items, strict), nil
}

func revParse(ctx context.Context, repoPath, ref string) (string, error) {
	out, _, err := runCmd(ctx, repoPath, "git", "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	sha := strings.TrimSpace(out)
	if err != nil || !reTreeOID.MatchString(sha) {
		return "", fmt.Errorf("geçerli bir commit değil")
	}
	return sha, nil
}
