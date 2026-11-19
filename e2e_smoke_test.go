package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gitarc/internal/gitengine"
)

// serveApp: gerçek bir TCP dinleyicisinde uygulamayı (oturum çerezi, Host denetimi dahil) sunar; belirteçli adresi döner.
func serveApp(t *testing.T, mgr *gitengine.Manager) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := ln.Addr().String()
	app := newApp(host, mgr)
	srv := &http.Server{Handler: app.routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return "http://" + host + "/?t=" + app.token
}

// TestBrowserSmoke: GELİŞTİRİCİ testi (tests/e2e/). Gerçek bir Chrome'da ana pencereleri tıklar: proje yok paneli, demo
// şeridi, tur, MCP, çakışma stüdyosu, birleşmiş dalın dosya satırı → Diff penceresi. Chrome ya da node yoksa atlanır;
// GITARC_E2E=0 ile de kapatılabilir. Uygulamaya gömülmez, son kullanıcıya hiçbir etkisi yoktur.
func TestBrowserSmoke(t *testing.T) {
	if os.Getenv("GITARC_E2E") == "0" {
		t.Skip("GITARC_E2E=0")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}

	// 1) Proje yok: çalışma klasörü ve verilen klasör depo değil.
	empty := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	noneURL := serveApp(t, gitengine.NewManager(empty))

	// 2) Demo: açıkça istenir.
	simURL := serveApp(t, gitengine.NewManager("sim"))

	// 3) Canlı depo: birleştirilmiş bir dal (iki dosya) içerir.
	repo := tempRepo(t)
	gitRun(t, repo, "checkout", "-b", "feat/x")
	for _, f := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte(f+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-m", "feat: x")
	gitRun(t, repo, "checkout", "main")
	gitRun(t, repo, "merge", "--no-ff", "feat/x", "-m", "Merge pull request #7 from acme/feat/x")
	gitRun(t, repo, "branch", "-D", "feat/x")
	liveURL := serveApp(t, gitengine.NewManager(repo))

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, filepath.Join(old, "tests", "e2e", "smoke.mjs"), noneURL, simURL, liveURL)
	cmd.Dir = old
	out, err := cmd.CombinedOutput()
	if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 77 {
		t.Skipf("Chrome bulunamadı:\n%s", out)
	}
	if err != nil {
		t.Fatalf("tarayıcı duman testi kaldı: %v\n%s", err, out)
	}
	t.Logf("\n%s", out)
}
