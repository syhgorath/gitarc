package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMergeUniqueFiles(t *testing.T) {
	got := mergeUniqueFiles([]string{"a.go", "", "b.go", "a.go"}, []string{"b.go", "c.go", ""})
	want := []string{"a.go", "b.go", "c.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if got := mergeUniqueFiles(nil, nil); len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestMergeUniquePatterns(t *testing.T) {
	got := mergeUniquePatterns([]string{".env", " .env ", ""}, []string{"*.pem", ".env", "  "})
	want := []string{".env", "*.pem"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestIsHex(t *testing.T) {
	cases := map[string]bool{
		"": false, "abc123": true, "ABCDEF": true, "deadbeefg": false, "12 34": false, "-1": false,
	}
	for in, want := range cases {
		if got := isHex(in); got != want {
			t.Errorf("isHex(%q)=%v want %v", in, got, want)
		}
	}
}

func TestIsSpecialAutoMergeFile(t *testing.T) {
	yes := []string{"CHANGELOG.md", "docs/changelog", "go.mod", "sub/go.sum", "VERSION", "web/package-lock.json", "Cargo.lock", "yarn.lock", "pnpm-lock.yaml", "components.lock"}
	no := []string{"main.go", "README.md", "changelog.txt", "version.go", ""}
	for _, p := range yes {
		if !IsSpecialAutoMergeFile(p) {
			t.Errorf("expected %q to be special", p)
		}
	}
	for _, p := range no {
		if IsSpecialAutoMergeFile(p) {
			t.Errorf("expected %q to be non-special", p)
		}
	}
}

func TestNormalizeGoPkg(t *testing.T) {
	cases := map[string]string{
		"keylab/internal/shellgold":      "./internal/shellgold",
		"github.com/org/repo/internal/x": "./internal/x",
		"github.com/org/repo/cmd/tool":   "./cmd/tool",
		"plainpkg":                       "plainpkg",
	}
	for in, want := range cases {
		if got := normalizeGoPkg(in); got != want {
			t.Errorf("normalizeGoPkg(%q)=%q want %q", in, got, want)
		}
	}
}

// newTempRepo creates a git repo with one commit on main and returns its path.
func newTempRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not installed")
	}
	dir := t.TempDir()
	// Anlık görüntü arka planda `git fetch`/`gh` başlatır; test dizini silinmeden ÖNCE bitmelerini bekle (Cleanup'lar ters sırada çalışır,
	// bu kayıt TempDir'inkinden sonra yapıldığı için ondan önce çalışır). Aksi hâlde yavaş CI'da "directory not empty" ile test düşer.
	t.Cleanup(func() { settleBackground(20 * time.Second) })
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com",
			"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.name", "Tester")
	run("config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "chore: initial commit")
	return dir, run
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestLiveCreateCheckoutAndDuplicateGuard(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatalf("SwitchRepo: %v", err)
	}

	snap, err := mgr.CreateBranch("feat/live-new", "lower", "")
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	found := false
	for _, b := range snap.Branches {
		if b.Name == "feat/live-new" {
			found = true
		}
	}
	if !found {
		t.Fatalf("created branch missing from snapshot")
	}

	// Duplicate (case-insensitive) must be rejected with a clear message.
	if _, err := mgr.CreateBranch("FEAT/live-new", "lower", ""); err == nil || !strings.Contains(err.Error(), "zaten mevcut") {
		t.Fatalf("expected duplicate branch error, got %v", err)
	}
	// Invalid SHA must be rejected.
	if _, err := mgr.CreateBranch("feat/bad-sha", "lower", "--evil"); err == nil {
		t.Fatalf("expected invalid SHA error")
	}

	if _, err := mgr.CheckoutBranch("feat/live-new"); err != nil {
		t.Fatalf("CheckoutBranch: %v", err)
	}
	if got := currentBranch(t, dir); got != "feat/live-new" {
		t.Fatalf("expected HEAD on feat/live-new, got %q", got)
	}
	if _, err := mgr.CheckoutBranch("does/not-exist"); err == nil {
		t.Fatalf("expected error for unknown branch")
	}
}

func TestLiveTeamModeBlocksSharedHEADMutations(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.CreateBranch("feat/team", "lower", ""); err != nil {
		t.Fatal(err)
	}
	mgr.SetTeamMode(true)
	if _, err := mgr.CheckoutBranch("feat/team"); err == nil {
		t.Fatalf("team mode must block CheckoutBranch")
	}
	if got := currentBranch(t, dir); got != "main" {
		t.Fatalf("HEAD must stay on main in team mode, got %q", got)
	}
}

func TestLiveAmendLastCommitRules(t *testing.T) {
	dir, run := newTempRepo(t)
	run("checkout", "-b", "feat/amend-me")
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "x.txt")
	run("commit", "-m", "feat: old message")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.AmendLastCommit("feat/amend-me", "feat: new message"); err != nil {
		t.Fatalf("AmendLastCommit: %v", err)
	}
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = dir
	out, _ := cmd.Output()
	if strings.TrimSpace(string(out)) != "feat: new message" {
		t.Fatalf("commit message not amended: %q", out)
	}
	// The default branch must never be amendable.
	if _, err := mgr.AmendLastCommit("main", "nope"); err == nil {
		t.Fatalf("expected error amending default branch")
	}
}
