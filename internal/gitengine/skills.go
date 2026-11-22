package gitengine

import (
	"fmt"
	"os"
	"path/filepath"

	"gitarc/internal/model"
	"gitarc/internal/skills"
)

// Skill kurulumu (yalnızca Claude Code: ~/.claude/skills/<ad>/SKILL.md).
//
// Güvenlik: ad yalnızca gömülü listeden gelir (istemciden yol alınmaz); hedef sabittir. Bu, yapay zekânın davranışını değiştiren
// KALICI bir dosyadır: yalnızca kullanıcının tıklamasıyla yazılır, farklı bir dosyanın üzerine ancak açık onayla (overwrite) yazılır,
// sembolik bağa yazılmaz, takım modunda (sunucu makinesinin ev dizini olurdu) kapalıdır. Tarayıcı dosya yazamadığı için çevrimdışı
// yedek motorda yalnızca elle kopyalama vardır (fallback.js).

func claudeSkillPath(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", fmt.Errorf("ev dizini bulunamadı")
	}
	return filepath.Join(home, ".claude", "skills", name, "SKILL.md"), nil
}

func skillInfoFor(s skills.Skill) model.SkillInfo {
	info := model.SkillInfo{Name: s.Name, Description: s.Description, Content: s.Content}
	p, err := claudeSkillPath(s.Name)
	if err != nil {
		info.Status, info.Note = "unreadable", err.Error()
		return info
	}
	info.Path = p
	fi, err := os.Lstat(p)
	switch {
	case os.IsNotExist(err):
		info.Status = "not_installed"
	case err != nil:
		info.Status, info.Note = "unreadable", err.Error()
	case fi.Mode()&os.ModeSymlink != 0:
		info.Status, info.Note = "unreadable", "sembolik bağ: GitArc bunun üzerine yazmaz; elle güncelleyin"
	default:
		b, rerr := os.ReadFile(p)
		switch {
		case rerr != nil:
			info.Status, info.Note = "unreadable", rerr.Error()
		case string(b) == s.Content:
			info.Status = "up_to_date"
		default:
			info.Status = "different"
		}
	}
	return info
}

// SkillsStatus: gömülü skill'ler ve kurulum durumları (salt okunur).
func (m *Manager) SkillsStatus() []model.SkillInfo {
	var out []model.SkillInfo
	for _, s := range skills.All() {
		out = append(out, skillInfoFor(s))
	}
	return out
}

// InstallSkill: skill'i ~/.claude/skills/<ad>/SKILL.md olarak yazar. Hedef farklı bir içerik taşıyorsa overwrite=true ister.
func (m *Manager) InstallSkill(name string, overwrite bool) (model.SkillInfo, error) {
	fail := func(err error) (model.SkillInfo, error) {
		m.recordLog("error", "Skill Kurulumu", name, err.Error(), "")
		return model.SkillInfo{}, err
	}
	if m.IsTeamMode() {
		return fail(fmt.Errorf("takım modunda skill kurulumu kapalı: dosya, sunucu makinesinin ev dizinine yazılırdı; skill'i kendi makinenizde elle kurun"))
	}
	s, ok := skills.Get(name)
	if !ok {
		return fail(fmt.Errorf("bilinmeyen skill: %q", name))
	}
	cur := skillInfoFor(s)
	switch cur.Status {
	case "up_to_date":
		return cur, nil // zaten güncel: yazmaya gerek yok
	case "unreadable":
		return fail(fmt.Errorf("%s kurulamadı: %s", name, cur.Note))
	case "different":
		if !overwrite {
			return fail(fmt.Errorf("%s zaten kurulu ve içeriği farklı (%s); üzerine yazmak için onay gerekir", name, cur.Path))
		}
	}
	dir := filepath.Dir(cur.Path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fail(fmt.Errorf("klasör oluşturulamadı: %v", err))
	}
	tmp, err := os.CreateTemp(dir, ".SKILL.md.tmp-*")
	if err != nil {
		return fail(fmt.Errorf("yazılamadı: %v", err))
	}
	tmpName := tmp.Name()
	_, werr := tmp.WriteString(s.Content)
	cerr := tmp.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmpName)
		return fail(fmt.Errorf("yazılamadı: %v %v", werr, cerr))
	}
	_ = os.Chmod(tmpName, 0o644) // CreateTemp 0600 açar; skill okunabilir olmalı
	if err := os.Rename(tmpName, cur.Path); err != nil {
		_ = os.Remove(tmpName)
		return fail(fmt.Errorf("yerine konulamadı: %v", err))
	}
	res := skillInfoFor(s)
	if res.Status != "up_to_date" {
		return fail(fmt.Errorf("yazıldı ama doğrulanamadı (durum: %s)", res.Status))
	}
	verb := "kuruldu"
	if cur.Status == "different" {
		verb = "güncellendi"
	}
	m.recordLog("ok", "Skill Kurulumu", name, fmt.Sprintf("%s %s: %s", name, verb, res.Path), "")
	return res, nil
}
