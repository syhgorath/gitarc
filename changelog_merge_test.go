package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CHANGELOG.md satır ekleme çakışmaları kuyruğu her seferinde durduruyordu. Depodaki .gitattributes (merge=union) bunu önler:
// iki dalın AYNI noktaya eklediği satırlar çakışmasız birleşir ve ikisi de korunur. Test gerçek dosyayı kullanır, kuralı silmek testi düşürür.
func TestChangelogAdditionsOnTheSameSpotMergeWithoutConflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git yok")
	}
	attrs, err := os.ReadFile(".gitattributes")
	if err != nil {
		t.Fatalf(".gitattributes okunamadı: %v", err)
	}
	if !strings.Contains(string(attrs), "CHANGELOG.md merge=union") {
		t.Fatalf(".gitattributes CHANGELOG.md için merge=union içermeli:\n%s", attrs)
	}

	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@t", "HOME="+dir)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q", "-b", "main")
	write("CHANGELOG.md", "# CL\n\n### Düzeltilenler\n- eski\n")
	git("add", "-A")
	git("commit", "-qm", "temel")
	for _, b := range []string{"a", "b"} {
		git("checkout", "-q", "-b", b, "main")
		write("CHANGELOG.md", "# CL\n\n### Düzeltilenler\n- yeni "+b+"\n- eski\n")
		git("commit", "-qam", b)
	}
	git("checkout", "-q", "main")
	write(".gitattributes", string(attrs)) // kural main'e girdi; a ve b ONDAN ÖNCE açılmış (ayarsız) eski dallar
	git("add", "-A")
	git("commit", "-qm", "attr")

	git("merge", "-q", "a", "-m", "ma")
	git("merge", "-q", "b", "-m", "mb") // ayar yokken burada CONFLICT olurdu
	got := git("show", "HEAD:CHANGELOG.md")
	if !strings.Contains(got, "- yeni a") || !strings.Contains(got, "- yeni b") || !strings.Contains(got, "- eski") || strings.Contains(got, "<<<<<<<") {
		t.Fatalf("iki taraf da korunmalı, işaret kalmamalı:\n%s", got)
	}
}
