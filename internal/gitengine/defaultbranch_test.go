package gitengine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectDefaultBranchPrefersRemoteHEAD(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	ctx := context.Background()

	// origin/HEAD yok (yerelde `git remote set-head` çalıştırılmadı): sezgi main'i bulur.
	_, _ = runGitOut(dir, "remote", "set-head", "origin", "-d")
	if got := detectDefaultBranch(ctx, dir); got != "main" {
		t.Fatalf("no origin/HEAD: want main, got %q", got)
	}

	// git-flow: uzak varsayılan dal develop, yerelde main de var → develop seçilmeli.
	run("checkout", "-b", "develop")
	run("push", "-u", "origin", "develop")
	run("remote", "set-head", "origin", "develop")
	run("checkout", "main")
	if got := detectDefaultBranch(ctx, dir); got != "develop" {
		t.Fatalf("git-flow: want develop (origin/HEAD), got %q", got)
	}

	// origin/HEAD yerelde bulunmayan bir dalı gösteriyorsa güvenilmez: sezgiye dön.
	run("branch", "-D", "develop")
	if got := detectDefaultBranch(ctx, dir); got != "main" {
		t.Fatalf("origin/HEAD points to a branch that is not local: want main, got %q", got)
	}
}

func TestDetectDefaultBranchMasterRepo(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	run("branch", "-m", "main", "master")
	run("push", "-u", "origin", "master")
	run("remote", "set-head", "origin", "master")
	if got := detectDefaultBranch(context.Background(), dir); got != "master" {
		t.Fatalf("want master, got %q", got)
	}
}

// Varsayılan dal develop iken dal diff'i develop'a göre alınmalı; eskiden sabit origin/main kullanılıyordu ve
// main'de olmayan develop commit'leri dalın değişikliğiymiş gibi görünüyordu.
func TestBranchDiffUsesTheRealDefaultBranch(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-m", "chore: "+name)
	}
	run("checkout", "-b", "develop")
	write("only-on-develop.txt", "d\n")
	run("push", "-u", "origin", "develop")
	run("remote", "set-head", "origin", "develop")
	// Uzağın (GitHub'ın) varsayılanı da develop olsun: GitArc artık uzağın gerçek HEAD'ini (ls-remote) yerel kayda tercih eder.
	bare, _ := runGitOut(dir, "remote", "get-url", "origin")
	if out, err := runGitOut(dir, "--git-dir="+strings.TrimSpace(bare), "symbolic-ref", "HEAD", "refs/heads/develop"); err != nil {
		t.Fatalf("uzak HEAD develop yapılamadı: %v %s", err, out)
	}
	run("checkout", "-b", "feat/x")
	write("feat-x.txt", "x\n")
	run("push", "-u", "origin", "feat/x")
	run("checkout", "develop")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}
	res, err := mgr.GetDiff("feat/x", "branch")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.RawDiff, "feat-x.txt") {
		t.Fatalf("diff must contain the branch's own change:\n%s", res.RawDiff)
	}
	if strings.Contains(res.RawDiff, "only-on-develop.txt") {
		t.Fatalf("diff leaked a develop-only commit (base was main, not develop):\n%s", res.RawDiff)
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DefaultBranch != "develop" {
		t.Fatalf("snapshot default branch = %q, want develop", snap.Repo.DefaultBranch)
	}
}
