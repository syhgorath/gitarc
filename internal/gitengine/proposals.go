package gitengine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	if strings.TrimSpace(message) == "" {
		return model.CommitProposal{}, fmt.Errorf("commit mesajı (message) boş olamaz")
	}

	// 1. files parametresi ZORUNLUDUR (Claude kuralı #1)
	if len(files) == 0 {
		return model.CommitProposal{}, fmt.Errorf("files parametresi zorunludur ve en az bir geçerli dosya yolu içermelidir; boş bırakılamaz")
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

	// 2. Merged dal kontrolü (yerel, uzak ve PR geçmişi) (Claude kuralı #2)
	for _, b := range snap.Branches {
		if strings.EqualFold(b.Name, branchName) {
			if b.State == "merged" || b.OriginGone || (b.PR != nil && b.PR.ReviewState == "MERGED") {
				return model.CommitProposal{}, fmt.Errorf("'%s' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın adı öneri için tekrar kullanılamaz!", branchName)
			}
		}
	}

	repoPath, repoName := "", ""
	cfg := DefaultGuardConfig
	if live, ok := m.activeLiveRepo(); ok {
		repoPath, repoName = live.path, live.name
		cfg = LoadGuardConfig(repoPath)
	}

	if repoPath != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()

		// (a) git branch -a --merged HEAD
		if out, _, err := runCmd(ctx, repoPath, "git", "branch", "-a", "--merged", "HEAD"); err == nil {
			for _, line := range strings.Split(out, "\n") {
				trimmed := strings.TrimSpace(strings.TrimPrefix(line, "*"))
				trimmed = strings.TrimPrefix(trimmed, "remotes/origin/")
				trimmed = strings.TrimPrefix(trimmed, "origin/")
				if strings.EqualFold(trimmed, branchName) && trimmed != defBranch {
					return model.CommitProposal{}, fmt.Errorf("'%s' dalı yerel veya uzak Git geçmişinde daha önce birleştirilmiş (merged). Birleşmiş bir dal adı önerilemez!", branchName)
				}
			}
		}

		// (b) git log merge commits referencing this branch
		if logOut, _, err := runCmd(ctx, repoPath, "git", "log", "-n", "250", "--merges", "--oneline"); err == nil {
			searchTerms := []string{
				"from " + branchName,
				"Merge branch '" + branchName + "'",
				"/" + branchName + " into",
				"into " + branchName,
			}
			for _, line := range strings.Split(logOut, "\n") {
				for _, term := range searchTerms {
					if strings.Contains(strings.ToLower(line), strings.ToLower(term)) {
						return model.CommitProposal{}, fmt.Errorf("'%s' dalı Git commit geçmişinde birleşmiş (PR/Merge) olarak bulundu. Birleşmiş bir dal adı tekrar kullanılamaz!", branchName)
					}
				}
			}
		}

		// (c) gh pr list --state merged --head <branchName>
		if _, err := exec.LookPath("gh"); err == nil {
			if ghOut, _, ghErr := runCmd(ctx, repoPath, "gh", "pr", "list", "--state", "merged", "--head", branchName, "--json", "number,headRefName"); ghErr == nil {
				if strings.Contains(strings.ToLower(ghOut), strings.ToLower(branchName)) {
					return model.CommitProposal{}, fmt.Errorf("'%s' dalı GitHub PR geçmişinde birleştirilmiş (MERGED) olarak bulundu. Birleşmiş bir dal adı önerilemez!", branchName)
				}
			}
		}
	}

	// 3. Gizli dosya ve özel anahtar kontrolleri
	var cleanFiles []string
	for _, f := range files {
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

	// 4. Diff stat hesaplama
	diffStat, _ := m.GetDiffStat()

	t := strings.TrimSpace(prTitle)
	if t == "" {
		t = message
	}
	b := strings.TrimSpace(prBody)
	if b == "" {
		b = fmt.Sprintf("Claude / AI önerisi ile oluşturuldu.\n\n### Değişiklikler\n- %s", strings.Join(cleanFiles, "\n- "))
	}

	// 5. Kalkan ve dosya sayısı eşleşme kontrolleri (Claude kuralı #3)
	shieldResult := fmt.Sprintf("✓ Güvenlik Kalkanı: 0 secret, 0 özel anahtar, 0 .gitignore ihlali (%d dosya güvenle onaylandı)", len(cleanFiles))
	countMatch, countMsg := evaluateMessageCountMatch(message, len(cleanFiles))

	// 6. Co-Authored-By ve tam mesaj koruması (Claude kuralı #4):
	// Message alanı olduğu gibi korunur.
	prop := model.CommitProposal{
		ID:                fmt.Sprintf("prop-%d", time.Now().UnixNano()),
		BranchName:        branchName,
		BaseBranch:        "origin/" + defBranch,
		Message:           message,
		Files:             cleanFiles,
		PRTitle:           t,
		PRBody:            b,
		DiffStat:          diffStat.Stat,
		FilesChanged:      len(cleanFiles),
		TotalLines:        diffStat.TotalLines,
		IsLargeDiff:       len(cleanFiles) > 10 || diffStat.TotalLines >= 1000,
		CreatedAt:         time.Now().Format("15:04:05"),
		ShieldResult:      shieldResult,
		CountCheckMatch:   countMatch,
		CountCheckMessage: countMsg,
		RepoPath:          repoPath,
		RepoName:          repoName,
	}

	m.mu.Lock()
	m.pendingProposal = &prop
	m.recordLogLocked("ok", "Öneri Kartı (MCP)", branchName, fmt.Sprintf("%s dalı için %d dosyalık öneri oluşturuldu (Kullanıcı onayı bekleniyor)", branchName, len(cleanFiles)), "")
	m.mu.Unlock()

	if repoPath != "" {
		// Kart arayüze bu dosya üzerinden ulaşır (MCP ve arayüz ayrı süreçlerdir): yazılamadıysa sessizce "iletildi" deme.
		if err := saveProposalToFile(repoPath, prop); err != nil {
			m.mu.Lock()
			m.pendingProposal = nil
			m.mu.Unlock()
			return model.CommitProposal{}, fmt.Errorf("öneri kartı kaydedilemedi, arayüze iletilemez: %v", err)
		}
	}

	return prop, nil
}

