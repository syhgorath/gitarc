package gitengine

import (
	"fmt"
	"strings"

	"gitarc/internal/model"
)

// ApproveProposal, kullanıcının onayladığı bekleyen AI commit/PR önerisini çalıştırır: yeni dalı açar, YALNIZCA
// önerideki dosyaları stage eder, commit atar, pushlar ve PR açar (canlı modda `commitWorkingTreeLive`, simülasyonda
// sanal dal). Kullanıcı yalnızca metinleri (dal adı, commit mesajı, PR başlığı/açıklaması) düzenleyebilir; dosya listesi
// her zaman önerinin kendisinden gelir, istemciden alınmaz.
//
// Güvenlik: öneri kimliği eşleşmelidir (bayat/başka öneri onaylanamaz), Team modunda HEAD değiştirilemez,
// gizli dosya ve birleşmiş dal engelleri `CommitWorkingTree` içinde yeniden uygulanır, aynı anda tek onay çalışır.
// Başarısız olursa öneri bekleyen durumda kalır (kullanıcı düzeltip yeniden deneyebilir).
func (m *Manager) ApproveProposal(proposalID, branchName, message, prTitle, prBody string) (model.CockpitSnapshot, error) {
	prop := m.GetPendingProposal()
	if prop == nil {
		err := fmt.Errorf("bekleyen AI önerisi yok (zaten onaylanmış, reddedilmiş veya süresi dolmuş olabilir)")
		m.recordLog("error", "AI Önerisi Onayı", "", err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if strings.TrimSpace(proposalID) != prop.ID {
		err := fmt.Errorf("öneri değişmiş veya artık geçerli değil; sayfayı yenileyip güncel öneriyi onaylayın")
		m.recordLog("error", "AI Önerisi Onayı", prop.BranchName, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}

	// Boş bırakılan alanlar önerinin kendi değerlerini kullanır.
	branchName = strings.TrimSpace(branchName)
	if branchName == "" {
		branchName = prop.BranchName
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = prop.Message
	}
	if strings.TrimSpace(prTitle) == "" {
		prTitle = prop.PRTitle
	}
	if strings.TrimSpace(prBody) == "" {
		prBody = prop.PRBody
	}

	if err := validateBranchName(branchName); err != nil {
		m.recordLog("error", "AI Önerisi Onayı", branchName, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	defBranch := "main"
	if snap, err := m.Snapshot(); err == nil && snap.Repo.DefaultBranch != "" {
		defBranch = snap.Repo.DefaultBranch
	}
	if branchName == defBranch {
		err := fmt.Errorf("PR açmak için %s dışında yeni bir özellik dalı adı girin", defBranch)
		m.recordLog("error", "AI Önerisi Onayı", branchName, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}

	m.mu.Lock()
	if m.proposalBusy {
		m.mu.Unlock()
		return model.CockpitSnapshot{}, fmt.Errorf("önerinin onayı zaten işleniyor; lütfen bekleyin")
	}
	m.proposalBusy = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.proposalBusy = false
		m.mu.Unlock()
	}()

	if _, err := m.CommitWorkingTree(branchName, message, prTitle, prBody, true, append([]string(nil), prop.Files...)); err != nil {
		return model.CockpitSnapshot{}, err
	}

	m.mu.Lock()
	m.pendingProposal = nil
	m.recordLogLocked("ok", "AI Önerisi Onaylandı", branchName, fmt.Sprintf("%s dalı %d dosyayla oluşturuldu, commitlendi ve PR açıldı", branchName, len(prop.Files)), "")
	m.mu.Unlock()
	if live, ok := m.activeLiveRepo(); ok {
		removeProposalFile(live.path)
	}
	return m.Snapshot()
}
