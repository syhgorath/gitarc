package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// updateWorld: GitHub benzeri bare uzak depo + yerel klon. origin/feat/x ve origin/feat/y var; sonra origin/main
// BAŞKA bir klondan ilerletilir (feat/x artık main'in 1 commit gerisinde).
type updateWorld struct {
	t     *testing.T
	dir   string // yerel klon (kullanıcının çalışma dizini)
	bare  string
	run   func(args ...string)
	other string // ikinci klon: "başkasının" push'unu temsil eder
}

func newUpdateWorld(t *testing.T) *updateWorld {
	t.Helper()
	isoHome(t)
	dir, run := newGitHubLikeRepo(t)
	bareOut, _ := runGitOut(dir, "remote", "get-url", "origin")
	w := &updateWorld{t: t, dir: dir, run: run, bare: strings.TrimSpace(bareOut)}
	for _, name := range []string{"feat/x", "feat/y"} {
		addFeatureBranch(t, dir, run, name)
		run("push", "-u", "origin", name)
	}
	w.other = filepath.Join(t.TempDir(), "other")
	if out, err := exec.Command("git", "clone", "-q", w.bare, w.other).CombinedOutput(); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	for _, kv := range [][]string{{"user.name", "Other"}, {"user.email", "o@e.x"}} {
		isoGit(t, w.other, "config", kv[0], kv[1])
	}
	return w
}

// pushToOrigin: ikinci klondan origin'e commit atar (branch üzerinde) ve yerel klonu tazeler.
func (w *updateWorld) pushToOrigin(branch, file, body string) {
	w.t.Helper()
	w.pushFromOther(branch, file, body)
	w.run("fetch", "-q", "origin")
}

// pushFromOther: yalnızca "başkasının push'u" (ikinci klon); yerel klona DOKUNMAZ. Arka planda kendi fetch'ini çalıştıran
// bir koşu varken kullanılır (eşzamanlı iki fetch aynı uzak-izleme ref kilidi için yarışır).
func (w *updateWorld) pushFromOther(branch, file, body string) {
	w.t.Helper()
	isoGit(w.t, w.other, "fetch", "-q", "origin")
	isoGit(w.t, w.other, "checkout", "-q", "-B", branch, "origin/"+branch)
	if err := os.WriteFile(filepath.Join(w.other, file), []byte(body), 0644); err != nil {
		w.t.Fatal(err)
	}
	isoGit(w.t, w.other, "add", "-A")
	isoGit(w.t, w.other, "commit", "-qm", "other: "+file)
	isoGit(w.t, w.other, "push", "-q", "origin", branch)
}

func (w *updateWorld) remoteTip(branch string) string {
	return isoGit(w.t, w.bare, "rev-parse", branch)
}

func TestBranchUpdatePlanReadyForCleanBehindBranch(t *testing.T) {
	w := newUpdateWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	mgr := switchLive(t, w.dir)

	plan, err := mgr.BranchUpdatePlan("feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "ready" || plan.Behind != 1 || plan.Fingerprint == "" || plan.Mode != "live" {
		t.Fatalf("temiz ve geride kalmış dal 'ready' olmalı: %+v", plan)
	}
	if plan.BaseRef != "origin/main" || plan.HeadSHA == "" || plan.BaseSHA == "" {
		t.Fatalf("taban ve uçlar belli olmalı: %+v", plan)
	}
}

// Plan SALT OKUNUR: uzak dal, yerel ref'ler, HEAD, indeks ve çalışma ağacı aynı kalır.
func TestBranchUpdatePlanNeverWrites(t *testing.T) {
	w := newUpdateWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	if err := os.WriteFile(filepath.Join(w.dir, "kirli.txt"), []byte("benim işim\n"), 0644); err != nil {
		t.Fatal(err)
	}
	state := func() string {
		return strings.Join([]string{
			isoGit(t, w.dir, "status", "--porcelain"), isoGit(t, w.dir, "for-each-ref"),
			isoGit(t, w.dir, "rev-parse", "HEAD"), w.remoteTip("feat/x"), w.remoteTip("main"),
		}, "\n--\n")
	}
	before := state()
	mgr := switchLive(t, w.dir)
	if _, err := mgr.BranchUpdatePlan("feat/x"); err != nil {
		t.Fatal(err)
	}
	if after := state(); after != before {
		t.Fatalf("plan yazmamalı:\nÖNCE:\n%s\nSONRA:\n%s", before, after)
	}
}

