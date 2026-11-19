package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoFileDiff = `diff --git a/a.go b/a.go
index 1..2 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,3 @@
 keep
+added a
-removed a
diff --git a/b.txt b/b.txt
new file mode 100644
--- /dev/null
+++ b/b.txt
@@ -0,0 +1 @@
+only b
`

// "Canlı Satır İçi Kod Diff" paneli dosyanın patch'ini okur; eskiden hiç doldurulmadığı için her dosya için
// "(İkili veya boş içerik)" yazıyordu: kullanıcı dosyanın içini göremiyordu.
func TestParseDiffOutputFillsAPatchPerFile(t *testing.T) {
	res := ParseDiffOutput(twoFileDiff, "x", "branch", "t")
	if len(res.Files) != 2 {
		t.Fatalf("files = %d", len(res.Files))
	}
	a, b := res.Files[0], res.Files[1]
	if !strings.Contains(a.Patch, "+added a") || !strings.Contains(a.Patch, "-removed a") || !strings.HasPrefix(a.Patch, "diff --git a/a.go") {
		t.Fatalf("a.go patch wrong: %q", a.Patch)
	}
	if strings.Contains(a.Patch, "only b") {
		t.Fatalf("a.go patch leaked the next file: %q", a.Patch)
	}
	if !strings.Contains(b.Patch, "+only b") || strings.Contains(b.Patch, "added a") {
		t.Fatalf("b.txt patch wrong: %q", b.Patch)
	}
	if a.Additions != 1 || a.Deletions != 1 || b.Status != "A" {
		t.Fatalf("counts/status must be unchanged: %+v %+v", a, b)
	}
}

func TestHugePatchesAreTrimmedButTheRawDiffStaysWhole(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("diff --git a/big.txt b/big.txt\n--- a/big.txt\n+++ b/big.txt\n@@ -0,0 +1,2000 @@\n")
	for i := 0; i < 2000; i++ {
		sb.WriteString("+line\n")
	}
	res := ParseDiffOutput(sb.String(), "x", "branch", "t")
	if got := strings.Count(res.Files[0].Patch, "\n"); got > maxPatchLines+2 {
		t.Fatalf("patch must be trimmed, has %d lines", got)
	}
	if !strings.Contains(res.Files[0].Patch, "satır daha") {
		t.Fatal("a trimmed patch must say so")
	}
	if strings.Count(res.RawDiff, "+line") != 2000 || res.Files[0].Additions != 2000 {
		t.Fatal("raw diff and counts must stay complete")
	}
}

// Yalnızca uzakta olan dalın (yerel kopyası yok) diff'i eskiden boş geliyordu.
func TestBranchDiffWorksForARemoteOnlyBranch(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	run("checkout", "-b", "feat/remote-only")
	if err := os.WriteFile(filepath.Join(dir, "r.txt"), []byte("remote only\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: remote only")
	run("push", "-u", "origin", "feat/remote-only")
	run("checkout", "main")
	run("branch", "-D", "feat/remote-only")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	res, err := mgr.GetDiff("feat/remote-only", "branch")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 1 || res.Files[0].Path != "r.txt" || !strings.Contains(res.Files[0].Patch, "+remote only") {
		t.Fatalf("a remote-only branch must be diffed against origin/<branch>: %+v", res.Files)
	}
}

func TestBranchDiffRejectsOptionLikeTargets(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"--output=/tmp/gitarc-pwn", "-p", "a..b"} {
		if _, err := mgr.GetDiff(bad, "branch"); err == nil {
			t.Errorf("GetDiff(%q, branch) must be rejected", bad)
		}
	}
	if _, err := os.Stat("/tmp/gitarc-pwn"); err == nil {
		t.Fatal("an option-like target must never reach git")
	}
}
