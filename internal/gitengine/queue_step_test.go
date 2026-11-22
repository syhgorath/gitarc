package gitengine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitarc/internal/model"
)

func TestRollupCIStateNeverInventsGreen(t *testing.T) {
	ok := ghRollupCheck{Status: "COMPLETED", Conclusion: "SUCCESS"}
	cases := []struct {
		name   string
		checks []ghRollupCheck
		want   string
	}{
		{"boş kontrol listesi bilinmiyor", nil, "UNKNOWN"},
		{"hepsi yeşil", []ghRollupCheck{ok, {Status: "COMPLETED", Conclusion: "SKIPPED"}}, "SUCCESS"},
		{"biri sürüyor", []ghRollupCheck{ok, {Status: "IN_PROGRESS"}}, "RUNNING"},
		{"sürüyor + kırmızı: kırmızı", []ghRollupCheck{{Status: "IN_PROGRESS"}, {Status: "COMPLETED", Conclusion: "FAILURE"}}, "FAILURE"},
		{"bağlam durumu PENDING", []ghRollupCheck{ok, {State: "PENDING"}}, "RUNNING"},
		{"bağlam durumu SUCCESS", []ghRollupCheck{{State: "SUCCESS"}}, "SUCCESS"},
		{"tamamlanmış ama sonuçsuz: yeşil sayılmaz", []ghRollupCheck{{Status: "COMPLETED"}}, "UNKNOWN"},
		{"tanınmayan sonuç: yeşil sayılmaz", []ghRollupCheck{ok, {Status: "COMPLETED", Conclusion: "STALE"}}, "UNKNOWN"},
		{"iptal edilmiş", []ghRollupCheck{{Status: "COMPLETED", Conclusion: "CANCELLED"}}, "FAILURE"},
	}
	for _, c := range cases {
		if got := rollupCIState(c.checks); got != c.want {
			t.Errorf("%s: %s bekleniyordu, %s geldi", c.name, c.want, got)
		}
	}
}

func TestDecideQueueStepRules(t *testing.T) {
	base := model.QueueStepPlan{Mode: "live", Branch: "feat/x", BranchID: "x", PR: 1}
	upd := func(status string, behind int) model.BranchUpdatePlan {
		return model.BranchUpdatePlan{Status: status, Behind: behind, HeadSHA: "h1", BaseSHA: "b1", BaseRef: "origin/main", ConflictFiles: []string{"f.txt"}, Reason: "neden"}
	}
	cases := []struct {
		name   string
		in     queueStepInput
		action string
		sub    string
	}{
		{"güncel + CI yeşil → merge", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: upd("up_to_date", 0)}, "merge", ""},
		{"geride + CI yeşil → önce güncelle", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: upd("ready", 2)}, "update", ""},
		{"geride + CI sürüyor → yine güncelle", queueStepInput{PRState: "OPEN", CI: "RUNNING", Update: upd("ready", 2)}, "update", ""},
		{"geride + CI kırmızı → engel (güncelleme düzeltmez)", queueStepInput{PRState: "OPEN", CI: "FAILURE", Update: upd("ready", 2)}, "blocked", "CI başarısız"},
		{"güncel + CI sürüyor → bekle", queueStepInput{PRState: "OPEN", CI: "RUNNING", Update: upd("up_to_date", 0)}, "wait", "sürüyor"},
		{"güncel + CI bilinmiyor → bekle (yeşil değil)", queueStepInput{PRState: "OPEN", CI: "UNKNOWN", Update: upd("up_to_date", 0)}, "wait", "yeşil varsayılmaz"},
		{"güncel + CI boş → bilinmiyor", queueStepInput{PRState: "OPEN", CI: "", Update: upd("up_to_date", 0)}, "wait", "yeşil varsayılmaz"},
		{"çakışma → engel", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: upd("conflict", 1)}, "blocked", "f.txt"},
		{"taslak → engel", queueStepInput{PRState: "OPEN", Draft: true, CI: "SUCCESS", Update: upd("up_to_date", 0)}, "blocked", "taslak"},
		{"kapalı PR → engel", queueStepInput{PRState: "MERGED", CI: "SUCCESS", Update: upd("up_to_date", 0)}, "blocked", "açık değil"},
		{"inceleme gerekli → engel", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Review: "REVIEW_REQUIRED", Update: upd("up_to_date", 0)}, "blocked", "inceleme"},
		{"değişiklik istendi → engel", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Review: "CHANGES_REQUESTED", Update: upd("up_to_date", 0)}, "blocked", "değişiklik"},
		{"onaylı inceleme engel değil", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Review: "APPROVED", Update: upd("up_to_date", 0)}, "merge", ""},
		{"GitHub ucu yerelden farklı → engel", queueStepInput{PRState: "OPEN", CI: "SUCCESS", GHHeadSHA: "baska", Update: upd("up_to_date", 0)}, "blocked", "git fetch"},
		{"güncelleme engelli → engel", queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: upd("blocked", 0)}, "blocked", "neden"},
	}
	for _, c := range cases {
		got := decideQueueStep(base, c.in)
		if got.Action != c.action {
			t.Errorf("%s: eylem %q bekleniyordu, %q geldi (%s)", c.name, c.action, got.Action, got.Reason)
			continue
		}
		if c.sub != "" && !strings.Contains(got.Reason, c.sub) {
			t.Errorf("%s: neden %q içermeli: %s", c.name, c.sub, got.Reason)
		}
		hasFP := got.Fingerprint != ""
		if hasFP != (c.action == "update" || c.action == "merge") {
			t.Errorf("%s: parmak izi yalnızca update/merge'de olmalı: %+v", c.name, got)
		}
	}
	// parmak izi CI durumuna ve uçlara bağlıdır
	a := decideQueueStep(base, queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: upd("up_to_date", 0)})
	u2 := upd("up_to_date", 0)
	u2.HeadSHA = "h2"
	b := decideQueueStep(base, queueStepInput{PRState: "OPEN", CI: "SUCCESS", Update: u2})
	if a.Fingerprint == b.Fingerprint {
		t.Fatal("dal ucu değişince parmak izi değişmeli")
	}
}

