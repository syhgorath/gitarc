package gitengine

import (
	"context"
	"strings"
	"testing"

	"gitarc/internal/model"
)

const openPRsJSON = `[
 {"number":7,"headRefName":"feat/green","title":"Green PR","body":"b","url":"https://github.com/acme/demo/pull/7",
  "author":{"login":"ziya"},"reviewDecision":"APPROVED",
  "statusCheckRollup":[{"name":"build","status":"COMPLETED","conclusion":"SUCCESS","workflowName":"CI"}]},
 {"number":8,"headRefName":"feat/red","title":"Red PR","body":"","url":"u8","author":{"login":"bob"},"reviewDecision":"",
  "statusCheckRollup":[
    {"name":"lint","status":"COMPLETED","conclusion":"SUCCESS"},
    {"name":"test","status":"COMPLETED","conclusion":"FAILURE","workflowName":"CI","detailsUrl":"https://github.com/acme/demo/actions/runs/555/job/1"}]},
 {"number":9,"headRefName":"feat/running","title":"Running PR","body":"","url":"u9","author":{"login":"eve"},"reviewDecision":"",
  "statusCheckRollup":[{"name":"build","status":"IN_PROGRESS","conclusion":""}]},
 {"number":10,"headRefName":"feat/nochecks","title":"No checks","body":"","url":"u10","author":{"login":"eve"},"reviewDecision":"","statusCheckRollup":[]},
 {"number":11,"headRefName":"feat/legacy-status","title":"Legacy","body":"","url":"u11","author":{"login":"eve"},"reviewDecision":"",
  "statusCheckRollup":[{"context":"ci/circle","state":"ERROR","targetUrl":"https://github.com/acme/demo/actions/runs/777"}]}
]`

const failedRunLog = `test	Run tests	2026-11-16T07:12:05.110Z === RUN   TestShellGoldCompare
test	Run tests	2026-11-16T07:12:06.520Z --- FAIL: TestShellGoldCompare (0.42s)
test	Run tests	2026-11-16T07:12:06.532Z FAIL	keylab/internal/shellgold	0.614s`

func TestFetchGHPRsParsesCIStatesAndFailureLog(t *testing.T) {
	gh := newGHStub(t)
	gh.set("pr_list_open", openPRsJSON, 0)
	gh.set("run_view", failedRunLog, 0)

	prs, err := fetchGHPRs(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("fetchGHPRs: %v", err)
	}
	if len(prs) != 5 {
		t.Fatalf("expected 5 PRs, got %d", len(prs))
	}

	green := prs["feat/green"]
	if green.Number != 7 || green.Author != "ziya" || green.ReviewState != "APPROVED" || green.CI.State != "SUCCESS" || green.CI.Workflow != "CI" {
		t.Fatalf("green PR parsed wrong: %+v / %+v", green, green.CI)
	}

	red := prs["feat/red"]
	if red.CI.State != "FAILURE" || red.CI.FailedStep != "test" || red.CI.RunID != "555" {
		t.Fatalf("red PR CI wrong: %+v", red.CI)
	}
	if red.ReviewState != "" {
		t.Fatalf("empty reviewDecision must stay empty (not invented as REVIEW_REQUIRED), got %q", red.ReviewState)
	}
	if !strings.Contains(red.CI.RawLog, "TestShellGoldCompare") || len(red.CI.Extracts) == 0 {
		t.Fatalf("failed run log must be fetched and parsed: %+v", red.CI)
	}
	if !gh.called("run view 555 --log-failed") {
		t.Fatalf("expected gh run view 555 --log-failed, calls=%v", gh.calls())
	}

	if prs["feat/running"].CI.State != "RUNNING" {
		t.Fatalf("in-progress check must be RUNNING, got %s", prs["feat/running"].CI.State)
	}
	if prs["feat/nochecks"].CI != nil {
		t.Fatalf("PR with no checks must have UNKNOWN CI (nil), not an invented state: %+v", prs["feat/nochecks"].CI)
	}
	legacy := prs["feat/legacy-status"]
	if legacy.CI.State != "FAILURE" || legacy.CI.FailedStep != "ci/circle" || legacy.CI.RunID != "777" {
		t.Fatalf("legacy commit-status failure not handled: %+v", legacy.CI)
	}
}