func TestUpdateBranchFromBasePushesAMergeCommitAndNothingElse(t *testing.T) {
	w := newUpdateWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	if err := os.WriteFile(filepath.Join(w.dir, "kirli.txt"), []byte("benim işim\n"), 0644); err != nil {
		t.Fatal(err)
	}
	headBefore := isoGit(t, w.dir, "rev-parse", "HEAD")
	oldTip := w.remoteTip("feat/x")
	yTip := w.remoteTip("feat/y")
	mainTip := w.remoteTip("main")
	mgr := switchLive(t, w.dir)

	plan, _ := mgr.BranchUpdatePlan("feat/x")
	// onaysız ve yanlış parmak izi: hiçbir şey olmaz
	for _, c := range []string{"", "yanlis"} {
		if _, err := mgr.UpdateBranchFromBase("feat/x", c); err == nil {
			t.Fatalf("confirm=%q reddedilmeli", c)
		}
	}
	if w.remoteTip("feat/x") != oldTip {
		t.Fatal("reddedilen denemeler uzak dalı değiştirmemeli")
	}

	if _, err := mgr.UpdateBranchFromBase("feat/x", plan.Fingerprint); err != nil {
		t.Fatalf("güncelleme: %v", err)
	}
	newTip := w.remoteTip("feat/x")
	if newTip == oldTip {
		t.Fatal("uzak dal ilerlemeli")
	}
	parents := strings.Fields(isoGit(t, w.bare, "rev-list", "--parents", "-n", "1", newTip))
	if len(parents) != 3 || parents[1] != oldTip || parents[2] != mainTip {
		t.Fatalf("yeni uç, [eski uç, main ucu] ebeveynli bir MERGE commit olmalı (rebase/force yok): %v", parents)
	}
	if out := isoGit(t, w.bare, "ls-tree", "--name-only", newTip); !strings.Contains(out, "main-yeni.txt") {
		t.Fatalf("main'deki yeni dosya dalda olmalı:\n%s", out)
	}
	// başka dallara ve main'e dokunulmadı; kullanıcının çalışma ağacı ve HEAD'i aynı
	if w.remoteTip("feat/y") != yTip || w.remoteTip("main") != mainTip {
		t.Fatal("başka uzak dallar değişmemeli")
	}
	if isoGit(t, w.dir, "rev-parse", "HEAD") != headBefore {
		t.Fatal("kullanıcının HEAD'i değişmemeli")
	}
	if b, _ := os.ReadFile(filepath.Join(w.dir, "kirli.txt")); string(b) != "benim işim\n" {
		t.Fatal("kirli dosya korunmalı")
	}
	// yerel dal uzakla aynı noktadaydı: ilerletilir
	if isoGit(t, w.dir, "rev-parse", "feat/x") != newTip {
		t.Fatal("uzakla aynı noktadaki yerel dal ilerletilmeli")
	}
	// artık güncel
	if p2, _ := mgr.BranchUpdatePlan("feat/x"); p2.Status != "up_to_date" {
		t.Fatalf("güncellemeden sonra 'up_to_date' olmalı: %+v", p2)
	}
	if _, err := mgr.UpdateBranchFromBase("feat/x", plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "güncel") {
		t.Fatalf("zaten güncel dal için uygulama reddedilmeli: %v", err)
	}
}

// Çakışma varsa HİÇBİR ŞEY yapılmaz: uzak dal aynı kalır, plan çakışan dosyayı söyler.
func TestUpdateBranchFromBaseRefusesWhenItConflicts(t *testing.T) {
	w := newUpdateWorld(t)
	// main, feat/x'in dokunduğu dosyayı farklı değiştirir
	w.pushToOrigin("main", "feat_x.txt", "main'in farklı içeriği\n")
	mgr := switchLive(t, w.dir)
	oldTip := w.remoteTip("feat/x")

	plan, err := mgr.BranchUpdatePlan("feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "conflict" || len(plan.ConflictFiles) != 1 || plan.ConflictFiles[0] != "feat_x.txt" || plan.Fingerprint != "" {
		t.Fatalf("çakışma planda görünmeli, parmak izi olmamalı: %+v", plan)
	}
	if _, err := mgr.UpdateBranchFromBase("feat/x", "herhangi"); err == nil || !strings.Contains(err.Error(), "çakış") {
		t.Fatalf("çakışan dal güncellenmemeli: %v", err)
	}
	if w.remoteTip("feat/x") != oldTip {
		t.Fatal("çakışmada uzak dal değişmemeli")
	}
}

