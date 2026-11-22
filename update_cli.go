package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"strings"

	"gitarc/internal/gitengine"
	"gitarc/internal/selfupdate"
)

// versionLine: `gitarc version` çıktısı. Kendini güncelleme, indirilen ikilinin sürümünü BUNUNLA doğrular ("gitarc X.Y.Z"); biçimi değiştirme.
func versionLine() string { return "gitarc " + gitengine.AppVersion }

// runUpdateCLI: `gitarc update [--check] [--yes]`. Çıkış kodu: 0 güncel/başarılı, 10 yeni sürüm var (--check), 1 hata/iptal.
// u nil ise gerçek GitHub'a bağlı güncelleyici kullanılır (testte sahte verilir).
func runUpdateCLI(args []string, in io.Reader, out, errOut io.Writer, u *selfupdate.Updater) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(errOut)
	check := fs.Bool("check", false, "yalnızca yeni sürüm var mı diye bak, kurma")
	yes := fs.Bool("yes", false, "onay sormadan kur")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if u == nil {
		u = &selfupdate.Updater{Version: gitengine.AppVersion}
	}
	u.Log = func(s string) { fmt.Fprintln(out, s) }
	fmt.Fprintf(out, "Mevcut sürüm: %s\n🔍 Son sürüm kontrol ediliyor (github.com/%s)...\n", u.Version, repoOf(u))
	res, err := u.Check()
	if err != nil {
		fmt.Fprintf(out, "⚠️  Denetlenemedi: %v\n", err)
		return 1
	}
	if !res.Available {
		fmt.Fprintf(out, "✅ Güncelsiniz (%s).\n", res.Current)
		return 0
	}
	fmt.Fprintf(out, "🆕 Yeni sürüm var: %s (mevcut: %s)\n", res.Latest, res.Current)
	if *check {
		return 10
	}
	if !res.Installable {
		fmt.Fprintf(out, "ℹ️  Otomatik kurulamıyor: %s\n   Elle: git pull --ff-only origin main && go build -o gitarc .\n", res.Note)
		return 1
	}
	if !*yes {
		fmt.Fprintf(errOut, "İndirilip imzası doğrulansın ve çalışan ikili (%s) değiştirilsin mi? (y/n) ", res.Asset)
		line, _ := bufio.NewReader(in).ReadString('\n')
		if a := strings.TrimSpace(line); a != "y" && a != "Y" {
			fmt.Fprintln(out, "İptal edildi.")
			return 1
		}
	}
	r, err := u.Apply(res.Latest)
	if err != nil {
		fmt.Fprintf(out, "❌ Güncelleme başarısız: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "\n✅ Güncellendi: %s → %s. GitArc'ı yeniden başlatın.\n", r.From, r.To)
	return 0
}

func repoOf(u *selfupdate.Updater) string {
	if u.Repo != "" {
		return u.Repo
	}
	return selfupdate.DefaultRepo
}
