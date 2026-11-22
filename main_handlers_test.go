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

type apiClient struct {
	t      *testing.T
	h      http.Handler
	host   string
	cookie *http.Cookie
}

func newAPIClient(t *testing.T, mgr *gitengine.Manager) *apiClient {
	t.Helper()
	host := "127.0.0.1:7432"
	h := newApp(host, mgr).routes()
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	cs := rec.Result().Cookies()
	if len(cs) == 0 {
		t.Fatal("no session cookie")
	}
	return &apiClient{t: t, h: h, host: host, cookie: cs[0]}
}

func (c *apiClient) do(method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://"+c.host+path, bytes.NewBufferString(body))
	r.Host = c.host
	r.AddCookie(c.cookie)
	if method == http.MethodPost {
		r.Header.Set("X-Requested-With", "gitarc")
		r.Header.Set("Origin", "http://"+c.host)
	}
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	return w
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.com")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func tempRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.name", "T")
	gitRun(t, dir, "config", "user.email", "t@e.com")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "chore: init")
	return dir
}

func TestPostOnlyEndpointsRejectGET(t *testing.T) {
	c := newAPIClient(t, gitengine.NewManager(""))
	for _, p := range []string{
		"/api/spine/sync", "/api/sync-spine", "/api/logs/clear", "/api/system/update-check",
		"/api/repo/protect", "/api/repo/protection/plan", "/api/queue/plan", "/api/branch/update/plan", "/api/branch/update", "/api/simulation/reset", "/api/reset",
		"/api/branch/create", "/api/branch/promote", "/api/working-tree/commit",
		"/api/repo/remove", "/api/branch/delete", "/api/pr/edit",
	} {
		if rec := c.do(http.MethodGet, p, ""); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s: expected 405, got %d", p, rec.Code)
		}
	}
}

// Sekme simgesi: index.html onu bağlar, ikili içinde SVG olarak servis edilir ve içinde çalıştırılabilir içerik yoktur (script/olay işleyicisi/dış kaynak).
func TestFaviconIsLinkedServedAsSVGAndIsInert(t *testing.T) {
	c := newAPIClient(t, gitengine.NewManager(""))
	get := func(p string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "http://"+c.host+p, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		return rec
	}
	if !strings.Contains(get("/").Body.String(), `rel="icon" type="image/svg+xml" href="favicon.svg"`) {
		t.Fatal("index.html sekme simgesini (favicon.svg) bağlamalı")
	}
	if !strings.Contains(get("/").Body.String(), `class="brand-mark" src="favicon.svg"`) {
		t.Fatal("üst çubuk logosu da favicon.svg olmalı (tek kaynak)")
	}
	rec := get("/favicon.svg")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "image/svg+xml") {
		t.Fatalf("favicon.svg: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := strings.ToLower(rec.Body.String())
	for _, bad := range []string{"<script", "onload", "onclick", "href=\"http", "xlink:href", "<image", "<foreignobject"} {
		if strings.Contains(body, bad) {
			t.Fatalf("favicon.svg etkisiz (inert) olmalı, %q içeriyor", bad)
		}
	}
}

func TestStaticAssetsAreNeverCachedAndHaveNoHardcodedVersionQuery(t *testing.T) {
	c := newAPIClient(t, gitengine.NewManager(""))
	for _, p := range []string{"/", "/app.js", "/tasarim.css", "/modules/scene.js", "/favicon.svg"} {
		req := httptest.NewRequest(http.MethodGet, "http://"+c.host+p, nil)
		req.Host = c.host
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d", p, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("%s: Cache-Control must contain no-store, got %q", p, cc)
		}
		if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: security headers missing", p)
		}
	}
	idx, err := webFS.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(idx), "?v=") {
		t.Error("index.html must not hard-code asset version queries (stale on version bump); no-store already prevents caching")
	}
}

