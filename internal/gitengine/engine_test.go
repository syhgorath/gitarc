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
	raw := `2026-11-16T07:12:05.110Z === RUN   TestShellGoldCompare
2026-11-16T07:12:06.489Z     compare_test.go:118: mismatch on key 'hardening_pve_apply'
2026-11-16T07:12:06.520Z --- FAIL: TestShellGoldCompare (0.42s)
2026-11-16T07:12:06.529Z panic: runtime error: index out of range [3] with length 3
2026-11-16T07:12:06.531Z 	go/internal/shellgold/compare.go:94 +0x1b4
2026-11-16T07:12:06.532Z FAIL	keylab/internal/shellgold	0.614s`

	extracts := ExtractCILog(raw)
	if len(extracts) != 2 {
		t.Fatalf("expected exactly 2 extracts (fail + panic, no false lint_fail), got %d: %+v", len(extracts), extracts)
	}

	var sawFail, sawPanic bool
	for _, ex := range extracts {
		if strings.Contains(ex.Message, "2026-11-16T") {
			t.Errorf("expected ISO timestamp stripped from message, got %q", ex.Message)
		}
		if ex.Kind == "test_fail" && ex.Symbol == "TestShellGoldCompare" && strings.Contains(ex.FileLine, "compare_test.go:118") {
			sawFail = true
			if !strings.Contains(ex.ReproCmd, "TestShellGoldCompare") || !strings.Contains(ex.ReproCmd, "./internal/shellgold") {
				t.Errorf("expected reproCmd to include package and test filter, got %q", ex.ReproCmd)
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
	if _, err := mgr.MergeBranch("br-open-421", "merge"); err == nil {
		t.Fatalf("expected merge to fail while CHANGELOG conflict is unresolved")
	}

	// AutoResolve then Merge should succeed and add a new merge node
	beforeNodes := len(snap.Nodes)
	snap, err = mgr.AutoResolveConflict("br-open-421")
	if err != nil {
		t.Fatalf("AutoResolveConflict failed: %v", err)
	}
	snap, err = mgr.MergeBranch("br-open-421", "squash")
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
	if featBranch.Hemisphere != "lower" {
		t.Fatalf("expected unpushed local branch without PR to open in lower hemisphere, got %q", featBranch.Hemisphere)
	}
	if featBranch.Conflict == nil || !featBranch.Conflict.HasConflict || !featBranch.Conflict.IsChangelogOrLock {
		t.Fatalf("expected git merge-tree to detect CHANGELOG.md conflict, got %+v", featBranch.Conflict)
	}
}

func TestSimulationDeleteBranch(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.DeleteBranch("br-local-exp")
	if err != nil {
		t.Fatalf("DeleteBranch failed: %v", err)
	}
	for _, b := range snap.Branches {
		if b.ID == "br-local-exp" {
			t.Fatalf("expected br-local-exp to be deleted")
		}
	}
	if _, err := mgr.DeleteBranch("non-existent"); err == nil {
		t.Fatalf("expected error deleting unknown branch")
	}
}

func TestFixCIAndMergeLock(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.MergeBranch("br-open-422", ""); err == nil {
		t.Fatalf("expected MergeBranch to fail while CI status is FAILURE")
	}
	snap, err := mgr.FixCI("br-open-422")
	if err != nil {
		t.Fatalf("FixCI failed: %v", err)
	}
	var fixed *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].ID == "br-open-422" {
			fixed = &snap.Branches[i]
		}
	}
	if fixed == nil || fixed.PR == nil || fixed.PR.CI == nil || fixed.PR.CI.State != "SUCCESS" {
		t.Fatalf("expected br-open-422 CI to be SUCCESS after FixCI, got %+v", fixed)
	}
	if _, err := mgr.MergeBranch("br-open-422", "rebase"); err != nil {
		t.Fatalf("expected MergeBranch to succeed after FixCI, got %v", err)
	}
}

