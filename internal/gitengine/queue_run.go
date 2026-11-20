package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitarc/internal/model"
)

// Merge kuyruğu 3b: OTOMATİK DÖNGÜ ("ilk sorunda dur").
//
// Kullanıcı bir kez onaylar; sunucu, onaylanan PR'ları SIRAYLA işler: her PR için 3a adımlarını (güncelle → CI bekle →
// birleştir) kendisi uygular, sonra sıradakine geçer. Güvenlik çizgisi:
//   - Başlatmak plan parmak iziyle olur (sıralı dal + uç listesi + ana dal ucu); durum değiştiyse hiçbir şey başlamaz.
//   - Her adım 3a'nın kurallarıyla yapılır (parmak izi, `git fetch` + yeniden hesap, `--match-head-commit`, koruma atlatma yok).
//   - Koşu YALNIZCA planın sorunsuz önekini içerir; ilk sorunda (çakışma, kırmızı CI, inceleme engeli, dışarıdan değişen
//     dal ucu, kapanan PR, okunamayan durum, zaman aşımı, değişen proje) durur ve hiçbir şeyi zorlamaz.
//   - CI beklerken zaman aşımı vardır (varsayılan 30 dk); CI bilinmiyorsa yeşil varsayılmaz (3a: "wait").
//   - Uygulama kapanırsa koşu KENDİLİĞİNDEN devam etmez: açılışta "interrupted" görünür, kullanıcı yeniden planlayıp onaylar.
//   - Proje başına tek koşu; `.git/gitarc_queue_run.json` kalıcıdır. Durdurma her zaman serbesttir (güvenliği artırır).

var (
	queueRunPoll = 20 * time.Second
	queueRunWait = 30 * time.Minute
)

const maxQueueRunEvents = 200

// queueRunStaleAfter: dosyada "running" yazan ama bu süreçte çalışmayan bir koşu, bu süredir güncellenmemişse (kalp atışı
// her bekleme turunda yazılır) uygulama kapanmış sayılır. Süre dolmadıysa koşu BAŞKA bir süreçte (örn. MCP sunucusu) sürüyordur.
var queueRunStaleAfter = 5 * time.Minute

// ---- saf döngü çekirdeği (testlenebilir) ----

type queueRunDeps struct {
	plan        func(branchID string) (model.QueueStepPlan, error)
	apply       func(branchID, confirm string) error
	head        func(branchID string) string
	sameProject func() bool
	now         func() time.Time
	sleep       func(ctx context.Context, d time.Duration) bool // false: iptal edildi
	poll, wait  time.Duration
	emit        func(model.QueueRunEvent)
	progress    func(current, merged int)
}

type queueRunOutcome struct{ Status, Reason string }

