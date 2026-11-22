package gitengine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddedChangelogBulletsOnlyTakesAddedBulletLines(t *testing.T) {
	diff := strings.Join([]string{
		"diff --git a/CHANGELOG.md b/CHANGELOG.md", "--- a/CHANGELOG.md", "+++ b/CHANGELOG.md", "@@ -1,3 +1,5 @@",
		" ## [Yayımlanmamış]", "- eski madde (bağlam satırı, EKLENMEDİ)", "-- silinen", "+- yeni madde bir", "+düz satır", "+- yeni madde iki",
	}, "\n")
	got := addedChangelogBullets(diff, 8)
	if len(got) != 2 || got[0] != "- yeni madde bir" || got[1] != "- yeni madde iki" {
		t.Fatalf("yalnızca eklenen madde satırları beklenirdi: %v", got)
	}
	if len(addedChangelogBullets(diff, 1)) != 1 {
		t.Fatal("sınır uygulanmalı")
	}
}

func TestCommitTypeComesFromTheBranchNameNeverFromTheVersion(t *testing.T) {
	cases := map[string]string{"fix/x": "fix", "feat/a-b": "feat", "docs/y": "docs", "refactor/z": "refactor", "main": "chore", "wip/working-tree": "chore", "kullanici/x": "chore", "": "chore"}
	for in, want := range cases {
		if got := commitTypeFor(in, []string{"main.go"}); got != want {
			t.Errorf("commitTypeFor(%q)=%q want %q", in, got, want)
		}
	}
	// dal adı bir tür söylemiyorsa değişen dosyalar söyler
	byFiles := []struct {
		files []string
		want  string
	}{{[]string{"README.md", "docs/a.md"}, "docs"}, {[]string{"a_test.go", "tests/js/b.mjs"}, "test"}, {[]string{".github/workflows/ci.yml"}, "ci"}, {[]string{"README.md", "main.go"}, "chore"}, {nil, "chore"}}
	for _, c := range byFiles {
		if got := commitTypeFor("main", c.files); got != c.want {
			t.Errorf("commitTypeFor(main, %v)=%q want %q", c.files, got, c.want)
		}
	}
}

func TestComposeUsesOnlyReliableSourcesAndNeverInventsFeatVersion(t *testing.T) {
	files := []string{"internal/selfupdate/selfupdate.go", "CHANGELOG.md"}
	// 1) CHANGELOG'a eklenen madde varsa başlık ondan
	_, title, body := composeCommitAndPR("feat/selfupdate", files, changeSummary{
		bullets:  []string{"- **İmzalı kendini güncelleme altyapısı (`internal/selfupdate`):** GitArc'ın kendi Release'lerinden indirir."},
		subjects: []string{"feat: imzali guncelleme"},
	})
	if title != "feat: İmzalı kendini güncelleme altyapısı (`internal/selfupdate`)" {
		t.Fatalf("başlık CHANGELOG maddesinden: %q", title)
	}
	if !strings.Contains(body, "İmzalı kendini güncelleme") || strings.Contains(body, "MIT") {
		t.Fatalf("gövde bu değişikliğin maddesini içermeli: %s", body)
	}
	// 2) CHANGELOG yok, tek commit → onun başlığı (kendi sözleri)
	_, title, body = composeCommitAndPR("fix/x", files, changeSummary{subjects: []string{"fix: radar eski hata kartları"}})
	if title != "fix: radar eski hata kartları" || !strings.Contains(body, "- fix: radar eski hata kartları") {
		t.Fatalf("tek commit başlığı: %q / %s", title, body)
	}
	// 3) Conventional olmayan commit başlığına dal türü eklenir
	_, title, _ = composeCommitAndPR("docs/y", files, changeSummary{subjects: []string{"README düzeltmesi"}})
	if title != "docs: README düzeltmesi" {
		t.Fatalf("tür eklenmeli: %q", title)
	}
	// 4) Güvenilir kaynak yok → yalnızca dosya bilgisi; sürüm numarası ASLA yok
	commit, title, body := composeCommitAndPR("main", files, changeSummary{})
	if strings.Contains(title+commit+body, "v0.") || !strings.HasPrefix(title, "chore: ") || !strings.Contains(body, "`internal/selfupdate/selfupdate.go`") {
		t.Fatalf("yalnızca dosya bilgisi beklenirdi: %q | %s", title, body)
	}
	if commit != title {
		t.Fatal("commit mesajı başlıkla aynı olmalı")
	}
	// uzun madde başlığı kısaltılır
	long := "- " + strings.Repeat("çok uzun bir değişiklik açıklaması ", 6)
	if _, tt, _ := composeCommitAndPR("feat/x", nil, changeSummary{bullets: []string{long}}); len(tt) > 90 {
		t.Fatalf("başlık kısaltılmalı: %d", len(tt))
	}
}

