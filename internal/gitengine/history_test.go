package gitengine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitarc/internal/model"
)

// mergeFeature creates origin-pushed feature branch with one commit and merges it with --no-ff into main.
func mergeFeature(t *testing.T, dir string, run func(args ...string), name string) {
	t.Helper()
	run("checkout", "-b", name)
	if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(name, "/", "_")+".txt"), []byte(name), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: "+name)
	run("push", "-u", "origin", name)
	run("checkout", "main")
	run("merge", "--no-ff", "-m", "Merge branch '"+name+"'", name)
}

func addMainCommits(t *testing.T, dir string, run func(args ...string), n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("filler_%d.txt", i)), []byte(fmt.Sprintf("%d-%d", time.Now().UnixNano(), i)), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-m", fmt.Sprintf("chore: filler %d", i))
	}
}

func findBranch(snap model.CockpitSnapshot, name string) *model.BranchArc {
	for i := range snap.Branches {
		if snap.Branches[i].Name == name {
			return &snap.Branches[i]
		}
	}
	return nil
}

// A merged branch whose merge commit fell out of the 12-commit spine window has no real position.
// It must be flagged HistoryOnly (not drawn as a fake full-width parabola), while a recent merge keeps its arch.
func TestMergedBranchOutsideSpineWindowIsHistoryOnly(t *testing.T) {
	dir, run := newTempRepo(t)
	bare := filepath.Join(t.TempDir(), "remote.git")
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", bare)
	run("push", "-u", "origin", "main")

	mergeFeature(t, dir, run, "feat/old-merged") // will fall out of the 12-commit window
	addMainCommits(t, dir, run, 16)
	mergeFeature(t, dir, run, "feat/recent-merged") // stays inside the window
	addMainCommits(t, dir, run, 2)
	run("push", "origin", "main")
	run("branch", "-D", "feat/old-merged")
	run("branch", "-D", "feat/recent-merged")

	snap, err := CollectLiveRepo("live-hist", "hist", dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	old := findBranch(snap, "feat/old-merged")
	if old == nil {
		t.Fatalf("old merged branch missing from snapshot: %+v", snap.Branches)
	}
	if old.State != "merged" || !old.HistoryOnly {
		t.Fatalf("branch merged outside the spine window must be merged+HistoryOnly, got state=%q historyOnly=%v", old.State, old.HistoryOnly)
	}

	recent := findBranch(snap, "feat/recent-merged")
	if recent == nil {
		t.Fatalf("recent merged branch missing from snapshot")
	}
	if recent.HistoryOnly {
		t.Fatalf("branch merged inside the spine window has a real merge point and must be drawn")
	}
	if recent.EndSpineIndex >= len(snap.Nodes)-1 {
		t.Fatalf("recent merge must sit at its real spine position, got end=%d of %d nodes", recent.EndSpineIndex, len(snap.Nodes))
	}

	// History-only branches never consume a lane.
	if old.Lane != 0 {
		t.Fatalf("history-only branch must not be lane-packed, got lane %d", old.Lane)
	}
}

func TestEnterpriseSimulationHasHistoryOnlyBranches(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.SwitchRepo("sim-enterprise")
	if err != nil {
		t.Fatal(err)
	}
	history := 0
	for _, b := range snap.Branches {
		if !b.HistoryOnly {
			continue
		}
		history++
		if b.State != "merged" {
			t.Errorf("history-only branch %s must be merged, got %q", b.Name, b.State)
		}
		if b.Lane != 0 {
			t.Errorf("history-only branch %s must not have a lane, got %d", b.Name, b.Lane)
		}
		if b.PR == nil || b.PR.Number == 0 {
			t.Errorf("history-only branch %s should carry its merged PR", b.Name)
		}
	}
	if history < 5 {
		t.Fatalf("enterprise demo must showcase the history chip, got %d history-only branches", history)
	}
}

func TestHistoryOnlyJSONContract(t *testing.T) {
	raw, err := json.Marshal(model.BranchArc{ID: "x", HistoryOnly: true})
	if err != nil || !strings.Contains(string(raw), `"historyOnly":true`) {
		t.Fatalf("flagged branch must serialize historyOnly=true: %s %v", raw, err)
	}
	raw, _ = json.Marshal(model.BranchArc{ID: "y"})
	if strings.Contains(string(raw), "historyOnly") {
		t.Fatalf("unflagged branch must omit historyOnly (backwards compatible): %s", raw)
	}
}