func TestFetchGHPRsFailsSoft(t *testing.T) {
	gh := newGHStub(t)

	gh.set("pr_list_open", "", 1)
	if got, err := fetchGHPRs(context.Background(), t.TempDir()); len(got) != 0 || err == nil {
		t.Fatalf("gh error must yield an empty map AND an error (never a silent empty result), got %v %v", got, err)
	}
	gh.set("pr_list_open", "{not json", 0)
	_ = removeStubKey(gh, "pr_list_open.exit")
	if got, err := fetchGHPRs(context.Background(), t.TempDir()); len(got) != 0 || err == nil {
		t.Fatalf("malformed JSON must yield an empty map and an error, got %v %v", got, err)
	}
}

func removeStubKey(s *ghStub, file string) error {
	return osRemove(s.dir + "/" + file)
}

func TestFetchGHMergedPRsBuildsArches(t *testing.T) {
	gh := newGHStub(t)
	nodes := []model.CommitNode{{ShortSHA: "aaa"}, {ShortSHA: "bbb"}, {ShortSHA: "ccc"}}
	shaToIdx := map[string]int{"sha-a": 0, "sha-b": 1, "sha-c": 2}
	gh.set("pr_list_merged", `[
	  {"number":1,"headRefName":"feat/one","title":"One","body":"","url":"u1","author":{"login":"a"},"mergeCommit":{"oid":"sha-b"}},
	  {"number":2,"headRefName":"feat/two","title":"Two","body":"","url":"u2","author":{"login":"b"},"mergeCommit":{"oid":"sha-c"}},
	  {"number":3,"headRefName":"feat/dup","title":"Dup","body":"","url":"u3","author":{"login":"b"},"mergeCommit":{"oid":"sha-c"}},
	  {"number":4,"headRefName":"feat/root","title":"Root","body":"","url":"u4","author":{"login":"b"},"mergeCommit":{"oid":"sha-a"}},
	  {"number":5,"headRefName":"feat/unknown","title":"U","body":"","url":"u5","author":{"login":"b"},"mergeCommit":{"oid":"nope"}},
	  {"number":6,"headRefName":"feat/nomerge","title":"N","body":"","url":"u6","author":{"login":"b"},"mergeCommit":null}
	]`, 0)

	merged, arches, err := fetchGHMergedPRs(context.Background(), t.TempDir(), nodes, shaToIdx, nil)
	if err != nil {
		t.Fatalf("fetchGHMergedPRs: %v", err)
	}
	if len(merged) != 6 {
		t.Fatalf("all heads must be in the merged map, got %d", len(merged))
	}
	if merged["feat/one"].ReviewState != "MERGED" {
		t.Fatalf("merged PR must carry MERGED state: %+v", merged["feat/one"])
	}
	// Only sha-b and sha-c yield arches: dup shares an index, root is index 0, unknown/nil skipped.
	if len(arches) != 2 {
		t.Fatalf("expected 2 arches, got %d: %+v", len(arches), arches)
	}
	for _, a := range arches {
		if a.State != "merged" || a.Hemisphere != "upper" || a.EndSpineIndex != a.StartSpineIndex+1 {
			t.Fatalf("bad arch %+v", a)
		}
	}
	if arches[0].ID != "gh-merged-1" || arches[0].BaseSHA != "aaa" || arches[0].MergeSHA != "bbb" {
		t.Fatalf("arch 1 wrong: %+v", arches[0])
	}

	// An already-drawn merge index is not duplicated.
	_, arches, _ = fetchGHMergedPRs(context.Background(), t.TempDir(), nodes, shaToIdx, []model.BranchArc{{EndSpineIndex: 1}})
	if len(arches) != 1 || arches[0].ID != "gh-merged-2" {
		t.Fatalf("existing arch index must be skipped, got %+v", arches)
	}
}

func TestFetchGHBranchProtectionVariants(t *testing.T) {
	gh := newGHStub(t)
	ctx := context.Background()

	gh.set("api_protection_get", "true", 0)
	if st, sum := fetchGHBranchProtection(ctx, t.TempDir(), "main"); st != "active" || !strings.Contains(sum, "strict checks: true") {
		t.Fatalf("classic protection: %q %q", st, sum)
	}

	gh.set("api_protection_get", `{"message":"Branch not protected"}`, 1)
	gh.set("api_rulesets", `[{"id":1,"name":"main-guard","target":"branch","enforcement":"active"}]`, 0)
	if st, sum := fetchGHBranchProtection(ctx, t.TempDir(), "main"); st != "active" || !strings.Contains(sum, "main-guard") {
		t.Fatalf("ruleset protection: %q %q", st, sum)
	}

	gh.set("api_rulesets", `[{"id":1,"name":"off","target":"branch","enforcement":"disabled"}]`, 0)
	if st, sum := fetchGHBranchProtection(ctx, t.TempDir(), "main"); st != "none" || !strings.Contains(sum, "Korumasız") {
		t.Fatalf("explicit 'Branch not protected' must be none: %q %q", st, sum)
	}

	gh.set("api_protection_get", "Upgrade to GitHub Pro or make this repository public", 1)
	gh.set("api_rulesets", "[]", 0)
	if st, sum := fetchGHBranchProtection(ctx, t.TempDir(), "main"); st != "none" || !strings.Contains(sum, "GitHub Free") {
		t.Fatalf("free-plan message expected: %q %q", st, sum)
	}
}

