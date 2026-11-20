package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Merge kuyruğu 3a: ADIM ADIM mod. Kuyruğun bir dalı için "sıradaki adım" planlanır ve onaylanınca TEK adım uygulanır:
//
//	geride mi? → dalı main'e göre güncelle (2. aşama) → CI yeşil mi? bekle → PR'ı birleştir.
//
// Güvenlik kuralları:
//   - Plan salt okunurdur; uygulama yalnızca planın Fingerprint'i (dal ucu + ana dal ucu + eylem + CI durumu) ile çalışır.
//   - Uygulamadan önce `git fetch` + yeniden hesap; durum değiştiyse hiçbir şey yapılmaz.
//   - Birleştirme `gh pr merge --merge --match-head-commit <SHA>` ile yapılır: dala bu arada biri push ederse GitHub reddeder.
//     `--admin` ya da koruma atlatma YOKTUR.
//   - CI bilinmiyorsa yeşil varsayılmaz (UNKNOWN → bekle); çakışma, kırmızı CI, inceleme engeli, taslak PR → "blocked".
//   - Çakışmayı kuyruk çözmez (Çakışma Stüdyosu); tek seferde tek adım (3b döngüsü sonraki aşamadır).

const queueStepTimeout = 120 * time.Second

func queueStepFingerprint(action, branch, headSHA, baseSHA, ci string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{action, branch, headSHA, baseSHA, ci}, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// queueStepInput: karar için gereken ham veriler (canlı: gh + git, simülasyon: bellek).
type queueStepInput struct {
	PRState   string // "OPEN" bekleniyor; boşsa bilinmiyor
	Draft     bool
	GHHeadSHA string // GitHub'ın bildirdiği dal ucu (boşsa karşılaştırılmaz)
	CI        string // SUCCESS | FAILURE | RUNNING | UNKNOWN
	Review    string
	Update    model.BranchUpdatePlan
}

// decideQueueStep: saf karar mantığı (testlenebilir). plan.Mode/Branch/PR dolu gelir.
func decideQueueStep(plan model.QueueStepPlan, in queueStepInput) model.QueueStepPlan {
	up := in.Update
	plan.BaseRef, plan.HeadSHA, plan.BaseSHA, plan.Behind = up.BaseRef, up.HeadSHA, up.BaseSHA, up.Behind
	plan.CI, plan.Review = in.CI, in.Review
	if plan.CI == "" {
		plan.CI = "UNKNOWN"
	}
	block := func(reason string) model.QueueStepPlan {
		plan.Action, plan.Reason, plan.Fingerprint = "blocked", reason, ""
		return plan
	}
	switch {
	case in.PRState != "" && in.PRState != "OPEN":
		return block("PR açık değil (" + in.PRState + ")")
	case in.Draft:
		return block("PR taslak (draft); önce \"Ready for review\" yapın")
	case up.Status == "conflict":
		plan.ConflictFiles = append([]string(nil), up.ConflictFiles...)
		return block(fmt.Sprintf("ana dalla çakışıyor (%s); önce Çakışma Stüdyosu'ndan çözün", strings.Join(up.ConflictFiles, ", ")))
	case up.Status == "blocked":
		return block(up.Reason)
	case in.GHHeadSHA != "" && up.HeadSHA != "" && in.GHHeadSHA != up.HeadSHA:
		return block("GitHub'daki dal ucu yerel uzak kaydından farklı; yenileyip (git fetch) planı yeniden hesaplayın")
	case in.Review == "CHANGES_REQUESTED":
		return block("incelemede değişiklik istendi")
	case in.Review == "REVIEW_REQUIRED":
		return block("inceleme onayı gerekli")
	case plan.CI == "FAILURE":
		return block("CI başarısız; kuyruk durur, önce testleri düzeltin")
	}
	if up.Status == "ready" {
		plan.Action = "update"
		plan.Reason = fmt.Sprintf("main'in %d yeni commit'i dalın içine merge edilip uzak PR dalına push edilir; CI yeniden başlar", up.Behind)
		plan.Fingerprint = queueStepFingerprint("update", plan.Branch, up.HeadSHA, up.BaseSHA, plan.CI)
		return plan
	}
	if up.Status != "up_to_date" {
		return block("dal durumu belirlenemedi: " + up.Reason)
	}
	switch plan.CI {
	case "SUCCESS":
		plan.Action = "merge"
		plan.Reason = "dal main'e göre güncel ve CI yeşil; PR birleştirilebilir (merge commit)"
		plan.Fingerprint = queueStepFingerprint("merge", plan.Branch, up.HeadSHA, up.BaseSHA, plan.CI)
	case "RUNNING":
		plan.Action, plan.Reason = "wait", "CI sürüyor; bitince bu adım yeniden planlanır"
	default:
		plan.Action, plan.Reason = "wait", "CI durumu bilinmiyor (kontrol yok ya da henüz başlamadı); yeşil varsayılmaz"
	}
	return plan
}

// rollupCIState: gh `statusCheckRollup` → SUCCESS | FAILURE | RUNNING | UNKNOWN (tanınmayan/boş = UNKNOWN, asla SUCCESS değil).
func rollupCIState(checks []ghRollupCheck) string {
	known, pending, unrecognised := 0, 0, 0
	for _, c := range checks {
		conc := strings.ToUpper(c.Conclusion)
		st := strings.ToUpper(c.Status)
		state := strings.ToUpper(c.State)
		if conc == "" && state != "" {
			conc = state
		}
		if conc == "" && st == "COMPLETED" {
			unrecognised++ // tamamlanmış ama sonucu yok: başarı sayılmaz
			continue
		}
		switch conc {
		case "FAILURE", "TIMED_OUT", "CANCELLED", "ERROR", "STARTUP_FAILURE", "ACTION_REQUIRED":
			return "FAILURE"
		case "SUCCESS", "NEUTRAL", "SKIPPED":
			known++
		case "PENDING", "EXPECTED":
			pending++
		case "":
			if st != "" {
				pending++
			}
		default:
			unrecognised++
		}
	}
	switch {
	case pending > 0:
		return "RUNNING"
	case unrecognised > 0 || known == 0:
		return "UNKNOWN"
	}
	return "SUCCESS"
}

type ghRollupCheck struct {
	Status     string `json:"status"`
	State      string `json:"state"`
	Conclusion string `json:"conclusion"`
}

type ghPRStepView struct {
	State             string          `json:"state"`
	IsDraft           bool            `json:"isDraft"`
	HeadRefOid        string          `json:"headRefOid"`
	ReviewDecision    string          `json:"reviewDecision"`
	StatusCheckRollup []ghRollupCheck `json:"statusCheckRollup"`
}

// readPRStepView: PR'ın GÜNCEL durumunu `gh pr view` ile okur (önbelleksiz). ok=false: okunamadı.
func readPRStepView(ctx context.Context, repoPath string, pr int) (ghPRStepView, bool) {
	var v ghPRStepView
	if _, err := exec.LookPath("gh"); err != nil {
		return v, false
	}
	out, _, err := runCmd(ctx, repoPath, "gh", "pr", "view", fmt.Sprint(pr), "--json", "state,isDraft,headRefOid,reviewDecision,statusCheckRollup")
	if err != nil || json.Unmarshal([]byte(strings.TrimSpace(out)), &v) != nil {
		return v, false
	}
	return v, true
}

func buildLiveQueueStepPlan(ctx context.Context, repoPath, defBranch, remoteName string, b model.BranchArc) model.QueueStepPlan {
	up := buildLiveBranchUpdatePlan(ctx, repoPath, defBranch, remoteName, b)
	plan := model.QueueStepPlan{Mode: "live", Branch: b.Name, BranchID: b.ID}
	if b.PR != nil {
		plan.PR = b.PR.Number
	}
	if plan.PR <= 0 {
		plan.Action, plan.Reason, plan.CI = "blocked", "bu dalın açık bir PR'ı yok", "UNKNOWN"
		plan.BaseRef = up.BaseRef
		return plan
	}
	view, ok := readPRStepView(ctx, repoPath, plan.PR)
	if !ok {
		plan.Action, plan.CI = "blocked", "UNKNOWN"
		plan.Reason = "PR durumu GitHub'dan okunamadı (gh); yeşil varsayılmaz"
		plan.BaseRef = up.BaseRef
		return plan
	}
	return decideQueueStep(plan, queueStepInput{
		PRState: strings.ToUpper(view.State), Draft: view.IsDraft, GHHeadSHA: view.HeadRefOid,
		CI: rollupCIState(view.StatusCheckRollup), Review: strings.ToUpper(view.ReviewDecision), Update: up,
	})
}

func checkQueueStepApplicable(plan model.QueueStepPlan, confirm string) error {
	switch plan.Action {
	case "update", "merge":
	case "wait":
		return fmt.Errorf("%s için şimdilik yapılacak bir şey yok: %s", plan.Branch, plan.Reason)
	default:
		return fmt.Errorf("%s için adım uygulanamaz: %s", plan.Branch, plan.Reason)
	}
	if confirm == "" || confirm != plan.Fingerprint {
		return fmt.Errorf("kuyruk adımı planı değişti veya onaylanmadı (dal, main ya da CI durumu ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey değiştirilmedi")
	}
	return nil
}

// QueueStepPlan: SALT OKUNUR. Kuyruğun bu dalı için sıradaki adımı.
func (m *Manager) QueueStepPlan(branchID string) (model.QueueStepPlan, error) {
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.Snapshot()
		if err != nil {
			return model.QueueStepPlan{}, err
		}
		b := pickOpenBranch(snap.Branches, branchID)
		if b == nil {
			return model.QueueStepPlan{}, fmt.Errorf("dal bulunamadı: %s", branchID)
		}
		if err := validateBranchName(b.Name); err != nil {
			return model.QueueStepPlan{}, err
		}
		ctx, cancel := context.WithTimeout(context.Background(), queueStepTimeout)
		defer cancel()
		return buildLiveQueueStepPlan(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, *b), nil
	}
	if m.NoProject() {
		return model.QueueStepPlan{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return simQueueStepPlanLocked(&m.simRepo, branchID)
}

func pickOpenBranch(branches []model.BranchArc, branchID string) *model.BranchArc {
	var b *model.BranchArc
	for i := range branches {
		if branches[i].ID == branchID || branches[i].Name == branchID {
			if b == nil || (branches[i].State == "open" && b.State != "open") {
				b = &branches[i]
			}
		}
	}
	return b
}

// QueueStepApply: planın TEK adımını uygular (güncelle: uzak PR dalına push; birleştir: PR'ı main'e merge eder).
// confirm, QueueStepPlan'ın Fingerprint'i olmalıdır.
func (m *Manager) QueueStepApply(branchID, confirm string) (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.queueStepApplyLive(live, branchID, confirm)
	}
	if m.NoProject() {
		return model.CockpitSnapshot{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.RLock()
	plan, err := simQueueStepPlanLocked(&m.simRepo, branchID)
	var upd model.BranchUpdatePlan
	if err == nil {
		err = checkQueueStepApplicable(plan, confirm)
		upd, _ = simBranchUpdatePlanLocked(&m.simRepo, branchID)
	}
	m.mu.RUnlock()
	if err != nil {
		m.recordLog("error", "Kuyruk Adımı", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if plan.Action == "update" {
		return m.UpdateBranchFromBase(plan.BranchID, upd.Fingerprint)
	}
	snap, err := m.MergeBranch(plan.BranchID, "merge")
	if err != nil {
		return snap, err
	}
	// Gerçek GitHub gibi: bir PR birleşince diğer açık PR'lar main'in gerisinde kalır.
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.simRepo.branches {
		if o := &m.simRepo.branches[i]; o.State == "open" && o.ID != plan.BranchID {
			o.BehindMain++
		}
	}
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

func simQueueStepPlanLocked(ws *simWorkspace, branchID string) (model.QueueStepPlan, error) {
	up, err := simBranchUpdatePlanLocked(ws, branchID)
	if err != nil {
		return model.QueueStepPlan{}, err
	}
	plan := model.QueueStepPlan{Mode: "simulation", Branch: up.Branch, BranchID: up.BranchID, PR: up.PR}
	if up.PR == 0 {
		plan.Action, plan.Reason, plan.CI, plan.BaseRef = "blocked", "bu dalın açık bir PR'ı yok", "UNKNOWN", up.BaseRef
		return plan, nil
	}
	ci := "UNKNOWN"
	for _, b := range ws.branches {
		if b.ID == up.BranchID && b.PR != nil && b.PR.CI != nil {
			switch b.PR.CI.State {
			case "SUCCESS", "FAILURE":
				ci = b.PR.CI.State
			default:
				ci = "RUNNING"
			}
		}
	}
	// Simülasyondaki PR'lar inceleme durumunu sabit taşır (gerçek inceleme akışı yok): inceleme engeli uygulanmaz.
	plan = decideQueueStep(plan, queueStepInput{PRState: "OPEN", CI: ci, Update: up})
	plan.Notes = append(plan.Notes, "Simülasyon: inceleme durumu değerlendirilmez.")
	return plan, nil
}

func (m *Manager) queueStepApplyLive(live *repoEntry, branchID, confirm string) (model.CockpitSnapshot, error) {
	fail := func(branch string, err error) (model.CockpitSnapshot, error) {
		m.recordLog("error", "Kuyruk Adımı", branch, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	snap, err := m.Snapshot()
	if err != nil {
		return fail(branchID, err)
	}
	b := pickOpenBranch(snap.Branches, branchID)
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
	ctx, cancel := context.WithTimeout(context.Background(), queueStepTimeout)
	defer cancel()

	// Uzak referansları tazele ve planı GÜNCEL duruma göre yeniden hesapla; durum değiştiyse parmak izi tutmaz.
	if out, err := gitFetchRetry(ctx, live.path, remoteName); err != nil {
		return fail(b.Name, fmt.Errorf("uzak depo okunamadı (git fetch): %s", shortErr(out, err)))
	}
	plan := buildLiveQueueStepPlan(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, *b)
	if err := checkQueueStepApplicable(plan, confirm); err != nil {
		return fail(b.Name, err)
	}

	if plan.Action == "update" {
		up := buildLiveBranchUpdatePlan(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, *b)
		out, err := m.updateBranchLive(live, b.ID, up.Fingerprint)
		if err != nil {
			return model.CockpitSnapshot{}, err
		}
		m.recordLog("ok", "Kuyruk Adımı", b.Name, fmt.Sprintf("#%d güncellendi; CI yeniden başladı, bitince sıradaki adım birleştirmedir", plan.PR), "")
		return out, nil
	}

	// merge: dalın ucu planla AYNI olmalı (--match-head-commit), koruma atlatma yok.
	out, _, err := runCmd(ctx, live.path, "gh", "pr", "merge", fmt.Sprint(plan.PR), "--merge", "--match-head-commit", plan.HeadSHA)
	if err != nil {
		return fail(b.Name, fmt.Errorf("PR #%d birleştirilemedi, hiçbir şey değiştirilmedi: %s", plan.PR, shortErr(out, err)))
	}
	defBranch := snap.Repo.DefaultBranch
	if defBranch == "" {
		defBranch = "main"
	}
	_ = syncDefaultBranchNonDestructive(ctx, live.path, defBranch)
	InvalidateGHCache(live.path)
	m.recordLog("ok", "Kuyruk Adımı", b.Name, fmt.Sprintf("PR #%d birleştirildi (merge commit, dal ucu %s)", plan.PR, shortSHA(plan.HeadSHA)), "")
	return m.Snapshot()
}

// gitFetchRetry: `git fetch` aynı depoda eşzamanlı çalışan başka bir fetch'le (arka plan senkronu, kullanıcının kendi fetch'i)
// uzak-izleme ref kilidi yüzünden geçici olarak başarısız olabilir ("cannot lock ref"). Otomatik koşu bu yüzden DURMAMALI:
// kısa aralıklarla birkaç kez yeniden dener; kalıcı hata (ağ, yetki) yine de ilk denemenin çıktısıyla bildirilir.
func gitFetchRetry(ctx context.Context, dir, remote string) (string, error) {
	var out string
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		if out, _, err = runCmd(ctx, dir, "git", "fetch", remote); err == nil {
			return out, nil
		}
		select {
		case <-ctx.Done():
			return out, err
		case <-time.After(time.Duration(attempt+1) * 250 * time.Millisecond):
		}
	}
	return out, err
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}
