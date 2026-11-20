package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

// Proje bağlama: sunucuda "etkin proje" tek ve geneldir. Başka bir sekme projeyi değiştirdiyse eski sekmedeki bir işlem
// (dal sil, merge, commit...) sessizce ÖTEKİ projeye giderdi. İstemci kendi gösterdiği projenin kimliğini X-GitArc-Repo
// başlığıyla gönderir; uyuşmazlıkta sunucu işlemi YAPMAZ (409).

func (c *apiClient) doWith(method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+c.host+path, bytes.NewBufferString(body))
	r.Host = c.host
	r.AddCookie(c.cookie)
	if method == http.MethodPost {
		r.Header.Set("X-Requested-With", "gitarc")
		r.Header.Set("Origin", "http://"+c.host)
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	return w
}

func sameDir(a, b string) bool {
	n := func(p string) string {
		if r, err := filepath.EvalSymlinks(filepath.Clean(p)); err == nil {
			return r
		}
		return filepath.Clean(p)
	}
	return n(a) == n(b)
}

func branchExists(t *testing.T, dir, name string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", name)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// iki gerçek depo: ikisinde de aynı adlı, farklı içerikli "feat/shared" dalı var.
func twoReposWithSharedBranch(t *testing.T) (mgr *gitengine.Manager, dirA, dirB, idA, idB string) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
	mk := func(label string) string {
		d := tempRepo(t)
		gitRun(t, d, "checkout", "-q", "-b", "feat/shared")
		if err := os.WriteFile(filepath.Join(d, "shared.txt"), []byte(label), 0644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, d, "add", "-A")
		gitRun(t, d, "commit", "-qm", "shared "+label)
		gitRun(t, d, "checkout", "-q", "main")
		return d
	}
	dirA, dirB = mk("a"), mk("b")
	mgr = gitengine.NewManager(dirA)
	snap, err := mgr.AddProject(dirB, "b") // etkin proje artık B
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range snap.Repos {
		if sameDir(r.Path, dirA) {
			idA = r.ID
		}
		if sameDir(r.Path, dirB) {
			idB = r.ID
		}
	}
	if idA == "" || idB == "" || snap.Repo.ID != idB {
		t.Fatalf("düzenek: A=%q B=%q etkin=%q", idA, idB, snap.Repo.ID)
	}
	return
}

// Gerçek senaryo: sekme 1 A'yı gösteriyor, sekme 2 sunucuyu B'ye geçirdi; sekme 1 "feat/shared" dalını silmek istiyor.
// Eskiden B'nin dalı silinirdi. Şimdi 409 ve hiçbir şey silinmez.
func TestStaleTabRequestIsRefusedNotAppliedToTheOtherProject(t *testing.T) {
	mgr, dirA, dirB, idA, idB := twoReposWithSharedBranch(t)
	c := newAPIClient(t, mgr)

	rec := c.doWith(http.MethodPost, "/api/branch/delete", `{"branchId":"feat/shared"}`, map[string]string{repoBindingHeader: idA})
	if rec.Code != http.StatusConflict {
		t.Fatalf("eski sekmenin isteği reddedilmeli (409), geldi: %d %s", rec.Code, rec.Body)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != "repo_mismatch" || body["activeRepoId"] != idB || body["error"] == "" {
		t.Fatalf("409 gövdesi sunucunun etkin projesini ve kodu taşımalı: %+v", body)
	}
	if !branchExists(t, dirA, "feat/shared") || !branchExists(t, dirB, "feat/shared") {
		t.Fatal("reddedilen istek hiçbir projede dal silmemeli")
	}

	// Doğru projeyi gösteren sekmenin isteği çalışır ve YALNIZCA B'ye dokunur.
	rec = c.doWith(http.MethodPost, "/api/branch/delete", `{"branchId":"feat/shared"}`, map[string]string{repoBindingHeader: idB})
	if rec.Code != http.StatusOK {
		t.Fatalf("doğru projeyle istek çalışmalı: %d %s", rec.Code, rec.Body)
	}
	if branchExists(t, dirB, "feat/shared") {
		t.Fatal("B'de dal silinmeliydi")
	}
	if !branchExists(t, dirA, "feat/shared") {
		t.Fatal("A'daki dal etkilenmemeli")
	}
}

// Başlık yoksa (CLI, eski istemci, mevcut testler) bağlama uygulanmaz; projeyi değiştiren/seçen uçlar muaftır.
func TestRepoBindingIsOptionalAndExemptEndpointsStillWork(t *testing.T) {
	mgr, dirA, dirB, idA, idB := twoReposWithSharedBranch(t)
	c := newAPIClient(t, mgr)

	// Muaf uç: tamamen yanlış bir başlıkla bile proje seçilebilir (sekme projeyi buradan öğrenir/değiştirir).
	rec := c.doWith(http.MethodPost, "/api/repo/switch", `{"repoId":"`+idA+`"}`, map[string]string{repoBindingHeader: "baska-bir-proje"})
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/repo/switch bağlamadan muaf olmalı: %d %s", rec.Code, rec.Body)
	}
	var snap model.CockpitSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil || snap.Repo.ID != idA {
		t.Fatalf("A seçilmeliydi: %v %s", err, snap.Repo.ID)
	}
	if rec := c.doWith(http.MethodGet, "/api/snapshot", "", map[string]string{repoBindingHeader: "baska-bir-proje"}); rec.Code != http.StatusOK {
		t.Fatalf("/api/snapshot bağlamadan muaf olmalı: %d", rec.Code)
	}

	// Başlık hiç yoksa: eski davranış (etkin projeye uygulanır).
	rec = c.do(http.MethodPost, "/api/branch/delete", `{"branchId":"feat/shared"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("başlıksız istek çalışmalı: %d %s", rec.Code, rec.Body)
	}
	if branchExists(t, dirA, "feat/shared") || !branchExists(t, dirB, "feat/shared") {
		t.Fatal("başlıksız istek etkin projeye (A) uygulanmalı")
	}
	_ = idB
}

// Proje değişince sekme güncel kimlikle devam eder: eski kimlik reddedilir, yeni kimlik kabul edilir.
func TestRepoBindingFollowsSwitches(t *testing.T) {
	mgr, _, _, idA, idB := twoReposWithSharedBranch(t)
	c := newAPIClient(t, mgr)
	post := func(id string) int {
		return c.doWith(http.MethodPost, "/api/spine/sync", `{}`, map[string]string{repoBindingHeader: id}).Code
	}
	if code := post(idA); code != http.StatusConflict {
		t.Fatalf("etkin B iken A kimliği reddedilmeli: %d", code)
	}
	if _, err := mgr.SwitchRepo(idA); err != nil {
		t.Fatal(err)
	}
	if code := post(idB); code != http.StatusConflict {
		t.Fatalf("etkin A iken B kimliği reddedilmeli: %d", code)
	}
	if code := post(idA); code == http.StatusConflict {
		t.Fatalf("etkin A iken A kimliği kabul edilmeli: %d", code)
	}
}
