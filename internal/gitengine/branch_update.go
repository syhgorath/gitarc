package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Dalı ana dala göre güncelle (merge kuyruğu 2. aşama).
//
// Korumalı depoda "dal güncel olmalı" açıkken bir PR, başka PR birleştikten sonra main'in gerisinde kalır ve birleşemez.
// Güncelleme = ana dalı PR dalının İÇİNE merge etmek (merge commit; rebase ve force-push YOK, kullanıcının merge-commit
// alışkanlığıyla uyumlu). Bu işlem uzak PR dalına PUSH eder (açık bir PR'ı değiştirir ve CI'ı yeniden başlatır); bu yüzden:
//   - önce salt okunur plan; uygulama yalnızca planın parmak iziyle (dalın ucu + ana dalın ucu) çalışır,
//   - uzak dal planla uygulama arasında değişmişse reddedilir (parmak izi tutmaz),
//   - çakışma varsa HİÇBİR ŞEY yapılmaz (çözüm Çakışma Stüdyosu'nda),
//   - çalışma ağacına dokunulmaz (izole git worktree), push normaldir (force yok).

const branchUpdateTimeout = 90 * time.Second

func branchUpdateFingerprint(branch, headSHA, baseSHA string) string {
	sum := sha256.Sum256([]byte(branch + "\x00" + headSHA + "\x00" + baseSHA))
	return hex.EncodeToString(sum[:8])
}

