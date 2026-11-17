package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

func TestWebAssetsStrictCSPCompliance(t *testing.T) {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		t.Fatal(err)
	}
	err = fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".html") && !strings.HasSuffix(path, ".js") {
			return nil
		}
		raw, err := fs.ReadFile(sub, path)
		if err != nil {
			return err
		}
		txt := string(raw)
		if strings.Contains(txt, `style="`) || strings.Contains(txt, `style='`) {
			t.Errorf("%s contains inline style attribute forbidden by style-src-attr 'none'", path)
		}
		if strings.HasSuffix(path, ".html") {
			cleaned := strings.ReplaceAll(txt, "http://www.w3.org/2000/svg", "")
			if strings.Contains(cleaned, "https://") || strings.Contains(cleaned, "http://") {
				t.Errorf("%s references external URL", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSecurityGuardsAndAPIFlow(t *testing.T) {
	host := "127.0.0.1:7432"
	mgr := gitengine.NewManager("")
	app := newApp(host, mgr)
	h := app.routes()

	// 1. DNS Rebinding protection rejects unknown Host
	reqBadHost := httptest.NewRequest(http.MethodGet, "http://evil.local/api/snapshot", nil)
	reqBadHost.Host = "evil.local"
	recBadHost := httptest.NewRecorder()
	h.ServeHTTP(recBadHost, reqBadHost)
	if recBadHost.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for bad host, got %d", recBadHost.Code)
	}

	// 2. Unauthenticated API request returns 401
	reqUnauth := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/snapshot", nil)
	reqUnauth.Host = host
	recUnauth := httptest.NewRecorder()
	h.ServeHTTP(recUnauth, reqUnauth)
	if recUnauth.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without session cookie, got %d", recUnauth.Code)
	}

	// 3. Initial GET / sets HttpOnly session cookie and CSP headers
	reqRoot := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	reqRoot.Host = host
	recRoot := httptest.NewRecorder()
	h.ServeHTTP(recRoot, reqRoot)
	if recRoot.Code != http.StatusOK {
		t.Fatalf("expected 200 on /, got %d", recRoot.Code)
	}
	if csp := recRoot.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "style-src-attr 'none'") {
		t.Fatalf("missing strict CSP header: %q", csp)
	}
	var sessionCookie *http.Cookie
	for _, c := range recRoot.Result().Cookies() {
		if c.Name == cookieName {
			sessionCookie = c
		}
	}
	if sessionCookie == nil {
		t.Fatalf("expected %s cookie on root request", cookieName)
	}

	// 4. POST without X-Requested-With header is rejected (CSRF guard)
	reqNoCSRF := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/branch/create", bytes.NewBufferString(`{"name":"feat/x"}`))
	reqNoCSRF.Host = host
	reqNoCSRF.AddCookie(sessionCookie)
	recNoCSRF := httptest.NewRecorder()
	h.ServeHTTP(recNoCSRF, reqNoCSRF)
	if recNoCSRF.Code != http.StatusForbidden {
		t.Fatalf("expected 403 without X-Requested-With, got %d", recNoCSRF.Code)
	}

	// 5. Authenticated GET /api/snapshot succeeds
	reqSnap := httptest.NewRequest(http.MethodGet, "http://"+host+"/api/snapshot", nil)
	reqSnap.Host = host
	reqSnap.AddCookie(sessionCookie)
	recSnap := httptest.NewRecorder()
	h.ServeHTTP(recSnap, reqSnap)
	if recSnap.Code != http.StatusOK {
		t.Fatalf("expected 200 on /api/snapshot, got %d: %s", recSnap.Code, recSnap.Body.String())
	}
	var snap model.CockpitSnapshot
	if err := json.Unmarshal(recSnap.Body.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode snapshot: %v", err)
	}
	if len(snap.Nodes) == 0 || len(snap.Branches) == 0 {
		t.Fatalf("expected populated snapshot")
	}

	// 6. Auto-Resolve + Merge over HTTP API
	doPost := func(path, payload string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://"+host+path, bytes.NewBufferString(payload))
		req.Host = host
		req.Header.Set("X-Requested-With", "gitarc")
		req.Header.Set("Origin", "http://"+host)
		req.AddCookie(sessionCookie)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	if rec := doPost("/api/branch/checkout", `{"branchId":"br-local-wip"}`); rec.Code != http.StatusOK {
		t.Fatalf("checkout failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/commit/amend", `{"branchId":"br-local-wip","message":"wip: updated commit message"}`); rec.Code != http.StatusOK {
		t.Fatalf("commit amend failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/pr/edit", `{"branchId":"br-open-421","title":"feat(audit): refined title","body":"Body text"}`); rec.Code != http.StatusOK {
		t.Fatalf("pr edit failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/branch/resolve", `{"branchId":"br-open-421"}`); rec.Code != http.StatusOK {
		t.Fatalf("resolve failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/branch/merge", `{"branchId":"br-open-421","strategy":"squash"}`); rec.Code != http.StatusOK {
		t.Fatalf("merge after resolve failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/branch/fix-ci", `{"branchId":"br-open-422"}`); rec.Code != http.StatusOK {
		t.Fatalf("fix-ci failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/branch/delete", `{"branchId":"br-local-exp"}`); rec.Code != http.StatusOK {
		t.Fatalf("delete failed: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doPost("/api/branch/prune", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("prune failed: %d %s", rec.Code, rec.Body.String())
	}

	// 7. Switch to Enterprise Simulation
	recEnt := doPost("/api/repo/switch", `{"repoId":"sim-enterprise"}`)
	if recEnt.Code != http.StatusOK {
		t.Fatalf("switch to sim-enterprise failed: %d %s", recEnt.Code, recEnt.Body.String())
	}
	var entSnap model.CockpitSnapshot
	if err := json.Unmarshal(recEnt.Body.Bytes(), &entSnap); err != nil {
		t.Fatalf("failed to decode enterprise snapshot: %v", err)
	}
	if entSnap.Repo.ID != "sim-enterprise" {
		t.Fatalf("expected active repo sim-enterprise, got %s", entSnap.Repo.ID)
	}
	if len(entSnap.Branches) < 10 {
		t.Fatalf("expected enterprise branches >= 10, got %d", len(entSnap.Branches))
	}

	// 8. Discard working tree in simulation
	if rec := doPost("/api/working-tree/discard", `{}`); rec.Code != http.StatusOK {
		t.Fatalf("discard working tree failed: %d %s", rec.Code, rec.Body.String())
	}

	// 9. Add project validation (reject empty path)
	if rec := doPost("/api/repo/add", `{"path":""}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty path, got %d", rec.Code)
	}
}
