package gitengine

import (
	"strings"
	"testing"

	"gitarc/internal/model"
)

func TestIsPrunableTable(t *testing.T) {
	cases := []struct {
		name string
		b    model.BranchArc
		want bool
	}{
		{"merged with local copy", model.BranchArc{State: "merged", HasLocal: true}, true},
		{"merged remote-only, nothing ahead", model.BranchArc{State: "merged", HasRemote: true}, true},
		{"merged remote-only but has unique commits", model.BranchArc{State: "merged", HasRemote: true, AheadMain: 2}, false},
		{"merged entry with no local and no remote (nothing to delete)", model.BranchArc{State: "merged", OriginGone: true}, false},
		{"open branch whose origin is gone, local exists", model.BranchArc{State: "open", OriginGone: true, HasLocal: true}, true},
		{"plain open branch", model.BranchArc{State: "open", HasLocal: true, HasRemote: true}, false},
		{"merged PR branch still open in state", model.BranchArc{State: "open", HasLocal: true, PR: &model.PRStatus{ReviewState: "MERGED"}}, true},
		{"long-lived is protected", model.BranchArc{State: "merged", HasLocal: true, LongLived: true}, false},
		{"virtual working tree", model.BranchArc{ID: "local-wip-working-tree", State: "open", HasLocal: true, OriginGone: true}, false},
		{"HEAD with uncommitted work", model.BranchArc{State: "merged", HasLocal: true, IsHEAD: true, UncommittedCount: 3}, false},
		{"HEAD clean", model.BranchArc{State: "merged", HasLocal: true, IsHEAD: true}, true},
	}
	for _, c := range cases {
		if got := IsPrunable(c.b); got != c.want {
			t.Errorf("%s: IsPrunable = %v, want %v", c.name, got, c.want)
		}
	}
}

// Kullanıcının yakaladığı hata: bir kez Temizle'ye basınca bazı dallar kalıyor ve sayı değişmiyordu
// (yerel kopya ilk tıkta, uzak kopya ancak ikinci tıkta siliniyordu).
func TestPruneGoneFinishesInOneClick(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	for _, n := range []string{"feat/both", "feat/local-only-left", "feat/remote-only"} {
		addFeatureBranch(t, dir, run, n)
		run("push", "-u", "origin", n)
		run("merge", "--no-ff", n, "-m", "Merge pull request #1 from "+n)
	}
	run("push", "origin", "main")
	run("push", "origin", "--delete", "feat/local-only-left")
	run("branch", "-D", "feat/remote-only")
	run("fetch", "--prune", "origin")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	before, _ := mgr.Snapshot()
	n := 0
	for _, b := range before.Branches {
		if b.Prunable {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("expected 3 prunable branches before cleanup, got %d", n)
	}

	snap, removed, err := mgr.PruneGone()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Errorf("removed = %d, want 3 (each branch counted once)", removed)
	}
	for _, b := range snap.Branches {
		if b.Prunable {
			t.Errorf("still prunable after ONE cleanup: %s (local=%v remote=%v)", b.Name, b.HasLocal, b.HasRemote)
		}
	}
	out, _ := runGitOut(dir, "branch", "-a")
	if strings.Contains(out, "feat/") {
		t.Errorf("leftover feature branches after one cleanup:\n%s", out)
	}
}