func TestLiveRepoHandlersEndToEnd(t *testing.T) {
	dir := tempRepo(t)
	mgr := gitengine.NewManager(dir)
	c := newAPIClient(t, mgr)

	if rec := c.do(http.MethodPost, "/api/repo/switch", `{"repoId":"live-custom"}`); rec.Code != http.StatusOK {
		t.Fatalf("switch: %d %s", rec.Code, rec.Body)
	}

	// Dirty file → diff-stat and diff endpoints see it.
	if err := os.WriteFile(filepath.Join(dir, "wip.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	rec := c.do(http.MethodPost, "/api/working-tree/diff-stat", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("diff-stat: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodGet, "/api/diff?target=working-tree&type=working", ""); rec.Code != http.StatusOK && rec.Code != http.StatusInternalServerError {
		t.Fatalf("diff unexpected status %d", rec.Code)
	}

	// Proposal dismiss is idempotent.
	if rec := c.do(http.MethodPost, "/api/proposal/dismiss", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("dismiss: %d %s", rec.Code, rec.Body)
	}

	// Commit working tree onto a fresh local branch (no push).
	rec = c.do(http.MethodPost, "/api/working-tree/commit",
		`{"branchName":"feat/wip-handler","commitMsg":"feat: wip handler","pushAndPr":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit: %d %s", rec.Code, rec.Body)
	}
	var snap model.CockpitSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.Repo.DirtyCount != 0 {
		t.Fatalf("tree must be clean after commit, dirty=%d", snap.Repo.DirtyCount)
	}

	// Invalid branch name rejected at the API boundary.
	if rec := c.do(http.MethodPost, "/api/branch/create", `{"name":"bad name","hemisphere":"lower"}`); rec.Code == http.StatusOK {
		t.Fatal("invalid branch name must not succeed")
	}
	// Valid create works, duplicate returns 400 with a clear message.
	if rec := c.do(http.MethodPost, "/api/branch/create", `{"name":"feat/api-new","hemisphere":"lower"}`); rec.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	rec = c.do(http.MethodPost, "/api/branch/create", `{"name":"feat/api-new","hemisphere":"lower"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "zaten mevcut") {
		t.Fatalf("duplicate create: %d %s", rec.Code, rec.Body)
	}

	// Promote with no remote/gh must fail with an explanatory error, not a raw CLI dump or 200.
	if rec := c.do(http.MethodPost, "/api/branch/promote", `{"branchId":"feat/api-new","title":"t","body":"b"}`); rec.Code == http.StatusOK {
		t.Fatal("promote of an empty branch must be refused")
	}

	// Conflict inspect on a clean branch must not 5xx.
	if rec := c.do(http.MethodPost, "/api/conflict/inspect", `{"branchId":"feat/api-new"}`); rec.Code >= 500 {
		t.Fatalf("inspect: %d %s", rec.Code, rec.Body)
	}

	// Discard and sync endpoints.
	if rec := c.do(http.MethodPost, "/api/working-tree/discard", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("discard: %d", rec.Code)
	}

	// Removing the live repo falls back to simulation.
	rec = c.do(http.MethodPost, "/api/repo/remove", `{"repoId":"live-custom"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/repo/remove", `{"repoId":"sim-sandbox"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("removing demo repo must be 400, got %d", rec.Code)
	}
}

func TestSimulationHandlers(t *testing.T) {
	c := newAPIClient(t, gitengine.NewManager(""))

	rec := c.do(http.MethodPost, "/api/repo/protection/plan", `{"profile":"temel"}`)
	var plan model.ProtectionPlan
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &plan) != nil || plan.Fingerprint == "" || len(plan.Changes) == 0 {
		t.Fatalf("protection plan: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/repo/protect", `{"profile":"temel"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("protect without confirm must be rejected: %d %s", rec.Code, rec.Body)
	}
	rec = c.do(http.MethodPost, "/api/repo/protect", `{"profile":"temel","confirm":"`+plan.Fingerprint+`"}`)
	var snap model.CockpitSnapshot
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &snap) != nil || !snap.Repo.BranchProtected {
		t.Fatalf("protect: %d %s", rec.Code, rec.Body)
	}

	rec = c.do(http.MethodPost, "/api/system/update-check", `{}`)
	var upd map[string]any
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &upd) != nil || upd["currentVersion"] != gitengine.AppVersion {
		t.Fatalf("update-check: %d %s", rec.Code, rec.Body)
	}

	if rec := c.do(http.MethodPost, "/api/simulation/reset", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("reset: %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/test/run", `{"cmd":"go test ./..."}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "output") {
		t.Fatalf("test/run: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/proposal/dismiss", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("dismiss: %d", rec.Code)
	}
	rec = c.do(http.MethodPost, "/api/working-tree/diff-stat", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("diff-stat: %d", rec.Code)
	}
}

func TestBaseBranchEndpoint(t *testing.T) {
	// Simülasyonda gerçek dal seçeneği yok: 400.
	sim := newAPIClient(t, gitengine.NewManager(""))
	if rec := sim.do(http.MethodPost, "/api/repo/base-branch", `{"branch":"develop"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("simulation must be refused with 400, got %d", rec.Code)
	}

	dir := tempRepo(t)
	if out, err := exec.Command("git", "-C", dir, "branch", "develop").CombinedOutput(); err != nil {
		t.Fatalf("git branch: %v %s", err, out)
	}
	c := newAPIClient(t, gitengine.NewManager(dir))
	if rec := c.do(http.MethodPost, "/api/repo/switch", `{"repoId":"live-custom"}`); rec.Code != http.StatusOK {
		t.Fatalf("switch: %d", rec.Code)
	}
	rec := c.do(http.MethodPost, "/api/repo/base-branch", `{"branch":"develop"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"defaultBranch":"develop"`) || !strings.Contains(rec.Body.String(), `"baseBranchPinned":true`) {
		t.Fatalf("pin: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/repo/base-branch", `{"branch":"yok"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown branch must be 400, got %d", rec.Code)
	}
	if rec := c.do(http.MethodGet, "/api/repo/base-branch", ``); rec.Code == http.StatusOK {
		t.Fatal("GET must not be accepted")
	}
	rec = c.do(http.MethodPost, "/api/repo/base-branch", `{"branch":""}`)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), `"baseBranchPinned":true`) {
		t.Fatalf("clear: %d %s", rec.Code, rec.Body)
	}
}

func TestNoProjectStateAPI(t *testing.T) {
	// Çalışma klasörü ve verilen yol depo değil: "proje yok" durumu (demo YOK).
	old, _ := os.Getwd()
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	c := newAPIClient(t, gitengine.NewManager(empty))
	rec := c.do(http.MethodGet, "/api/snapshot", ``)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"none"`) {
		t.Fatalf("snapshot must report mode none: %d %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "sim-sandbox") {
		t.Fatal("no demo repo may be offered")
	}
	// Sahte bir depoya işlem yapılmasın: mutasyonlar 409.
	for _, p := range []string{"/api/branch/create", "/api/branch/merge", "/api/spine/sync", "/api/branch/prune"} {
		if rec := c.do(http.MethodPost, p, `{}`); rec.Code != http.StatusConflict {
			t.Errorf("%s must be refused with 409 while there is no project, got %d", p, rec.Code)
		}
	}
	// Proje ekleme ve demo isteme çalışır / reddedilir.
	if rec := c.do(http.MethodPost, "/api/repo/switch", `{"repoId":"sim-sandbox"}`); rec.Code == http.StatusOK {
		t.Fatal("the demo must not be switchable by default")
	}
	repo := tempRepo(t)
	sub := filepath.Join(repo, "pkg")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	rec = c.do(http.MethodPost, "/api/repo/add", `{"path":"`+sub+`"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"mode":"live"`) {
		t.Fatalf("adding a project (even from a subfolder) must open it: %d %s", rec.Code, rec.Body)
	}
	if rec := c.do(http.MethodPost, "/api/spine/sync", `{}`); rec.Code == http.StatusConflict {
		t.Fatal("once a project is open the mutation guard must step aside")
	}
}

func TestSkillEndpointsListInstallAndGuardRules(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	c := newAPIClient(t, gitengine.NewManager(""))

	if rec := c.do(http.MethodPost, "/api/system/skills", `{}`); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("liste GET olmalı: %d", rec.Code)
	}
	rec := c.do(http.MethodGet, "/api/system/skills", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ci-fix"`) || !strings.Contains(rec.Body.String(), `"not_installed"`) {
		t.Fatalf("liste: %d %s", rec.Code, rec.Body.String())
	}
	if rec := c.do(http.MethodGet, "/api/system/skills/install", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("kurulum POST olmalı: %d", rec.Code)
	}
	// istemci yol gönderemez: ad listede yoksa reddedilir, hiçbir şey yazılmaz
	if rec := c.do(http.MethodPost, "/api/system/skills/install", `{"name":"../../evil"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("kötü ad: %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/system/skills/install", `{"name":"ci-fix"}`); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"up_to_date"`) {
		t.Fatalf("kurulum: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "ci-fix", "SKILL.md")); err != nil {
		t.Fatalf("dosya yazılmalı: %v", err)
	}
	// CSRF: X-Requested-With olmadan kurulum yapılamaz
	r := httptest.NewRequest(http.MethodPost, "http://"+c.host+"/api/system/skills/install", bytes.NewBufferString(`{"name":"ci-fix"}`))
	r.Host = c.host
	r.AddCookie(c.cookie)
	w := httptest.NewRecorder()
	c.h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("başlıksız POST reddedilmeli: %d", w.Code)
	}
}
