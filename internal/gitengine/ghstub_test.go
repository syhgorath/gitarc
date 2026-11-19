package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ghStub is a fake `gh` executable placed first on PATH. Behaviour is driven by
// files in its directory: <key>.out (stdout) and <key>.exit (exit code), so
// tests never touch the network or the developer's real GitHub account.
//
// Keys: pr_list_open, pr_list_merged, pr_list_merged_head, pr_create, pr_merge,
// pr_edit, pr_close, run_view, api_protection_get, api_protection_put, api_rulesets.
// Every invocation is appended to calls.log; stdin of `api -X PUT` is saved to
// api_protection_put.stdin.
type ghStub struct {
	t   *testing.T
	dir string
}

const ghStubScript = `#!/bin/sh
d="$GH_STUB_DIR"
echo "$*" >> "$d/calls.log"
all="$*"
key="other"
case "$1" in
  pr)
    case "$2" in
      list)
        case "$all" in
          *"--state merged"*"--head"*) key="pr_list_merged_head" ;;
          *"--state merged"*) key="pr_list_merged" ;;
          *) key="pr_list_open" ;;
        esac ;;
      create) key="pr_create" ;;
      merge) key="pr_merge" ;;
      edit) key="pr_edit" ;;
      close) key="pr_close" ;;
    esac ;;
  run) key="run_view" ;;
  api)
    case "$all" in
      *"-X PUT"*) key="api_protection_put"; cat > "$d/$key.stdin" ;;
      *rulesets*) key="api_rulesets" ;;
      *protection*) key="api_protection_get" ;;
    esac ;;
esac
if [ -f "$d/$key.out" ]; then cat "$d/$key.out"
else
  case "$key" in pr_list_*) echo "[]" ;; esac
fi
if [ -f "$d/$key.exit" ]; then exit "$(cat "$d/$key.exit")"; fi
exit 0
`

func newGHStub(t *testing.T) *ghStub {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("gh stub needs a POSIX shell")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(ghStubScript), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GH_STUB_DIR", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	return &ghStub{t: t, dir: dir}
}

func (s *ghStub) set(key, out string, exit int) {
	s.t.Helper()
	if out != "" {
		if err := os.WriteFile(filepath.Join(s.dir, key+".out"), []byte(out), 0644); err != nil {
			s.t.Fatal(err)
		}
	}
	if exit != 0 {
		if err := os.WriteFile(filepath.Join(s.dir, key+".exit"), []byte(itoa(exit)), 0644); err != nil {
			s.t.Fatal(err)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func (s *ghStub) calls() []string {
	b, err := os.ReadFile(filepath.Join(s.dir, "calls.log"))
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(b)), "\n")
}

// called reports whether any recorded gh invocation contains all given substrings.
func (s *ghStub) called(parts ...string) bool {
	for _, c := range s.calls() {
		ok := true
		for _, p := range parts {
			if !strings.Contains(c, p) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func (s *ghStub) stdin(key string) string {
	b, _ := os.ReadFile(filepath.Join(s.dir, key+".stdin"))
	return string(b)
}

// newGitHubLikeRepo returns a work repo whose `origin` is a local bare repo whose
// path contains "github.com", so GitArc treats it as a GitHub remote while every
// push/fetch stays on local disk.
func newGitHubLikeRepo(t *testing.T) (string, func(args ...string)) {
	t.Helper()
	dir, run := newTempRepo(t)
	bare := filepath.Join(t.TempDir(), "github.com", "acme", "demo.git")
	if err := os.MkdirAll(bare, 0755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", bare)
	run("push", "-u", "origin", "main")
	return dir, run
}

// addFeatureBranch creates a local branch with one commit, then returns to main.
func addFeatureBranch(t *testing.T, dir string, run func(args ...string), name string) {
	t.Helper()
	run("checkout", "-b", name)
	if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(name, "/", "_")+".txt"), []byte(name), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: "+name)
	run("checkout", "main")
}

func osRemove(p string) error { return os.Remove(p) }

func runGitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
