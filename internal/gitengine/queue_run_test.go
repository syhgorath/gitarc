package gitengine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitarc/internal/model"
)

// ---- saf döngü: sahte bağımlılıklar, sahte saat ----

type fakeRun struct {
	t        *testing.T
	mu       sync.Mutex
	clock    time.Time
	plans    map[string][]model.QueueStepPlan // dal başına sıradaki planlar (son plan tekrarlanır)
	applied  []string
	events   []model.QueueRunEvent
	heads    map[string]string
	sameProj func() bool
	applyErr error
	progress [][2]int
	cancel   context.CancelFunc
	onSleep  func()
}

func newFakeRun(t *testing.T) *fakeRun {
	return &fakeRun{t: t, clock: time.Unix(1000, 0), plans: map[string][]model.QueueStepPlan{}, heads: map[string]string{}}
}

func (f *fakeRun) deps() queueRunDeps {
	return queueRunDeps{
		plan: func(id string) (model.QueueStepPlan, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			q := f.plans[id]
			if len(q) == 0 {
				return model.QueueStepPlan{}, fmt.Errorf("plan yok: %s", id)
			}
			p := q[0]
			if len(q) > 1 {
				f.plans[id] = q[1:]
			}
			return p, nil
		},
		apply: func(id, fp string) error {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.applied = append(f.applied, id+":"+fp)
			return f.applyErr
		},
		head:        func(id string) string { return f.heads[id] },
		sameProject: f.sameProj,
		now:         func() time.Time { return f.clock },
		sleep: func(ctx context.Context, d time.Duration) bool {
			f.clock = f.clock.Add(d)
			if f.onSleep != nil {
				f.onSleep()
			}
			return ctx.Err() == nil
		},
		poll: 10 * time.Second, wait: time.Minute,
		emit:     func(ev model.QueueRunEvent) { f.events = append(f.events, ev) },
		progress: func(c, m int) { f.progress = append(f.progress, [2]int{c, m}) },
	}
}

func step(action, head, fp string) model.QueueStepPlan {
	return model.QueueStepPlan{Action: action, HeadSHA: head, Fingerprint: fp, Reason: action + "-neden"}
}

func items(ids ...string) []model.QueueRunItem {
	var out []model.QueueRunItem
	for i, id := range ids {
		out = append(out, model.QueueRunItem{Position: i + 1, BranchID: id, Branch: id, PR: i + 1, HeadSHA: "h-" + id})
	}
	return out
}

func TestQueueLoopMergesEveryPRInOrderAndWaitsForCI(t *testing.T) {
	f := newFakeRun(t)
	// a: doğrudan birleşir. b: önce güncelle, CI bekle (2 tur), sonra birleştir.
	f.plans["a"] = []model.QueueStepPlan{step("merge", "h-a", "fa")}
	f.plans["b"] = []model.QueueStepPlan{step("update", "h-b", "fb1"), step("wait", "h-b2", ""), step("wait", "h-b2", ""), step("merge", "h-b2", "fb2")}
	f.heads["b"] = "h-b2"
	out := runQueueLoop(context.Background(), items("a", "b"), f.deps())
	if out.Status != "completed" {
		t.Fatalf("tüm PR'lar birleşmeli: %+v", out)
	}
	if got := strings.Join(f.applied, ","); got != "a:fa,b:fb1,b:fb2" {
		t.Fatalf("adımlar sırayla uygulanmalı (a birleş, b güncelle, b birleş): %s", got)
	}
	if last := f.progress[len(f.progress)-1]; last != [2]int{2, 2} {
		t.Fatalf("ilerleme (2 PR, 2 birleşti) olmalı: %v", last)
	}
	waits := 0
	for _, ev := range f.events {
		if ev.Kind == "wait" {
			waits++
		}
	}
	if waits != 1 {
		t.Fatalf("aynı bekleme nedeni günlüğü şişirmemeli (1 'wait' olayı): %d", waits)
	}
}

