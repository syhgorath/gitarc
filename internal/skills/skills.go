// Package skills: GitArc'ın birlikte getirdiği Claude Code skill'leri (yalnızca standart kütüphane, go:embed).
// Tek kaynak bu klasördür (data/<ad>/SKILL.md); arayüzdeki "Claude'a kur" düğmesi ve elle kopyalama aynı metni kullanır.
package skills

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed data
var dataFS embed.FS

// Skill: gömülü bir skill.
type Skill struct {
	Name        string
	Description string
	Content     string // SKILL.md'nin tamamı (frontmatter dahil)
}

// All: gömülü skill'ler, ada göre sıralı.
func All() []Skill {
	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return nil
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if s, ok := Get(e.Name()); ok {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Get: yalnızca gömülü listedeki adlar döner; istemciden gelen ad ASLA yol olarak kullanılmaz.
func Get(name string) (Skill, bool) {
	if name == "" || strings.ContainsAny(name, `/\.`) {
		return Skill{}, false
	}
	b, err := dataFS.ReadFile(fmt.Sprintf("data/%s/SKILL.md", name))
	if err != nil {
		return Skill{}, false
	}
	content := string(b)
	return Skill{Name: name, Description: frontmatterValue(content, "description"), Content: content}, true
}

// frontmatterValue: `---` ile çevrili başlıktaki tek satırlık `key: value`.
func frontmatterValue(content, key string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ""
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if v, ok := strings.CutPrefix(l, key+":"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
