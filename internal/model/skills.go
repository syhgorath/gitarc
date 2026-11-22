package model

// SkillInfo: GitArc'ın getirdiği bir Claude Code skill'i ve bu makinedeki kurulum durumu.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`              // kurulacak/kurulu SKILL.md (örn. ~/.claude/skills/ci-fix/SKILL.md)
	Status      string `json:"status"`            // not_installed | up_to_date | different | unreadable
	Content     string `json:"content,omitempty"` // elle kopyalama için tam metin
	Note        string `json:"note,omitempty"`
}
