package gitengine

import (
	"context"
	"strings"
	"testing"
)

func TestSetBaseBranchPinsAndClears(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	run("branch", "develop")
	run("branch", "feat/x")
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DefaultBranch != "main" || snap.Repo.BaseBranchPinned {
		t.Fatalf("auto detection first: %+v", snap.Repo)
	}
	got := strings.Join(snap.Repo.BaseCandidates, ",")
	if !strings.HasPrefix(got, "main,develop") || !strings.Contains(got, "feat/x") {
		t.Fatalf("candidates should list the current base first, then long-lived, then the rest: %s", got)
	}

	snap, err = mgr.SetBaseBranch("develop")
	if err != nil {
		t.Fatalf("SetBaseBranch: %v", err)
	}
	if snap.Repo.DefaultBranch != "develop" || !snap.Repo.BaseBranchPinned {
		t.Fatalf("pin did not take effect: %+v", snap.Repo)
	}
	if detectDefaultBranch(context.Background(), dir) != "develop" {
		t.Fatal("detectDefaultBranch must honour the pin")
	}
	// Sabitleme kalıcıdır: yeni bir Manager aynı depoda aynı seçimi görür.
	mgr2 := NewManager(dir)
	if _, err := mgr2.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	if s2, _ := mgr2.Snapshot(); s2.Repo.DefaultBranch != "develop" {
		t.Fatalf("pin must persist across managers, got %q", s2.Repo.DefaultBranch)
	}

	snap, err = mgr.SetBaseBranch("")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DefaultBranch != "main" || snap.Repo.BaseBranchPinned {
		t.Fatalf("clearing must return to auto detection: %+v", snap.Repo)
	}
}

func TestSetBaseBranchRejectsBadInput(t *testing.T) {
	dir, _ := newGitHubLikeRepo(t)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"-rf", "a..b", "yok-boyle-bir-dal"} {
		if _, err := mgr.SetBaseBranch(bad); err == nil {
			t.Errorf("SetBaseBranch(%q) must fail", bad)
		}
	}
	if s, _ := mgr.Snapshot(); s.Repo.BaseBranchPinned {
		t.Fatal("a rejected request must not pin anything")
	}
}

func TestStalePinFallsBackToAutoDetection(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	run("branch", "develop")
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.SetBaseBranch("develop"); err != nil {
		t.Fatal(err)
	}
	run("branch", "-D", "develop")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DefaultBranch != "main" || snap.Repo.BaseBranchPinned {
		t.Fatalf("a pin to a deleted branch must be ignored: %+v", snap.Repo)
	}
}

func TestSetBaseBranchRefusedInSimulation(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.SetBaseBranch("develop"); err == nil || !strings.Contains(err.Error(), "simülasyon") {
		t.Fatalf("simulation must refuse, got %v", err)
	}
}
