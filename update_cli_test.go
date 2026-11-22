package main

import (
	"bytes"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
	"gitarc/internal/selfupdate"
)

func fixtureBytes(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("internal", "selfupdate", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// cliWorld: sahte GitHub + ikili; ağa çıkılmaz.
func cliWorld(t *testing.T, latestVer string) (*selfupdate.Updater, string) {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitarc")
	if err := os.WriteFile(exe, []byte("ESKI\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := "https://github.com/syhgorath/gitarc/releases/download/v" + latestVer + "/gitarc-" + latestVer + "-linux-amd64"
	assets := map[string][]byte{base: fixtureBytes(t, "fake-binary"), base + ".sig": fixtureBytes(t, "fake-binary.sig")}
	signers := selfupdate.ParseAllowedSigners(string(fixtureBytes(t, "signers")))
	u := &selfupdate.Updater{
		Version: gitengine.AppVersion, ExePath: exe, GOOS: "linux", GOARCH: "amd64", Signers: signers,
		Head: func(u string) (int, string) {
			if strings.HasSuffix(u, "/releases/latest") {
				return http.StatusFound, "https://github.com/syhgorath/gitarc/releases/tag/v" + latestVer
			}
			if _, ok := assets[u]; ok {
				return 200, ""
			}
			return 404, ""
		},
		Download: func(u, dest string, max int64) error {
			b, ok := assets[u]
			if !ok {
				return errors.New("HTTP 404")
			}
			return os.WriteFile(dest, b, 0o600)
		},
		RunVersion: func(string) (string, error) { return "gitarc " + latestVer, nil },
	}
	return u, exe
}

func runCLI(t *testing.T, u *selfupdate.Updater, stdin string, args ...string) (code int, out string) {
	t.Helper()
	var o, e bytes.Buffer
	code = runUpdateCLI(args, strings.NewReader(stdin), &o, &e, u)
	return code, o.String() + e.String()
}

func TestVersionLineIsExactlyWhatSelfUpdateVerifies(t *testing.T) {
	if got := versionLine(); got != "gitarc "+gitengine.AppVersion {
		t.Fatalf("`gitarc version` çıktısı `gitarc X.Y.Z` olmalı (kendini güncelleme bununla doğrular): %q", got)
	}
}

func TestUpdateCLICheckReportsAvailableUpToDateAndUnreachable(t *testing.T) {
	u, exe := cliWorld(t, "99.0.0")
	if code, out := runCLI(t, u, "", "--check"); code != 10 || !strings.Contains(out, "Yeni sürüm var: 99.0.0") {
		t.Fatalf("yeni sürümde çıkış 10 beklenirdi: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); string(b) != "ESKI\n" {
		t.Fatal("--check hiçbir şeyi değiştirmemeli")
	}
	same, _ := cliWorld(t, gitengine.AppVersion)
	if code, out := runCLI(t, same, "", "--check"); code != 0 || !strings.Contains(out, "Güncelsiniz") {
		t.Fatalf("aynı sürümde çıkış 0: %d %s", code, out)
	}
	u.Head = func(string) (int, string) { return 0, "" }
	if code, out := runCLI(t, u, "", "--check"); code != 1 || !strings.Contains(out, "Denetlenemedi") {
		t.Fatalf("ağ yokken çıkış 1 ve 'denetlenemedi' (asla güncel değil): %d %s", code, out)
	}
}

func TestUpdateCLIInstallsOnlyAfterConfirmationAndOnlyVerified(t *testing.T) {
	// onay verilmezse (n) hiçbir şey değişmez
	u, exe := cliWorld(t, "99.0.0")
	if code, out := runCLI(t, u, "n\n"); code != 1 || !strings.Contains(out, "İptal") {
		t.Fatalf("onaysız iptal: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); string(b) != "ESKI\n" {
		t.Fatal("iptalde ikili değişmemeli")
	}
	// onay (y) → kurulur
	if code, out := runCLI(t, u, "y\n"); code != 0 || !strings.Contains(out, "Güncellendi") {
		t.Fatalf("onayla kurulmalı: %d %s", code, out)
	}
	if b, _ := os.ReadFile(exe); string(b) != string(fixtureBytes(t, "fake-binary")) {
		t.Fatal("ikili yeni içerikle değişmeliydi")
	}
	// --yes onay sormaz; imza UYUŞMAZSA kurulmaz
	bad, badExe := cliWorld(t, "99.0.0")
	bad.Download = func(u, dest string, max int64) error {
		if strings.HasSuffix(u, ".sig") {
			return os.WriteFile(dest, fixtureBytes(t, "fake-binary.sig"), 0o600)
		}
		return os.WriteFile(dest, []byte("DEĞİŞTİRİLMİŞ"), 0o600)
	}
	if code, out := runCLI(t, bad, "", "--yes"); code != 1 || !strings.Contains(out, "İMZA") {
		t.Fatalf("imza uyuşmazsa reddedilmeli: %d %s", code, out)
	}
	if b, _ := os.ReadFile(badExe); string(b) != "ESKI\n" {
		t.Fatal("reddedilen güncellemede ikili değişmemeli")
	}
	// imzacı tanımlı değil → kurulamaz, elle komut önerilir
	none, _ := cliWorld(t, "99.0.0")
	none.Signers = []selfupdate.Signer{}
	if code, out := runCLI(t, none, "", "--yes"); code != 1 || !strings.Contains(out, "Otomatik kurulamıyor") || !strings.Contains(out, "git pull") {
		t.Fatalf("imzacı yoksa elle yol önerilmeli: %d %s", code, out)
	}
}

// HTTP uç noktası: yalnızca POST, geçerli JSON ve sürüm; takım modunda kapalı. Gerçek indirme yok.
func TestUpdateApplyEndpointValidatesAndRefusesInTeamMode(t *testing.T) {
	mgr := gitengine.NewManager("")
	c := newAPIClient(t, mgr)
	if rec := c.do(http.MethodGet, "/api/system/update-apply", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET reddedilmeli: %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/system/update-apply", "{bozuk"); rec.Code != http.StatusBadRequest {
		t.Fatalf("bozuk JSON: %d", rec.Code)
	}
	// ağa çıkmayan güncelleyici: sürüm biçimi geçersiz / indirme yok → 400, hiçbir şey kurulmaz
	for _, body := range []string{`{"version":""}`, `{"version":"x.y.z"}`, `{"version":"99.0.0"}`} {
		if rec := c.do(http.MethodPost, "/api/system/update-apply", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: 400 beklenirdi, %d %s", body, rec.Code, rec.Body)
		}
	}
	mgr.SetTeamMode(true)
	rec := c.do(http.MethodPost, "/api/system/update-apply", `{"version":"99.0.0"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "takım") {
		t.Fatalf("takım modunda kapalı olmalı: %d %s", rec.Code, rec.Body)
	}
}

func TestUpdateApplyIsAllowedWithoutAProjectAndNeedsNoRepoBinding(t *testing.T) {
	if !noProjectAllowed["/api/system/update-apply"] || !repoBindingExempt("/api/system/update-apply") {
		t.Fatal("güncelleme projeden bağımsızdır: proje yokken ve başka projeyi gösteren sekmeden de çalışmalı")
	}
}
