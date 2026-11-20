package gitengine

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitarc/internal/model"
)

func runsJSON(items ...string) string { return "[" + strings.Join(items, ",") + "]" }

func run(sha, status, conclusion, wf string) string {
	return fmt.Sprintf(`{"status":%q,"conclusion":%q,"headSha":%q,"workflowName":%q,"url":"https://github.com/acme/demo/actions/runs/%s","displayTitle":"Merge pull request #7","updatedAt":"2026-11-19T10:00:00Z"}`, status, conclusion, sha, wf, wf)
}

// originTip: test deposunun origin/main ucu (fetchMainCI'ın karşılaştırdığı gerçek omurga ucu).
func originTip(t *testing.T, dir string) string {
	t.Helper()
	sha, err := revParse(context.Background(), dir, "origin/main")
	if err != nil {
		t.Fatalf("origin/main ucu okunamadı: %v", err)
	}
	return sha
}

// Merge'ten hemen sonra GitHub yeni commit için koşu kaydını henüz açmamıştır: "en son koşu" bir ÖNCEKİ commit'indir ve YEŞİL.
// Rozet bu eski yeşili yeni merge'ün sonucu diye göstermemeli (kullanıcı "main'de test kaldı mı" sorusunu bu rozetle yanıtlar).
func TestMainCIDoesNotShowStaleGreenForFreshTip(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	ctx := context.Background()
	stub.set("run_list", runsJSON(run("dddddd4444444444444444444444444444444ddd", "completed", "success", "CI")), 0)

	got, err := fetchMainCI(ctx, dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.State != "RUNNING" {
		t.Fatalf("yeni uç için koşu görünmüyorsa yeşil değil RUNNING (başlıyor) olmalı: %+v", got)
	}
	if got.SHA != originTip(t, dir)[:7] {
		t.Fatalf("rozet eski commit'i değil omurganın ucunu göstermeli: %+v", got)
	}

	// Uç eskiyse (iş akışı bu push'ta hiç koşmuyor olabilir) sahte bekleme de yok: sonuç yok.
	old := mainCIPendingWindow
	mainCIPendingWindow = -time.Hour
	defer func() { mainCIPendingWindow = old }()
	if got, _ := fetchMainCI(ctx, dir, "main"); got != nil {
		t.Fatalf("eski uç + koşu kaydı yok: sonuç yok (nil) olmalı, geldi %+v", got)
	}
}

func TestFetchMainCIStates(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	ctx := context.Background()
	head := originTip(t, dir) // koşular omurganın GERÇEK ucuna ait olmalı
	old := "bbbbbbb2222222222222222222222222222222bb"

	cases := []struct {
		name string
		out  string
		want string // "" = nil (sonuç yok)
	}{
		{"all passed", runsJSON(run(head, "completed", "success", "CI"), run(head, "completed", "success", "CodeQL")), "SUCCESS"},
		{"one workflow still running", runsJSON(run(head, "completed", "success", "CI"), run(head, "in_progress", "", "CodeQL")), "RUNNING"},
		{"one workflow failed", runsJSON(run(head, "completed", "failure", "CI"), run(head, "completed", "success", "CodeQL")), "FAILURE"},
		{"failure beats running", runsJSON(run(head, "in_progress", "", "CodeQL"), run(head, "completed", "timed_out", "CI")), "FAILURE"},
		{"an older commit's failure is irrelevant", runsJSON(run(head, "completed", "success", "CI"), run(old, "completed", "failure", "CI")), "SUCCESS"},
		{"only skipped runs: no result, never green", runsJSON(run(head, "completed", "skipped", "CI")), ""},
		{"no runs at all", "[]", ""},
	}
	for _, c := range cases {
		stub.set("run_list", c.out, 0)
		got, err := fetchMainCI(ctx, dir, "main")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if c.want == "" {
			if got != nil {
				t.Errorf("%s: want nil, got %+v", c.name, got)
			}
			continue
		}
		if got == nil || got.State != c.want {
			t.Errorf("%s: want %s, got %+v", c.name, c.want, got)
		}
		if got != nil && (got.SHA != head[:7] || got.Branch != "main" || !strings.HasPrefix(got.URL, "https://")) {
			t.Errorf("%s: metadata wrong: %+v", c.name, got)
		}
	}

	stub.set("run_list", "", 1) // gh hata veriyor
	if _, err := fetchMainCI(ctx, dir, "main"); err == nil {
		t.Error("a gh error must surface as an error, not as an empty/green result")
	}
}

func TestSnapshotShowsBrokenMainAndRadarWarning(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	head := originTip(t, dir)
	stub.set("run_list", runsJSON(run(head, "completed", "failure", "CI")), 0)

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	var snap model.CockpitSnapshot
	for i := 0; i < 20; i++ { // arka plan yenilemesi tamamlanana kadar kısa bekle
		var err error
		if snap, err = mgr.Snapshot(); err != nil {
			t.Fatal(err)
		}
		if snap.Repo.MainCI != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if snap.Repo.MainCI == nil || snap.Repo.MainCI.State != "FAILURE" {
		t.Fatalf("snapshot must carry the failing main CI, got %+v", snap.Repo.MainCI)
	}
	found := false
	for _, d := range snap.Diagnostics {
		if d.Kind == "main_ci_failure" {
			found = true
			if d.Severity != "critical" || !strings.HasPrefix(d.URL, "https://") {
				t.Errorf("main CI diagnostic must be critical with a link: %+v", d)
			}
		}
	}
	if !found {
		t.Fatal("a failing main CI must show up in the radar")
	}
}

// Eskiden birleşmiş her PR sabit "SUCCESS" CI taşıyordu: main kırıkken bile yeşil görünüyordu.
func TestMergedPRsDoNotInventAGreenCI(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("pr_list_merged", `[{"number":9,"headRefName":"feat/done","title":"feat: done","body":"","author":{"login":"t"},"mergeCommit":{"oid":"abc"},"url":"https://github.com/acme/demo/pull/9"}]`, 0)
	merged, _, err := fetchGHMergedPRs(context.Background(), dir, nil, map[string]int{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	pr, ok := merged["feat/done"]
	if !ok {
		t.Fatal("merged PR missing")
	}
	if pr.CI != nil {
		t.Fatalf("a merged PR must not claim a CI result it never read: %+v", pr.CI)
	}
}

func TestSimulationMergeStartsMainTests(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.MainCI == nil || snap.Repo.MainCI.State != "SUCCESS" {
		t.Fatalf("demo starts with a green main: %+v", snap.Repo.MainCI)
	}
	target := ""
	for _, b := range snap.Branches {
		if b.State == "open" && b.PR != nil && (b.Conflict == nil || !b.Conflict.HasConflict) && (b.PR.CI == nil || b.PR.CI.State != "FAILURE") {
			target = b.ID
			break
		}
	}
	if target == "" {
		t.Skip("no mergeable demo branch")
	}
	snap, err = mgr.MergeBranch(target, "")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if snap.Repo.MainCI == nil || snap.Repo.MainCI.State != "RUNNING" {
		t.Fatalf("right after a merge main's tests are running: %+v", snap.Repo.MainCI)
	}
	// Zamana bağlı: eski bir RUNNING SUCCESS olur.
	meta := model.RepoMeta{DefaultBranch: "main", MainCI: &model.MainCIStatus{State: "RUNNING", UpdatedAt: time.Now().Add(-time.Minute).Format(time.RFC3339)}}
	if got := simMainCI(meta, nil); got.State != "SUCCESS" {
		t.Fatalf("a stale simulated run completes: %+v", got)
	}
	if meta.MainCI.State != "RUNNING" {
		t.Fatal("simMainCI must not mutate the stored state")
	}
}

// Merge commit'inden türetilen birleşmiş dallar (gh olmadan) da sahte CI taşımamalı.
func TestMergeCommitDerivedBranchesHaveNoInventedCI(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/derived")
	run("push", "-u", "origin", "feat/derived")
	run("merge", "--no-ff", "feat/derived", "-m", "Merge pull request #5 from acme/feat/derived")
	run("push", "origin", "main")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, b := range snap.Branches {
		if b.State == "merged" && b.PR != nil {
			seen++
			if b.PR.CI != nil {
				t.Errorf("%s: merged PR must not carry an invented CI: %+v", b.Name, b.PR.CI)
			}
		}
	}
	if seen == 0 {
		t.Fatal("expected a merged branch derived from the merge commit")
	}
}