// ---- gerçek git + sahte gh ----

type stepWorld struct {
	*updateWorld
	gh *ghStub
}

func newStepWorld(t *testing.T) *stepWorld {
	t.Helper()
	gh := newGHStub(t)
	w := newUpdateWorld(t)
	gh.set("pr_list_open", `[{"number":1,"headRefName":"feat/x","title":"x","body":"","url":"https://github.com/acme/demo/pull/1","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]},
	 {"number":2,"headRefName":"feat/y","title":"y","body":"","url":"https://github.com/acme/demo/pull/2","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]}]`, 0)
	return &stepWorld{updateWorld: w, gh: gh}
}

func (w *stepWorld) view(state, headBranch, rollup, review string) {
	w.gh.set("pr_view", fmt.Sprintf(`{"state":%q,"isDraft":false,"headRefOid":%q,"reviewDecision":%q,"statusCheckRollup":%s}`,
		state, w.remoteTip(headBranch), review, rollup), 0)
}

const rollGreen = `[{"name":"test","status":"COMPLETED","conclusion":"SUCCESS"}]`

func waitPR(t *testing.T, mgr *Manager, branch string) {
	t.Helper()
	waitFor(t, 10*time.Second, "PR bilgisi anlık görüntüye gelmeli", func() bool {
		snap, err := mgr.Snapshot()
		return err == nil && prsOn(snap, branch) != nil
	})
}