func TestCheckoutEditPRAndAmendCommit(t *testing.T) {
	mgr := NewManager("")
	// Non-HEAD branch cannot amend commit
	if _, err := mgr.AmendLastCommit("br-local-wip", "updated msg"); err == nil {
		t.Fatalf("expected AmendLastCommit to fail for non-HEAD branch")
	}
	// Checkout local branch makes it HEAD and allows AmendLastCommit
	snap, err := mgr.CheckoutBranch("br-local-wip")
	if err != nil {
		t.Fatalf("CheckoutBranch failed: %v", err)
	}
	var wip *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].ID == "br-local-wip" {
			wip = &snap.Branches[i]
		}
	}
	if wip == nil || !wip.IsHEAD || !wip.CanAmendCommit {
		t.Fatalf("expected checked-out br-local-wip to be HEAD and amendable: %+v", wip)
	}
	snap, err = mgr.AmendLastCommit("br-local-wip", "feat(doctor): refined keychain ACL check")
	if err != nil {
		t.Fatalf("AmendLastCommit failed: %v", err)
	}
	for _, b := range snap.Branches {
		if b.ID == "br-local-wip" && b.LastCommitMsg != "feat(doctor): refined keychain ACL check" {
			t.Fatalf("unexpected amended commit msg: %q", b.LastCommitMsg)
		}
	}

	// Update PR metadata
	snap, err = mgr.UpdatePRMeta("br-open-421", "feat(report): clean audit export", "Updated PR body")
	if err != nil {
		t.Fatalf("UpdatePRMeta failed: %v", err)
	}
	for _, b := range snap.Branches {
		if b.ID == "br-open-421" && (b.PR.Title != "feat(report): clean audit export" || b.PR.Body != "Updated PR body") {
			t.Fatalf("unexpected PR metadata after UpdatePRMeta: %+v", b.PR)
		}
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

func TestCollectLiveRepoRemoteOnlyAndConflictMutations(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	originDir := t.TempDir()
	workDir := t.TempDir()

	runGitIn := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=tester@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, string(out))
		}
		return string(out)
	}

	runGitIn(originDir, "init", "--bare", "-b", "main")
	runGitIn(workDir, "init", "-b", "main")
	runGitIn(workDir, "config", "user.name", "Tester")
	runGitIn(workDir, "config", "user.email", "tester@example.com")
	runGitIn(workDir, "remote", "add", "origin", originDir)

	// Başlangıç omurgası (main)
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# GitArc Test Repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "add", "CHANGELOG.md", "README.md")
	runGitIn(workDir, "commit", "-m", "chore: initial commit")
	runGitIn(workDir, "push", "-u", "origin", "main")

	// Mutasyon 1: Yerelde silinmiş ama origin üzerinde açık & çakışmalı dal
	runGitIn(workDir, "checkout", "-b", "feat/old-conflicted-pr")
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- feature conflicting entry\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "feat: conflicting changelog update")
	runGitIn(workDir, "push", "-u", "origin", "feat/old-conflicted-pr")
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "branch", "-D", "feat/old-conflicted-pr")

	// Mutasyon 2: Merge edilmiş ama origin'de kalıntısı bırakılmış artık dal
	runGitIn(workDir, "checkout", "-b", "feat/stale-merged-remote")
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# GitArc Test Repo\n- merged feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "feat: merged feature")
	runGitIn(workDir, "push", "-u", "origin", "feat/stale-merged-remote")
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "merge", "--no-ff", "feat/stale-merged-remote", "-m", "Merge pull request #77 from test/feat/stale-merged-remote")
	runGitIn(workDir, "branch", "-d", "feat/stale-merged-remote")

	// Mutasyon 3: main ilerlemesi ve çakışma tetikleme
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- main release conflict update\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "chore: release update on main")
	runGitIn(workDir, "push", "origin", "main")

	// Doğrulama 1: CollectLiveRepo remote-only çakışmalı dalı ve stale merged dalı görmeli
	snap, err := CollectLiveRepo("live-mut", "mut-repo", workDir, nil)
	if err != nil {
		t.Fatalf("CollectLiveRepo failed: %v", err)
	}

	var oldConflicted *model.BranchArc
	var staleEntries []model.BranchArc
	for i := range snap.Branches {
		b := snap.Branches[i]
		if b.Name == "feat/old-conflicted-pr" {
			cp := b
			oldConflicted = &cp
		}
		if b.Name == "feat/stale-merged-remote" {
			staleEntries = append(staleEntries, b)
		}
	}

	if oldConflicted == nil {
		t.Fatalf("expected remote-only branch feat/old-conflicted-pr to be discovered, got branches: %+v", snap.Branches)
	}
	if oldConflicted.HasLocal || !oldConflicted.HasRemote {
		t.Fatalf("expected HasLocal=false HasRemote=true for remote-only branch, got %+v", oldConflicted)
	}
	if oldConflicted.Hemisphere != "upper" || oldConflicted.State != "open" {
		t.Fatalf("expected upper/open branch arc, got hemisphere=%q state=%q", oldConflicted.Hemisphere, oldConflicted.State)
	}
	if oldConflicted.AheadMain != 1 || oldConflicted.BehindMain < 1 {
		t.Fatalf("expected AheadMain=1 BehindMain>=1, got ahead=%d behind=%d", oldConflicted.AheadMain, oldConflicted.BehindMain)
	}
	if oldConflicted.Conflict == nil || !oldConflicted.Conflict.HasConflict || !oldConflicted.Conflict.IsChangelogOrLock {
		t.Fatalf("expected git merge-tree to detect CHANGELOG conflict on remote-only branch, got %+v", oldConflicted.Conflict)
	}

	if len(staleEntries) == 0 {
		t.Fatalf("expected feat/stale-merged-remote to appear as merged arch, got none")
	}
	for _, st := range staleEntries {
		if st.State == "open" {
			t.Fatalf("expected stale merged remote branch to not be open, got %+v", st)
		}
	}

	// Mutasyon 4: Manager.DeleteBranch ile remote-only dalı silme
	mgr := NewManager(workDir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo(live-custom) failed: %v", err)
	}
	afterDel, err := mgr.DeleteBranch(oldConflicted.ID)
	if err != nil {
		t.Fatalf("DeleteBranch(%q) failed: %v", oldConflicted.ID, err)
	}
	for _, b := range afterDel.Branches {
		if b.Name == "feat/old-conflicted-pr" {
			t.Fatalf("expected feat/old-conflicted-pr to be removed after DeleteBranch, still found: %+v", b)
		}
	}

	checkRef := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/feat/old-conflicted-pr")
	checkRef.Dir = originDir
	if err := checkRef.Run(); err == nil {
		t.Fatalf("expected refs/heads/feat/old-conflicted-pr to be deleted in bare origin repo")
	}

	// Mutasyon 5: PruneGone ile merge edilmiş uzak kalıntı dalın origin'den temizlenmesi
	_, removed, err := mgr.PruneGone()
	if err != nil {
		t.Fatalf("PruneGone failed: %v", err)
	}
	if removed < 1 {
		t.Fatalf("expected PruneGone to delete at least 1 stale remote branch, got removed=%d", removed)
	}
	checkStaleRef := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/feat/stale-merged-remote")
	checkStaleRef.Dir = originDir
	if err := checkStaleRef.Run(); err == nil {
		t.Fatalf("expected refs/heads/feat/stale-merged-remote to be deleted in bare origin repo after PruneGone")
	}
}

