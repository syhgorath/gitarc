package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

func TestAssignLanesIntervalPacking(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "long-open", Name: "feat/long", Hemisphere: "upper", State: "open", StartSpineIndex: 0, EndSpineIndex: 5},
		{ID: "short-merged", Name: "fix/short", Hemisphere: "upper", State: "merged", StartSpineIndex: 1, EndSpineIndex: 2},
		{ID: "second-merged", Name: "fix/next", Hemisphere: "upper", State: "merged", StartSpineIndex: 2, EndSpineIndex: 4},
		{ID: "lower-local", Name: "wip/local", Hemisphere: "lower", State: "open", StartSpineIndex: 1, EndSpineIndex: 3},
	}

	AssignLanes(branches)

	byID := map[string]model.BranchArc{}
	for _, b := range branches {
		byID[b.ID] = b
	}

	if byID["short-merged"].Lane != 1 {
		t.Fatalf("expected short-merged on Lane 1, got %d", byID["short-merged"].Lane)
	}
	if byID["second-merged"].Lane != 1 {
		t.Fatalf("expected back-to-back second-merged to reuse Lane 1, got %d", byID["second-merged"].Lane)
	}
	if byID["long-open"].Lane <= 1 {
		t.Fatalf("expected overlapping long-open to move to outer lane > 1, got %d", byID["long-open"].Lane)
	}
	if byID["lower-local"].Lane != 1 {
		t.Fatalf("expected lower hemisphere lane packing to start independently at Lane 1, got %d", byID["lower-local"].Lane)
	}
}

func TestExtractCILog(t *testing.T) {
	raw := `2026-11-16T07:12:05Z === RUN   TestShellGoldCompare
    compare_test.go:118: mismatch on key 'hardening_pve_apply'
--- FAIL: TestShellGoldCompare (0.42s)
panic: runtime error: index out of range [3] with length 3
	go/internal/shellgold/compare.go:94 +0x1b4
FAIL	keylab/internal/shellgold	0.614s`

	extracts := ExtractCILog(raw)
	if len(extracts) < 2 {
		t.Fatalf("expected at least 2 extracts (fail + panic), got %d: %+v", len(extracts), extracts)
	}

	var sawFail, sawPanic bool
	for _, ex := range extracts {
		if ex.Kind == "test_fail" && ex.Symbol == "TestShellGoldCompare" && strings.Contains(ex.FileLine, "compare_test.go:118") {
			sawFail = true
			if !strings.Contains(ex.ReproCmd, "TestShellGoldCompare") {
				t.Errorf("expected reproCmd to include test filter, got %q", ex.ReproCmd)
			}
		}
		if ex.Kind == "panic" && strings.Contains(ex.Message, "index out of range") && strings.Contains(ex.FileLine, "compare.go:94") {
			sawPanic = true
		}
	}
	if !sawFail || !sawPanic {
		t.Fatalf("missing expected extracts: sawFail=%v sawPanic=%v (%+v)", sawFail, sawPanic, extracts)
	}
}

func TestCrossPRCollisions(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "a", Name: "feat/a", State: "open", PR: &model.PRStatus{Number: 10}, ChangedFiles: []string{"CHANGELOG.md", "a.go"}},
		{ID: "b", Name: "feat/b", State: "open", PR: &model.PRStatus{Number: 11}, ChangedFiles: []string{"CHANGELOG.md", "b.go"}},
		{ID: "m", Name: "feat/merged", State: "merged", PR: &model.PRStatus{Number: 9}, ChangedFiles: []string{"CHANGELOG.md"}},
	}

	ComputeCrossPRCollisions(branches)

	if len(branches[0].CrossPR) != 1 || branches[0].CrossPR[0].OtherPRNumber != 11 {
		t.Fatalf("expected feat/a to collide only with #11, got %+v", branches[0].CrossPR)
	}
	if len(branches[1].CrossPR) != 1 || branches[1].CrossPR[0].SharedFiles[0] != "CHANGELOG.md" {
		t.Fatalf("expected feat/b shared file CHANGELOG.md, got %+v", branches[1].CrossPR)
	}
	if len(branches[2].CrossPR) != 0 {
		t.Fatalf("merged branch should not trigger cross-PR collision, got %+v", branches[2].CrossPR)
	}
}

func TestParseMergeTreeOutput(t *testing.T) {
	if c := ParseMergeTreeOutput(0, "tree-oid\n"); c != nil {
		t.Fatalf("expected nil conflict on clean merge-tree, got %+v", c)
	}

	out := `deadbeef1234
CONFLICT (content): Merge conflict in CHANGELOG.md
CONFLICT (content): Merge conflict in go.mod
`
	c := ParseMergeTreeOutput(1, out)
	if c == nil || !c.HasConflict || !c.IsChangelogOrLock || len(c.Files) != 2 {
		t.Fatalf("unexpected ParseMergeTreeOutput result: %+v", c)
	}
	if c.Strategy != "lock-tidy" {
		t.Fatalf("expected lock-tidy strategy when go.mod present, got %q", c.Strategy)
	}
}

