package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestRepoRootForSubfolderWorktreeAndNonRepo(t *testing.T) {
	dir, run := newTempRepo(t)
	sub := filepath.Join(dir, "internal", "deep")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if got := repoRootFor(dir); got != dir {
		t.Fatalf("a repo root must be kept exactly as given, got %q", got)
	}
	gotSub := repoRootFor(sub)
	if gotSub == "" {
		t.Fatal("a subfolder must resolve to its repo root")
	}
	if a, _ := filepath.EvalSymlinks(gotSub); a != mustReal(t, dir) {
		t.Fatalf("subfolder root = %q, want %q", gotSub, dir)
	}

	wt := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-q", "-b", "wt-branch", wt)
	if repoRootFor(wt) == "" {
		t.Fatal("a git worktree (its .git is a FILE) must be recognised as a repo")
	}

	if got := repoRootFor(t.TempDir()); got != "" {
		t.Fatalf("a plain folder is not a repo, got %q", got)
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Eskiden alt klasörden veya worktree'den başlatınca GitArc sessizce demo (sahte veri) deposuna düşüyordu.
func TestStartupFromASubfolderOpensTheRealRepo(t *testing.T) {
	dir, _ := newTempRepo(t)
	sub := filepath.Join(dir, "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(sub)
	if mgr.NoProject() {
		t.Fatal("a subfolder of a repo must open that repo")
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Mode != "live" {
		t.Fatalf("must be live, got %q (%s)", snap.Repo.Mode, snap.Repo.Name)
	}
}

func TestNoRepoMeansNoProjectNotADemo(t *testing.T) {
	empty := t.TempDir()
	chdir(t, empty) // çalışma klasörü de depo değil
	mgr := NewManager(empty)
	if !mgr.NoProject() {
		t.Fatal("no repo and no saved project: must be the no-project state")
	}
	if mgr.DemoAllowed() {
		t.Fatal("demos are not allowed by default")
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Mode != "none" || snap.Repo.ID != "none" {
		t.Fatalf("snapshot must say mode=none, got %+v", snap.Repo)
	}
	if len(snap.Repos) != 0 {
		t.Fatalf("no demo repos may be offered in the picker, got %+v", snap.Repos)
	}
	if snap.Branches == nil || snap.Nodes == nil {
		t.Fatal("arrays must be empty, not null (the UI iterates them)")
	}
	if _, err := mgr.SwitchRepo("sim-sandbox"); err == nil || !strings.Contains(err.Error(), "demo") {
		t.Fatalf("switching to a demo must be refused by default, got %v", err)
	}
}

func TestDemoIsOptInOnly(t *testing.T) {
	m := NewManager("sim")
	if m.NoProject() || !m.DemoAllowed() {
		t.Fatal("`-repo sim` explicitly opens the demo")
	}
	snap, _ := m.Snapshot()
	if snap.Repo.Mode != "simulation" || len(snap.Repos) < 2 {
		t.Fatalf("demo must list its repos, got %+v", snap.Repo)
	}
	legacy := NewManager("")
	if legacy.NoProject() || !legacy.DemoAllowed() {
		t.Fatal("the embedded/test constructor keeps the simulation")
	}
}

func TestRemovingTheLastProjectShowsNoProjectNotTheDemo(t *testing.T) {
	dir, _ := newTempRepo(t)
	chdir(t, t.TempDir())
	mgr := NewManager(dir)
	if mgr.NoProject() {
		t.Fatal("precondition: the repo is open")
	}
	snap, err := mgr.RemoveProject("live-custom")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Mode != "none" || !mgr.NoProject() {
		t.Fatalf("after removing the last project: no-project, got %+v", snap.Repo)
	}
}

func TestAddProjectAcceptsASubfolderAndOpensIt(t *testing.T) {
	dir, _ := newTempRepo(t)
	sub := filepath.Join(dir, "x", "y")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	chdir(t, t.TempDir())
	mgr := NewManager(t.TempDir())
	if !mgr.NoProject() {
		t.Fatal("precondition: nothing is open")
	}
	snap, err := mgr.AddProject(sub, "")
	if err != nil {
		t.Fatalf("AddProject(subfolder): %v", err)
	}
	if snap.Repo.Mode != "live" || mgr.NoProject() {
		t.Fatalf("the project must open live, got %+v", snap.Repo)
	}
	if _, err := mgr.AddProject(t.TempDir(), ""); err == nil {
		t.Fatal("a non-repo folder must still be rejected")
	}
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-dir").CombinedOutput(); err != nil {
		t.Fatalf("sanity: %v %s", err, out)
	}
}