// BranchUpdatePlan: SALT OKUNUR. Hiçbir şey yazmaz (ağa da çıkmaz; yalnızca PR'ın fork olup olmadığını `gh` ile sorar).
func (m *Manager) BranchUpdatePlan(branchID string) (model.BranchUpdatePlan, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.branchUpdatePlanLive(live, branchID)
	}
	if m.NoProject() {
		return model.BranchUpdatePlan{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return simBranchUpdatePlanLocked(&m.simRepo, branchID)
}

// UpdateBranchFromBase: planı uygular (uzak PR dalına PUSH eder). confirm, BranchUpdatePlan'ın Fingerprint'i olmalıdır.
func (m *Manager) UpdateBranchFromBase(branchID, confirm string) (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.updateBranchLive(live, branchID, confirm)
	}
	if m.NoProject() {
		return model.CockpitSnapshot{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, err := simBranchUpdatePlanLocked(&m.simRepo, branchID)
	if err == nil {
		err = checkUpdateApplicable(plan, confirm)
	}
	if err != nil {
		m.recordLogLocked("error", "Dal Güncelleme", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	for i := range m.simRepo.branches {
		if m.simRepo.branches[i].ID == plan.BranchID {
			m.simRepo.branches[i].BehindMain = 0
		}
	}
	m.recordLogLocked("ok", "Dal Güncelleme", plan.Branch, fmt.Sprintf("%s ana dala göre güncellendi (%d commit)", plan.Branch, plan.Behind), "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

func checkUpdateApplicable(plan model.BranchUpdatePlan, confirm string) error {
	switch plan.Status {
	case "up_to_date":
		return fmt.Errorf("%s zaten ana dala göre güncel; yapılacak bir şey yok", plan.Branch)
	case "conflict":
		return fmt.Errorf("%s ana dalla çakışıyor (%s); güncelleme yapılmadı, önce çakışmayı Çakışma Stüdyosu'ndan çözün", plan.Branch, strings.Join(plan.ConflictFiles, ", "))
	case "ready":
	default:
		return fmt.Errorf("%s güncellenemez: %s", plan.Branch, plan.Reason)
	}
	if confirm == "" || confirm != plan.Fingerprint {
		return fmt.Errorf("güncelleme planı değişti veya onaylanmadı (dal ya da ana dal ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey değiştirilmedi")
	}
	return nil
}

func simBranchUpdatePlanLocked(ws *simWorkspace, branchID string) (model.BranchUpdatePlan, error) {
	for _, b := range ws.branches {
		if b.ID != branchID && b.Name != branchID {
			continue
		}
		plan := model.BranchUpdatePlan{Mode: "simulation", Branch: b.Name, BranchID: b.ID, BaseRef: ws.repo.DefaultBranch, Behind: b.BehindMain, Ahead: b.AheadMain}
		if b.PR != nil {
			plan.PR = b.PR.Number
		}
		switch {
		case b.State != "open":
			plan.Status, plan.Reason = "blocked", "dal açık değil (birleşmiş ya da silinmiş)"
		case b.Conflict != nil && b.Conflict.HasConflict:
			plan.Status, plan.Reason, plan.ConflictFiles = "conflict", "ana dalla çakışıyor", append([]string(nil), b.Conflict.Files...)
		case b.BehindMain == 0:
			plan.Status, plan.Reason = "up_to_date", "ana dala göre güncel"
		default:
			plan.Status, plan.Reason = "ready", fmt.Sprintf("ana dalın %d yeni commit'i temiz merge edilir", b.BehindMain)
			plan.Fingerprint = branchUpdateFingerprint(b.Name, fmt.Sprint(b.AheadMain), fmt.Sprint(b.BehindMain))
		}
		return plan, nil
	}
	return model.BranchUpdatePlan{}, fmt.Errorf("dal bulunamadı: %s", branchID)
}

// ---- Canlı ----

func (m *Manager) branchUpdatePlanLive(live *repoEntry, branchID string) (model.BranchUpdatePlan, error) {
	snap, err := m.Snapshot()
	if err != nil {
		return model.BranchUpdatePlan{}, err
	}
	var b *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].ID == branchID || snap.Branches[i].Name == branchID {
			if b == nil || (snap.Branches[i].State == "open" && b.State != "open") {
				b = &snap.Branches[i]
			}
		}
	}
	if b == nil {
		return model.BranchUpdatePlan{}, fmt.Errorf("dal bulunamadı: %s", branchID)
	}
	if err := validateBranchName(b.Name); err != nil {
		return model.BranchUpdatePlan{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), branchUpdateTimeout)
	defer cancel()
	return buildLiveBranchUpdatePlan(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, *b), nil
}

// buildLiveBranchUpdatePlan: ana dalın ucu ile PR dalının UZAKTAKİ ucunu karşılaştırır (güncellemenin gerçek hedefi uzak dal).
func buildLiveBranchUpdatePlan(ctx context.Context, repoPath, defBranch, remoteName string, b model.BranchArc) model.BranchUpdatePlan {
	if defBranch == "" {
		defBranch = "main"
	}
	if remoteName == "" {
		remoteName = "origin"
	}
	plan := model.BranchUpdatePlan{Mode: "live", Branch: b.Name, BranchID: b.ID}
	if b.PR != nil {
		plan.PR = b.PR.Number
	}
	blocked := func(reason string) model.BranchUpdatePlan {
		plan.Status, plan.Reason = "blocked", reason
		return plan
	}
	if b.State != "open" || b.HistoryOnly {
		return blocked("dal açık değil (birleşmiş ya da silinmiş)")
	}
	if b.Name == defBranch {
		return blocked("ana dalın kendisi güncellenemez")
	}
	if !b.HasRemote {
		return blocked("dal uzakta yok; önce push edin (güncelleme uzak PR dalını günceller)")
	}

	baseRef := defBranch
	if _, _, err := runCmd(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/remotes/"+remoteName+"/"+defBranch); err == nil {
		baseRef = remoteName + "/" + defBranch
	}
	plan.BaseRef = baseRef
	headRef := remoteName + "/" + b.Name
	headSHA, herr := revParse(ctx, repoPath, headRef)
	baseSHA, berr := revParse(ctx, repoPath, baseRef)
	if herr != nil || berr != nil {
		return blocked("dalın ya da ana dalın ucu çözümlenemedi (uzak referanslar eski olabilir; `git fetch` gerekebilir)")
	}
	plan.HeadSHA, plan.BaseSHA = headSHA, baseSHA

	if out, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--left-right", "--count", baseSHA+"..."+headSHA); err == nil {
		var behind, ahead int
		if n, _ := fmt.Sscanf(strings.TrimSpace(out), "%d %d", &behind, &ahead); n == 2 {
			plan.Behind, plan.Ahead = behind, ahead
		}
	}
	if plan.Behind == 0 {
		plan.Status, plan.Reason = "up_to_date", "ana dala göre güncel"
		return plan
	}

	out, code, _ := runCmd(ctx, repoPath, "git", "merge-tree", "--write-tree", headSHA, baseSHA)
	switch {
	case ctx.Err() != nil:
		return blocked("simülasyon zaman aşımına uğradı")
	case code == 0:
		plan.Status = "ready"
		plan.Reason = fmt.Sprintf("%s'in %d yeni commit'i temiz merge edilir", baseRef, plan.Behind)
		plan.Fingerprint = branchUpdateFingerprint(b.Name, headSHA, baseSHA)
	case code == 1:
		plan.Status, plan.Reason = "conflict", fmt.Sprintf("%s ile çakışıyor", baseRef)
		if c := ParseMergeTreeOutput(code, out); c != nil {
			plan.ConflictFiles = append([]string(nil), c.Files...)
		}
		return plan
	default:
		return blocked(fmt.Sprintf("simülasyon başarısız (git merge-tree hata kodu %d)", code))
	}

	// Uzak PR dalına yazılabilir mi? (fork PR'ında yazma izni olmayabilir.)
	if plan.PR > 0 {
		if cross, canModify, ok := prWriteAccess(ctx, repoPath, plan.PR); ok && cross && !canModify {
			plan.Status, plan.Reason, plan.Fingerprint = "blocked", "PR bir fork'tan geliyor ve bakımcıların düzenlemesine izin vermiyor; dal güncellenemez", ""
			return plan
		} else if !ok {
			plan.Notes = append(plan.Notes, "PR'ın fork olup olmadığı `gh` ile okunamadı; fork PR'ında push reddedilebilir.")
		}
	}
	if b.HasLocal {
		plan.Notes = append(plan.Notes, "Yerel dal yalnızca uzakla AYNI noktadaysa ilerletilir; yerel-only commit'leriniz varsa dokunulmaz.")
	}
	plan.Notes = append(plan.Notes, "Güncelleme uzak PR dalına merge commit ekler (rebase/force-push yok) ve CI'ı yeniden başlatır.")
	return plan
}

// prWriteAccess: PR bir fork'tan mı geliyor ve bakımcı düzenleyebiliyor mu. ok=false: `gh` okunamadı.
func prWriteAccess(ctx context.Context, repoPath string, pr int) (cross, canModify, ok bool) {
	out, _, err := runCmd(ctx, repoPath, "gh", "pr", "view", fmt.Sprint(pr), "--json", "isCrossRepository,maintainerCanModify")
	if err != nil {
		return false, false, false
	}
	var v struct {
		IsCrossRepository   bool `json:"isCrossRepository"`
		MaintainerCanModify bool `json:"maintainerCanModify"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &v) != nil {
		return false, false, false
	}
	return v.IsCrossRepository, v.MaintainerCanModify, true
}

func (m *Manager) updateBranchLive(live *repoEntry, branchID, confirm string) (model.CockpitSnapshot, error) {
	fail := func(branch string, err error) (model.CockpitSnapshot, error) {
		m.recordLog("error", "Dal Güncelleme", branch, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	snap, err := m.Snapshot()
	if err != nil {
		return fail(branchID, err)
	}
	var b *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].ID == branchID || snap.Branches[i].Name == branchID {
			if b == nil || (snap.Branches[i].State == "open" && b.State != "open") {
				b = &snap.Branches[i]
			}
		}
	}
	if b == nil {
		return fail(branchID, fmt.Errorf("dal bulunamadı: %s", branchID))
	}
	if err := validateBranchName(b.Name); err != nil {
		return fail(b.Name, err)
	}
	remoteName := snap.Repo.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}

	ctx, cancel := context.WithTimeout(context.Background(), branchUpdateTimeout)
	defer cancel()

	// Uzak referansları tazele: plan, uzaktaki GÜNCEL uçlara göre yeniden hesaplanır. Uzak dal planla onay arasında
	// değiştiyse parmak izi tutmaz ve işlem reddedilir.
	if out, err := gitFetchRetry(ctx, live.path, remoteName); err != nil {
		return fail(b.Name, fmt.Errorf("uzak depo okunamadı (git fetch): %s", shortErr(out, err)))
	}
	plan := buildLiveBranchUpdatePlan(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, *b)
	if err := checkUpdateApplicable(plan, confirm); err != nil {
		return fail(b.Name, err)
	}

	wtDir, err := os.MkdirTemp("", "gitarc-upd-*")
	if err != nil {
		return fail(b.Name, err)
	}
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_, _, _ = runCmd(cleanCtx, live.path, "git", "worktree", "remove", "--force", wtDir)
		_ = os.RemoveAll(wtDir)
	}()
	if out, _, err := runCmd(ctx, live.path, "git", "worktree", "add", "--detach", wtDir, plan.HeadSHA); err != nil {
		return fail(b.Name, fmt.Errorf("izole worktree oluşturulamadı: %s", shortErr(out, err)))
	}
	identity := buildGitIdentityArgs(ctx, live.path)
	mergeArgs := append(append([]string{}, identity...), "merge", "--no-edit", "--no-ff", "-m",
		fmt.Sprintf("Merge %s into %s (GitArc merge kuyruğu)", plan.BaseRef, b.Name), plan.BaseSHA)
	if out, _, err := runCmd(ctx, wtDir, "git", mergeArgs...); err != nil {
		_, _, _ = runCmd(ctx, wtDir, "git", "merge", "--abort")
		return fail(b.Name, fmt.Errorf("ana dal dalın içine merge edilemedi, uzak dal DEĞİŞTİRİLMEDİ: %s", shortErr(out, err)))
	}
	newSHA, err := revParse(ctx, wtDir, "HEAD")
	if err != nil {
		return fail(b.Name, fmt.Errorf("yeni commit çözümlenemedi: %v", err))
	}

	// Normal push (force YOK): uzak dal bu arada ilerlediyse git reddeder.
	if out, _, err := runCmd(ctx, wtDir, "git", "push", remoteName, newSHA+":refs/heads/"+b.Name); err != nil {
		return fail(b.Name, fmt.Errorf("güncelleme pushlanamadı, uzak dal DEĞİŞTİRİLMEDİ (uzak dal değişmiş ya da yazma izni yok olabilir): %s", shortErr(out, err)))
	}

	note := ""
	if b.HasLocal {
		note = m.advanceLocalBranch(ctx, live.path, b, plan.HeadSHA, newSHA, snap.Repo.DirtyCount)
	}
	_, _, _ = runCmd(ctx, live.path, "git", "fetch", remoteName, b.Name)
	InvalidateGHCache(live.path)
	m.recordLog("ok", "Dal Güncelleme", b.Name, fmt.Sprintf("%s, %s'e göre güncellendi (%d commit, yeni uç %s)%s", b.Name, plan.BaseRef, plan.Behind, newSHA[:7], note), "")
	return m.Snapshot()
}

// advanceLocalBranch: yerel dal yalnızca uzakla (güncellemeden önceki uç) AYNI noktadaysa ilerletilir; yerel-only
// commit'ler ya da kaydedilmemiş değişiklikler varsa yerel dala dokunulmaz.
func (m *Manager) advanceLocalBranch(ctx context.Context, repoPath string, b *model.BranchArc, oldHead, newSHA string, dirty int) string {
	localSHA, err := revParse(ctx, repoPath, "refs/heads/"+b.Name)
	if err != nil || localSHA != oldHead {
		return " (yerel dal uzakla aynı noktada değildi, dokunulmadı)"
	}
	if b.IsHEAD {
		if dirty > 0 {
			return " (aktif dalda kaydedilmemiş değişiklik var, yerel dal ilerletilmedi)"
		}
		if _, _, err := runCmd(ctx, repoPath, "git", "merge", "--ff-only", newSHA); err != nil {
			return " (yerel dal ilerletilemedi)"
		}
		return ""
	}
	if _, _, err := runCmd(ctx, repoPath, "git", "branch", "-f", b.Name, newSHA); err != nil {
		return " (yerel dal ilerletilemedi)"
	}
	return ""
}
