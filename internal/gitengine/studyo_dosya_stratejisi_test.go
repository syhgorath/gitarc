package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// İki dosya çakışıyor: CHANGELOG benzeri bir dosyada İKİSİ korunmalı (union), koddaki dosyada main alınmalı (ours). Eskiden tek bir strateji
// tüm dosyalara uygulanıyordu; bu yüzden böyle karışık bir çözüm stüdyoyla yapılamıyordu (yalnızca her şeyi elle yazarak).
func TestResolveConflictAppliesAStrategyPerFile(t *testing.T) {
	isoHome(t)
	dir, run := newGitHubLikeRepo(t)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("NOTLAR.md", "# Notlar\n")
	write("kod.txt", "deger=1\n")
	run("add", "-A")
	run("commit", "-qm", "temel")
	run("push", "-q", "origin", "main")

	run("checkout", "-q", "-b", "feat/x")
	write("NOTLAR.md", "# Notlar\n- dal maddesi\n")
	write("kod.txt", "deger=dal\n")
	run("add", "-A")
	run("commit", "-qm", "dal degisikligi")
	run("push", "-q", "-u", "origin", "feat/x")
	run("checkout", "-q", "main")
	run("branch", "-D", "feat/x")

	write("NOTLAR.md", "# Notlar\n- main maddesi\n")
	write("kod.txt", "deger=main\n")
	run("add", "-A")
	run("commit", "-qm", "main degisikligi")
	run("push", "-q", "origin", "main")

	mgr := switchLive(t, dir)
	studio, err := mgr.InspectConflictStudio("remote-feat-x")
	if err != nil || len(studio.Files) != 2 {
		t.Fatalf("iki dosya çakışmalı: %+v %v", studio.Files, err)
	}
	if _, err := mgr.ResolveBranchConflictFiles("remote-feat-x", "union",
		map[string]string{"NOTLAR.md": "union", "kod.txt": "ours"}, nil, false); err != nil {
		t.Fatalf("dosya başına çözüm: %v", err)
	}

	run("fetch", "-q", "origin")
	notlar := mustGit(t, dir, "show", "origin/feat/x:NOTLAR.md")
	kod := mustGit(t, dir, "show", "origin/feat/x:kod.txt")
	if !strings.Contains(notlar, "main maddesi") || !strings.Contains(notlar, "dal maddesi") {
		t.Fatalf("NOTLAR.md'de İKİ taraf da korunmalı (union): %q", notlar)
	}
	if strings.TrimSpace(kod) != "deger=main" {
		t.Fatalf("kod.txt'de main alınmalı (ours), dal değişikliği düşmeli: %q", kod)
	}
	msg := mustGit(t, dir, "log", "-1", "--format=%s", "origin/feat/x")
	if !strings.Contains(msg, "karma") || !strings.Contains(msg, "NOTLAR.md=union") || !strings.Contains(msg, "kod.txt=ours") {
		t.Fatalf("commit mesajı karma çözümü dosya başına özetlemeli: %q", msg)
	}
}

// Dosya başına seçim yoksa eski davranış (tek strateji hepsine) aynen sürer; "özel" olmayan dosyada eski özel metin uygulanmaz.
func TestResolveConflictWithoutPerFileMapKeepsSingleStrategyAndIgnoresStaleCustomText(t *testing.T) {
	if got := strategyLabel("union", nil); got != "union" {
		t.Fatalf("harita yokken strateji adı: %q", got)
	}
	if got := strategyLabel("union", map[string]string{"a": "ours", "b": "main"}); got != "ours" {
		t.Fatalf("hepsi aynıysa o strateji (main→ours normalleşir): %q", got)
	}
	if got := strategyLabel("union", map[string]string{"a": "ours", "b": "theirs"}); got != "karma" {
		t.Fatalf("farklıysa karma: %q", got)
	}
	if d := strategyDetail(map[string]string{"b": "theirs", "a": "ours"}); d != "a=ours, b=theirs" {
		t.Fatalf("özet sıralı olmalı: %q", d)
	}
	if d := strategyDetail(map[string]string{"a": "ours"}); d != "" {
		t.Fatalf("karma değilse özet yok: %q", d)
	}
}
