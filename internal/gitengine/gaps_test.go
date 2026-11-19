package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func switchLive(t *testing.T, dir string) *Manager {
	t.Helper()
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}
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
	snap, err := mgr.ProtectDefaultBranch()
	if err != nil {
		t.Fatal(err)
	}
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

func TestCheckForUpdatesLiveReadsRemoteTags(t *testing.T) {
	dir, run := newTempRepo(t)
	remote := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", remote).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", remote)
	run("push", "-u", "origin", "main")
	run("tag", "v99.0.0")
	run("tag", "v1.2.0")
	run("push", "origin", "--tags")

	mgr := switchLive(t, dir)
	res, err := mgr.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if res["latestVersion"] != "99.0.0" || res["updateAvailable"] != true {
		t.Fatalf("expected latest 99.0.0 with update available, got %+v", res)
	}
	if res["currentVersion"] != AppVersion {
		t.Fatalf("currentVersion mismatch: %+v", res)
	}

	// No remote tags → no update.
	dir2, _ := newTempRepo(t)
	mgr2 := switchLive(t, dir2)
	res, _ = mgr2.CheckForUpdates()
	if res["updateAvailable"] != false {
		t.Fatalf("no tags must not report an update: %+v", res)
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
