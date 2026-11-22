package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/skills"
)

func skillHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

func ciFix(t *testing.T) skills.Skill {
	t.Helper()
	s, ok := skills.Get("ci-fix")
	if !ok {
		t.Fatal("ci-fix gömülü değil")
	}
	return s
}

func TestInstallSkillWritesTheEmbeddedFileThenReportsUpToDate(t *testing.T) {
	home := skillHome(t)
	m := NewManager("")
	st := m.SkillsStatus()
	if len(st) == 0 || st[0].Status != "not_installed" || st[0].Path != filepath.Join(home, ".claude", "skills", "ci-fix", "SKILL.md") {
		t.Fatalf("başlangıç durumu: %+v", st)
	}
	res, err := m.InstallSkill("ci-fix", false)
	if err != nil || res.Status != "up_to_date" {
		t.Fatalf("kurulum: %+v %v", res, err)
	}
	b, err := os.ReadFile(res.Path)
	if err != nil || string(b) != ciFix(t).Content {
		t.Fatalf("yazılan içerik gömülüyle aynı olmalı: %v", err)
	}
	if fi, _ := os.Stat(res.Path); fi.Mode().Perm() != 0o644 {
		t.Fatalf("izin 0644 olmalı: %v", fi.Mode().Perm())
	}
	if ents, _ := os.ReadDir(filepath.Dir(res.Path)); len(ents) != 1 {
		t.Fatalf("geçici dosya kalmamalı: %v", ents)
	}
	// tekrar kurmak sessiz başarı (zaten güncel)
	if again, err := m.InstallSkill("ci-fix", false); err != nil || again.Status != "up_to_date" {
		t.Fatalf("ikinci kurulum: %+v %v", again, err)
	}
}

func TestInstallSkillNeverOverwritesADifferentFileWithoutConsent(t *testing.T) {
	skillHome(t)
	m := NewManager("")
	p, _ := claudeSkillPath("ci-fix")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("benim kendi skill'im"), 0o644); err != nil {
		t.Fatal(err)
	}
	if st := m.SkillsStatus()[0]; st.Status != "different" {
		t.Fatalf("durum different olmalı: %+v", st)
	}
	if _, err := m.InstallSkill("ci-fix", false); err == nil || !strings.Contains(err.Error(), "onay") {
		t.Fatalf("onaysız üzerine yazılmamalı: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "benim kendi skill'im" {
		t.Fatal("reddedilen kurulum dosyaya dokunmamalı")
	}
	if res, err := m.InstallSkill("ci-fix", true); err != nil || res.Status != "up_to_date" {
		t.Fatalf("onayla güncelleme: %+v %v", res, err)
	}
	if b, _ := os.ReadFile(p); string(b) != ciFix(t).Content {
		t.Fatal("onaydan sonra gömülü içerik yazılmalı")
	}
}

func TestInstallSkillRefusesSymlinkTeamModeAndUnknownNames(t *testing.T) {
	home := skillHome(t)
	m := NewManager("")
	// sembolik bağ: üzerine yazılmaz (dotfiles deposunu bozmasın)
	p, _ := claudeSkillPath("ci-fix")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	target := filepath.Join(home, "dotfiles-skill.md")
	_ = os.WriteFile(target, []byte("dotfiles"), 0o644)
	if err := os.Symlink(target, p); err != nil {
		t.Skip("sembolik bağ kurulamadı:", err)
	}
	if _, err := m.InstallSkill("ci-fix", true); err == nil || !strings.Contains(err.Error(), "sembolik") {
		t.Fatalf("sembolik bağ reddedilmeli: %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "dotfiles" {
		t.Fatal("bağın hedefi değişmemeli")
	}
	_ = os.Remove(p)

	for _, bad := range []string{"", "../etc", "ci-fix/../x", "yok-boyle-skill"} {
		if _, err := m.InstallSkill(bad, true); err == nil {
			t.Fatalf("%q reddedilmeli", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "etc")); err == nil {
		t.Fatal("ev dizininin dışına/içine beklenmedik yazım")
	}

	m.SetTeamMode(true)
	if _, err := m.InstallSkill("ci-fix", false); err == nil || !strings.Contains(err.Error(), "takım modunda") {
		t.Fatalf("takım modunda kapalı olmalı: %v", err)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("takım modunda dosya yazılmamalı")
	}
}