// Prometheus regression: a non-admin gets 404 "Not Found" for branch protection. That is NOT "unprotected".
func TestFetchGHBranchProtectionNotFoundIsUnknownNotUnprotected(t *testing.T) {
	gh := newGHStub(t)
	gh.set("api_protection_get", `{"message":"Not Found","status":"404"}`, 1)
	gh.set("api_rulesets", "[]", 0)
	st, sum := fetchGHBranchProtection(context.Background(), t.TempDir(), "main")
	if st != "unknown" {
		t.Fatalf("404 Not Found (no admin rights) must be 'unknown', got %q (%s)", st, sum)
	}
	if strings.Contains(sum, "Korumasız") || !strings.Contains(sum, "yetki") {
		t.Fatalf("summary must explain missing permission, got %q", sum)
	}
}

func TestLiveSnapshotAttachesGHPullRequests(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green")
	run("push", "-u", "origin", "feat/green")
	gh.set("pr_list_open", openPRsJSON, 0)
	gh.set("api_protection_get", "true", 0)

	snap, err := CollectLiveRepo("live-gh", "gh-repo", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Repo.BranchProtected {
		t.Fatalf("protection from gh not reflected: %+v", snap.Repo)
	}
	var found *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].Name == "feat/green" {
			found = &snap.Branches[i]
		}
	}
	if found == nil || found.PR == nil || found.PR.Number != 7 || found.PR.CI.State != "SUCCESS" {
		t.Fatalf("PR #7 must attach to feat/green: %+v", found)
	}
	if found.Hemisphere != "upper" {
		t.Fatalf("pushed branch with PR belongs to upper hemisphere, got %q", found.Hemisphere)
	}
}

func TestPromoteBranchLiveCreatesPRViaGH(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/promote-me")
	mgr := switchLive(t, dir)
	InvalidateGHCache(dir)

	if _, err := mgr.PromoteBranchWithMeta("feat/promote-me", "feat: özel başlık", "özel gövde"); err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !gh.called("pr create", "--head feat/promote-me", "--title feat: özel başlık", "--body özel gövde") {
		t.Fatalf("gh pr create not invoked with custom meta: %v", gh.calls())
	}
	// The branch really reached the (local) remote.
	if out, err := runGitOut(dir, "ls-remote", "--heads", "origin", "feat/promote-me"); err != nil || !strings.Contains(out, "feat/promote-me") {
		t.Fatalf("branch must be pushed: %q %v", out, err)
	}
}

func TestPromoteBranchLiveDefaultsBodyAndToleratesExistingPR(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/already")
	mgr := switchLive(t, dir)
	gh.set("pr_create", "a pull request for branch feat/already into main already exists", 1)
	InvalidateGHCache(dir)

	if _, err := mgr.PromoteBranchWithMeta("feat/already", "", ""); err != nil {
		t.Fatalf("'already exists' must be tolerated: %v", err)
	}
	if !gh.called("pr create", "GitArc canlı kokpitinden açıldı.") {
		t.Fatalf("default body expected: %v", gh.calls())
	}
}

func TestPromoteBranchLiveSurfacesPRCreateError(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/boom")
	mgr := switchLive(t, dir)
	gh.set("pr_create", "GraphQL: something exploded", 1)
	InvalidateGHCache(dir)

	_, err := mgr.PromoteBranchWithMeta("feat/boom", "t", "b")
	if err == nil || !strings.Contains(err.Error(), "PR açılamadı") {
		t.Fatalf("expected PR açılamadı error, got %v", err)
	}
}

func TestPromoteBranchLiveEmptyBranchNeverCallsGH(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	run("branch", "feat/empty")
	mgr := switchLive(t, dir)
	InvalidateGHCache(dir)

	_, err := mgr.PromoteBranchWithMeta("feat/empty", "t", "b")
	if err == nil || !strings.Contains(err.Error(), "yeni bir commit") {
		t.Fatalf("empty branch must be refused with a clear message, got %v", err)
	}
	if gh.called("pr create") {
		t.Fatal("gh pr create must not run for an empty branch")
	}
}

