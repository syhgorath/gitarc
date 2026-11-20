package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// git worktree klasöründe `.git` bir DOSYADIR; GitArc bunu depo saymalı (demo depoya düşmemeli) ve HEAD/index ile
// kendi durum dosyalarını worktree'nin gerçek git dizininde aramalıdır.
func TestWorktreeFolderIsTreatedAsRepo(t *testing.T) {
	main, run := newTempRepo(t)
	wt := filepath.Join(t.TempDir(), "wt")
	run("worktree", "add", "-b", "feat/wt", wt)

	if st, err := os.Stat(filepath.Join(wt, ".git")); err != nil || st.IsDir() {
		t.Fatalf("worktree .git bir dosya olmalı: %v", err)
	}
	if !isGitDir(wt) {
		t.Fatal("worktree klasörü depo sayılmadı")
	}
	if isGitDir(t.TempDir()) {
		t.Fatal("boş klasör depo sayıldı")
	}
	gd := gitDirOf(wt)
	if gd == "" || !strings.Contains(filepath.ToSlash(gd), "/worktrees/") {
		t.Fatalf("gerçek git dizini bulunamadı: %q", gd)
	}
	if _, err := os.Stat(filepath.Join(gd, "HEAD")); err != nil {
		t.Fatalf("worktree HEAD dosyası yok: %v", err)
	}
	if got := proposalFilePath(wt); filepath.Dir(got) != gd {
		t.Fatalf("öneri dosyası git dizininde olmalı: %s", got)
	}
	if got := queueRunFile(wt); filepath.Dir(got) != gd {
		t.Fatalf("kuyruk koşusu dosyası git dizininde olmalı: %s", got)
	}

	m := NewManager(wt)
	if m.NoProject() {
		t.Fatal("worktree ile başlatılınca proje yok görünüyor")
	}
	snap, err := m.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.Mode != "live" {
		t.Fatalf("worktree canlı depo olmalı, mod=%q", snap.Repo.Mode)
	}
	sig := m.QuickRepoSignature()
	if strings.HasPrefix(sig, "sim-") {
		t.Fatalf("imza simülasyon imzası: %s", sig)
	}
	_ = main
}

// MCP yapılandırması için anlık görüntü, AÇIK PROJENİN değil çalışan ikilinin yolunu taşır.
func TestSnapshotCarriesExecutablePathNotRepoPath(t *testing.T) {
	dir, _ := newTempRepo(t)
	m := NewManager(dir)
	snap, err := m.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	if snap.Repo.ExecPath == "" || !filepath.IsAbs(snap.Repo.ExecPath) {
		t.Fatalf("execPath mutlak yol olmalı: %q", snap.Repo.ExecPath)
	}
	if strings.HasPrefix(snap.Repo.ExecPath, dir) {
		t.Fatalf("execPath açık projenin klasöründe olamaz: %q (proje %q)", snap.Repo.ExecPath, dir)
	}
	if filepath.Base(snap.Repo.ExecPath) != filepath.Base(exe) {
		t.Fatalf("execPath çalışan ikili olmalı: %q vs %q", snap.Repo.ExecPath, exe)
	}
}
