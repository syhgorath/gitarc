package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitarc/internal/gitengine"
)

func TestSessionValid(t *testing.T) {
	app := newApp("127.0.0.1:7432", gitengine.NewManager(""))
	if app.sessionValid("") {
		t.Fatal("empty token must be invalid")
	}
	if app.sessionValid("unknown") {
		t.Fatal("unknown token must be invalid")
	}
	tok := app.newSession()
	if !app.sessionValid(tok) {
		t.Fatal("fresh token must be valid")
	}
	app.sessMu.Lock()
	app.sessions[tok] = time.Now().Add(-time.Minute)
	app.sessMu.Unlock()
	if app.sessionValid(tok) {
		t.Fatal("expired token must be invalid")
	}
	app.sessMu.RLock()
	_, still := app.sessions[tok]
	app.sessMu.RUnlock()
	if still {
		t.Fatal("expired token must be evicted")
	}
}

func TestDecodePOST(t *testing.T) {
	var dst struct {
		Name string `json:"name"`
	}

	rec := httptest.NewRecorder()
	if decodePOST(rec, httptest.NewRequest(http.MethodGet, "/x", nil), &dst) || rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET must be rejected with 405, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if decodePOST(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{bad")), &dst) || rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid JSON must give 400, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	if !decodePOST(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"name":"ok"}`)), &dst) || dst.Name != "ok" {
		t.Fatalf("valid JSON must decode, got %+v", dst)
	}

	// Bodies over 1 MB are rejected.
	big := `{"name":"` + strings.Repeat("a", 1<<20+10) + `"}`
	rec = httptest.NewRecorder()
	if decodePOST(rec, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(big)), &dst) {
		t.Fatal("oversized body must be rejected")
	}
}

func TestGuardRejectsForeignOriginAndBadToken(t *testing.T) {
	host := "127.0.0.1:7432"
	app := newApp(host, gitengine.NewManager(""))
	h := app.routes()

	// Wrong ?t= token must not issue a session cookie.
	req := httptest.NewRequest(http.MethodGet, "http://"+host+"/?t=wrong", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("wrong token must not set a cookie")
	}

	// Valid session but foreign Origin on POST is forbidden.
	req = httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	req.Host = host
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}
	if !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie must be HttpOnly+SameSite=Strict: %+v", cookies[0])
	}

	post := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/logs/clear", bytes.NewBufferString(`{}`))
	post.Host = host
	post.Header.Set("X-Requested-With", "gitarc")
	post.Header.Set("Origin", "http://evil.example")
	post.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, post)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign Origin must be 403, got %d", rec.Code)
	}
}

func TestAPIEndpointsInSimulation(t *testing.T) {
	host := "127.0.0.1:7432"
	app := newApp(host, gitengine.NewManager(""))
	h := app.routes()

	root := httptest.NewRequest(http.MethodGet, "http://"+host+"/", nil)
	root.Host = host
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, root)
	cookie := rr.Result().Cookies()[0]

	post := func(path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, "http://"+host+path, bytes.NewBufferString(body))
		r.Host = host
		r.Header.Set("X-Requested-With", "gitarc")
		r.Header.Set("Origin", "http://"+host)
		r.AddCookie(cookie)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	for _, p := range []string{"/api/logs/clear", "/api/spine/sync", "/api/simulation/reset", "/api/system/update-check"} {
		if rec := post(p, `{}`); rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d: %s", p, rec.Code, rec.Body.String())
		}
	}
	// Invalid branch names must be rejected, never reach git.
	if rec := post("/api/branch/create", `{"name":"bad name; rm -rf /","hemisphere":"lower"}`); rec.Code == http.StatusOK {
		t.Errorf("invalid branch name must not succeed")
	}
	// Malformed JSON gives 400.
	if rec := post("/api/branch/create", `{`); rec.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: expected 400, got %d", rec.Code)
	}
}
