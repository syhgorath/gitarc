package gitengine

import (
	"testing"
	"time"

	"gitarc/internal/model"
)

// Süren kuyruk koşusu varken arayüzde BAŞKA projeye geçmek koşuyu durdurmamalı ve işlem yanlış projeye gitmemeli: koşu kendi projesine
// sabitlenir (pinnedFor). Eskiden "etkin proje değişti; koşu durduruldu" diye kesilirdi.
func TestQueueRunKeepsGoingWhenTheUserSwitchesToAnotherProject(t *testing.T) {
	fastRun(t)
	w := newStepWorld(t)
	_ = w.fakeGitHub(map[string]int{"feat/x": 1, "feat/y": 2})
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	waitPR(t, mgr, "feat/y")
	time.Sleep(300 * time.Millisecond)

	ids := []string{"feat/x", "feat/y"}
	plan, err := mgr.QueueRunPlan(ids)
	if err != nil || !plan.Startable {
		t.Fatalf("plan: %+v %v", plan, err)
	}
	liveA, _ := mgr.activeLiveRepo()
	keyA := queueRunKey(liveA, true)
	idA := liveA.id

	if _, err := mgr.StartQueueRun(ids, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	// Koşu sürerken kullanıcı BAŞKA bir projeye geçer (sunucuda etkin proje değişir).
	dirB, _ := newTempRepo(t)
	if _, err := mgr.AddProject(dirB, "b"); err != nil {
		t.Fatal(err)
	}
	if mgr.ActiveRepoID() == idA {
		t.Fatal("ön koşul: etkin proje B olmalı")
	}

	var run = func() (st string, merged int, reason string) {
		r, _ := mgr.queueRunStateFor(keyA, liveA.path, true)
		if r == nil {
			return "", 0, ""
		}
		return r.Status, r.Merged, r.StopReason
	}
	waitFor(t, 25*time.Second, "A projesindeki koşu bitmeli", func() bool { s, _, _ := run(); return s != "" && s != "running" })
	st, merged, reason := run()
	if st != "completed" || merged != 2 {
		t.Fatalf("başka projeye geçmek koşuyu durdurmamalı: durum=%s birleşen=%d neden=%q", st, merged, reason)
	}
	if mgr.ActiveRepoID() == idA {
		t.Fatal("koşu bitince etkin proje kendiliğinden A'ya DÖNMEMELİ (kullanıcının seçimi korunur)")
	}
	// Koşunun günlüğü KENDİ projesine (A) yazılır, o sırada etkin olan B'ye değil.
	var inA, inB bool
	logsA, _, _ := mgr.AuditLogs(idA, 50)
	logsB, _, _ := mgr.AuditLogs(mgr.ActiveRepoID(), 50)
	for _, l := range logsA {
		if l.Action == "Kuyruk Koşusu" {
			inA = true
		}
	}
	for _, l := range logsB {
		if l.Action == "Kuyruk Koşusu" {
			inB = true
		}
	}
	if !inA || inB {
		t.Fatalf("Kuyruk Koşusu günlüğü A'ya yazılmalı (A=%v), B'ye değil (B=%v)", inA, inB)
	}
}

// Proje seçicide arka planda koşusu SÜREN proje ⏳ ile işaretlenir (başka projedeyken de görünsün).
func TestRepoOptionMarksAProjectWithARunningQueueRun(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := switchLive(t, dir)
	live, _ := mgr.activeLiveRepo()
	key := queueRunKey(live, true)
	s := mgr.qr()
	s.mu.Lock()
	s.state[key] = &model.QueueRun{ID: "r", Status: "running", UpdatedAt: time.Now().Format(time.RFC3339), Items: []model.QueueRunItem{{BranchID: "a", Branch: "a"}}}
	s.active[key] = &activeQueueRun{cancel: func() {}, done: make(chan struct{})}
	s.mu.Unlock()
	mgr.mu.RLock()
	opts := mgr.repoOptionsLocked()
	mgr.mu.RUnlock()
	var marked bool
	for _, o := range opts {
		if o.Path == live.path {
			marked = o.QueueRunning
		}
	}
	if !marked {
		t.Fatal("koşusu süren proje seçicide QueueRunning=true olmalı")
	}
}