// Uzak dal plan alındıktan SONRA başkası tarafından ilerletilirse parmak izi tutmaz: işlem reddedilir.
func TestUpdateBranchFromBaseRefusesWhenTheRemoteBranchMovedAfterThePlan(t *testing.T) {
	w := newUpdateWorld(t)
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	mgr := switchLive(t, w.dir)
	plan, _ := mgr.BranchUpdatePlan("feat/x")

	w.pushToOrigin("feat/x", "baskasi.txt", "başkasının commit'i\n") // plan alındıktan sonra
	movedTip := w.remoteTip("feat/x")
	if _, err := mgr.UpdateBranchFromBase("feat/x", plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "planı yeniden") {
		t.Fatalf("eski plan reddedilmeli: %v", err)
	}
	if w.remoteTip("feat/x") != movedTip {
		t.Fatal("başkasının commit'i ezilmemeli, uzak dal değişmemeli")
	}
}

// Yerelde push edilmemiş commit varsa yerel dala dokunulmaz (force/sıfırlama yok); uzak yine güncellenir.
func TestUpdateBranchFromBaseNeverRewritesALocalBranchThatHasUnpushedCommits(t *testing.T) {
	w := newUpdateWorld(t)
	w.run("checkout", "-q", "feat/x")
	if err := os.WriteFile(filepath.Join(w.dir, "yerel.txt"), []byte("yerel\n"), 0644); err != nil {
		t.Fatal(err)
	}
	w.run("add", "-A")
	w.run("commit", "-qm", "yerel-only commit")
	w.run("checkout", "-q", "main")
	localTip := isoGit(t, w.dir, "rev-parse", "feat/x")
	w.pushToOrigin("main", "main-yeni.txt", "m\n")
	mgr := switchLive(t, w.dir)

	plan, _ := mgr.BranchUpdatePlan("feat/x")
	if _, err := mgr.UpdateBranchFromBase("feat/x", plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if isoGit(t, w.dir, "rev-parse", "feat/x") != localTip {
		t.Fatal("yerel-only commit'i olan yerel dal olduğu gibi kalmalı")
	}
}

func TestBranchUpdatePlanBlockedCases(t *testing.T) {
	w := newUpdateWorld(t)
	w.run("checkout", "-q", "-b", "yalniz-yerel")
	w.run("checkout", "-q", "main")
	mgr := switchLive(t, w.dir)
	if p, err := mgr.BranchUpdatePlan("yalniz-yerel"); err != nil || p.Status != "blocked" || !strings.Contains(p.Reason, "uzakta yok") {
		t.Fatalf("uzakta olmayan dal engellenmeli: %+v %v", p, err)
	}
	if _, err := mgr.BranchUpdatePlan("yok-boyle-dal"); err == nil {
		t.Fatal("bilinmeyen dal hata vermeli")
	}
	if _, err := mgr.UpdateBranchFromBase("yalniz-yerel", "x"); err == nil {
		t.Fatal("engellenen dal güncellenmemeli")
	}
}

func TestBranchUpdateInSimulationFollowsTheSameRules(t *testing.T) {
	mgr := NewManager("")
	snap, _ := mgr.Snapshot()
	var behindID string
	for _, b := range snap.Branches {
		if b.State == "open" && b.BehindMain > 0 && (b.Conflict == nil || !b.Conflict.HasConflict) {
			behindID = b.ID
			break
		}
	}
	if behindID == "" {
		t.Skip("demo verisinde geride kalmış temiz dal yok")
	}
	plan, err := mgr.BranchUpdatePlan(behindID)
	if err != nil || plan.Status != "ready" || plan.Fingerprint == "" || plan.Mode != "simulation" {
		t.Fatalf("simülasyon planı: %+v %v", plan, err)
	}
	if _, err := mgr.UpdateBranchFromBase(behindID, "yanlis"); err == nil {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if _, err := mgr.UpdateBranchFromBase(behindID, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	if p2, _ := mgr.BranchUpdatePlan(behindID); p2.Status != "up_to_date" {
		t.Fatalf("güncellemeden sonra güncel olmalı: %+v", p2)
	}
}
