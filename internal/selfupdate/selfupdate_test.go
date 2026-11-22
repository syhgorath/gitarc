package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const latestLoc = "https://github.com/syhgorath/gitarc/releases/tag/v0.11.1"

// world: sahte GitHub + ikili klasörü. Ağa hiç çıkılmaz.
type world struct {
	t        *testing.T
	dir      string
	exe      string
	u        *Updater
	assets   map[string][]byte // url → içerik
	reported string
}

func newWorld(t *testing.T) *world {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitarc")
	if err := os.WriteFile(exe, []byte("ESKI IKILI 0.11.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := &world{t: t, dir: dir, exe: exe, assets: map[string][]byte{}, reported: "gitarc 0.11.1"}
	base := "https://github.com/syhgorath/gitarc/releases/download/v0.11.1/gitarc-0.11.1-linux-amd64"
	w.assets[base] = fixture(t, "fake-binary")
	w.assets[base+".sig"] = fixture(t, "fake-binary.sig")
	w.u = &Updater{
		Version: "0.11.0", ExePath: exe, GOOS: "linux", GOARCH: "amd64", Signers: testSigners(t),
		Head: func(u string) (int, string) {
			switch {
			case u == "https://github.com/syhgorath/gitarc/releases/latest":
				return 302, latestLoc
			default:
				if _, ok := w.assets[u]; ok {
					return 200, ""
				}
				return 404, ""
			}
		},
		Download: func(u, dest string, max int64) error {
			b, ok := w.assets[u]
			if !ok {
				return errors.New("HTTP 404")
			}
			if int64(len(b)) > max {
				return errors.New("büyük")
			}
			return os.WriteFile(dest, b, 0o600)
		},
		RunVersion: func(string) (string, error) { return w.reported, nil },
	}
	return w
}

func (w *world) exeContent() string {
	b, _ := os.ReadFile(w.exe)
	return string(b)
}

func TestLatestOnlyAcceptsTheExactReleaseTagUrlOfThisRepo(t *testing.T) {
	w := newWorld(t)
	if v, ok := w.u.Latest(); !ok || v != "0.11.1" {
		t.Fatalf("0.11.1 beklenirdi: %q %v", v, ok)
	}
	for _, bad := range []string{"", "https://github.com/baska/repo/releases/tag/v0.11.1", "https://evil.example/syhgorath/gitarc/releases/tag/v0.11.1",
		"https://github.com/syhgorath/gitarc/releases/tag/surum-1", "https://github.com/syhgorath/gitarc/releases/tag/v0.11.1-rc1", "http://github.com/syhgorath/gitarc/releases/tag/v0.11.1"} {
		w.u.Head = func(string) (int, string) { return 302, bad }
		if v, ok := w.u.Latest(); ok {
			t.Errorf("%q kabul edilmemeli (sürüm %q)", bad, v)
		}
	}
}

func TestCheckReportsAvailableInstallableAndWhyNot(t *testing.T) {
	w := newWorld(t)
	r, err := w.u.Check()
	if err != nil || !r.Available || !r.Installable || r.Latest != "0.11.1" || r.Asset != "gitarc-0.11.1-linux-amd64" {
		t.Fatalf("yeni sürüm yüklenebilir olmalı: %+v %v", r, err)
	}
	// aynı sürüm → güncel
	w.u.Version = "0.11.1"
	if r, _ := w.u.Check(); r.Available {
		t.Fatalf("aynı sürümde güncelleme olmamalı: %+v", r)
	}
	// uzak sürüm ESKİ → güncelleme yok (geri sürüme düşülmez)
	w.u.Version = "0.12.0"
	if r, _ := w.u.Check(); r.Available {
		t.Fatalf("eski uzak sürüm güncelleme sayılmamalı: %+v", r)
	}
	w.u.Version = "0.11.0"
	// bu platform için dosya yok
	delete(w.assets, "https://github.com/syhgorath/gitarc/releases/download/v0.11.1/gitarc-0.11.1-linux-amd64")
	if r, _ := w.u.Check(); !r.Available || r.Installable || !strings.Contains(r.Note, "bulunamadı") {
		t.Fatalf("dosya yoksa Installable=false ve neden yazılmalı: %+v", r)
	}
	// imzacı yok → kapalı
	w2 := newWorld(t)
	w2.u.Signers = []Signer{} // boş ama nil değil
	if r, _ := w2.u.Check(); !r.Available || r.Installable || !strings.Contains(r.Note, "imza anahtarı") {
		t.Fatalf("imzacı yoksa kapalı olmalı: %+v", r)
	}
	// ağ yok → hata (asla "güncel" denmez)
	w3 := newWorld(t)
	w3.u.Head = func(string) (int, string) { return 0, "" }
	if _, err := w3.u.Check(); err == nil {
		t.Fatal("ağ yokken denetim hata vermeli")
	}
	// bozuk çalışan sürüm
	w3.u.Version = "dev"
	if _, err := w3.u.Check(); err == nil {
		t.Fatal("çalışan sürüm okunamıyorsa hata vermeli")
	}
}

func TestApplyInstallsAVerifiedBinaryAtomicallyWithABackup(t *testing.T) {
	w := newWorld(t)
	var logs []string
	w.u.Log = func(s string) { logs = append(logs, s) }
	res, err := w.u.Apply("0.11.1")
	if err != nil {
		t.Fatal(err)
	}
	if w.exeContent() != string(fixture(t, "fake-binary")) {
		t.Fatal("ikili yeni içerikle değişmeliydi")
	}
	if st, _ := os.Stat(w.exe); st.Mode()&0o111 == 0 {
		t.Fatal("yeni ikili çalıştırılabilir olmalı")
	}
	b, _ := os.ReadFile(res.Backup)
	if string(b) != "ESKI IKILI 0.11.0\n" || !strings.HasSuffix(res.Backup, "gitarc-0.11.0") {
		t.Fatalf("eski ikili yedeklenmeli: %q %s", b, res.Backup)
	}
	if res.From != "0.11.0" || res.To != "0.11.1" || len(logs) < 3 {
		t.Fatalf("sonuç/günlük: %+v %v", res, logs)
	}
	if entries, _ := filepath.Glob(filepath.Join(w.dir, ".gitarc.new.*")); len(entries) != 0 {
		t.Fatalf("geçici dosya kalmamalı: %v", entries)
	}
}

// Her reddedilen durumda çalışan ikili DEĞİŞMEZ ve yedek/yeni dosya bırakılmaz.
func TestApplyRefusesAndLeavesTheBinaryUntouched(t *testing.T) {
	base := "https://github.com/syhgorath/gitarc/releases/download/v0.11.1/gitarc-0.11.1-linux-amd64"
	cases := []struct {
		name    string
		mutate  func(w *world)
		version string
		want    string
	}{
		{"içerik değiştirilmiş", func(w *world) { b := append([]byte(nil), w.assets[base]...); b[0] ^= 1; w.assets[base] = b }, "0.11.1", "İMZA"},
		{"yanlış ad alanıyla imzalı", func(w *world) { w.assets[base+".sig"] = fixture(t, "fake-binary.wrongns.sig") }, "0.11.1", "İMZA"},
		{"tanınmayan anahtarla imzalı", func(w *world) { w.assets[base+".sig"] = fixture(t, "fake-binary.otherkey.sig") }, "0.11.1", "İMZA"},
		{"imzacı tanımlı değil", func(w *world) { w.u.Signers = []Signer{} }, "0.11.1", "imza anahtarı tanımlı değil"},
		{"ikili başka sürüm bildiriyor", func(w *world) { w.reported = "gitarc 0.11.2" }, "0.11.1", "uyuşmuyor"},
		{"ikili sürüm çalıştırılamıyor", func(w *world) {
			w.u.RunVersion = func(string) (string, error) { return "", errors.New("çalışmıyor") }
		}, "0.11.1", "uyuşmuyor"},
		{"imza dosyası yok", func(w *world) { delete(w.assets, base+".sig") }, "0.11.1", "imza dosyası"},
		{"ikili dosyası yok", func(w *world) { delete(w.assets, base) }, "0.11.1", "indirme"},
		{"aynı sürüm", func(w *world) {}, "0.11.0", "zaten güncel"},
		{"geri sürüme düşme", func(w *world) { w.u.Version = "0.12.0" }, "0.11.1", "geri sürüme"},
		{"bozuk sürüm biçimi", func(w *world) {}, "0.11", "biçimi"},
		{"go run ile çalışıyor", func(w *world) { w.u.ExePath = filepath.Join(w.dir, "go-build123", "exe", "gitarc") }, "0.11.1", "go run"},
	}
	for _, c := range cases {
		w := newWorld(t)
		c.mutate(w)
		before := w.exeContent()
		_, err := w.u.Apply(c.version)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %q içeren hata beklenirdi, geldi %v", c.name, c.want, err)
			continue
		}
		if w.u.ExePath == w.exe && w.exeContent() != before {
			t.Errorf("%s: ikili DEĞİŞMEMELİYDİ", c.name)
		}
		if _, err := os.Stat(filepath.Join(w.dir, ".update-backup")); err == nil {
			t.Errorf("%s: reddedilen güncellemede yedek klasörü oluşmamalı", c.name)
		}
	}
}

func TestApplyOnWindowsRenamesTheRunningExeAside(t *testing.T) {
	w := newWorld(t)
	w.u.GOOS = "windows"
	exe := filepath.Join(w.dir, "gitarc.exe")
	if err := os.Rename(w.exe, exe); err != nil {
		t.Fatal(err)
	}
	w.exe, w.u.ExePath = exe, exe
	base := "https://github.com/syhgorath/gitarc/releases/download/v0.11.1/gitarc-0.11.1-windows-amd64.exe"
	w.u.GOARCH = "amd64"
	w.assets[base] = fixture(t, "fake-binary")
	w.assets[base+".sig"] = fixture(t, "fake-binary.sig")
	if _, err := w.u.Apply("0.11.1"); err != nil {
		t.Fatal(err)
	}
	if w.exeContent() != string(fixture(t, "fake-binary")) {
		t.Fatal("yeni .exe yerinde olmalı")
	}
	if old, _ := os.ReadFile(exe + ".old"); string(old) != "ESKI IKILI 0.11.0\n" {
		t.Fatalf("eski çalışan .exe .old olarak durmalı: %q", old)
	}
	if got := w.u.AssetName("1.2.3"); got != "gitarc-1.2.3-windows-amd64.exe" {
		t.Fatalf("Windows dosya adı .exe ile bitmeli: %s", got)
	}
}

func TestVersionCmp(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{{"0.11.1", "0.11.0", 1}, {"0.11.0", "0.11.0", 0}, {"0.9.9", "0.10.0", -1}, {"1.0.0", "0.99.99", 1}, {"v0.2.0", "0.10.0", -1}}
	for _, c := range cases {
		if got := VersionCmp(c.a, c.b); got != c.want {
			t.Errorf("VersionCmp(%q,%q)=%d want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestHTTPSDownloadRefusesPlainHTTP(t *testing.T) {
	if err := HTTPSDownload("http://github.com/x/y/releases/download/v1/a", filepath.Join(t.TempDir(), "a"), 1<<20); err == nil {
		t.Fatal("düz http indirilmemeli")
	}
	if err := HTTPSDownload("file:///etc/passwd", filepath.Join(t.TempDir(), "a"), 1<<20); err == nil {
		t.Fatal("file:// indirilmemeli")
	}
}

func TestAssetNameUsesTheRunningPlatformByDefault(t *testing.T) {
	u := &Updater{}
	if n := u.AssetName("0.11.1"); !strings.HasPrefix(n, "gitarc-0.11.1-") {
		t.Fatalf("beklenmeyen ad: %s", n)
	}
}