// Gerçek hata: CHANGELOG'un en üstünde ESKİ maddeler dururken yeni dalın eklediği madde başka bölümdeydi; öneri eski maddelerden geliyordu.
func TestSuggestionDescribesThisBranchNotTheTopOfTheChangelog(t *testing.T) {
	dir, run := newTempRepo(t)
	changelog := "# Changelog\n\n## [Yayımlanmamış]\n\n### Belgeler\n- **MIT lisansı eklendi:** eski madde\n- **README rozetleri:** eski madde\n- **Sponsor düğmesi:** eski madde\n\n### Eklenenler\n- ilk eski eklenen\n"
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "docs: changelog")
	run("checkout", "-b", "feat/selfupdate")
	// yeni madde "Eklenenler" bölümüne (dosyanın üstünde DEĞİL)
	changelog2 := strings.Replace(changelog, "### Eklenenler\n", "### Eklenenler\n- **İmzalı kendini güncelleme altyapısı:** bu dalın gerçek maddesi\n", 1)
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte(changelog2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "selfupdate.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: imzali kendini guncelleme cekirdegi")
	sha := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))

	_, title, body := buildSuggestedCommitAndPR(context.Background(), dir, "feat/selfupdate", []string{"CHANGELOG.md", "selfupdate.go"}, "main", "feat/selfupdate", sha, false)
	if title != "feat: İmzalı kendini güncelleme altyapısı" {
		t.Fatalf("başlık bu dalın maddesinden olmalı (MIT eski maddesinden değil): %q", title)
	}
	if strings.Contains(body, "MIT") || strings.Contains(body, "Sponsor") || !strings.Contains(body, "bu dalın gerçek maddesi") {
		t.Fatalf("gövde yalnızca bu dalın eklediği maddeyi içermeli: %s", body)
	}
	// aynı çağrı önbellekten aynı sonucu verir
	_, title2, _ := buildSuggestedCommitAndPR(context.Background(), dir, "feat/selfupdate", nil, "main", "feat/selfupdate", sha, false)
	if title2 != title {
		t.Fatalf("önbellek tutarsız: %q", title2)
	}

	// CHANGELOG'a hiç dokunmayan dal → kendi commit başlığı
	run("checkout", "main")
	run("checkout", "-b", "fix/baska")
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("package b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "fix: baska bir duzeltme")
	sha2 := strings.TrimSpace(mustGit(t, dir, "rev-parse", "HEAD"))
	_, title3, body3 := buildSuggestedCommitAndPR(context.Background(), dir, "fix/baska", []string{"b.go"}, "main", "fix/baska", sha2, false)
	if title3 != "fix: baska bir duzeltme" || strings.Contains(body3, "MIT") {
		t.Fatalf("CHANGELOG'a dokunmayan dal kendi commit başlığını kullanmalı: %q | %s", title3, body3)
	}
}

// Çalışma ağacı: yalnızca KAYDEDİLMEMİŞ CHANGELOG eklemesi öneriye girer.
func TestWorkingTreeSuggestionUsesOnlyUncommittedChangelogAdditions(t *testing.T) {
	dir, run := newTempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("## [Yayımlanmamış]\n- **eski:** madde\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "docs: cl")
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG.md"), []byte("## [Yayımlanmamış]\n- **yeni:** kaydedilmemiş madde\n- **eski:** madde\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commit, title, body := buildSuggestedCommitAndPR(context.Background(), dir, "main", []string{"CHANGELOG.md"}, "", "", "", false)
	if title != "docs: yeni: kaydedilmemiş madde" || commit != title || strings.Contains(body, "eski") {
		t.Fatalf("yalnızca kaydedilmemiş madde: %q | %s", title, body)
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := runGitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return out
}
