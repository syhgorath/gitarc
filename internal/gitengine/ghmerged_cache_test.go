package gitengine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func headCalls(s *ghStub, branch string) int {
	n := 0
	for _, c := range s.calls() {
		if strings.Contains(c, "--state merged") && strings.Contains(c, "--head "+branch) {
			n++
		}
	}
	return n
}

// Dal başına `gh pr list --head` ~1 sn'lik bir ağ çağrısıdır; her anlık görüntüde her dal için tekrarlanınca
// büyük bir depoda anlık görüntü 13 sn'ye çıkıyordu. Sonuç önbelleğe alınır.
func TestGhHasMergedPRIsCached(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	ctx := context.Background()

	stub.set("pr_list_merged_head", `[{"number":5}]`, 0)
	for i := 0; i < 3; i++ {
		if !ghHasMergedPR(ctx, dir, "feat/yes") {
			t.Fatal("merged PR must be reported")
		}
	}
	if got := headCalls(stub, "feat/yes"); got != 1 {
		t.Fatalf("positive answers must be cached for good: %d gh calls", got)
	}

	stub.set("pr_list_merged_head", "", 0) // stub varsayılanı: []
	if err := osRemove(stub.dir + "/pr_list_merged_head.out"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if ghHasMergedPR(ctx, dir, "feat/no") {
			t.Fatal("no merged PR expected")
		}
	}
	if got := headCalls(stub, "feat/no"); got != 1 {
		t.Fatalf("negative answers are cached for a while: %d gh calls", got)
	}

	// Mutasyondan sonra olumsuz yanıt yeniden sorulur (PR az önce birleşmiş olabilir); olumlu kalır.
	InvalidateGHCache(dir)
	ghHasMergedPR(ctx, dir, "feat/no")
	ghHasMergedPR(ctx, dir, "feat/yes")
	if got := headCalls(stub, "feat/no"); got != 2 {
		t.Fatalf("negative cache must be dropped on invalidate, calls=%d", got)
	}
	if got := headCalls(stub, "feat/yes"); got != 1 {
		t.Fatalf("positive cache must survive invalidate, calls=%d", got)
	}
}

func TestGhHasMergedPRDoesNotCacheFailures(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("pr_list_merged_head", "", 1) // gh hata veriyor
	for i := 0; i < 2; i++ {
		if ghHasMergedPR(context.Background(), dir, "feat/err") {
			t.Fatal("an error must not claim a merged PR")
		}
	}
	if got := headCalls(stub, "feat/err"); got != 2 {
		t.Fatalf("failed lookups must be retried, not cached: %d calls", got)
	}
}

// Sonucu değiştiremeyecek dallar için gh hiç çağrılmamalı: uzak dal PR'sizse ve ana dalın gerisinde değilse
// (aheadMain == 0) zaten "birleşmiş kalıntı" sayılır.
func TestSnapshotDoesNotAskGHPerBranchWhenNotNeeded(t *testing.T) {
	stub := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	for _, n := range []string{"feat/r1", "feat/r2", "feat/r3", "feat/r4"} {
		run("branch", n) // ana dalın ucunda: ahead == 0
		run("push", "-u", "origin", n)
		run("branch", "-D", n) // yalnızca uzak kopya
	}
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Snapshot(); err != nil {
		t.Fatal(err)
	}
	for _, c := range stub.calls() {
		if strings.Contains(c, "--state merged") && strings.Contains(c, "--head") {
			t.Errorf("unexpected per-branch gh lookup: %s", c)
		}
	}
}

// Prometheus regresyonu: yüzlerce uzak dalı olan depoda ilk anlık görüntü her dal için sıralı bir `gh` sorgusu
// çalıştırıyor ve 20 sn'lik üst sınıra dayanıyordu. Ön plan yolu artık ağa çıkmaz; sorgu arka plana bırakılır.
func TestMergedPRLookupNeverBlocksTheForeground(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.slow("pr_list_merged_head", "1.0")
	stub.set("pr_list_merged_head", `[{"number":9}]`, 0)

	start := time.Now()
	for i := 0; i < 20; i++ {
		if ghHasMergedPRNoWait(dir, "feat/slow") {
			t.Fatal("unknown must be reported as not-merged until the background lookup finishes")
		}
	}
	if d := time.Since(start); d > 300*time.Millisecond {
		t.Fatalf("the foreground path must not wait for gh, took %v", d)
	}
	v0 := bgVersion(dir)
	waitFor(t, 8*time.Second, "background lookup to find the merged PR", func() bool { return ghHasMergedPRNoWait(dir, "feat/slow") })
	if got := headCalls(stub, "feat/slow"); got != 1 {
		t.Fatalf("20 rapid calls must trigger exactly ONE background query, got %d", got)
	}
	if bgVersion(dir) <= v0 {
		t.Fatal("finding a merged PR in the background must bump the background version so the UI refreshes")
	}
}

func TestBackgroundLookupFailureIsNotCachedAsNotMerged(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("pr_list_merged_head", "", 1) // gh hata veriyor
	ghHasMergedPRNoWait(dir, "feat/err")
	waitFor(t, 5*time.Second, "the failed lookup to be recorded", func() bool {
		ghMergedMu.Lock()
		defer ghMergedMu.Unlock()
		e, ok := ghMergedCache[filepath.Clean(dir)+"|feat/err"]
		return ok && e.failed
	})
	// Başarısız sorgu kısa süre yeniden denenmez ama "birleşmemiş" diye de saklanmaz.
	calls := headCalls(stub, "feat/err")
	ghHasMergedPRNoWait(dir, "feat/err")
	time.Sleep(200 * time.Millisecond)
	if got := headCalls(stub, "feat/err"); got != calls {
		t.Fatalf("a failed lookup must not be retried immediately (%d → %d)", calls, got)
	}
	if ghHasMergedPRNoWait(dir, "feat/err") {
		t.Fatal("a failed lookup must not claim a merged PR")
	}
}

func TestSnapshotWithManyRemoteBranchesStaysFastDespiteSlowGH(t *testing.T) {
	stub := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("feat/many-%d", i)
		addFeatureBranch(t, dir, run, name) // main'in ilerisinde commit'i var: eskiden her biri için gh sorgusu çalışırdı
		run("push", "-u", "origin", name)
		run("branch", "-D", name)
	}
	stub.slow("pr_list_merged_head", "2")
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := mgr.Snapshot(); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("12 remote branches with a slow gh must not make the snapshot slow, took %v", d)
	}
}
