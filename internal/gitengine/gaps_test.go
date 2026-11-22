package gitengine

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitarc/internal/selfupdate"
)

// settleBackground: arka plandaki `git fetch` / `gh` sorguları (anlık görüntü başlatır) BİTENE kadar bekler. Beklenmezse test dizini
// silinirken bu işlemler hâlâ .git içine yazabilir ve `TempDir RemoveAll cleanup: directory not empty` ile test (yalnızca yavaş CI'da) düşer.
func settleBackground(limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		bgMu.Lock()
		states := make([]*repoBG, 0, len(bgStates))
		for _, st := range bgStates {
			states = append(states, st)
		}
		bgMu.Unlock()
		busy := false
		for _, st := range states {
			st.mu.Lock()
			if st.fetchBusy || st.ghBusy {
				busy = true
			}
			st.mu.Unlock()
		}
		if !busy {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func switchLive(t *testing.T, dir string) *Manager {
	t.Helper()
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}
	// t.Cleanup'lar TERS sırada çalışır: bu, t.TempDir'in silinmesinden ÖNCE çalışır.
	t.Cleanup(func() { settleBackground(20 * time.Second) })
	return mgr
}

func hasBranch(snapNames []string, name string) bool {
	for _, n := range snapNames {
		if n == name {
			return true
		}
	}
	return false
}

func TestSwitchRepoUnknownAndSimulationTargets(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.SwitchRepo("nope"); err == nil {
		t.Fatal("unknown repo id must error")
	}
	snap, err := mgr.SwitchRepo("sim-enterprise")
	if err != nil || snap.Repo.ID != "sim-enterprise" || len(snap.Branches) < 10 {
		t.Fatalf("enterprise switch failed: %v %+v", err, snap.Repo)
	}
	snap, err = mgr.SwitchRepo("sim-keylab") // legacy alias
	if err != nil || snap.Repo.ID != "sim-sandbox" {
		t.Fatalf("legacy alias must map to sandbox: %v %s", err, snap.Repo.ID)
	}
}

func TestResetSimulationRestoresState(t *testing.T) {
	mgr := NewManager("")
	before, _ := mgr.Snapshot()
	if _, err := mgr.CreateBranch("feat/temp-reset", "lower", ""); err != nil {
		t.Fatal(err)
	}
	mid, _ := mgr.Snapshot()
	if len(mid.Branches) <= len(before.Branches) {
		t.Fatal("expected extra branch before reset")
	}
	after := mgr.ResetSimulation()
	if len(after.Branches) != len(before.Branches) {
		t.Fatalf("reset must restore branch count: %d vs %d", len(after.Branches), len(before.Branches))
	}

	if _, err := mgr.SwitchRepo("sim-enterprise"); err != nil {
		t.Fatal(err)
	}
	ent := mgr.ResetSimulation()
	if ent.Repo.ID != "sim-enterprise" {
		t.Fatalf("reset must stay on enterprise, got %s", ent.Repo.ID)
	}
}

func TestClearLogsEmptiesBufferAndFile(t *testing.T) {
	// "Temizle" artık yalnızca etkin projenin kayıtlarını siler (başka projelerinki kalır; bkz. TestIso_ClearLogsOnlyClearsTheActiveProject).
	// Ortak HOME'daki başka testlerin kayıtları dosyada kalabileceğinden bu test yalıtılmış bir HOME kullanır.
	isoHome(t)
	mgr := NewManager("")
	mgr.recordLog("info", "Test", "", "mesaj", "")
	snap, _ := mgr.Snapshot()
	if len(snap.Logs) == 0 {
		t.Fatal("expected at least one log before clear")
	}
	snap, err := mgr.ClearLogs()
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Logs) != 0 {
		t.Fatalf("logs must be empty after clear, got %d", len(snap.Logs))
	}
	if saved := loadSavedLogs(); len(saved) != 0 {
		t.Fatalf("persisted logs must be cleared, got %d", len(saved))
	}
}