func TestConflictStudioAndLiveWorktreeResolution(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	originDir := t.TempDir()
	workDir := t.TempDir()

	runGitIn := func(dir string, args ...string) {
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
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, string(out))
		}
	}

	runGitIn(originDir, "init", "--bare", "-b", "main")
	runGitIn(workDir, "init", "-b", "main")
	runGitIn(workDir, "config", "user.name", "Tester")
	runGitIn(workDir, "config", "user.email", "tester@example.com")
	runGitIn(workDir, "remote", "add", "origin", originDir)

	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# Repo\ninitial\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "app.go"), []byte("package main\nfunc val() int { return 1 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "add", "CHANGELOG.md", "README.md", "app.go")
	runGitIn(workDir, "commit", "-m", "chore: init")
	runGitIn(workDir, "push", "-u", "origin", "main")

	// Çakışacak Go kaynak kod dalı -> origin'e pushla ve yerelden sil
	runGitIn(workDir, "checkout", "-b", "feat/go-code-conflict")
	if err := os.WriteFile(filepath.Join(workDir, "app.go"), []byte("package main\nfunc val() int { return 2 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "feat: go code change")
	runGitIn(workDir, "push", "-u", "origin", "feat/go-code-conflict")
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "branch", "-D", "feat/go-code-conflict")

	// Çakışacak özellik dalı -> origin'e pushla ve yerelden sil
	runGitIn(workDir, "checkout", "-b", "feat/conflict-studio")
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- feat branch change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "feat: studio branch change")
	runGitIn(workDir, "push", "-u", "origin", "feat/conflict-studio")
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "branch", "-D", "feat/conflict-studio")

	// main üzerinde çakışma üret
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- main branch change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "app.go"), []byte("package main\nfunc val() int { return 99 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "chore: main release change")
	runGitIn(workDir, "push", "origin", "main")

	// Aktif çalışma dizininde (main) uncommitted kirli dosya bırak
	dirtyText := "# Repo\nuncommitted work on main must stay intact\n"
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte(dirtyText), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(workDir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo(live-custom) failed: %v", err)
	}

	studio, err := mgr.InspectConflictStudio("remote-feat-conflict-studio")
	if err != nil {
		t.Fatalf("InspectConflictStudio failed: %v", err)
	}
	if len(studio.Files) != 1 || studio.Files[0].Path != "CHANGELOG.md" {
		t.Fatalf("expected 1 conflicted file CHANGELOG.md, got %+v", studio.Files)
	}
	if studio.Recommended != "union" || studio.RecommendationReason == "" || len(studio.Strategies) != 4 {
		t.Fatalf("expected CHANGELOG conflict to recommend 'union' with guidance, got rec=%q reason=%q len=%d", studio.Recommended, studio.RecommendationReason, len(studio.Strategies))
	}

	goStudio, err := mgr.InspectConflictStudio("remote-feat-go-code-conflict")
	if err != nil {
		t.Fatalf("InspectConflictStudio(go) failed: %v", err)
	}
	if goStudio.Recommended != "ours" || goStudio.RecommendationReason == "" {
		t.Fatalf("expected .go conflict to recommend 'ours' with reason, got rec=%q reason=%q", goStudio.Recommended, goStudio.RecommendationReason)
	}
	fDiff := studio.Files[0]
	if !strings.Contains(fDiff.ConflictHunks, "<<<<<<<") || fDiff.UnionPreview == "" || fDiff.OursPreview == "" || fDiff.TheirsPreview == "" {
		t.Fatalf("expected populated ConflictHunks and all 3 previews, got %+v", fDiff)
	}
	if !strings.Contains(fDiff.OursPreview, "main branch change") || strings.Contains(fDiff.OursPreview, "feat branch change") {
		t.Fatalf("unexpected OursPreview content: %q", fDiff.OursPreview)
	}
	if !strings.Contains(fDiff.TheirsPreview, "feat branch change") || strings.Contains(fDiff.TheirsPreview, "main branch change") {
		t.Fatalf("unexpected TheirsPreview content: %q", fDiff.TheirsPreview)
	}

	// Çözüm metninde <<<<<<< işareti kalırsa hata dönmeli ve snap.Logs'a yazılmalı
	if _, err := mgr.ResolveBranchConflict("remote-feat-conflict-studio", "custom", map[string]string{
		"CHANGELOG.md": "<<<<<<< bad\nfoo\n=======\nbar\n>>>>>>> bad",
	}, true); err == nil {
		t.Fatalf("expected stray conflict markers in custom content to be rejected")
	}
	errSnap, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot after failed resolve failed: %v", err)
	}
	hasErrLog := false
	for _, lg := range errSnap.Logs {
		if lg.Level == "error" && strings.Contains(lg.Message, "<<<<<<<") {
			hasErrLog = true
		}
	}
	if !hasErrLog {
		t.Fatalf("expected error log entry in snap.Logs after failed resolution, got %+v", errSnap.Logs)
	}

	// İzole worktree üzerinde "ours" (main'i baz al) ile çöz
	snap, err := mgr.ResolveBranchConflict("remote-feat-conflict-studio", "ours", nil, true)
	if err != nil {
		t.Fatalf("ResolveBranchConflict(ours) failed: %v", err)
	}

	// Açık çalışma dizinindeki dirty dosya bozulmamış olmalı
	gotDirty, err := os.ReadFile(filepath.Join(workDir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotDirty) != dirtyText {
		t.Fatalf("expected dirty README.md to remain untouched; got %q, want %q", string(gotDirty), dirtyText)
	}

	var resolvedBranch *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].Name == "feat/conflict-studio" {
			cp := snap.Branches[i]
			resolvedBranch = &cp
			break
		}
	}
	if resolvedBranch == nil {
		t.Fatalf("expected feat/conflict-studio to still exist as resolved open branch")
	}
	if resolvedBranch.Conflict != nil && resolvedBranch.Conflict.HasConflict {
		t.Fatalf("expected feat/conflict-studio conflict to be cleared after worktree resolution, got %+v", resolvedBranch.Conflict)
	}
}

