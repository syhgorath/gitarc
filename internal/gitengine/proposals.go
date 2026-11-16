package gitengine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gitarc/internal/model"
)

// ProposeCommit creates a staged proposal for a commit & PR without executing any Git changes.
// It performs pre-flight security checks, .gitignore verification, secret pattern scanning,
// diff calculation, and surfaces the proposal to the GitArc UI for explicit user confirmation.
func (m *Manager) ProposeCommit(branchName, message string, files []string, prTitle, prBody string) (model.CommitProposal, error) {
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		return model.CommitProposal{}, fmt.Errorf("dal adı (branch_name) boş olamaz")
	}
	if err := validateBranchName(branchName); err != nil {
		return model.CommitProposal{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return model.CommitProposal{}, fmt.Errorf("commit mesajı (message) boş olamaz")
	}

	snap, err := m.Snapshot()
	if err != nil {
		return model.CommitProposal{}, err
	}

	defBranch := snap.Repo.DefaultBranch
	if defBranch == "" {
		defBranch = "main"
	}
	if branchName == defBranch {
		return model.CommitProposal{}, fmt.Errorf("yeni dal adı '%s' ile aynı olamaz; lütfen yeni bir özellik dalı adı belirleyin", defBranch)
	}

	// 1. Merged bir dalın adını yeniden kullanma kuralı
	for _, b := range snap.Branches {
		if strings.EqualFold(b.Name, branchName) {
			if b.State == "merged" || b.OriginGone || (b.PR != nil && b.PR.ReviewState == "MERGED") {
				return model.CommitProposal{}, fmt.Errorf("'%s' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın adı öneri için tekrar kullanılamaz!", branchName)
			}
		}
	}

	// 2. Gizli dosya ve özel anahtar kontrolleri
	cfg := DefaultGuardConfig
	repoPath := ""
	if live, ok := m.activeLiveRepo(); ok {
		repoPath = live.path
		cfg = LoadGuardConfig(repoPath)
	}

	candidateFiles := files
	if len(candidateFiles) == 0 {
		if repoPath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			out, _, _ := runCmd(ctx, repoPath, "git", "status", "--porcelain")
			candidateFiles = parsePorcelainFiles(out)
		} else {
			candidateFiles = []string{"CHANGELOG.md"}
		}
	}

	var cleanFiles []string
	for _, f := range candidateFiles {
		f = filepath.ToSlash(filepath.Clean(strings.TrimSpace(f)))
		f = strings.TrimPrefix(f, "./")
		if f == "" {
			continue
		}

		if repoPath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if _, _, err := runCmd(ctx, repoPath, "git", "check-ignore", "-q", "--", f); err == nil {
				cancel()
				return model.CommitProposal{}, fmt.Errorf("GÜVENLİK ENGELİ: '%s' dosyası .gitignore tarafından yoksayılıyor. Asla commitlenemez!", f)
			}
			cancel()
		}

		if isSecret, pat := IsSecretPath(f, cfg.SecretPatterns); isSecret {
			return model.CommitProposal{}, fmt.Errorf("GÜVENLİK ENGELİ: Gizli/korumalı dosya tespit edildi: '%s' (eşleşen kural: %s). Öneri reddedildi!", f, pat)
		}

		if repoPath != "" {
			if hasSec, pat := CheckFileContentForSecrets(repoPath, f, cfg.ContentPatterns); hasSec {
				return model.CommitProposal{}, fmt.Errorf("GÜVENLİK ENGELİ: '%s' dosyası içinde özel anahtar (%s) tespit edildi. Öneri reddedildi!", f, pat)
			}
		}

		cleanFiles = append(cleanFiles, f)
	}

	if len(cleanFiles) == 0 {
		return model.CommitProposal{}, fmt.Errorf("önerilecek geçerli ve güvenli dosya bulunamadı")
	}

	// 3. Diff stat hesaplama
	diffStat, _ := m.GetDiffStat()

	t := strings.TrimSpace(prTitle)
	if t == "" {
		t = message
	}
	b := strings.TrimSpace(prBody)
	if b == "" {
		b = fmt.Sprintf("Claude / AI önerisi ile oluşturuldu.\n\n### Değişiklikler\n- %s", strings.Join(cleanFiles, "\n- "))
	}

	prop := model.CommitProposal{
		ID:           fmt.Sprintf("prop-%d", time.Now().UnixNano()),
		BranchName:   branchName,
		BaseBranch:   "origin/" + defBranch,
		Message:      message,
		Files:        cleanFiles,
		PRTitle:      t,
		PRBody:       b,
		DiffStat:     diffStat.Stat,
		FilesChanged: len(cleanFiles),
		TotalLines:   diffStat.TotalLines,
		IsLargeDiff:  len(cleanFiles) > 10 || diffStat.TotalLines >= 1000,
		CreatedAt:    time.Now().Format("15:04:05"),
	}

	m.mu.Lock()
	m.pendingProposal = &prop
	m.recordLogLocked("ok", "Öneri Kartı (MCP)", branchName, fmt.Sprintf("%s dalı için %d dosyalık öneri oluşturuldu (Kullanıcı onayı bekleniyor)", branchName, len(cleanFiles)), "")
	m.mu.Unlock()

	return prop, nil
}

// DismissProposal discards the pending proposal from memory.
func (m *Manager) DismissProposal() (model.CockpitSnapshot, error) {
	m.mu.Lock()
	m.pendingProposal = nil
	m.recordLogLocked("ok", "Öneri Kapatıldı", "", "Bekleyen öneri kartı kullanıcı tarafından reddedildi/kapatıldı", "")
	m.mu.Unlock()
	return m.Snapshot()
}

// GetPendingProposal returns active proposal or nil.
func (m *Manager) GetPendingProposal() *model.CommitProposal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingProposal
}