func TestSimulationLifecycleActions(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("initial snapshot failed: %v", err)
	}
	if len(snap.Nodes) == 0 || len(snap.Branches) == 0 || len(snap.Diagnostics) == 0 {
		t.Fatalf("expected rich initial simulation snapshot")
	}

	// Merging conflicted br-open-421 before AutoResolve must fail
	if _, err := mgr.MergeBranch("br-open-421"); err == nil {
		t.Fatalf("expected merge to fail while CHANGELOG conflict is unresolved")
	}

	// AutoResolve then Merge should succeed and add a new merge node
	beforeNodes := len(snap.Nodes)
	snap, err = mgr.AutoResolveConflict("br-open-421")
	if err != nil {
		t.Fatalf("AutoResolveConflict failed: %v", err)
	}
	snap, err = mgr.MergeBranch("br-open-421")
	if err != nil {
		t.Fatalf("MergeBranch after AutoResolve failed: %v", err)
	}
	if len(snap.Nodes) != beforeNodes+1 {
		t.Fatalf("expected spine node count to increment by 1, got %d -> %d", beforeNodes, len(snap.Nodes))
	}

	// Promote local lower branch to upper PR
	snap, err = mgr.PromoteBranch("br-local-wip")
	if err != nil {
		t.Fatalf("PromoteBranch failed: %v", err)
	}
	var promoted *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].ID == "br-local-wip" {
			promoted = &snap.Branches[i]
		}
	}
	if promoted == nil || promoted.Hemisphere != "upper" || promoted.PR == nil {
		t.Fatalf("expected promoted branch in upper hemisphere with PR, got %+v", promoted)
	}

	// Create new branch
	snap, err = mgr.CreateBranch("feat/new-svg-badge", "lower", "")
	if err != nil {
		t.Fatalf("CreateBranch failed: %v", err)
	}
	foundNew := false
	for _, b := range snap.Branches {
		if b.Name == "feat/new-svg-badge" && b.IsHEAD {
			foundNew = true
		}
	}
	if !foundNew {
		t.Fatalf("newly created branch not found as HEAD")
	}

	// Prune gone branches
	snap, removed, err := mgr.PruneGone()
	if err != nil {
		t.Fatalf("PruneGone failed: %v", err)
	}
	if removed == 0 {
		t.Fatalf("expected at least 1 gone branch pruned")
	}
	for _, b := range snap.Branches {
		if b.OriginGone || b.State == "gone" {
			t.Fatalf("found unpruned gone branch %s", b.Name)
		}
	}

	// Sync spine
	snap, err = mgr.SyncSpine()
	if err != nil {
		t.Fatalf("SyncSpine failed: %v", err)
	}
	if snap.Repo.Ahead != 0 || snap.Repo.Behind != 0 {
		t.Fatalf("expected spine ahead/behind to be 0 after SyncSpine, got %d/%d", snap.Repo.Ahead, snap.Repo.Behind)
	}
}

func TestCollectLiveRepoTemp(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	dir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=tester@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
	}

	runGit("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n- init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "CHANGELOG.md")
	runGit("commit", "-m", "chore: initial commit")

	runGit("checkout", "-b", "feat/changelog-entry")
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n- feature change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("commit", "-am", "feat: add changelog entry")

	runGit("checkout", "main")
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("# Changelog\n- main release change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("commit", "-am", "chore: main release update")

	snap, err := CollectLiveRepo("live-test", "temp-repo", dir, nil)
	if err != nil {
		t.Fatalf("CollectLiveRepo failed: %v", err)
	}
	if snap.Repo.Mode != "live" || len(snap.Nodes) < 2 {
		t.Fatalf("unexpected live repo snapshot: %+v", snap.Repo)
	}
	var featBranch *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].Name == "feat/changelog-entry" {
			featBranch = &snap.Branches[i]
		}
	}
	if featBranch == nil {
		t.Fatalf("expected feat/changelog-entry in live branches, got %+v", snap.Branches)
	}
	if featBranch.Conflict == nil || !featBranch.Conflict.HasConflict || !featBranch.Conflict.IsChangelogOrLock {
		t.Fatalf("expected git merge-tree to detect CHANGELOG.md conflict, got %+v", featBranch.Conflict)
	}
}

func TestParseUpstreamTrack(t *testing.T) {
	cases := []struct {
		track  string
		ahead  int
		behind int
		gone   bool
	}{
		{"[ahead 3, behind 1]", 3, 1, false},
		{"[ahead 2]", 2, 0, false},
		{"[behind 5]", 0, 5, false},
		{"[gone]", 0, 0, true},
		{"", 0, 0, false},
	}
	for _, tc := range cases {
		a, b, g := parseUpstreamTrack(tc.track)
		if a != tc.ahead || b != tc.behind || g != tc.gone {
			t.Errorf("parseUpstreamTrack(%q) = (%d,%d,%v), want (%d,%d,%v)", tc.track, a, b, g, tc.ahead, tc.behind, tc.gone)
		}
	}
}

func TestExtractRunIDFromURL(t *testing.T) {
	u := "https://github.com/syhgorath/gitarc/actions/runs/1198432109/job/334455"
	if got := extractRunIDFromURL(u); got != "1198432109" {
		t.Fatalf("expected 1198432109, got %q", got)
	}
	if got := extractRunIDFromURL("https://example.com/status"); got != "" {
		t.Fatalf("expected empty run id, got %q", got)
	}
}

func TestValidateBranchName(t *testing.T) {
	valid := []string{"feat/dogfooding", "fix/v0.2.0-track", "chore_ci"}
	for _, name := range valid {
		if err := validateBranchName(name); err != nil {
			t.Errorf("expected %q to be valid, got %v", name, err)
		}
	}
	invalid := []string{"", "-upload-pack", "--exec", "feat/../main", "feat//bar", "feat/", "foo.lock", "bad space"}
	for _, name := range invalid {
		if err := validateBranchName(name); err == nil {
			t.Errorf("expected %q to be rejected by validateBranchName", name)
		}
	}
}
