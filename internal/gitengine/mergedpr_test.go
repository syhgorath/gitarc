package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

func TestEnrichMergedArchesFillsRealPRDetails(t *testing.T) {
	arches := []model.BranchArc{
		{State: "merged", Name: "fix/a", PR: &model.PRStatus{Number: 83, Title: "Merge pull request #83 from u/fix/a", ReviewState: "MERGED"}},
		{State: "merged", Name: "fix/b", PR: &model.PRStatus{Number: 99, Title: "Merge pull request #99 from u/fix/b", ReviewState: "MERGED"}},
		{State: "open", Name: "feat/open", PR: &model.PRStatus{Number: 83, Title: "keep"}},
		{State: "merged", Name: "no-pr"},
	}
	gh := map[string]model.PRStatus{
		"fix/a": {Number: 83, Title: "fix(ui): gerçek başlık", Body: "## Ne değişti\n- şey", Author: "ziya", URL: "https://github.com/a/b/pull/83"},
	}
	enrichMergedArches(arches, gh)

	a := arches[0].PR
	if a.Title != "fix(ui): gerçek başlık" || !strings.Contains(a.Body, "Ne değişti") || a.Author != "ziya" || a.URL == "" || a.ReviewState != "MERGED" {
		t.Fatalf("PR #83 must carry the real GitHub details: %+v", a)
	}
	if arches[1].PR.Title != "Merge pull request #99 from u/fix/b" || arches[1].PR.Body != "" {
		t.Fatalf("a PR GitHub did not return stays untouched: %+v", arches[1].PR)
	}
	if arches[2].PR.Title != "keep" {
		t.Fatal("open branches are never enriched from the merged list")
	}
	enrichMergedArches(arches, nil) // boş veri: çökmemeli
}

func TestSuggestedCommandNeverTellsToDeleteWhatIsNotThere(t *testing.T) {
	gone := model.BranchArc{Name: "fix/x", State: "merged", OriginGone: true}
	if got := BuildSuggestedCommand(gone, "main"); got != "" {
		t.Fatalf("no local and no remote copy: nothing to clean, got %q", got)
	}
	withLocal := model.BranchArc{Name: "fix/x", State: "merged", OriginGone: true, HasLocal: true}
	if got := BuildSuggestedCommand(withLocal, "main"); got != "git branch -D fix/x && git fetch --prune origin" {
		t.Fatalf("local leftover: %q", got)
	}
	remoteOnly := model.BranchArc{Name: "fix/x", State: "merged", HasRemote: true}
	if got := BuildSuggestedCommand(remoteOnly, "main"); !strings.Contains(got, "git push origin --delete fix/x") {
		t.Fatalf("remote leftover: %q", got)
	}
}

func TestMergeDiffShowsWhatTheMergeBrought(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/m")
	run("merge", "--no-ff", "feat/m", "-m", "Merge pull request #4 from acme/feat/m")
	run("rev-parse", "HEAD")
	sha, err := runGitOut(dir, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	sha = strings.TrimSpace(sha)

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	res, err := mgr.GetDiff(sha, "merge")
	if err != nil {
		t.Fatalf("merge diff: %v", err)
	}
	if len(res.Files) != 1 || !strings.Contains(res.Files[0].Path, "feat_m.txt") {
		t.Fatalf("expected exactly the branch's file, got %+v", res.Files)
	}
	if res.Files[0].Additions != 1 || res.TotalAdds != 1 {
		t.Fatalf("additions must be counted: %+v", res)
	}
	if !strings.Contains(res.Title, "Merge pull request #4") {
		t.Fatalf("title should be the merge subject, got %q", res.Title)
	}

	// Tek ebeveynli (squash) commit de çalışır.
	run("checkout", "-b", "feat/sq")
	if err := os.WriteFile(filepath.Join(dir, "sq.txt"), []byte("s\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: squash me")
	run("checkout", "main")
	run("merge", "--squash", "feat/sq")
	run("commit", "-m", "feat: squashed (#5)")
	sq, _ := runGitOut(dir, "rev-parse", "HEAD")
	resSq, err := mgr.GetDiff(strings.TrimSpace(sq), "merge")
	if err != nil || len(resSq.Files) != 1 || resSq.Files[0].Path != "sq.txt" {
		t.Fatalf("squash commit diff: %+v %v", resSq, err)
	}

	for _, bad := range []string{"--output=/tmp/x", "main", "zzzzzzz", "abc"} {
		if _, err := mgr.GetDiff(bad, "merge"); err == nil {
			t.Errorf("GetDiff(%q, merge) must be rejected", bad)
		}
	}
}