func TestQueueStepLiveFullCycleUpdateThenMerge(t *testing.T) {
	w := newStepWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")

	// 1) geride ve CI yeşil: önce güncelle
	w.view("OPEN", "feat/x", rollGreen, "")
	p1, err := mgr.QueueStepPlan("feat/x")
	if err != nil || p1.Action != "update" || p1.Fingerprint == "" || p1.PR != 1 {
		t.Fatalf("geride kalan dal için 'update' planı bekleniyordu: %+v %v", p1, err)
	}
	oldTip := w.remoteTip("feat/x")
	if _, err := mgr.QueueStepApply("feat/x", "yanlis"); err == nil {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if _, err := mgr.QueueStepApply("feat/x", ""); err == nil {
		t.Fatal("onaysız adım reddedilmeli")
	}
	if w.remoteTip("feat/x") != oldTip {
		t.Fatal("reddedilen adım uzak dalı değiştirmemeli")
	}
	if _, err := mgr.QueueStepApply("feat/x", p1.Fingerprint); err != nil {
		t.Fatalf("güncelleme adımı: %v", err)
	}
	newTip := w.remoteTip("feat/x")
	if newTip == oldTip {
		t.Fatal("güncelleme uzak dalı ilerletmeli")
	}
	if w.gh.called("pr merge") {
		t.Fatal("güncelleme adımı PR birleştirmemeli")
	}

	// 2) güncel ama yeni uçta CI henüz başlamadı (boş): bekle, merge YOK
	w.view("OPEN", "feat/x", `[]`, "")
	p2, _ := mgr.QueueStepPlan("feat/x")
	if p2.Action != "wait" || p2.CI != "UNKNOWN" {
		t.Fatalf("CI bilinmiyorken bekle bekleniyordu: %+v", p2)
	}
	if _, err := mgr.QueueStepApply("feat/x", p2.Fingerprint); err == nil || w.gh.called("pr merge") {
		t.Fatalf("bekleyen adım uygulanmamalı: %v", err)
	}
	w.view("OPEN", "feat/x", `[{"name":"test","status":"IN_PROGRESS"}]`, "")
	if p, _ := mgr.QueueStepPlan("feat/x"); p.Action != "wait" || p.CI != "RUNNING" {
		t.Fatalf("CI sürerken bekle bekleniyordu: %+v", p)
	}

	// 3) CI yeşil: merge, ucu sabitleyerek
	w.view("OPEN", "feat/x", rollGreen, "")
	p3, _ := mgr.QueueStepPlan("feat/x")
	if p3.Action != "merge" || p3.Fingerprint == "" || p3.HeadSHA != newTip {
		t.Fatalf("merge planı bekleniyordu: %+v", p3)
	}
	// eski (update) parmak izi merge'ü onaylayamaz
	if _, err := mgr.QueueStepApply("feat/x", p1.Fingerprint); err == nil || w.gh.called("pr merge") {
		t.Fatalf("eski plan merge'ü onaylamamalı: %v", err)
	}
	if _, err := mgr.QueueStepApply("feat/x", p3.Fingerprint); err != nil {
		t.Fatalf("merge adımı: %v", err)
	}
	if !w.gh.called("pr merge 1", "--merge", "--match-head-commit "+newTip) {
		t.Fatalf("merge, dal ucu sabitlenerek (--match-head-commit) çağrılmalı: %v", w.gh.calls())
	}
	if w.gh.called("--admin") {
		t.Fatal("koruma atlatılmamalı (--admin yok)")
	}
}

func TestQueueStepNeverMergesWhenStateChangedAfterPlan(t *testing.T) {
	w := newStepWorld(t)
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	w.view("OPEN", "feat/x", rollGreen, "")
	plan, _ := mgr.QueueStepPlan("feat/x")
	if plan.Action != "merge" {
		t.Fatalf("merge planı bekleniyordu: %+v", plan)
	}
	// planla onay arasında BAŞKASI dala push etti
	w.pushToOrigin("feat/x", "baskasi.txt", "b\n")
	w.view("OPEN", "feat/x", rollGreen, "")
	if _, err := mgr.QueueStepApply("feat/x", plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "planı yeniden") {
		t.Fatalf("eski plan reddedilmeli: %v", err)
	}
	if w.gh.called("pr merge") {
		t.Fatal("durum değişmişken PR birleştirilmemeli")
	}
}

func TestQueueStepBlockedCases(t *testing.T) {
	w := newStepWorld(t)
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")

	w.view("OPEN", "feat/x", `[{"name":"test","status":"COMPLETED","conclusion":"FAILURE"}]`, "")
	if p, _ := mgr.QueueStepPlan("feat/x"); p.Action != "blocked" || p.CI != "FAILURE" {
		t.Fatalf("kırmızı CI engel olmalı: %+v", p)
	}
	w.view("OPEN", "feat/x", rollGreen, "REVIEW_REQUIRED")
	if p, _ := mgr.QueueStepPlan("feat/x"); p.Action != "blocked" {
		t.Fatalf("inceleme gerekliyken engel olmalı: %+v", p)
	}
	// gh okunamıyorsa yeşil varsayılmaz
	w.gh.set("pr_view", "HTTP 502", 1)
	p, _ := mgr.QueueStepPlan("feat/x")
	if p.Action != "blocked" || p.CI != "UNKNOWN" || !strings.Contains(p.Reason, "okunamadı") {
		t.Fatalf("PR okunamayınca engel + CI bilinmiyor bekleniyordu: %+v", p)
	}
	if _, err := mgr.QueueStepApply("feat/x", "x"); err == nil || w.gh.called("pr merge") {
		t.Fatalf("engelli adım uygulanmamalı: %v", err)
	}
	if _, err := mgr.QueueStepPlan("yok-boyle-dal"); err == nil {
		t.Fatal("bilinmeyen dal hata vermeli")
	}
}

// Plan SALT OKUNUR: uzak dallar, yerel ref'ler, HEAD ve çalışma ağacı aynı kalır; gh'ye yazan çağrı yapılmaz.
func TestQueueStepPlanNeverWrites(t *testing.T) {
	w := newStepWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	mgr := switchLive(t, w.dir)
	waitPR(t, mgr, "feat/x")
	w.view("OPEN", "feat/x", rollGreen, "")
	state := func() string {
		return strings.Join([]string{isoGit(t, w.dir, "status", "--porcelain"), isoGit(t, w.dir, "for-each-ref"),
			isoGit(t, w.dir, "rev-parse", "HEAD"), w.remoteTip("feat/x"), w.remoteTip("main")}, "\n--\n")
	}
	before := state()
	if _, err := mgr.QueueStepPlan("feat/x"); err != nil {
		t.Fatal(err)
	}
	if after := state(); after != before {
		t.Fatalf("plan yazmamalı:\n%s\n---\n%s", before, after)
	}
	if w.gh.called("pr merge") {
		t.Fatal("plan merge çağırmamalı")
	}
}

func TestQueueStepInSimulationFollowsTheSameRules(t *testing.T) {
	mgr := NewManager("")
	setCI := func(id, state string) {
		mgr.mu.Lock()
		defer mgr.mu.Unlock()
		for i := range mgr.simRepo.branches {
			if mgr.simRepo.branches[i].ID == id {
				mgr.simRepo.branches[i].PR.CI.State = state
			}
		}
	}
	plan := func(id string) model.QueueStepPlan {
		p, err := mgr.QueueStepPlan(id)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	// demo verisi: 421 çakışır, 422 CI kırmızı (geride), 424 CI sürüyor (güncel)
	if p := plan("br-open-421"); p.Action != "blocked" || p.Mode != "simulation" {
		t.Fatalf("çakışan PR engel olmalı: %+v", p)
	}
	if p := plan("br-open-422"); p.Action != "blocked" || p.CI != "FAILURE" {
		t.Fatalf("kırmızı CI engel olmalı: %+v", p)
	}
	if p := plan("br-open-424"); p.Action != "wait" || p.CI != "RUNNING" {
		t.Fatalf("CI sürerken bekle: %+v", p)
	}
	if _, err := mgr.QueueStepApply("br-open-424", "x"); err == nil {
		t.Fatal("bekleyen adım uygulanmamalı")
	}

	// 424 yeşile döner → birleştir; sonra 422 (yeşile döndü, geride) → önce güncelle, sonra birleştir
	setCI("br-open-424", "SUCCESS")
	p := plan("br-open-424")
	if p.Action != "merge" || p.Fingerprint == "" {
		t.Fatalf("merge bekleniyordu: %+v", p)
	}
	if _, err := mgr.QueueStepApply("br-open-424", "yanlis"); err == nil {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if _, err := mgr.QueueStepApply("br-open-424", p.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if p2 := plan("br-open-424"); p2.Action != "blocked" {
		t.Fatalf("birleşen PR bir daha adım almamalı: %+v", p2)
	}
	setCI("br-open-422", "SUCCESS")
	u := plan("br-open-422")
	if u.Action != "update" || u.Behind < 2 {
		t.Fatalf("birleşme sonrası diğer PR daha da geride kalmalı ve önce güncellenmeli: %+v", u)
	}
	if _, err := mgr.QueueStepApply("br-open-422", u.Fingerprint); err != nil {
		t.Fatal(err)
	}
	m := plan("br-open-422")
	if m.Action != "merge" || m.Behind != 0 {
		t.Fatalf("güncellemeden sonra merge adımı bekleniyordu: %+v", m)
	}
}

func TestQueueStepWithoutProjectIsRefused(t *testing.T) {
	isoHome(t)
	mgr := NewManager(t.TempDir())
	if mgr.NoProject() {
		if _, err := mgr.QueueStepPlan("x"); err == nil {
			t.Fatal("proje yokken demo veriyle çalışmamalı")
		}
		if _, err := mgr.QueueStepApply("x", "y"); err == nil {
			t.Fatal("proje yokken demo veriyle çalışmamalı")
		}
	}
}

// Yavaş makinede CI, plan ile uygulama arasında RUNNING→SUCCESS dönebilir; "update" adımı CI'a bakmadan karar verildiği için
// parmak izi bundan etkilenmemeli (CI'da görülen kuyruk-koşusu testi kırılganlığı). "merge" ise CI'a BAĞLI kalır.
func TestQueueStepUpdateFingerprintIgnoresCIButMergeDoesNot(t *testing.T) {
	up := model.BranchUpdatePlan{Status: "ready", HeadSHA: "h", BaseSHA: "b", Behind: 2}
	fp := func(ci string) string {
		return decideQueueStep(model.QueueStepPlan{Branch: "x", PR: 1}, queueStepInput{PRState: "OPEN", CI: ci, Update: up}).Fingerprint
	}
	if fp("RUNNING") == "" || fp("RUNNING") != fp("SUCCESS") {
		t.Fatalf("update parmak izi CI'dan bağımsız olmalı: %q vs %q", fp("RUNNING"), fp("SUCCESS"))
	}
	up.Status = "up_to_date"
	if fp("SUCCESS") == "" || fp("SUCCESS") == queueStepFingerprint("merge", "x", "h", "b", "UNKNOWN") {
		t.Fatal("merge parmak izi CI'a bağlı kalmalı")
	}
}