func runQueueLoop(ctx context.Context, items []model.QueueRunItem, d queueRunDeps) queueRunOutcome {
	expected := map[string]string{}
	for _, it := range items {
		expected[it.BranchID] = it.HeadSHA
	}
	label := func(it model.QueueRunItem) string {
		if it.PR > 0 {
			return fmt.Sprintf("#%d %s", it.PR, it.Branch)
		}
		return it.Branch
	}
	stopped := queueRunOutcome{"stopped", "kullanıcı durdurdu"}
	merged := 0
outer:
	for idx, it := range items {
		d.progress(idx, merged)
		waitStart := d.now()
		lastWait := ""
		for {
			if ctx.Err() != nil {
				return stopped
			}
			if d.sameProject != nil && !d.sameProject() {
				return queueRunOutcome{"blocked", "etkin proje değişti; koşu durduruldu (başka projeye işlem gitmesin)"}
			}
			plan, err := d.plan(it.BranchID)
			if err != nil {
				return queueRunOutcome{"blocked", fmt.Sprintf("%s için plan alınamadı: %v", label(it), err)}
			}
			if exp := expected[it.BranchID]; exp != "" && plan.HeadSHA != "" && plan.HeadSHA != exp {
				return queueRunOutcome{"blocked", fmt.Sprintf("%s dalının ucu onaylanandan farklı (beklenen %s, bulunan %s): biri dışarıdan push etmiş olabilir", label(it), shortSHA(exp), shortSHA(plan.HeadSHA))}
			}
			switch plan.Action {
			case "blocked":
				return queueRunOutcome{"blocked", fmt.Sprintf("%s: %s", label(it), plan.Reason)}
			case "wait":
				if d.now().Sub(waitStart) > d.wait {
					return queueRunOutcome{"timeout", fmt.Sprintf("%s için CI %d dk içinde bitmedi (%s)", label(it), int(d.wait.Minutes()), plan.Reason)}
				}
				if plan.Reason != lastWait {
					lastWait = plan.Reason
					d.emit(model.QueueRunEvent{Kind: "wait", Branch: it.Branch, PR: it.PR, Message: plan.Reason})
				}
				if !d.sleep(ctx, d.poll) {
					return stopped
				}
			case "update", "merge":
				if err := d.apply(it.BranchID, plan.Fingerprint); err != nil {
					return queueRunOutcome{"blocked", fmt.Sprintf("%s adımı uygulanamadı: %v", label(it), err)}
				}
				waitStart, lastWait = d.now(), ""
				if plan.Action == "update" {
					expected[it.BranchID] = d.head(it.BranchID)
					d.emit(model.QueueRunEvent{Kind: "update", Branch: it.Branch, PR: it.PR, Message: fmt.Sprintf("%s main'e göre güncellendi; CI yeniden başladı", label(it))})
					continue
				}
				merged++
				d.emit(model.QueueRunEvent{Kind: "merge", Branch: it.Branch, PR: it.PR, Message: fmt.Sprintf("%s birleştirildi", label(it))})
				continue outer
			default:
				return queueRunOutcome{"blocked", fmt.Sprintf("%s için bilinmeyen adım: %q", label(it), plan.Action)}
			}
		}
	}
	d.progress(len(items), merged)
	return queueRunOutcome{"completed", ""}
}

// ---- plan ----