func evaluateMessageCountMatch(msg string, fileCount int) (match bool, resultMsg string) {
	re := regexp.MustCompile(`(?i)(?:update\s+(\d+)\s+files?|(\d+)\s+files?(?:\s+changed)?|(\d+)\s+dosya(?:da)?)`)
	matches := re.FindStringSubmatch(msg)
	foundNum := 0
	found := false
	if len(matches) > 0 {
		for i := 1; i < len(matches); i++ {
			if matches[i] != "" {
				if n, err := strconv.Atoi(matches[i]); err == nil && n > 0 {
					foundNum = n
					found = true
					break
				}
			}
		}
	}

	if !found {
		return true, fmt.Sprintf("✓ %d dosya seçildi (mesajda sayı belirtilmedi)", fileCount)
	}

	if foundNum == fileCount {
		return true, fmt.Sprintf("✓ Dosya sayısı commit mesajıyla tam eşleşiyor (%d dosya)", fileCount)
	}

	return false, fmt.Sprintf("⚠ UYARI: Commit mesajında '%d' dosya belirtilmiş, ancak %d dosya seçildi!", foundNum, fileCount)
}

// DismissProposal discards the pending proposal from memory and disk.
func (m *Manager) DismissProposal() (model.CockpitSnapshot, error) {
	m.mu.Lock()
	m.pendingProposal = nil
	m.recordLogLocked("ok", "Öneri Kapatıldı", "", "Bekleyen öneri kartı kullanıcı tarafından reddedildi/kapatıldı", "")
	m.mu.Unlock()

	if live, ok := m.activeLiveRepo(); ok {
		removeProposalFile(live.path)
	}
	return m.Snapshot()
}

// GetPendingProposal bekleyen öneriyi döndürür. Canlı depoda `.git/gitarc_proposal.json` TEK GERÇEK KAYNAKTIR: öneriyi
// MCP süreci yazar, arayüz süreci okur, onay/ret/mutasyon dosyayı siler. Bellekteki kopya dosyayı geriden izler
// (eskiden bellekteki kart dosyadaki yeni kartın önüne geçiyor, başka süreçte onaylanan kart arayüzde kalıyordu).
// Simülasyonda dosya yoktur; bellek esastır.
func (m *Manager) GetPendingProposal() *model.CommitProposal {
	if live, ok := m.activeLiveRepo(); ok {
		prop, err := loadProposalFromFile(live.path)
		m.mu.Lock()
		defer m.mu.Unlock()
		switch {
		case err == nil && prop != nil:
			m.pendingProposal = prop
		case err != nil && errors.Is(err, os.ErrNotExist):
			m.pendingProposal = nil
		}
		return m.pendingProposal
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pendingProposal
}

func proposalFilePath(repoPath string) string {
	return filepath.Join(gitDirOrDefault(repoPath), "gitarc_proposal.json")
}

func saveProposalToFile(repoPath string, prop model.CommitProposal) error {
	b, err := json.MarshalIndent(prop, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(proposalFilePath(repoPath), b, 0644)
}

// proposalFileSig: kart dosyasının imzası (yoksa ""). Başka bir süreç (MCP) kartı yazınca/silince yoklama imzası değişsin.
func proposalFileSig(repoPath string) string {
	st, err := os.Stat(proposalFilePath(repoPath))
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
}

func loadProposalFromFile(repoPath string) (*model.CommitProposal, error) {
	p := proposalFilePath(repoPath)
	data, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var prop model.CommitProposal
	if err := json.Unmarshal(data, &prop); err != nil {
		return nil, err
	}
	return &prop, nil
}

func removeProposalFile(repoPath string) {
	_ = os.Remove(proposalFilePath(repoPath))
}