func TestMegaRepoCacheTeamModeAndProtection(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	originDir := t.TempDir()
	workDir := t.TempDir()

	runGitIn := func(dir string, args ...string) {
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
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, string(out))
		}
	}

	runGitIn(originDir, "init", "--bare", "-b", "main")
	runGitIn(workDir, "init", "-b", "main")
	runGitIn(workDir, "config", "user.name", "Tester")
	runGitIn(workDir, "config", "user.email", "tester@example.com")
	runGitIn(workDir, "remote", "add", "origin", originDir)

	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- init\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "add", "CHANGELOG.md")
	runGitIn(workDir, "commit", "-m", "chore: init")
	runGitIn(workDir, "push", "-u", "origin", "main")

	// Yerel dal
	runGitIn(workDir, "checkout", "-b", "feat/alpha")
	if err := os.WriteFile(filepath.Join(workDir, "alpha.txt"), []byte("alpha\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "add", "alpha.txt")
	runGitIn(workDir, "commit", "-m", "feat: alpha")

	// Remote-only çakışmalı dal
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "checkout", "-b", "feat/team-conflict")
	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- team branch change\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "feat: team branch change")
	runGitIn(workDir, "push", "-u", "origin", "feat/team-conflict")
	runGitIn(workDir, "checkout", "main")
	runGitIn(workDir, "branch", "-D", "feat/team-conflict")

	if err := os.WriteFile(filepath.Join(workDir, "CHANGELOG.md"), []byte("# Changelog\n- main line\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "commit", "-am", "chore: main update")
	runGitIn(workDir, "push", "origin", "main")

	// 1. SHA bazlı evalCache doğrulaması (1. çağrı doldurur, 2. çağrı hit döndürür)
	snap1, err := CollectLiveRepo("live-cache", "cache-repo", workDir, nil)
	if err != nil {
		t.Fatalf("first CollectLiveRepo failed: %v", err)
	}
	if snap1.Repo.CurrentUser != "Tester" {
		t.Fatalf("expected CurrentUser=Tester, got %q", snap1.Repo.CurrentUser)
	}
	snap2, err := CollectLiveRepo("live-cache", "cache-repo", workDir, nil)
	if err != nil {
		t.Fatalf("second CollectLiveRepo failed: %v", err)
	}
	if snap2.Repo.CacheHits == 0 {
		t.Fatalf("expected CacheHits > 0 on repeated snapshot when branch/base SHAs are unchanged")
	}

	// 2. Team Server Modu: checkout/amend kilitli ama izole worktree çözümü + delete açık olmalı
	mgr := NewManager(workDir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo failed: %v", err)
	}
	mgr.SetTeamMode(true)
	if !mgr.IsTeamMode() {
		t.Fatalf("expected IsTeamMode() == true")
	}
	if _, err := mgr.CheckoutBranch("local-feat-alpha"); err == nil || !strings.Contains(err.Error(), "Team Server") {
		t.Fatalf("expected CheckoutBranch to be blocked in TeamMode, got err=%v", err)
	}
	if _, err := mgr.AmendLastCommit("local-feat-alpha", "blocked"); err == nil || !strings.Contains(err.Error(), "Team Server") {
		t.Fatalf("expected AmendLastCommit to be blocked in TeamMode, got err=%v", err)
	}
	if _, err := mgr.ResolveBranchConflict("remote-feat-team-conflict", "ours", nil, true); err != nil {
		t.Fatalf("expected isolated worktree ResolveBranchConflict to succeed in TeamMode, got %v", err)
	}
	if _, err := mgr.DeleteBranch("remote-feat-team-conflict"); err != nil {
		t.Fatalf("expected remote DeleteBranch to succeed in TeamMode, got %v", err)
	}

	// 3. Branch Protection ve Update kontrolcüsü (Simülasyon)
	simMgr := NewManager("")
	simSnap, err := simMgr.Snapshot()
	if err != nil {
		t.Fatalf("sim Snapshot failed: %v", err)
	}
	if simSnap.Repo.BranchProtected {
		t.Fatalf("expected simulation initial BranchProtected == false")
	}
	protSnap, err := simMgr.ProtectDefaultBranch()
	if err != nil {
		t.Fatalf("ProtectDefaultBranch failed: %v", err)
	}
	if !protSnap.Repo.BranchProtected {
		t.Fatalf("expected BranchProtected == true after ProtectDefaultBranch")
	}
	updRes, err := simMgr.CheckForUpdates()
	if err != nil {
		t.Fatalf("CheckForUpdates failed: %v", err)
	}
	if updRes["currentVersion"] != AppVersion {
		t.Fatalf("expected CheckForUpdates currentVersion=%q, got %v", AppVersion, updRes["currentVersion"])
	}
}

func TestCommitWorkingTreeAndPromoteWithCustomPRMeta(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	workDir := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = workDir
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
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# GitArc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit("add", "README.md")
	runGit("commit", "-m", "chore: initial commit")

	// Uncommitted (dirty) dosya oluştur
	if err := os.WriteFile(filepath.Join(workDir, "wip_feature.txt"), []byte("yeni ozellik\n"), 0644); err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(workDir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo failed: %v", err)
	}
	snapBefore, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	if snapBefore.Repo.DirtyCount == 0 {
		t.Fatalf("expected DirtyCount > 0 before CommitWorkingTree")
	}

	var foundWip bool
	for _, b := range snapBefore.Branches {
		if b.ID == "local-wip-working-tree" {
			foundWip = true
			if b.DraftPRTitle == "" {
				t.Fatalf("expected DraftPRTitle to be populated for local-wip-working-tree")
			}
		}
	}
	if !foundWip {
		t.Fatalf("expected local-wip-working-tree arc on dirty repo")
	}

	snapAfter, err := mgr.CommitWorkingTree("feat/ui-commit", "feat: commit from ui", "Custom PR Title", "Custom PR Body", false)
	if err != nil {
		t.Fatalf("CommitWorkingTree failed: %v", err)
	}
	if snapAfter.Repo.DirtyCount != 0 {
		t.Fatalf("expected DirtyCount == 0 after CommitWorkingTree, got %d", snapAfter.Repo.DirtyCount)
	}

	var committedBranch *model.BranchArc
	for i := range snapAfter.Branches {
		if snapAfter.Branches[i].Name == "feat/ui-commit" {
			committedBranch = &snapAfter.Branches[i]
			break
		}
	}
	if committedBranch == nil {
		t.Fatalf("expected feat/ui-commit branch after CommitWorkingTree")
	}
	if !committedBranch.IsHEAD || committedBranch.LastCommitMsg != "feat: commit from ui" {
		t.Fatalf("unexpected committed branch state: %+v", *committedBranch)
	}

	// Simülasyonda local dalı pushlamadan önce girilen özel PR başlık & açıklama ile yükseltme
	simMgr := NewManager("")
	promoSnap, err := simMgr.PromoteBranchWithMeta("br-local-wip", "Özel PR Başlığı", "Özel PR Gövdesi")
	if err != nil {
		t.Fatalf("PromoteBranchWithMeta failed: %v", err)
	}
	var promoted *model.BranchArc
	for i := range promoSnap.Branches {
		if promoSnap.Branches[i].ID == "br-local-wip" {
			promoted = &promoSnap.Branches[i]
			break
		}
	}
	if promoted == nil || promoted.Hemisphere != "upper" || promoted.PR == nil {
		t.Fatalf("expected br-local-wip to move to upper hemisphere with PR, got %+v", promoted)
	}
	if promoted.PR.Title != "Özel PR Başlığı" || promoted.PR.Body != "Özel PR Gövdesi" {
		t.Fatalf("expected custom PR title/body preserved, got PR=%+v", *promoted.PR)
	}
}

func TestPruneAndDeleteAutoSwitchFromMergedHEAD(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}

	originDir := t.TempDir()
	workDir := t.TempDir()

	runGitIn := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester",
			"GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=Tester",
			"GIT_COMMITTER_EMAIL=tester@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s failed: %v\n%s", args, dir, err, string(out))
		}
		return strings.TrimSpace(string(out))
	}

	runGitIn(originDir, "init", "--bare", "-b", "main")
	runGitIn(workDir, "init", "-b", "main")
	runGitIn(workDir, "config", "user.name", "Tester")
	runGitIn(workDir, "config", "user.email", "tester@example.com")
	runGitIn(workDir, "remote", "add", "origin", originDir)

	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte("# GitArc\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGitIn(workDir, "add", "README.md")
	runGitIn(workDir, "commit", "-m", "chore: init")
	runGitIn(workDir, "push", "-u", "origin", "main")

	createAndMergeGoneBranch := func(name, file string) {
		runGitIn(workDir, "checkout", "-b", name)
		if err := os.WriteFile(filepath.Join(workDir, file), []byte(name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		runGitIn(workDir, "add", file)
		runGitIn(workDir, "commit", "-m", "feat: "+name)
		runGitIn(workDir, "push", "-u", "origin", name)

		runGitIn(workDir, "checkout", "main")
		runGitIn(workDir, "merge", "--no-ff", name, "-m", "Merge pull request #99 from "+name)
		runGitIn(workDir, "push", "origin", "main")
		runGitIn(workDir, "push", "origin", "--delete", name)
		runGitIn(workDir, "fetch", "--prune", "origin")
		// Kullanıcı bu birleşmiş dalın üstünde kalmış olsun (HEAD = name)
		runGitIn(workDir, "checkout", name)
	}

	mgr := NewManager(workDir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo failed: %v", err)
	}

	// 1. DeleteBranch tek harekette HEAD -> main geçişi yapıp dalı silmeli
	createAndMergeGoneBranch("feat/single-delete-head", "a.txt")
	snap1, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	var targetID string
	for _, b := range snap1.Branches {
		if b.Name == "feat/single-delete-head" {
			targetID = b.ID
			break
		}
	}
	if targetID == "" {
		t.Fatalf("expected feat/single-delete-head in snapshot")
	}
	if _, err := mgr.DeleteBranch(targetID); err != nil {
		t.Fatalf("DeleteBranch on merged HEAD branch failed: %v", err)
	}
	if cur := runGitIn(workDir, "branch", "--show-current"); cur != "main" {
		t.Fatalf("expected HEAD auto-switched to main, got %q", cur)
	}
	if listed := runGitIn(workDir, "branch", "--list", "feat/single-delete-head"); listed != "" {
		t.Fatalf("expected feat/single-delete-head to be deleted, still got %q", listed)
	}

	// 2. PruneGone da HEAD üzerindeki birleşmiş dalı main'e geçip temizlemeli
	createAndMergeGoneBranch("feat/prune-head", "b.txt")
	_, removed, err := mgr.PruneGone()
	if err != nil {
		t.Fatalf("PruneGone on merged HEAD branch failed: %v", err)
	}
	if removed < 1 {
		t.Fatalf("expected at least 1 removed branch, got %d", removed)
	}
	if cur := runGitIn(workDir, "branch", "--show-current"); cur != "main" {
		t.Fatalf("expected HEAD auto-switched to main after PruneGone, got %q", cur)
	}
	if listed := runGitIn(workDir, "branch", "--list", "feat/prune-head"); listed != "" {
		t.Fatalf("expected feat/prune-head to be deleted after PruneGone, still got %q", listed)
	}

	// 3. Simülasyonda PruneGone yeşil birleşmiş yayları koruyup yalnızca yerel kalıntıyı temizlemeli
	simMgr := NewManager("")
	simSnap, simRemoved, err := simMgr.PruneGone()
	if err != nil {
		t.Fatalf("sim PruneGone failed: %v", err)
	}
	if simRemoved == 0 {
		t.Fatalf("expected simulation PruneGone removed > 0")
	}
	foundMerged415 := false
	for _, b := range simSnap.Branches {
		if b.ID == "br-merged-415" {
			foundMerged415 = true
			if b.HasLocal || b.OriginGone || b.IsHEAD {
				t.Fatalf("expected br-merged-415 local remnants cleared, got %+v", b)
			}
		}
	}
	if !foundMerged415 {
		t.Fatalf("expected br-merged-415 merged arch to remain visible as clean green arch")
	}

	// 4. Henüz birleşmemiş boş açık dal (feat/open-empty-head) HEAD iken Push&PR uyarı vermeli, DeleteBranch main'e geçip silebilmeli
	runGitIn(workDir, "checkout", "-b", "feat/open-empty-head")
	snapOpen, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	var openEmptyID string
	for _, b := range snapOpen.Branches {
		if b.Name == "feat/open-empty-head" {
			openEmptyID = b.ID
			break
		}
	}
	if openEmptyID == "" {
		t.Fatalf("expected feat/open-empty-head in snapshot")
	}
	if _, err := mgr.PromoteBranchWithMeta(openEmptyID, "Empty", ""); err == nil || !strings.Contains(err.Error(), "henüz yeni bir commit") {
		t.Fatalf("expected empty branch PromoteBranchWithMeta to fail cleanly before push, got %v", err)
	}
	if _, err := mgr.DeleteBranch(openEmptyID); err != nil {
		t.Fatalf("DeleteBranch on open clean HEAD branch failed: %v", err)
	}
	if cur := runGitIn(workDir, "branch", "--show-current"); cur != "main" {
		t.Fatalf("expected HEAD auto-switched to main after deleting open HEAD branch, got %q", cur)
	}
	if listed := runGitIn(workDir, "branch", "--list", "feat/open-empty-head"); listed != "" {
		t.Fatalf("expected feat/open-empty-head to be deleted, still got %q", listed)
	}
}

func TestRealWorldSquashCrossPRAndProtectionAccuracy(t *testing.T) {
	// 1. Cross-PR hesaplaması YALNIZCA açık PR'lar arasında yapılmalı (Madde 2)
	branches := []model.BranchArc{
		{
			ID:           "b1",
			Name:         "feat/one",
			State:        "open",
			PR:           &model.PRStatus{Number: 101, ReviewState: "OPEN"},
			ChangedFiles: []string{"main.go", "config.json"},
		},
		{
			ID:           "b2",
			Name:         "feat/two",
			State:        "open",
			PR:           &model.PRStatus{Number: 102, ReviewState: "OPEN"},
			ChangedFiles: []string{"main.go", "other.go"},
		},
		{
			ID:           "b3-merged-squash",
			Name:         "feat/three-squash",
			State:        "merged", // Squash merge ile birleşmiş
			PR:           &model.PRStatus{Number: 99, ReviewState: "MERGED"},
			ChangedFiles: []string{"main.go"},
		},
		{
			ID:           "b4-local-no-pr",
			Name:         "feat/four-local",
			State:        "open",
			PR:           nil, // PR açılmamış yerel dal
			ChangedFiles: []string{"main.go"},
		},
	}

	ComputeCrossPRCollisions(branches)

	// b1 ve b2 açık PR olduğu için çarpışmalı
	if len(branches[0].CrossPR) != 1 || branches[0].CrossPR[0].OtherPRNumber != 102 {
		t.Fatalf("expected b1 CrossPR with b2 (#102), got %+v", branches[0].CrossPR)
	}
	if len(branches[1].CrossPR) != 1 || branches[1].CrossPR[0].OtherPRNumber != 101 {
		t.Fatalf("expected b2 CrossPR with b1 (#101), got %+v", branches[1].CrossPR)
	}
	// b3 (merged) ve b4 (PR'ı yok) asla Cross-PR hesaplarına girmemeli (Madde 2)
	if len(branches[2].CrossPR) != 0 {
		t.Fatalf("expected merged branch b3 to have 0 CrossPR, got %+v", branches[2].CrossPR)
	}
	if len(branches[3].CrossPR) != 0 {
		t.Fatalf("expected local branch without PR b4 to have 0 CrossPR, got %+v", branches[3].CrossPR)
	}

	// 2. Auto-Resolve stratejisi: VERSION ve components.lock için insan onayı istemeli (Madde 4)
	branchStub := model.BranchArc{Name: "feat/upgrade"}
	recVer, reasonVer, _ := buildStrategyAdvice(branchStub, []model.ConflictFileDiff{
		{Path: "VERSION", StartLine: 1, EndLine: 1},
	})
	if recVer != "custom" || !strings.Contains(reasonVer, "VERSION") {
		t.Fatalf("expected VERSION to recommend custom/human review, got rec=%q reason=%q", recVer, reasonVer)
	}

	recLock, reasonLock, _ := buildStrategyAdvice(branchStub, []model.ConflictFileDiff{
		{Path: "components.lock", StartLine: 1, EndLine: 5},
	})
	if recLock != "custom" || !strings.Contains(reasonLock, "components.lock") {
		t.Fatalf("expected components.lock to recommend custom/human review, got rec=%q reason=%q", recLock, reasonLock)
	}

	recCL, _, _ := buildStrategyAdvice(branchStub, []model.ConflictFileDiff{
		{Path: "CHANGELOG.md", StartLine: 1, EndLine: 10},
	})
	if recCL != "union" {
		t.Fatalf("expected CHANGELOG.md to recommend union, got rec=%q", recCL)
	}

	// 3. ParseMergeTreeOutput sahte metin (“// main üzerinde ... güncellendi”) içermemeli (Madde 3)
	fakeTreeOutput := "CONFLICT (content): Merge conflict in go/main.go\nAuto-merging go/main.go"
	detail := ParseMergeTreeOutput(1, fakeTreeOutput)
	if detail == nil {
		t.Fatalf("expected non-nil ConflictDetail")
	}
	if strings.Contains(detail.PreviewUnified, "// main üzerinde") {
		t.Fatalf("forbidden placeholder text found in PreviewUnified: %q", detail.PreviewUnified)
	}
	if !strings.Contains(detail.PreviewUnified, "CONFLICT") {
		t.Fatalf("expected real git merge-tree conflict markers in PreviewUnified, got %q", detail.PreviewUnified)
	}

	// 4. Dirty sayacı staged ve untracked dosyaları eksiksiz saymalı (Madde 1)
	porcelainSample := `M  docs/hardening-kullanim.md
?? .mcp.json
?? img/logoyeni.jpg
?? referans/
`
	dirtyFiles := parsePorcelainFiles(porcelainSample)
	if len(dirtyFiles) != 4 {
		t.Fatalf("expected 4 dirty files (1 staged + 3 untracked), got %d (%+v)", len(dirtyFiles), dirtyFiles)
	}

	// 5. HEAD olan dal için git branch -D doğrudan önerilmemeli; git switch main eklenmeli ve kirliyse uyarılmalı (Madde 2)
	headDirtyBranch := model.BranchArc{
		Name:             "belgeler-3",
		State:            "merged",
		IsHEAD:           true,
		UncommittedCount: 4,
	}
	cmdDirty := BuildSuggestedCommand(headDirtyBranch, "main")
	if !strings.Contains(cmdDirty, "⚠️ DİKKAT") || !strings.Contains(cmdDirty, "git switch main && git branch -D belgeler-3") {
		t.Fatalf("expected warning and git switch main in suggested cmd for dirty HEAD branch, got: %q", cmdDirty)
	}

	headCleanBranch := model.BranchArc{
		Name:             "belgeler-3",
		State:            "merged",
		IsHEAD:           true,
		UncommittedCount: 0,
	}
	cmdClean := BuildSuggestedCommand(headCleanBranch, "main")
	if strings.Contains(cmdClean, "⚠️ DİKKAT") || !strings.HasPrefix(cmdClean, "git switch main && git branch -D belgeler-3") {
		t.Fatalf("expected git switch main without warning for clean HEAD branch, got: %q", cmdClean)
	}

	notHeadBranch := model.BranchArc{
		Name:   "belgeler-2",
		State:  "merged",
		IsHEAD: false,
	}
	cmdNotHead := BuildSuggestedCommand(notHeadBranch, "main")
	if strings.Contains(cmdNotHead, "git switch") || !strings.HasPrefix(cmdNotHead, "git branch -D belgeler-2") {
		t.Fatalf("expected direct git branch -D for non-HEAD branch, got: %q", cmdNotHead)
	}
}