func prSetup(t *testing.T, gh *ghStub, branch string) (*Manager, string) {
	t.Helper()
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, branch)
	run("push", "-u", "origin", branch)
	gh.set("pr_list_open", `[{"number":7,"headRefName":"`+branch+`","title":"T","body":"B","url":"u","author":{"login":"z"},"reviewDecision":"",
	  "statusCheckRollup":[{"name":"build","status":"COMPLETED","conclusion":"SUCCESS"}]}]`, 0)
	InvalidateGHCache(dir)
	return switchLive(t, dir), dir
}

func TestMergeBranchLiveUsesGHStrategies(t *testing.T) {
	cases := []struct {
		strategy string
		wantArgs []string
	}{
		{"squash", []string{"pr merge 7", "--squash", "--delete-branch"}},
		{"rebase", []string{"pr merge 7", "--rebase", "--delete-branch"}},
		{"merge", []string{"pr merge 7", "--merge", "--delete-branch"}},
		{"auto", []string{"pr merge 7", "--merge --auto", "--delete-branch"}},
	}
	for _, c := range cases {
		t.Run(c.strategy, func(t *testing.T) {
			gh := newGHStub(t)
			mgr, dir := prSetup(t, gh, "feat/ship")
			InvalidateGHCache(dir)
			if _, err := mgr.MergeBranch("feat/ship", c.strategy); err != nil {
				t.Fatalf("merge: %v", err)
			}
			if !gh.called(c.wantArgs...) {
				t.Fatalf("expected gh %v, calls=%v", c.wantArgs, gh.calls())
			}
		})
	}
}

func TestMergeBranchLiveBlocksOnFailingCIAndSurfacesGHError(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/red")
	run("push", "-u", "origin", "feat/red")
	gh.set("pr_list_open", `[{"number":8,"headRefName":"feat/red","title":"R","body":"","url":"u","author":{"login":"z"},"reviewDecision":"",
	  "statusCheckRollup":[{"name":"test","status":"COMPLETED","conclusion":"FAILURE"}]}]`, 0)
	InvalidateGHCache(dir)
	mgr := switchLive(t, dir)

	if _, err := mgr.MergeBranch("feat/red", "squash"); err == nil || !strings.Contains(err.Error(), "CI başarısız") {
		t.Fatalf("failing CI must block merge, got %v", err)
	}
	if gh.called("pr merge") {
		t.Fatal("gh pr merge must not run when CI fails")
	}

	// gh failure is surfaced with PR number and gh output.
	gh2 := newGHStub(t)
	mgr2, _ := prSetup(t, gh2, "feat/ghfail")
	gh2.set("pr_merge", "Pull request is not mergeable", 1)
	_, err := mgr2.MergeBranch("feat/ghfail", "merge")
	if err == nil || !strings.Contains(err.Error(), "PR #7 merge edilemedi") || !strings.Contains(err.Error(), "not mergeable") {
		t.Fatalf("gh merge error not surfaced: %v", err)
	}
}

// Korumalı dalda zorunlu kontrol bitmeden merge: ham `--auto/--admin` metni sızmaz, "CI Bitince Merge Et"e yönlendirilir.
func TestMergeBlockedByPolicyGivesActionableTurkishError(t *testing.T) {
	gh := newGHStub(t)
	mgr, _ := prSetup(t, gh, "feat/policy")
	gh.set("pr_merge", "X Pull request o/r#7 is not mergeable: the base branch policy prohibits the merge. To have the pull request merged after all the requirements have been met, add the `--auto` flag. To use administrator privileges to immediately merge the pull request, add the `--admin` flag.", 1)
	_, err := mgr.MergeBranch("feat/policy", "merge")
	if err == nil {
		t.Fatal("merge must fail")
	}
	msg := err.Error()
	if !strings.Contains(msg, "CI Bitince Merge Et") || strings.Contains(msg, "--admin") || strings.Contains(msg, "--auto") {
		t.Fatalf("error must be actionable Turkish without raw gh flags: %s", msg)
	}
}

