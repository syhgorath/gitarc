package selfupdate

import (
	_ "embed"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultRepo: GitArc'ın kendi deposu (sürüm denetimi ve indirme yalnızca buradan).
	DefaultRepo = "syhgorath/gitarc"
	Namespace   = "gitarc-release"
	Principal   = "gitarc-release"
	maxBinary   = 200 << 20 // indirilen ikilinin üst sınırı
	maxSig      = 4096
)

//go:embed release_signers
var embeddedSigners string

// EmbeddedSigners: ikiliye gömülü imzacılar (anahtar yoksa boş liste: güncelleme kapalı).
func EmbeddedSigners() []Signer { return ParseAllowedSigners(embeddedSigners) }

var (
	reVer = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	reTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
)

// VersionCmp: -1 (a<b), 0, 1 (a>b); "X.Y.Z" biçimi.
func VersionCmp(a, b string) int {
	pa, pb := splitVer(a), splitVer(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func splitVer(v string) [3]int {
	var out [3]int
	for i, p := range strings.SplitN(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

// DownloadFunc: url → dest dosyası (en fazla max bayt).
type DownloadFunc func(rawURL, dest string, max int64) error

// HTTPSDownload: yalnızca HTTPS; yönlendirmeler de HTTPS olmalı (GitHub, release dosyalarını başka bir HTTPS ana makineye yönlendirir).
func HTTPSDownload(rawURL, dest string, max int64) error {
	if u, err := url.Parse(rawURL); err != nil || u.Scheme != "https" {
		return fmt.Errorf("yalnızca https indirilir: %s", rawURL)
	}
	client := &http.Client{
		Timeout: 10 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 5 {
				return errors.New("çok fazla yönlendirme")
			}
			if req.URL.Scheme != "https" {
				return errors.New("https olmayan yönlendirme reddedildi")
			}
			return nil
		},
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("indirme başarısız: HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > max {
		return fmt.Errorf("dosya çok büyük (%d bayt)", resp.ContentLength)
	}
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	n, cerr := io.Copy(f, io.LimitReader(resp.Body, max+1))
	if e := f.Close(); cerr == nil {
		cerr = e
	}
	if cerr != nil {
		return cerr
	}
	if n > max {
		return errors.New("dosya sınırı aştı")
	}
	return nil
}

// httpHead: yönlendirmeyi TAKİP ETMEDEN ilk yanıtın durumu ve Location başlığı.
func httpHead(rawURL string) (status int, location string) {
	client := &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Head(rawURL)
	if err != nil {
		return 0, ""
	}
	resp.Body.Close()
	return resp.StatusCode, resp.Header.Get("Location")
}

// Updater: bağımlılıklar testte değiştirilir; sıfır değerler gerçek davranışı verir.
type Updater struct {
	Repo         string
	Signers      []Signer // nil = gömülü imzacılar
	ExePath      string   // güncellenecek ikili; boş = çalışan ikili
	BackupDir    string   // boş = <ikilinin klasörü>/.update-backup
	Version      string   // çalışan sürüm
	GOOS, GOARCH string
	Download     DownloadFunc
	Head         func(url string) (status int, location string) // nil = ağ
	RunVersion   func(path string) (string, error)              // yeni ikilinin bildirdiği sürüm; nil = `path version`
	Log          func(msg string)                               // ilerleme (isteğe bağlı)
}

func (u *Updater) logf(f string, a ...any) {
	if u.Log != nil {
		u.Log(fmt.Sprintf(f, a...))
	}
}

func (u *Updater) repo() string {
	if u.Repo != "" {
		return u.Repo
	}
	return DefaultRepo
}

func (u *Updater) head(rawURL string) (int, string) {
	if u.Head != nil {
		return u.Head(rawURL)
	}
	return httpHead(rawURL)
}

func (u *Updater) platform() (goos, arch string) {
	goos, arch = u.GOOS, u.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if arch == "" {
		arch = runtime.GOARCH
	}
	return goos, arch
}

// AssetName: bu platform için yayın dosyasının adı: gitarc-<sürüm>-<os>-<mimari>[.exe].
func (u *Updater) AssetName(ver string) string {
	goos, arch := u.platform()
	name := fmt.Sprintf("gitarc-%s-%s-%s", ver, goos, arch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

func (u *Updater) releaseBase(ver string) string {
	return "https://github.com/" + u.repo() + "/releases/download/v" + ver + "/"
}

// Latest: GitHub'ın `releases/latest` yönlendirmesinden en yeni YAYINLANMIŞ sürüm; yönlendirme tam olarak bu reponun etiket adresi olmalı.
func (u *Updater) Latest() (string, bool) {
	_, loc := u.head("https://github.com/" + u.repo() + "/releases/latest")
	if loc == "" {
		return "", false
	}
	tag := loc[strings.LastIndex(loc, "/")+1:]
	if !reTag.MatchString(tag) || loc != "https://github.com/"+u.repo()+"/releases/tag/"+tag {
		return "", false
	}
	return tag[1:], true
}

// CheckResult: sürüm denetiminin sonucu. Available: uzak sürüm daha yeni. Installable: bu platform için imzalı dosya var ve imzacı tanımlı.
type CheckResult struct {
	Current     string `json:"current"`
	Latest      string `json:"latest"`
	Available   bool   `json:"available"`
	Installable bool   `json:"installable"`
	Asset       string `json:"asset,omitempty"`
	Note        string `json:"note,omitempty"` // yüklenemiyorsa nedeni
}

// Check: salt okunur denetim. Ağ yoksa/yayınlanmış sürüm yoksa hata döner (arayüz "denetlenemedi" der, asla "güncel" demez).
func (u *Updater) Check() (CheckResult, error) {
	res := CheckResult{Current: u.Version}
	if !reVer.MatchString(u.Version) {
		return res, fmt.Errorf("çalışan sürüm okunamadı (%q)", u.Version)
	}
	latest, ok := u.Latest()
	if !ok {
		return res, errors.New("son sürüm öğrenilemedi (ağ yok ya da yayınlanmış sürüm yok)")
	}
	res.Latest = latest
	cmp := VersionCmp(latest, u.Version)
	res.Available = cmp > 0
	if !res.Available {
		return res, nil
	}
	res.Asset = u.AssetName(latest)
	signers := u.Signers
	if signers == nil {
		signers = EmbeddedSigners()
	}
	switch {
	case len(signers) == 0:
		res.Note = "bu sürümde imza anahtarı tanımlı değil: otomatik güncelleme kapalı"
	default:
		st, _ := u.head(u.releaseBase(latest) + res.Asset)
		if st == 0 || st >= 400 {
			res.Note = fmt.Sprintf("bu platform (%s) için imzalı dosya bulunamadı", res.Asset)
		} else {
			res.Installable = true
		}
	}
	return res, nil
}

func (u *Updater) runVersion(path string) (string, error) {
	if u.RunVersion != nil {
		return u.RunVersion(path)
	}
	o, err := exec.Command(path, "version").Output()
	return strings.TrimSpace(string(o)), err
}

// ApplyResult: yerleştirme sonucu.
type ApplyResult struct {
	From, To string
	Backup   string
}

// Apply: `latest` sürümünü indirir, İMZASINI doğrular, ikilinin bildirdiği sürümü denetler ve çalışan ikiliyi ATOMİK değiştirir.
// Herhangi bir adım başarısızsa hiçbir şey değişmez. Yeniden başlatma çağıranındır.
func (u *Updater) Apply(latest string) (ApplyResult, error) {
	cur := u.Version
	res := ApplyResult{From: cur, To: latest}
	if !reVer.MatchString(cur) || !reVer.MatchString(latest) {
		return res, errors.New("sürüm biçimi geçersiz")
	}
	switch VersionCmp(latest, cur) {
	case 0:
		return res, fmt.Errorf("zaten güncel (%s)", cur)
	case -1:
		return res, fmt.Errorf("uzaktaki sürüm (%s) kurulu olandan eski; geri sürüme düşülmez", latest)
	}
	signers := u.Signers
	if signers == nil {
		signers = EmbeddedSigners()
	}
	if len(signers) == 0 {
		return res, errors.New("imza anahtarı tanımlı değil: otomatik güncelleme kapalı (paket KURULMADI)")
	}
	exe := u.ExePath
	if exe == "" {
		p, err := os.Executable()
		if err != nil {
			return res, fmt.Errorf("çalışan ikili bulunamadı: %v", err)
		}
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		exe = p
	}
	if strings.Contains(filepath.ToSlash(exe), "/go-build") {
		return res, errors.New("`go run` ile çalışıyorsunuz: kendini güncelleme yalnızca derlenmiş ikilide çalışır")
	}
	work, err := os.MkdirTemp("", "gitarc-update.")
	if err != nil {
		return res, errors.New("geçici klasör oluşturulamadı")
	}
	defer os.RemoveAll(work)
	dl := u.Download
	if dl == nil {
		dl = HTTPSDownload
	}
	asset := u.AssetName(latest)
	bin, sig := filepath.Join(work, "gitarc.new"), filepath.Join(work, "gitarc.new.sig")
	u.logf("⬇️  İndiriliyor: %s", asset)
	if err := dl(u.releaseBase(latest)+asset, bin, maxBinary); err != nil {
		return res, fmt.Errorf("indirme başarısız: %v", err)
	}
	if err := dl(u.releaseBase(latest)+asset+".sig", sig, maxSig); err != nil {
		return res, fmt.Errorf("imza dosyası indirilemedi: %v", err)
	}
	u.logf("🔐 İmza doğrulanıyor...")
	sigRaw, rerr := os.ReadFile(sig)
	f, oerr := os.Open(bin)
	if rerr != nil || oerr != nil {
		if f != nil {
			f.Close()
		}
		return res, errors.New("indirilen dosyalar okunamadı")
	}
	verr := Verify(signers, Principal, Namespace, sigRaw, f)
	f.Close()
	if verr != nil {
		return res, errors.New("İMZA DOĞRULANAMADI: paket KURULMADI (sahte ya da bozuk paket olabilir)")
	}
	u.logf("✅ İmza geçerli")
	if err := os.Chmod(bin, 0o755); err != nil {
		return res, errors.New("dosya izni ayarlanamadı")
	}
	reported, verr2 := u.runVersion(bin)
	if verr2 != nil || reported != "gitarc "+latest {
		return res, fmt.Errorf("ikilinin bildirdiği sürüm (%s) istenenle (%s) uyuşmuyor: kurulmadı", strings.TrimPrefix(reported, "gitarc "), latest)
	}
	backup, err := u.apply(exe, bin, cur)
	if err != nil {
		return res, err
	}
	res.Backup = backup
	u.logf("💾 Yedek: %s", backup)
	return res, nil
}

// apply: eski ikiliyi yedekler, yenisini atomik yerleştirir; hata olursa eskisini geri koyar. Windows'ta çalışan .exe üzerine yazılamaz ama
// yeniden adlandırılabilir: önce eski `.old` yapılır, sonra yenisi yerine konur.
func (u *Updater) apply(exe, newBin, oldVer string) (string, error) {
	dir := filepath.Dir(exe)
	tmp, err := os.CreateTemp(dir, ".gitarc.new.")
	if err != nil {
		return "", fmt.Errorf("hedef klasör yazılabilir değil: %s", dir)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	src, err := os.Open(newBin)
	if err != nil {
		tmp.Close()
		return "", err
	}
	_, cerr := io.Copy(tmp, src)
	src.Close()
	if e := tmp.Close(); cerr == nil {
		cerr = e
	}
	if cerr != nil {
		return "", errors.New("yeni ikili yazılamadı")
	}
	if err := os.Chmod(tmpName, 0o755); err != nil {
		return "", err
	}
	bdir := u.BackupDir
	if bdir == "" {
		bdir = filepath.Join(dir, ".update-backup")
	}
	if err := os.MkdirAll(bdir, 0o700); err != nil {
		return "", errors.New("yedek klasörü oluşturulamadı")
	}
	backup := filepath.Join(bdir, "gitarc-"+oldVer)
	if err := copyFile(exe, backup, 0o755); err != nil {
		return "", errors.New("yedek alınamadı")
	}
	goos, _ := u.platform()
	if goos == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			return "", fmt.Errorf("çalışan ikili yeniden adlandırılamadı: %v", err)
		}
		if err := os.Rename(tmpName, exe); err != nil {
			_ = os.Rename(old, exe) // geri koy
			return "", fmt.Errorf("ikili yerleştirilemedi: %v", err)
		}
		return backup, nil
	}
	if err := os.Rename(tmpName, exe); err != nil {
		return "", fmt.Errorf("ikili yerleştirilemedi: %v", err)
	}
	return backup, nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