func TestQueueLoopStopsAtTheFirstProblemAndDoesNotTouchTheRest(t *testing.T) {
	f := newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("merge", "h-a", "fa")}
	f.plans["b"] = []model.QueueStepPlan{step("blocked", "h-b", "")}
	f.plans["c"] = []model.QueueStepPlan{step("merge", "h-c", "fc")}
	out := runQueueLoop(context.Background(), items("a", "b", "c"), f.deps())
	if out.Status != "blocked" || !strings.Contains(out.Reason, "#2 b: blocked-neden") {
		t.Fatalf("b'de durmalı: %+v", out)
	}
	if got := strings.Join(f.applied, ","); got != "a:fa" {
		t.Fatalf("yalnızca a birleşmeli, b/c'ye dokunulmamalı: %s", got)
	}
}

func TestQueueLoopTimesOutWhileWaitingAndMergesNothing(t *testing.T) {
	f := newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("wait", "h-a", "")}
	out := runQueueLoop(context.Background(), items("a"), f.deps())
	if out.Status != "timeout" || len(f.applied) != 0 {
		t.Fatalf("zaman aşımında hiçbir şey uygulanmamalı: %+v %v", out, f.applied)
	}
}

func TestQueueLoopStopsWhenSomeoneElsePushesToTheBranch(t *testing.T) {
	f := newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("merge", "BASKASININ-COMMITI", "fa")}
	out := runQueueLoop(context.Background(), items("a"), f.deps())
	if out.Status != "blocked" || !strings.Contains(out.Reason, "onaylanandan farklı") || len(f.applied) != 0 {
		t.Fatalf("dışarıdan değişen uçta durmalı, uygulamamalı: %+v %v", out, f.applied)
	}
	// güncellemeden sonra yeni uç beklenen olur: bizim push'umuz dışarıdan değişiklik sayılmaz
	f = newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("update", "h-a", "f1"), step("merge", "h-a-yeni", "f2")}
	f.heads["a"] = "h-a-yeni"
	if out := runQueueLoop(context.Background(), items("a"), f.deps()); out.Status != "completed" {
		t.Fatalf("kendi push'umuz dışarıdan değişiklik sayılmamalı: %+v", out)
	}
}

func TestQueueLoopStopsOnCancelAndWhenProjectChanges(t *testing.T) {
	f := newFakeRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	f.plans["a"] = []model.QueueStepPlan{step("wait", "h-a", "")}
	f.onSleep = cancel
	if out := runQueueLoop(ctx, items("a", "b"), f.deps()); out.Status != "stopped" || len(f.applied) != 0 {
		t.Fatalf("iptalde durmalı: %+v", out)
	}

	f = newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("merge", "h-a", "fa")}
	f.sameProj = func() bool { return false }
	out := runQueueLoop(context.Background(), items("a"), f.deps())
	if out.Status != "blocked" || !strings.Contains(out.Reason, "proje değişti") || len(f.applied) != 0 {
		t.Fatalf("proje değişince hiçbir işlem yapılmamalı: %+v %v", out, f.applied)
	}
}

func TestQueueLoopStopsWhenAStepFailsOrThePlanIsUnreadable(t *testing.T) {
	f := newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{step("merge", "h-a", "fa")}
	f.applyErr = fmt.Errorf("GitHub reddetti")
	if out := runQueueLoop(context.Background(), items("a", "b"), f.deps()); out.Status != "blocked" || !strings.Contains(out.Reason, "GitHub reddetti") {
		t.Fatalf("adım hatasında durmalı: %+v", out)
	}
	f = newFakeRun(t) // plan yok → hata
	if out := runQueueLoop(context.Background(), items("a"), f.deps()); out.Status != "blocked" {
		t.Fatalf("plan alınamazsa durmalı: %+v", out)
	}
	f = newFakeRun(t)
	f.plans["a"] = []model.QueueStepPlan{{Action: "garip"}}
	if out := runQueueLoop(context.Background(), items("a"), f.deps()); out.Status != "blocked" {
		t.Fatalf("bilinmeyen eylemde durmalı: %+v", out)
	}
}