func TestProtectDefaultBranchSimulation(t *testing.T) {
	mgr := NewManager("")
	snap := applyProfile(t, mgr, "temel", nil)
	if !snap.Repo.BranchProtected || snap.Repo.ProtectionSummary == "" {
		t.Fatalf("expected protected sim repo: %+v", snap.Repo)
	}
}

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.8.0", "0.8.0", 0}, {"0.9.0", "0.8.0", 1}, {"0.8.0", "0.8.1", -1},
		{"v1.0.0", "0.9.9", 1}, {"1.0", "1.0.0", 0}, {"1.0.0-rc1", "1.0.0", 0}, {"2", "10", -1},
	}
	for _, c := range cases {
		if got := compareSemver(c.a, c.b); got != c.want {
			t.Errorf("compareSemver(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

// fakeUpdater: ağa çıkmadan GitHub release'lerini taklit eder.
func fakeUpdater(latestLoc string, assets map[string][]byte, signers []selfupdate.Signer) func() *selfupdate.Updater {
	return func() *selfupdate.Updater {
		return &selfupdate.Updater{
			Version: AppVersion, GOOS: "linux", GOARCH: "amd64", Signers: signers,
			Head: func(u string) (int, string) {
				if strings.HasSuffix(u, "/releases/latest") {
					return 302, latestLoc
				}
				if _, ok := assets[u]; ok {
					return 200, ""
				}
				return 404, ""
			},
			Download: func(u, dest string, max int64) error {
				b, ok := assets[u]
				if !ok {
					return fmt.Errorf("HTTP 404")
				}
				return os.WriteFile(dest, b, 0o600)
			},
			RunVersion: func(string) (string, error) { return "gitarc 99.0.0", nil },
		}
	}
}

// Sürüm kontrolü GitArc'ın KENDİ release'lerine bakar; açık projenin etiketlerine DEĞİL.
func TestCheckForUpdatesReadsGitArcsOwnReleasesNotTheOpenProjects(t *testing.T) {
	signers := selfupdate.ParseAllowedSigners(readFileT(t, "../selfupdate/testdata/signers"))
	base := "https://github.com/syhgorath/gitarc/releases/download/v99.0.0/gitarc-99.0.0-linux-amd64"
	assets := map[string][]byte{base: []byte("x"), base + ".sig": []byte("y")}

	dir, _ := newTempRepo(t) // açık proje: etiketi/release'i olmayan başka bir depo
	mgr := switchLive(t, dir)
	mgr.SetUpdater(fakeUpdater("https://github.com/syhgorath/gitarc/releases/tag/v99.0.0", assets, signers))
	res, err := mgr.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if res["latestVersion"] != "99.0.0" || res["updateAvailable"] != true || res["installable"] != true || res["asset"] != "gitarc-99.0.0-linux-amd64" {
		t.Fatalf("yeni sürüm kurulabilir olmalı: %+v", res)
	}
	if _, bad := res["checkError"]; bad {
		t.Fatalf("okundu: checkError olmamalı: %+v", res)
	}
	// imzacı yok → güncelleme VAR ama kurulamaz (düğme gösterilmez), neden yazılır
	mgr.SetUpdater(fakeUpdater("https://github.com/syhgorath/gitarc/releases/tag/v99.0.0", assets, []selfupdate.Signer{}))
	res, _ = mgr.CheckForUpdates()
	if res["updateAvailable"] != true || res["installable"] != false || res["note"] == nil {
		t.Fatalf("imzacı yoksa kurulamaz ve neden yazılmalı: %+v", res)
	}
	// takım sunucusu modu → kurulamaz
	mgr.SetUpdater(fakeUpdater("https://github.com/syhgorath/gitarc/releases/tag/v99.0.0", assets, signers))
	mgr.SetTeamMode(true)
	res, _ = mgr.CheckForUpdates()
	if res["installable"] != false || !strings.Contains(fmt.Sprint(res["note"]), "takım") {
		t.Fatalf("takım modunda otomatik güncelleme kapalı olmalı: %+v", res)
	}
	mgr.SetTeamMode(false)
	// release yok / aynı sürüm → güncelleme yok
	mgr.SetUpdater(fakeUpdater("https://github.com/syhgorath/gitarc/releases/tag/v"+AppVersion, assets, signers))
	res, _ = mgr.CheckForUpdates()
	if res["updateAvailable"] != false {
		t.Fatalf("aynı sürümde güncelleme olmamalı: %+v", res)
	}
}

// Sürüm listesi okunamazsa (ağ yok) "güncel" DENMEZ: checkError döner ve updateAvailable false kalır; arayüz "denetlenemedi" yazar.
func TestCheckForUpdatesUnreachableIsReportedNotGreen(t *testing.T) {
	mgr := NewManager("")
	mgr.SetUpdater(func() *selfupdate.Updater {
		return &selfupdate.Updater{Version: AppVersion, Head: func(string) (int, string) { return 0, "" }}
	})
	res, err := mgr.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	msg, _ := res["checkError"].(string)
	if msg == "" || res["updateAvailable"] != false || res["installable"] != false {
		t.Fatalf("okunamadı: checkError dolu, güncelleme/kurulabilir false beklenirdi: %+v", res)
	}
}

func readFileT(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ApplyUpdate: takım modunda kapalı; imza/sürüm hataları kullanıcı günlüğüne error olarak yazılır ve çalışan ikili değişmez.
func TestApplyUpdateIsBlockedInTeamModeAndLogsFailures(t *testing.T) {
	mgr := NewManager("")
	mgr.SetTeamMode(true)
	if _, err := mgr.ApplyUpdate("99.0.0"); err == nil || !strings.Contains(err.Error(), "takım") {
		t.Fatalf("takım modunda kapalı olmalı: %v", err)
	}
	mgr.SetTeamMode(false)
	signers := selfupdate.ParseAllowedSigners(readFileT(t, "../selfupdate/testdata/signers"))
	// indirilen içerik imzayla UYUŞMUYOR → reddedilir
	base := "https://github.com/syhgorath/gitarc/releases/download/v99.0.0/gitarc-99.0.0-linux-amd64"
	assets := map[string][]byte{base: []byte("sahte"), base + ".sig": []byte(readFileT(t, "../selfupdate/testdata/fake-binary.sig"))}
	exe := filepath.Join(t.TempDir(), "gitarc")
	if err := os.WriteFile(exe, []byte("ESKI"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr.SetUpdater(func() *selfupdate.Updater {
		u := fakeUpdater("https://github.com/syhgorath/gitarc/releases/tag/v99.0.0", assets, signers)()
		u.ExePath = exe
		return u
	})
	if _, err := mgr.ApplyUpdate("99.0.0"); err == nil || !strings.Contains(err.Error(), "İMZA") {
		t.Fatalf("imza uyuşmuyorsa reddedilmeli: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "ESKI" {
		t.Fatal("reddedilen güncellemede ikili değişmemeli")
	}
	var logged bool
	for _, l := range mgr.recentLogs() {
		if l.Action == "Kendini Güncelleme" && l.Level == "error" {
			logged = true
		}
	}
	if !logged {
		t.Fatal("başarısız güncelleme günlüğe error olarak yazılmalı")
	}
}

func TestQuickRepoSignatureChangesWithHEAD(t *testing.T) {
	mgr := NewManager("")
	if sig := mgr.QuickRepoSignature(); !strings.HasPrefix(sig, "sim-") {
		t.Fatalf("simulation signature must start with sim-, got %q", sig)
	}

	dir, run := newTempRepo(t)
	live := switchLive(t, dir)
	before := live.QuickRepoSignature()
	if !strings.HasPrefix(before, "live-custom:") {
		t.Fatalf("unexpected live signature %q", before)
	}
	run("checkout", "-b", "feat/sig")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	after := live.QuickRepoSignature()
	if before == after {
		t.Fatalf("signature should change after HEAD/index change: %q", after)
	}
}

func TestDismissProposalAndFileRoundTrip(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := switchLive(t, dir)

	path := proposalFilePath(dir)
	if err := os.WriteFile(path, []byte(`{"branchName":"feat/x","commitMessage":"feat: x"}`), 0644); err != nil {
		t.Fatal(err)
	}
	prop := mgr.GetPendingProposal()
	if prop == nil {
		t.Fatal("proposal must load from .git IPC file")
	}
	snap, err := mgr.DismissProposal()
	if err != nil {
		t.Fatal(err)
	}
	if snap.PendingProposal != nil {
		t.Fatal("proposal must be cleared in snapshot")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("proposal file must be removed from disk")
	}

	// Corrupt JSON must not yield a proposal.
	_ = os.WriteFile(path, []byte("{broken"), 0644)
	if p, err := loadProposalFromFile(dir); err == nil || p != nil {
		t.Fatalf("corrupt proposal must error, got %v %v", p, err)
	}
}

func TestAddAndRemoveProject(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := NewManager("")

	if _, err := mgr.AddProject("  ", ""); err == nil {
		t.Fatal("empty path must error")
	}
	if _, err := mgr.AddProject(t.TempDir(), ""); err == nil {
		t.Fatal("non-git dir must error")
	}

	snap, err := mgr.AddProject(dir, "Demo")
	if err != nil {
		t.Fatalf("AddProject: %v", err)
	}
	if snap.Repo.Mode != "live" {
		t.Fatalf("expected live snapshot, got %+v", snap.Repo)
	}
	saved := loadSavedProjects()
	if len(saved) != 1 || saved[0].Name != "Demo" {
		t.Fatalf("project not persisted to isolated HOME: %+v", saved)
	}

	// Adding the same path again renames, never duplicates.
	if _, err := mgr.AddProject(dir, "Renamed"); err != nil {
		t.Fatal(err)
	}
	saved = loadSavedProjects()
	if len(saved) != 1 || saved[0].Name != "Renamed" {
		t.Fatalf("duplicate add must rename in place: %+v", saved)
	}

	if _, err := mgr.RemoveProject("sim-sandbox"); err == nil {
		t.Fatal("demo repo must not be removable")
	}
	snap, err = mgr.RemoveProject(snap.Repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Mode == "live" {
		t.Fatalf("after removing the only live repo expect simulation, got %+v", snap.Repo)
	}
	if got := loadSavedProjects(); len(got) != 0 {
		t.Fatalf("project must be removed from projects.json: %+v", got)
	}
}

func TestDiscardWorkingTree(t *testing.T) {
	// Simulation
	sim := NewManager("")
	snap, err := sim.DiscardWorkingTree()
	if err != nil || snap.Repo.DirtyCount != 0 {
		t.Fatalf("sim discard failed: %v dirty=%d", err, snap.Repo.DirtyCount)
	}

	// Live
	dir, _ := newTempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("u"), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := switchLive(t, dir)
	snap, err = mgr.DiscardWorkingTree()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DirtyCount != 0 {
		t.Fatalf("expected clean tree, dirty=%d", snap.Repo.DirtyCount)
	}
	if _, err := os.Stat(filepath.Join(dir, "untracked.txt")); !os.IsNotExist(err) {
		t.Fatal("untracked file must be removed")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "README.md"))
	if string(b) != "# demo\n" {
		t.Fatalf("tracked file must be restored, got %q", b)
	}
}

func TestSyncSpineLiveFastForwardsFromOrigin(t *testing.T) {
	dir, run := newTempRepo(t)
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "main")

	// A second clone pushes a new commit to origin/main.
	other := t.TempDir()
	if out, err := exec.Command("git", "clone", remote, other).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	env := append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.com")
	for _, args := range [][]string{{"commit", "--allow-empty", "-m", "feat: upstream"}, {"push", "origin", "main"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = other
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}

	mgr := switchLive(t, dir)
	if _, err := mgr.SyncSpine(); err != nil {
		t.Fatalf("SyncSpine: %v", err)
	}
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "feat: upstream" {
		t.Fatalf("local main must fast-forward to origin, got %q", out)
	}
}

func TestMergeBranchLocalNoFFAndDirtyGuard(t *testing.T) {
	dir, run := newTempRepo(t)
	run("checkout", "-b", "feat/to-merge")
	if err := os.WriteFile(filepath.Join(dir, "m.txt"), []byte("m\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "m.txt")
	run("commit", "-m", "feat: add m")
	run("checkout", "main")

	// Dirty tree blocks a local merge.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("d"), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := switchLive(t, dir)
	if _, err := mgr.MergeBranch("feat/to-merge", "merge"); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("dirty tree must block merge, got %v", err)
	}
	_ = os.Remove(filepath.Join(dir, "dirty.txt"))

	if _, err := mgr.MergeBranch("feat/to-merge", "merge"); err != nil {
		t.Fatalf("local merge failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "m.txt")); err != nil {
		t.Fatal("merged file must exist on main")
	}
	if _, err := mgr.MergeBranch("ghost/branch", "merge"); err == nil {
		t.Fatal("unknown branch must error")
	}
}

func TestMergeBranchSquashLocal(t *testing.T) {
	dir, run := newTempRepo(t)
	run("checkout", "-b", "feat/squash-me")
	for _, f := range []string{"s1.txt", "s2.txt"} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(f), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", f)
		run("commit", "-m", "feat: "+f)
	}
	run("checkout", "main")
	mgr := switchLive(t, dir)
	if _, err := mgr.MergeBranch("feat/squash-me", "squash"); err != nil {
		t.Fatalf("squash merge failed: %v", err)
	}
	out, _ := exec.Command("git", "-C", dir, "log", "-1", "--format=%s").Output()
	if !strings.Contains(string(out), "squash merge feat/squash-me") {
		t.Fatalf("expected squash commit, got %q", out)
	}
}

func TestUpdatePRMetaLiveRequiresOpenPR(t *testing.T) {
	dir, run := newTempRepo(t)
	run("checkout", "-b", "feat/no-pr")
	mgr := switchLive(t, dir)
	_, err := mgr.UpdatePRMeta("feat/no-pr", "t", "b")
	if err == nil {
		t.Fatal("branch without PR must error")
	}
}

func TestSimUpdatePRMetaAndCreateBranchGuards(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.UpdatePRMeta("br-open-421", "yeni başlık", "yeni gövde"); err != nil {
		t.Fatalf("sim UpdatePRMeta: %v", err)
	}
	snap, _ := mgr.Snapshot()
	var found bool
	for _, b := range snap.Branches {
		if b.ID == "br-open-421" && b.PR != nil && b.PR.Title == "yeni başlık" {
			found = true
		}
	}
	if !found {
		t.Fatal("PR title not updated in simulation")
	}
	if _, err := mgr.UpdatePRMeta("br-local-wip", "x", "y"); err == nil {
		t.Fatal("branch without PR must error")
	}
	if _, err := mgr.UpdatePRMeta("unknown-id", "x", "y"); err == nil {
		t.Fatal("unknown branch must error")
	}
	if _, err := mgr.CreateBranch("", "lower", ""); err == nil {
		t.Fatal("empty name must error")
	}
	if _, err := mgr.CreateBranch("feat/dup-sim", "weird", ""); err != nil {
		t.Fatalf("invalid hemisphere must fall back to lower: %v", err)
	}
	if _, err := mgr.CreateBranch("feat/dup-sim", "lower", ""); err == nil {
		t.Fatal("duplicate sim branch must error")
	}
}

func TestPlaygroundDefaultPathUsesHome(t *testing.T) {
	p, err := DefaultPlaygroundPath()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if p != filepath.Join(home, ".gitarc", "playground") {
		t.Fatalf("unexpected playground path %q", p)
	}
}

// settleBackground gerçekten bekler: meşgul bir arka plan işi bitmeden dönmez, bitince hemen döner (testlerin dizin silme yarışını önler).
func TestSettleBackgroundWaitsForBusyBackgroundWork(t *testing.T) {
	st := bgFor(t.TempDir())
	st.mu.Lock()
	st.fetchBusy = true
	st.mu.Unlock()
	done := make(chan time.Duration, 1)
	go func() {
		t0 := time.Now()
		settleBackground(5 * time.Second)
		done <- time.Since(t0)
	}()
	time.Sleep(150 * time.Millisecond)
	select {
	case d := <-done:
		t.Fatalf("iş sürerken dönmemeli (döndü: %v)", d)
	default:
	}
	st.mu.Lock()
	st.fetchBusy = false
	st.mu.Unlock()
	select {
	case d := <-done:
		if d < 140*time.Millisecond {
			t.Fatalf("iş bitmeden önce dönmüş: %v", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("iş bitince dönmeli")
	}
}
