package gitengine

import (
	"context"
	"strings"
	"testing"
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