// ---- Manager: simülasyon ----

func fastRun(t *testing.T) {
	t.Helper()
	oldPoll, oldWait := queueRunPoll, queueRunWait
	queueRunPoll, queueRunWait = 15*time.Millisecond, 20*time.Second
	t.Cleanup(func() { queueRunPoll, queueRunWait = oldPoll, oldWait })
}

func waitRunDone(t *testing.T, mgr *Manager) *model.QueueRun {
	t.Helper()
	var run *model.QueueRun
	waitFor(t, 20*time.Second, "koşu bitmeli", func() bool {
		r, err := mgr.QueueRunStatus()
		run = r
		return err == nil && r != nil && r.Status != "running"
	})
	return run
}

func TestQueueRunInSimulationMergesTheGreenPrefixAndStopsAtTheFirstProblem(t *testing.T) {
	fastRun(t)
	mgr := NewManager("")
	// demo: 424 CI sürüyor, 422 CI kırmızı, 421 çakışıyor
	if p, _ := mgr.QueueRunPlan(nil); p.Startable {
		t.Fatalf("ilk PR (#421) çakışıyorken koşu başlatılamaz: %+v", p)
	}
	mgr.mu.Lock()
	for i := range mgr.simRepo.branches {
		if mgr.simRepo.branches[i].ID == "br-open-424" {
			mgr.simRepo.branches[i].PR.CI.State = "SUCCESS"
		}
	}
	mgr.mu.Unlock()

	plan, err := mgr.QueueRunPlan([]string{"br-open-424", "br-open-422"})
	if err != nil || !plan.Startable || len(plan.Items) != 2 || plan.Fingerprint == "" {
		t.Fatalf("plan başlatılabilir olmalı: %+v %v", plan, err)
	}
	for _, c := range []string{"", "yanlis"} {
		if _, err := mgr.StartQueueRun([]string{"br-open-424", "br-open-422"}, c); err == nil {
			t.Fatalf("confirm=%q ile koşu başlamamalı", c)
		}
	}
	if r, _ := mgr.QueueRunStatus(); r != nil {
		t.Fatalf("reddedilen başlatma koşu oluşturmamalı: %+v", r)
	}
	run, err := mgr.StartQueueRun([]string{"br-open-424", "br-open-422"}, plan.Fingerprint)
	if err != nil || run.Status != "running" {
		t.Fatalf("başlamalı: %+v %v", run, err)
	}
	final := waitRunDone(t, mgr)
	// 424 birleşir; 422'nin CI'ı kırmızı: koşu orada durur ve onu birleştirmez
	if final.Status != "blocked" || final.Merged != 1 || !strings.Contains(final.StopReason, "CI başarısız") {
		t.Fatalf("424 birleşip 422'de durmalı: %+v", final)
	}
	snap, _ := mgr.Snapshot()
	for _, b := range snap.Branches {
		if b.ID == "br-open-424" && b.State != "merged" {
			t.Fatal("424 birleşmiş olmalı")
		}
		if b.ID == "br-open-422" && b.State == "merged" {
			t.Fatal("422 kırmızı CI ile birleştirilmemeli")
		}
	}
}