func TestUpdatePRMetaLiveCallsGHEdit(t *testing.T) {
	gh := newGHStub(t)
	mgr, _ := prSetup(t, gh, "feat/edit")
	if _, err := mgr.UpdatePRMeta("feat/edit", "yeni başlık", "yeni gövde"); err != nil {
		t.Fatalf("UpdatePRMeta: %v", err)
	}
	if !gh.called("pr edit 7", "--title yeni başlık", "--body yeni gövde") {
		t.Fatalf("gh pr edit not invoked: %v", gh.calls())
	}

	gh.set("pr_edit", "HTTP 403", 1)
	if _, err := mgr.UpdatePRMeta("feat/edit", "x", "y"); err == nil || !strings.Contains(err.Error(), "güncellenemedi") {
		t.Fatalf("gh edit failure must surface, got %v", err)
	}
}

func TestDeleteBranchLiveClosesOpenPR(t *testing.T) {
	gh := newGHStub(t)
	mgr, dir := prSetup(t, gh, "feat/close-me")
	if _, err := mgr.DeleteBranch("feat/close-me"); err != nil {
		t.Fatalf("DeleteBranch: %v", err)
	}
	if !gh.called("pr close 7", "--delete-branch") {
		t.Fatalf("open PR must be closed via gh: %v", gh.calls())
	}
	if out, _ := runGitOut(dir, "branch", "--list", "feat/close-me"); strings.TrimSpace(out) != "" {
		t.Fatalf("local branch must be gone, got %q", out)
	}

	// gh failure aborts before any local deletion.
	gh2 := newGHStub(t)
	mgr2, dir2 := prSetup(t, gh2, "feat/keep-me")
	gh2.set("pr_close", "cannot close", 1)
	if _, err := mgr2.DeleteBranch("feat/keep-me"); err == nil || !strings.Contains(err.Error(), "kapatılamadı") {
		t.Fatalf("expected kapatılamadı error, got %v", err)
	}
	if out, _ := runGitOut(dir2, "branch", "--list", "feat/keep-me"); strings.TrimSpace(out) == "" {
		t.Fatal("branch must survive a failed PR close")
	}
}

func TestNonGitHubRemoteNeverCallsGH(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newTempRepo(t)
	bare := t.TempDir()
	if out, err := runGitOut(bare, "init", "--bare", "-b", "main", bare); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", bare)
	run("push", "-u", "origin", "main")

	snap, err := CollectLiveRepo("live-plain", "plain", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(snap.Repo.ProtectionSummary, "Saf Git") {
		t.Fatalf("expected pure-git summary, got %q", snap.Repo.ProtectionSummary)
	}
	if gh.called("pr list") || gh.called("api") {
		t.Fatalf("non-GitHub remote must not query gh: %v", gh.calls())
	}
}

// Dürüstlük: GitHub'ın boş bıraktığı değerler uydurulmaz. Kontrolü olmayan PR "CI sürüyor", tanınmayan kayıtlar "CI başarılı"
// olmaz (eskiden öyleydi); inceleme kararı boşsa "inceleme gerekli" yazılmaz.
func TestFetchGHPRsNeverInventsReviewOrCIState(t *testing.T) {
	gh := newGHStub(t)
	gh.set("pr_list_open", `[
 {"number":1,"headRefName":"feat/bos","title":"bos","body":"","url":"u","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]},
 {"number":2,"headRefName":"feat/taninmayan","title":"t","body":"","url":"u","author":{"login":"a"},"reviewDecision":"APPROVED",
  "statusCheckRollup":[{"name":"x"},{"workflowName":"CI"}]},
 {"number":3,"headRefName":"feat/gercek","title":"g","body":"","url":"u","author":{"login":"a"},"reviewDecision":"CHANGES_REQUESTED",
  "statusCheckRollup":[{"name":"build","status":"COMPLETED","conclusion":"SUCCESS"}]}
]`, 0)
	prs, err := fetchGHPRs(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if bos := prs["feat/bos"]; bos.ReviewState != "" || bos.CI != nil {
		t.Fatalf("boş karar ve boş kontrol listesi bilinmiyor kalmalı: review=%q ci=%+v", bos.ReviewState, bos.CI)
	}
	if t2 := prs["feat/taninmayan"]; t2.CI != nil || t2.ReviewState != "APPROVED" {
		t.Fatalf("durumu/sonucu olmayan kontrol kayıtları SUCCESS sayılmamalı (CI nil), gerçek inceleme korunmalı: %+v", t2)
	}
	if g := prs["feat/gercek"]; g.CI == nil || g.CI.State != "SUCCESS" || g.ReviewState != "CHANGES_REQUESTED" {
		t.Fatalf("gerçek veri aynen korunmalı: %+v / %+v", g, g.CI)
	}
}
