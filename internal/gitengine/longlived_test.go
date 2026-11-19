package gitengine

import (
	"os/exec"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// Aynı tablo tests/js/fallback-longlived.test.mjs içinde de var: Go ve tarayıcı yedeği birebir aynı kararı vermeli.
var longLivedCases = []struct {
	name string
	want bool
}{
	{"develop", true}, {"Develop", true}, {"dev", true}, {"staging", true}, {"production", true},
	{"release/1.2", true}, {"release-2.x", true}, {"releases/v3", true}, {"support/legacy", true},
	{"maintenance/2024", true}, {"stable-1", true}, {"1.x", true}, {"v2", true}, {"2.3.4", true}, {"v1.2.x", true},
	{"master", true}, // varsayılan dal main iken master ayrı bir uzun ömürlü daldır
	{"main", false},  // varsayılan dal omurgadır, şerit sayılmaz
	{"feat/develop-docs", false}, {"fix/release-notes", false}, {"feature/dev", false},
	{"devops", false}, {"hotfix/x", false}, {"nextgen", false}, {"", false},
}

func TestIsLongLivedBranch(t *testing.T) {
	for _, c := range longLivedCases {
		if got := IsLongLivedBranch(c.name, "main"); got != c.want {
			t.Errorf("IsLongLivedBranch(%q) = %v, want %v", c.name, got, c.want)
		}
	}
	if IsLongLivedBranch("master", "master") {
		t.Error("the default branch itself must not count as a long-lived lane")
	}
}

func TestMarkLongLivedFlagsOnlyMatchingBranches(t *testing.T) {
	bs := []model.BranchArc{{Name: "develop"}, {Name: "feat/x"}, {Name: "release/1.0"}}
	markLongLived(bs, "main")
	if !bs[0].LongLived || bs[1].LongLived || !bs[2].LongLived {
		t.Fatalf("unexpected flags: %+v", bs)
	}
}

func TestSimulationKeepsLongLivedBranches(t *testing.T) {
	mgr := NewManager("")
	mgr.mu.Lock()
	mgr.simRepo.branches = append(mgr.simRepo.branches, model.BranchArc{
		ID: "br-develop", Name: "develop", Hemisphere: "upper", State: "merged", HasLocal: true, OriginGone: true, Lane: 1,
	})
	mgr.mu.Unlock()

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, b := range snap.Branches {
		if b.ID == "br-develop" {
			found = true
			if !b.LongLived {
				t.Fatal("develop must be flagged longLived in the snapshot")
			}
		}
	}
	if !found {
		t.Fatal("develop missing from snapshot")
	}
	if _, err := mgr.DeleteBranch("br-develop"); err == nil || !strings.Contains(err.Error(), "uzun ömürlü") {
		t.Fatalf("expected a long-lived refusal, got %v", err)
	}
	snap, _, err = mgr.PruneGone()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range snap.Branches {
		if b.ID == "br-develop" && (!b.HasLocal || !b.OriginGone) {
			t.Fatalf("PruneGone must leave develop untouched, got %+v", b)
		}
	}
}

func TestLiveKeepsMergedDevelopOnPruneAndDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}
	dir, run := newGitHubLikeRepo(t)
	for _, name := range []string{"develop", "feat/done"} {
		addFeatureBranch(t, dir, run, name)
		run("push", "-u", "origin", name)
		run("merge", "--no-ff", name, "-m", "Merge pull request #7 from "+name)
	}
	run("push", "origin", "main")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var devID string
	for _, b := range snap.Branches {
		if b.Name == "develop" {
			devID = b.ID
			if !b.LongLived {
				t.Fatalf("develop must be flagged longLived: %+v", b)
			}
		}
		if b.Name == "feat/done" && b.LongLived {
			t.Fatal("a feature branch must not be flagged")
		}
	}
	if devID == "" {
		t.Fatal("develop not in snapshot")
	}

	if _, err := mgr.DeleteBranch(devID); err == nil || !strings.Contains(err.Error(), "uzun ömürlü") {
		t.Fatalf("DeleteBranch(develop) must be refused, got %v", err)
	}
	if _, _, err := mgr.PruneGone(); err != nil {
		t.Fatalf("PruneGone: %v", err)
	}
	if out, _ := runGitOut(dir, "branch", "--list", "develop"); !strings.Contains(out, "develop") {
		t.Fatal("local develop was deleted by PruneGone")
	}
	if out, _ := runGitOut(dir, "ls-remote", "--heads", "origin", "develop"); !strings.Contains(out, "refs/heads/develop") {
		t.Fatal("remote develop was deleted by PruneGone")
	}
	if out, _ := runGitOut(dir, "branch", "--list", "feat/done"); strings.Contains(out, "feat/done") {
		t.Fatalf("the merged feature branch should have been pruned, got %q", out)
	}
}

func TestDiagnosticsDoNotOfferToPruneLongLivedBranches(t *testing.T) {
	bs := []model.BranchArc{
		{ID: "a", Name: "develop", State: "merged", OriginGone: true, LongLived: true},
		{ID: "b", Name: "feat/old", State: "merged", OriginGone: true},
	}
	var ids []string
	for _, d := range BuildDiagnostics(bs) {
		if d.Kind == "prune_candidate" {
			ids = append(ids, d.BranchName)
		}
	}
	if len(ids) != 1 || ids[0] != "feat/old" {
		t.Fatalf("only the feature branch may be a prune candidate, got %v", ids)
	}
}