func TestQueueRunRefusesASecondRunAndCanBeStopped(t *testing.T) {
	fastRun(t)
	mgr := NewManager("")
	ids := []string{"br-open-424"} // CI sürüyor: koşu bekler
	plan, _ := mgr.QueueRunPlan(ids)
	if !plan.Startable {
		t.Fatalf("424 tek başına başlatılabilir olmalı: %+v", plan)
	}
	if _, err := mgr.StopQueueRun(); err == nil {
		t.Fatal("çalışan koşu yokken durdurma hata vermeli")
	}
	if _, err := mgr.StartQueueRun(ids, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.StartQueueRun(ids, plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "zaten çalışan") {
		t.Fatalf("ikinci koşu reddedilmeli: %v", err)
	}
	st, err := mgr.StopQueueRun()
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "stopped" || st.Merged != 0 {
		t.Fatalf("durdurulmalı ve hiçbir şey birleştirmemeli: %+v", st)
	}
	snap, _ := mgr.Snapshot()
	for _, b := range snap.Branches {
		if b.ID == "br-open-424" && b.State == "merged" {
			t.Fatal("durdurulan koşu 424'ü birleştirmemeli")
		}
	}
}

// ---- Manager: gerçek git + sahte gh ----

type fakeGitHub struct {
	t       *testing.T
	w       *stepWorld
	prs     map[string]int // dal → PR no
	mu      sync.Mutex
	merged  map[string]bool
	lastTip map[string]string
	runTill map[string]time.Time
	stop    chan struct{}
	handled int
	hold    map[string]bool // true: bu dalın CI'ı hiç bitmez
	ciDelay time.Duration
}

func (w *stepWorld) fakeGitHub(prs map[string]int) *fakeGitHub {
	g := &fakeGitHub{t: w.t, w: w, prs: prs, merged: map[string]bool{}, lastTip: map[string]string{}, runTill: map[string]time.Time{}, stop: make(chan struct{}), hold: map[string]bool{}, ciDelay: 120 * time.Millisecond}
	go g.loop()
	w.t.Cleanup(func() { close(g.stop) })
	return g
}

func (g *fakeGitHub) loop() {
	for {
		select {
		case <-g.stop:
			return
		case <-time.After(10 * time.Millisecond):
		}
		g.mu.Lock()
		calls := g.w.gh.calls()
		for ; g.handled < len(calls); g.handled++ {
			if rest, ok := strings.CutPrefix(calls[g.handled], "pr merge "); ok {
				var n int
				fmt.Sscanf(rest, "%d", &n)
				for br, pr := range g.prs {
					if pr == n {
						g.mergeOnRemote(br)
						g.merged[br] = true
					}
				}
			}
		}
		for br, pr := range g.prs {
			tip, err := runGitOut(g.w.bare, "rev-parse", br)
			tip = strings.TrimSpace(tip)
			if err != nil {
				continue
			}
			if g.lastTip[br] != tip {
				g.lastTip[br] = tip
				g.runTill[br] = time.Now().Add(g.ciDelay) // yeni uçta CI yeniden başlar
			}
			state, roll := "OPEN", rollGreen
			if g.merged[br] {
				state = "MERGED"
			} else if g.hold[br] || time.Now().Before(g.runTill[br]) {
				roll = `[{"name":"test","status":"IN_PROGRESS"}]`
			}
			out := fmt.Sprintf(`{"state":%q,"isDraft":false,"headRefOid":%q,"reviewDecision":"","statusCheckRollup":%s}`, state, tip, roll)
			tmp := filepath.Join(g.w.gh.dir, fmt.Sprintf("pr_view_%d.out.tmp", pr))
			_ = os.WriteFile(tmp, []byte(out), 0o644)
			_ = os.Rename(tmp, filepath.Join(g.w.gh.dir, fmt.Sprintf("pr_view_%d.out", pr)))
		}
		g.mu.Unlock()
	}
}

// mergeOnRemote: GitHub'ın PR'ı main'e merge etmesi (merge commit) — ikinci klon üzerinden.
func (g *fakeGitHub) mergeOnRemote(branch string) {
	o := g.w.other
	for _, args := range [][]string{{"fetch", "-q", "origin"}, {"checkout", "-q", "-B", "main", "origin/main"},
		{"merge", "-q", "--no-ff", "-m", "Merge pull request from " + branch, "origin/" + branch}, {"push", "-q", "origin", "main"}} {
		if out, err := exec.Command("git", append([]string{"-C", o}, args...)...).CombinedOutput(); err != nil {
			g.t.Errorf("sahte GitHub merge (%v): %v %s", args, err, out)
			return
		}
	}
}

func (g *fakeGitHub) setHold(branch string, hold bool) {
	g.mu.Lock()
	g.hold[branch] = hold
	g.mu.Unlock()
}

func TestQueueRunLiveMergesTwoPRsInOrderUpdatingTheSecondAfterTheFirstLands(t *testing.T) {
	fastRun(t)
	w := newStepWorld(t)
	g := w.fakeGitHub(map[string]int{"feat/x": 1, "feat/y": 2})
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	waitPR(t, mgr, "feat/y")
	time.Sleep(300 * time.Millisecond) // sahte GitHub ilk durumu yazsın, CI yeşile dönsün
	_ = g

	ids := []string{"feat/x", "feat/y"}
	plan, err := mgr.QueueRunPlan(ids)
	if err != nil || !plan.Startable || len(plan.Items) != 2 || plan.Items[0].HeadSHA == "" {
		t.Fatalf("canlı plan: %+v %v", plan, err)
	}
	if err := os.WriteFile(filepath.Join(w.dir, "kirli.txt"), []byte("benim işim\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	headBefore := isoGit(t, w.dir, "rev-parse", "HEAD")
	branchBefore := isoGit(t, w.dir, "branch", "--show-current")
	yTipBefore := w.remoteTip("feat/y")
	if _, err := mgr.StartQueueRun(ids, "yanlis"); err == nil {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if w.gh.called("pr merge") || w.remoteTip("feat/y") != yTipBefore {
		t.Fatal("reddedilen başlatma hiçbir şey yapmamalı")
	}
	if _, err := mgr.StartQueueRun(ids, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	final := waitRunDone(t, mgr)
	if final.Status != "completed" || final.Merged != 2 {
		t.Fatalf("iki PR de birleşmeli: %+v", final)
	}

	// sıra: önce x birleşti, SONRA y güncellendi (main'i içine aldı), en son y birleşti
	calls := w.gh.calls()
	idx := func(sub string) int {
		for i, c := range calls {
			if strings.Contains(c, sub) {
				return i
			}
		}
		return -1
	}
	if ix, iy := idx("pr merge 1"), idx("pr merge 2"); ix < 0 || iy < 0 || ix > iy {
		t.Fatalf("x, y'den önce birleşmeli: %d %d\n%v", ix, iy, calls)
	}
	if w.gh.called("--admin") {
		t.Fatal("koruma atlatılmamalı")
	}
	yTip := w.remoteTip("feat/y")
	if yTip == yTipBefore {
		t.Fatal("y, x birleşince geride kaldığı için güncellenmeli (uzak ucu ilerlemeli)")
	}
	parents := strings.Fields(isoGit(t, w.bare, "rev-list", "--parents", "-n", "1", yTip))
	if len(parents) != 3 {
		t.Fatalf("y'nin ucu main'i içine alan bir MERGE commit olmalı: %v", parents)
	}
	if !w.gh.called("pr merge 2", "--match-head-commit "+yTip) {
		t.Fatalf("y, güncellenmiş ucuna sabitlenerek birleştirilmeli: %v", calls)
	}
	// kullanıcının dalı/çalışma ağacı korunur; yerel main yalnızca ileri sarılabilir (birleşme sonrası güvenli eşitleme)
	if got := isoGit(t, w.dir, "branch", "--show-current"); got != branchBefore {
		t.Fatalf("kullanıcının dalı değişmemeli: %s -> %s", branchBefore, got)
	}
	if err := exec.Command("git", "-C", w.dir, "merge-base", "--is-ancestor", headBefore, "HEAD").Run(); err != nil {
		t.Fatal("yerel HEAD geri/yana gitmemeli (yalnızca ileri sarılabilir)")
	}
	if got, err := os.ReadFile(filepath.Join(w.dir, "kirli.txt")); err != nil || string(got) != "benim işim\n" {
		t.Fatalf("kullanıcının kirli dosyası korunmalı: %q %v", got, err)
	}
	if final.Events[len(final.Events)-1].Kind != "stop" {
		t.Fatalf("günlük 'stop' ile bitmeli: %+v", final.Events)
	}
}

func TestQueueRunLiveStopsWhenSomeoneElsePushesWhileWaitingForCI(t *testing.T) {
	fastRun(t)
	w := newStepWorld(t)
	g := w.fakeGitHub(map[string]int{"feat/x": 1, "feat/y": 2})
	g.setHold("feat/x", true) // x'in CI'ı hiç bitmiyor: koşu bekliyor
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	time.Sleep(200 * time.Millisecond)

	plan, _ := mgr.QueueRunPlan([]string{"feat/x"})
	if !plan.Startable {
		t.Fatalf("başlatılabilir olmalı: %+v", plan)
	}
	if _, err := mgr.StartQueueRun([]string{"feat/x"}, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond) // koşu bekleme döngüsünde
	if snap, err := mgr.Snapshot(); err != nil || snap.Repo.QueueRun == nil || snap.Repo.QueueRun.Total != 1 || snap.Repo.QueueRun.Items[0].State != "current" {
		t.Fatalf("çalışan koşu anlık görüntüde görünmeli (kart rozeti): %+v %v", snap.Repo.QueueRun, err)
	}
	w.pushFromOther("feat/x", "baskasi.txt", "b\n") // koşu kendi fetch'ini çalıştırıyor: yerel klona dokunma
	final := waitRunDone(t, mgr)
	// Dışarıdan push iki yoldan fark edilir (hangisi önce olduğu zamanlamaya bağlı): koşunun kendi yenilediği uç onaylanandan
	// farklı ("onaylanandan farklı") ya da GitHub'ın bildirdiği uç yerel kayıttan ileride ("yerel uzak kaydından farklı").
	if final.Status != "blocked" || !(strings.Contains(final.StopReason, "onaylanandan farklı") || strings.Contains(final.StopReason, "yerel uzak kaydından farklı")) {
		t.Fatalf("dışarıdan push'ta durmalı: %+v", final)
	}
	if w.gh.called("pr merge") {
		t.Fatal("değişen dal birleştirilmemeli")
	}
}

func TestQueueRunStatusAfterRestartIsInterruptedAndNeverResumesByItself(t *testing.T) {
	fastRun(t)
	w := newStepWorld(t)
	mgr := switchLive(t, w.dir)
	// "uygulama kapandı": dosyada running kalmış bir koşu
	stale := model.QueueRun{ID: "run-1", Status: "running", Mode: "live", StartedAt: time.Now().Format(time.RFC3339), Items: items("feat/x")}
	persistQueueRun(&repoEntry{path: w.dir}, true, &stale)
	r, err := mgr.QueueRunStatus()
	if err != nil || r == nil || r.Status != "interrupted" || !strings.Contains(r.StopReason, "devam etmez") {
		t.Fatalf("yarım kalan koşu 'interrupted' görünmeli: %+v %v", r, err)
	}
	time.Sleep(200 * time.Millisecond)
	if w.gh.called("pr merge") {
		t.Fatal("kendiliğinden devam edip birleştirmemeli")
	}
}

// MCP süreci koşuyu çalıştırırken arayüz sürecinin aynı depoyu görmesi: taze "running" koşu BAŞKA süreçte sürüyordur;
// "interrupted" diye bozulmamalı, ikinci koşu başlatılamamalı, durdurma dürüstçe yönlendirmeli.
func TestQueueRunStartedByAnotherProcessIsNotMarkedInterruptedWhileItsHeartbeatIsFresh(t *testing.T) {
	fastRun(t)
	w := newStepWorld(t)
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	fresh := model.QueueRun{ID: "run-mcp", Status: "running", Mode: "live", Items: items("feat/x"), StartedAt: time.Now().Format(time.RFC3339), UpdatedAt: time.Now().Format(time.RFC3339)}
	persistQueueRun(&repoEntry{path: w.dir}, true, &fresh)
	r, err := mgr.QueueRunStatus()
	if err != nil || r == nil || r.Status != "running" {
		t.Fatalf("kalp atışı tazeyken koşu 'running' kalmalı: %+v %v", r, err)
	}
	if _, err := mgr.StartQueueRun([]string{"feat/x"}, "x"); err == nil || !strings.Contains(err.Error(), "zaten çalışan") {
		t.Fatalf("başka süreçte koşu sürerken ikinci koşu başlamamalı: %v", err)
	}
	if _, err := mgr.StopQueueRun(); err == nil || !strings.Contains(err.Error(), "başka bir süreçte") {
		t.Fatalf("durdurma, koşunun başka süreçte olduğunu söylemeli: %v", err)
	}
}

// Kart rozeti: çalışan koşu anlık görüntüde (kuyruk penceresi kapalıyken de) görünür; biten/durdurulan koşu görünmez; imza değişir.
func TestRunningQueueIsInTheSnapshotForCardBadgesAndTheSignatureChanges(t *testing.T) {
	fastRun(t)
	mgr := NewManager("")
	sigIdle := mgr.QuickRepoSignature()
	if snap, _ := mgr.Snapshot(); snap.Repo.QueueRun != nil {
		t.Fatalf("koşu yokken rozet verisi olmamalı: %+v", snap.Repo.QueueRun)
	}
	ids := []string{"br-open-424"} // CI sürüyor: koşu bekler
	plan, _ := mgr.QueueRunPlan(ids)
	if _, err := mgr.StartQueueRun(ids, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "bekleme olayı", func() bool {
		r, _ := mgr.QueueRunStatus()
		return r != nil && len(r.Events) > 1
	})
	snap, _ := mgr.Snapshot()
	q := snap.Repo.QueueRun
	if q == nil || q.Total != 1 || q.Merged != 0 || len(q.Items) != 1 || q.Items[0].BranchID != "br-open-424" || q.Items[0].State != "current" {
		t.Fatalf("çalışan koşu özeti: %+v", q)
	}
	if q.Phase != "CI bekleniyor" {
		t.Fatalf("CI beklerken aşama 'CI bekleniyor' olmalı: %q", q.Phase)
	}
	if mgr.QuickRepoSignature() == sigIdle {
		t.Fatal("koşu başlayınca anlık görüntü imzası değişmeli (arayüz kartları yenilesin)")
	}
	if _, err := mgr.StopQueueRun(); err != nil {
		t.Fatal(err)
	}
	if snap, _ := mgr.Snapshot(); snap.Repo.QueueRun != nil {
		t.Fatalf("durdurulan koşu için rozet verisi kalkmalı: %+v", snap.Repo.QueueRun)
	}
}

func TestQueueRunBriefMarksDoneCurrentAndWaitingItems(t *testing.T) {
	mgr := NewManager("")
	run := &model.QueueRun{ID: "r", Status: "running", Mode: "simulation", Items: items("a", "b", "c"), Merged: 1, UpdatedAt: time.Now().Format(time.RFC3339),
		Events: []model.QueueRunEvent{{Kind: "info"}, {Kind: "merge", Branch: "a"}, {Kind: "update", Branch: "b"}}}
	s := mgr.qr()
	s.mu.Lock()
	s.state["simulation"] = run
	s.active["simulation"] = &activeQueueRun{}
	s.mu.Unlock()
	b := mgr.queueRunBrief("simulation", "", false)
	if b == nil || b.Total != 3 || b.Merged != 1 || b.Current != 1 {
		t.Fatalf("özet: %+v", b)
	}
	got := []string{b.Items[0].State, b.Items[1].State, b.Items[2].State}
	if strings.Join(got, ",") != "done,current,waiting" || b.Phase != "CI bekleniyor" {
		t.Fatalf("durumlar/aşama: %v %q", got, b.Phase)
	}
	s.mu.Lock()
	run.Status = "blocked"
	s.mu.Unlock()
	if mgr.queueRunBrief("simulation", "", false) != nil {
		t.Fatal("bitmiş/durmuş koşu rozet vermemeli")
	}
}
