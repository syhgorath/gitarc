package gitengine

import (
	"context"
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

func TestParseSymrefHead(t *testing.T) {
	cases := map[string]string{
		"ref: refs/heads/main\tHEAD\n1111111111111111111111111111111111111111\tHEAD\n": "main",
		"ref: refs/heads/feature/x\tHEAD\nabc\tHEAD\n":                                 "feature/x",
		"1111111111111111111111111111111111111111\tHEAD\n":                             "", // sembolik değil (detached)
		"":                          "",
		"ref: refs/tags/v1\tHEAD\n": "",
	}
	for in, want := range cases {
		if got := parseSymrefHead(in); got != want {
			t.Errorf("parseSymrefHead(%q) = %q, want %q", in, got, want)
		}
	}
}

// GitHub'da varsayılan dal değişince klonun `origin/HEAD` kaydı ESKİ kalır (git fetch onu güncellemez). Omurga uzağın gerçek
// varsayılanından alınmalı; yoksa asıl dal "yarım parabol" gibi çizilir, koruma/PR tabanı yanlış dalı gösterir.
func TestDefaultBranchFollowsRemoteWhenLocalOriginHeadIsStale(t *testing.T) {
	dir, run := newGitHubLikeRepo(t) // uzak (bare) varsayılanı: main
	run("branch", "old")
	run("push", "origin", "old")
	run("remote", "set-head", "origin", "old") // klonlama günlerinden kalan eski kayıt
	ctx := context.Background()

	if got := detectDefaultBranch(ctx, dir); got != "old" {
		t.Fatalf("ön koşul: uzak bilgisi okunmadan eski origin/HEAD kullanılır, geldi %q", got)
	}
	fetchRemoteCached(ctx, dir, "origin") // arka plan fetch uzağın gerçek varsayılanını da okur (tamamlanana kadar bekler)
	if got := remoteDefaultBranch(dir); got != "main" {
		t.Fatalf("uzağın varsayılanı main okunmalıydı, geldi %q", got)
	}
	if got := detectDefaultBranch(ctx, dir); got != "main" {
		t.Fatalf("omurga uzağın gerçek varsayılanı (main) olmalı, eski origin/HEAD'in 'old'u değil: %q", got)
	}
}

func TestPRBaseArgsOnlyWhenBaseExistsOnRemote(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	run("branch", "yerel-sadece")
	ctx := context.Background()
	if got := prBaseArgs(ctx, dir, "main"); len(got) != 2 || got[0] != "--base" || got[1] != "main" {
		t.Fatalf("uzakta var olan omurga açıkça taban verilmeli: %v", got)
	}
	if got := prBaseArgs(ctx, dir, "yerel-sadece"); got != nil {
		t.Fatalf("yalnızca yerelde olan omurga taban verilmemeli (gh hata verirdi): %v", got)
	}
	if got := prBaseArgs(ctx, dir, ""); got != nil {
		t.Fatalf("boş omurga: taban yok, geldi %v", got)
	}
}