func queueRunFingerprint(items []model.QueueRunItem, baseSHA string) string {
	parts := []string{baseSHA}
	for _, it := range items {
		parts = append(parts, it.BranchID+"|"+it.HeadSHA)
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// QueueRunPlan: SALT OKUNUR. Verilen sırayla (boşsa tüm açık PR'lar) otomatik koşunun planı.
func (m *Manager) QueueRunPlan(branchIDs []string) (model.QueueRunPlan, error) {
	qp, err := m.QueuePlan(branchIDs)
	if err != nil {
		return model.QueueRunPlan{}, err
	}
	plan := model.QueueRunPlan{
		Mode: qp.Mode, BaseBranch: qp.BaseBranch,
		PollSeconds: int(queueRunPoll / time.Second), TimeoutMinutes: int(queueRunWait / time.Minute),
		Items: []model.QueueRunItem{},
	}
	limit := len(qp.Items)
	if qp.StopsAt > 0 {
		limit = qp.StopsAt - 1
	}
	live, isLive := m.activeLiveRepo()
	remote, base := "origin", qp.BaseBranch
	if isLive {
		if snap, err := m.Snapshot(); err == nil && snap.Repo.RemoteName != "" {
			remote = snap.Repo.RemoteName
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cut := ""
	for i, it := range qp.Items {
		ri := model.QueueRunItem{Position: i + 1, BranchID: it.BranchID, Branch: it.Branch, PR: it.PR, Title: it.Title}
		if isLive && i < limit {
			ri.HeadSHA, _ = revParse(ctx, live.path, remote+"/"+it.Branch)
		}
		if i >= limit {
			plan.Excluded = append(plan.Excluded, ri)
			continue
		}
		if it.PR <= 0 {
			plan.Excluded = append(plan.Excluded, ri)
			if cut == "" {
				cut = fmt.Sprintf("%s açık bir PR'a bağlı değil; koşu PR'sız dalı birleştirmez", it.Branch)
			}
			limit = i
			continue
		}
		plan.Items = append(plan.Items, ri)
	}
	if isLive {
		plan.BaseSHA, _ = revParse(ctx, live.path, remote+"/"+base)
		for _, it := range plan.Items {
			if it.HeadSHA == "" {
				plan.Reason = fmt.Sprintf("%s dalının uzak ucu okunamadı (git fetch gerekebilir); koşu başlatılamaz", it.Branch)
			}
		}
	} else {
		plan.BaseSHA = "sim"
	}
	switch {
	case len(plan.Items) == 0:
		plan.Reason = "kuyruğun ilk PR'ı sorunsuz birleşemiyor; koşu başlatılamaz"
		if cut != "" {
			plan.Reason = cut
		}
	case plan.Reason == "":
		plan.Startable = true
		plan.Fingerprint = queueRunFingerprint(plan.Items, plan.BaseSHA)
	}
	if cut != "" && len(plan.Items) > 0 {
		plan.Notes = append(plan.Notes, cut)
	}
	if len(plan.Excluded) > 0 {
		plan.Notes = append(plan.Notes, fmt.Sprintf("%d PR koşuya girmez (ilk sorunlu PR ve sonrakiler); koşu bitince yeniden planlayın.", len(plan.Excluded)))
	}
	plan.Notes = append(plan.Notes,
		"Her PR için sırayla: gerekirse dalı main'e göre güncelle (uzak PR dalına push) → CI yeşil olana kadar bekle → PR'ı birleştir (--match-head-commit; koruma atlatılmaz).",
		fmt.Sprintf("İlk sorunda durur (çakışma, kırmızı CI, inceleme engeli, dışarıdan değişen dal ucu, zaman aşımı); CI beklemesi en fazla %d dk. CI bilinmiyorsa yeşil varsayılmaz.", plan.TimeoutMinutes),
		"GitArc kapanırsa koşu kendiliğinden devam etmez; yeniden planlayıp onaylarsınız. İstediğiniz an durdurabilirsiniz.")
	return plan, nil
}

// ---- koşu durumu ----

type activeQueueRun struct {
	cancel context.CancelFunc
	done   chan struct{}
}

type queueRunStore struct {
	mu     sync.Mutex
	active map[string]*activeQueueRun
	state  map[string]*model.QueueRun
}

func (m *Manager) qr() *queueRunStore {
	m.qrOnce.Do(func() {
		m.qrStore = &queueRunStore{active: map[string]*activeQueueRun{}, state: map[string]*model.QueueRun{}}
	})
	return m.qrStore
}

func queueRunKey(live *repoEntry, isLive bool) string {
	if isLive {
		return live.path
	}
	return "simulation"
}

func queueRunFile(repoPath string) string {
	return filepath.Join(repoPath, ".git", "gitarc_queue_run.json")
}

func persistQueueRun(live *repoEntry, isLive bool, run *model.QueueRun) {
	if !isLive || run == nil {
		return
	}
	if b, err := json.MarshalIndent(run, "", "  "); err == nil {
		_ = os.WriteFile(queueRunFile(live.path), b, 0o644)
	}
}

func queueRunStale(r *model.QueueRun) bool {
	t, err := time.Parse(time.RFC3339, r.UpdatedAt)
	return err != nil || time.Since(t) > queueRunStaleAfter
}

func copyQueueRun(r *model.QueueRun) *model.QueueRun {
	if r == nil {
		return nil
	}
	c := *r
	c.Items = append([]model.QueueRunItem(nil), r.Items...)
	c.Events = append([]model.QueueRunEvent(nil), r.Events...)
	return &c
}

// QueueRunStatus: etkin projenin son koşusu (yoksa nil). Dosyada "running" kalmış ama çalışan koşu yoksa (uygulama
// kapanmış) "interrupted" olarak işaretlenir; kendiliğinden devam ETMEZ.
func (m *Manager) QueueRunStatus() (*model.QueueRun, error) {
	live, isLive := m.activeLiveRepo()
	if !isLive && m.NoProject() {
		return nil, fmt.Errorf("henüz açık bir proje yok")
	}
	key := queueRunKey(live, isLive)
	s := m.qr()
	s.mu.Lock()
	defer s.mu.Unlock()
	run := s.state[key]
	if run == nil && isLive {
		if b, err := os.ReadFile(queueRunFile(live.path)); err == nil {
			var r model.QueueRun
			if json.Unmarshal(b, &r) == nil && r.ID != "" {
				run = &r
				s.state[key] = run
			}
		}
	}
	if run != nil && run.Status == "running" && s.active[key] == nil && queueRunStale(run) {
		run.Status = "interrupted"
		run.StopReason = "GitArc kapandı ya da yeniden başladı; koşu kendiliğinden devam etmez. Yeniden planlayıp onaylayın."
		run.FinishedAt = time.Now().Format(time.RFC3339)
		persistQueueRun(live, isLive, run)
	}
	return copyQueueRun(run), nil
}

// StartQueueRun: onaylanan planı ARKA PLANDA çalıştırmaya başlar. confirm, QueueRunPlan'ın Fingerprint'i olmalıdır.
func (m *Manager) StartQueueRun(branchIDs []string, confirm string) (*model.QueueRun, error) {
	live, isLive := m.activeLiveRepo()
	if !isLive && m.NoProject() {
		return nil, fmt.Errorf("henüz açık bir proje yok")
	}
	key := queueRunKey(live, isLive)
	s := m.qr()
	s.mu.Lock()
	if s.active[key] != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("bu projede zaten çalışan bir kuyruk koşusu var; önce durdurun ya da bitmesini bekleyin")
	}
	s.mu.Unlock()
	if cur, _ := m.QueueRunStatus(); cur != nil && cur.Status == "running" {
		return nil, fmt.Errorf("bu projede zaten çalışan bir kuyruk koşusu var (başka bir süreçte, örn. MCP; orada durdurun)")
	}

	if isLive {
		// Plan, uzaktaki GÜNCEL uçlara göre yeniden hesaplanır; durum değiştiyse parmak izi tutmaz.
		snap, err := m.Snapshot()
		if err != nil {
			return nil, err
		}
		remote := snap.Repo.RemoteName
		if remote == "" {
			remote = "origin"
		}
		fctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		out, _, ferr := runCmd(fctx, live.path, "git", "fetch", remote)
		cancel()
		if ferr != nil {
			err := fmt.Errorf("uzak depo okunamadı (git fetch): %s", shortErr(out, ferr))
			m.recordLog("error", "Kuyruk Koşusu", "", err.Error(), "")
			return nil, err
		}
	}
	plan, err := m.QueueRunPlan(branchIDs)
	if err == nil && !plan.Startable {
		err = fmt.Errorf("koşu başlatılamaz: %s", plan.Reason)
	}
	if err == nil && (confirm == "" || confirm != plan.Fingerprint) {
		err = fmt.Errorf("koşu planı değişti veya onaylanmadı (dal ucu ya da main ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey başlatılmadı")
	}
	if err != nil {
		m.recordLog("error", "Kuyruk Koşusu", "", err.Error(), "")
		return nil, err
	}

	now := time.Now()
	run := &model.QueueRun{
		ID: fmt.Sprintf("run-%d", now.UnixNano()), Status: "running", Mode: plan.Mode, Items: plan.Items,
		StartedAt: now.Format(time.RFC3339), UpdatedAt: now.Format(time.RFC3339), TimeoutMinutes: plan.TimeoutMinutes,
		Events: []model.QueueRunEvent{{Time: now.Format(time.RFC3339), Kind: "info", Message: fmt.Sprintf("Koşu başladı: %d PR sırayla işlenecek", len(plan.Items))}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	act := &activeQueueRun{cancel: cancel, done: make(chan struct{})}
	s.mu.Lock()
	if s.active[key] != nil { // iki eşzamanlı başlatma isteği
		s.mu.Unlock()
		cancel()
		return nil, fmt.Errorf("bu projede zaten çalışan bir kuyruk koşusu var")
	}
	s.active[key], s.state[key] = act, run
	persistQueueRun(live, isLive, run)
	snapshot := copyQueueRun(run) // goroutine başlamadan, kilit altında kopya: koşu durumunu eşzamanlı değiştirir
	s.mu.Unlock()
	m.recordLog("ok", "Kuyruk Koşusu", "", snapshot.Events[0].Message, "")

	go m.driveQueueRun(ctx, key, live, isLive, run, act)
	return snapshot, nil
}

func (m *Manager) driveQueueRun(ctx context.Context, key string, live *repoEntry, isLive bool, run *model.QueueRun, act *activeQueueRun) {
	s := m.qr()
	defer close(act.done)
	update := func(f func(r *model.QueueRun)) {
		s.mu.Lock()
		f(run)
		run.UpdatedAt = time.Now().Format(time.RFC3339)
		persistQueueRun(live, isLive, run)
		s.mu.Unlock()
	}
	remote := "origin"
	if isLive {
		if snap, err := m.Snapshot(); err == nil && snap.Repo.RemoteName != "" {
			remote = snap.Repo.RemoteName
		}
	}
	byID := map[string]model.QueueRunItem{}
	for _, it := range run.Items {
		byID[it.BranchID] = it
	}
	deps := queueRunDeps{
		plan: func(id string) (model.QueueStepPlan, error) {
			if isLive {
				fctx, cancel := context.WithTimeout(ctx, 60*time.Second)
				defer cancel()
				_, _, _ = runCmd(fctx, live.path, "git", "fetch", remote) // taze uçlar; başarısızlık planda görünür
			}
			return m.QueueStepPlan(id)
		},
		apply: func(id, fp string) error { _, err := m.QueueStepApply(id, fp); return err },
		head: func(id string) string {
			if !isLive {
				return ""
			}
			hctx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			sha, _ := revParse(hctx, live.path, remote+"/"+byID[id].Branch)
			return sha
		},
		sameProject: func() bool {
			cur, ok := m.activeLiveRepo()
			if isLive {
				return ok && sameRepoPath(cur.path, live.path)
			}
			return !ok && !m.NoProject()
		},
		now: time.Now,
		sleep: func(c context.Context, d time.Duration) bool {
			update(func(*model.QueueRun) {}) // kalp atışı: başka süreç "kapandı" sanmasın
			t := time.NewTimer(d)
			defer t.Stop()
			select {
			case <-c.Done():
				return false
			case <-t.C:
				return true
			}
		},
		poll: queueRunPoll, wait: queueRunWait,
		emit: func(ev model.QueueRunEvent) {
			ev.Time = time.Now().Format(time.RFC3339)
			update(func(r *model.QueueRun) {
				r.Events = append(r.Events, ev)
				if len(r.Events) > maxQueueRunEvents {
					r.Events = r.Events[len(r.Events)-maxQueueRunEvents:]
				}
				r.Message = ev.Message
			})
		},
		progress: func(current, merged int) {
			update(func(r *model.QueueRun) { r.Current, r.Merged = current, merged })
		},
	}
	out := runQueueLoop(ctx, run.Items, deps)
	update(func(r *model.QueueRun) {
		r.Status, r.StopReason = out.Status, out.Reason
		r.FinishedAt = time.Now().Format(time.RFC3339)
		msg := map[string]string{"completed": fmt.Sprintf("Koşu bitti: %d PR birleştirildi", r.Merged)}[out.Status]
		if msg == "" {
			msg = fmt.Sprintf("Koşu durdu (%s): %s", out.Status, out.Reason)
		}
		r.Message = msg
		r.Events = append(r.Events, model.QueueRunEvent{Time: r.FinishedAt, Kind: "stop", Message: msg})
	})
	level := "ok"
	if out.Status != "completed" && out.Status != "stopped" {
		level = "error"
	}
	s.mu.Lock()
	finalMsg := run.Message
	s.mu.Unlock()
	m.recordLog(level, "Kuyruk Koşusu", "", finalMsg, "")
	if isLive {
		InvalidateGHCache(live.path)
	}
	s.mu.Lock()
	delete(s.active, key)
	s.mu.Unlock()
}

// StopQueueRun: çalışan koşuyu durdurur (güvenliği artırır; her zaman serbest). Süren bir adım tamamlanır, sonrası yapılmaz.
func (m *Manager) StopQueueRun() (*model.QueueRun, error) {
	live, isLive := m.activeLiveRepo()
	if !isLive && m.NoProject() {
		return nil, fmt.Errorf("henüz açık bir proje yok")
	}
	key := queueRunKey(live, isLive)
	s := m.qr()
	s.mu.Lock()
	act := s.active[key]
	s.mu.Unlock()
	if act == nil {
		if cur, _ := m.QueueRunStatus(); cur != nil && cur.Status == "running" {
			return nil, fmt.Errorf("koşu başka bir süreçte (örn. MCP sunucusu) çalışıyor; oradan durdurun")
		}
		return nil, fmt.Errorf("bu projede çalışan bir kuyruk koşusu yok")
	}
	act.cancel()
	select {
	case <-act.done:
	case <-time.After(5 * time.Second): // süren adım (push/merge) bitene kadar beklemeyiz; durum arka planda kesinleşir
	}
	return m.QueueRunStatus()
}
