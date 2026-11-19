package gitengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Omurga (varsayılan) dalın elle seçimi. Tespit (`detectDefaultBranch`) yanlışsa kullanıcı projesi için dalı
// sabitler. Seçim `~/.gitarc/base-branches.json` içinde depo yoluna göre tutulur (proje listesine dokunmaz,
// `-repo` ile açılan kayıtsız depolar için de çalışır). Boş seçim = otomatik tespit.
const maxBaseCandidates = 100

func baseBranchesFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gitarc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "base-branches.json"), nil
}

func loadBaseOverrides() map[string]string {
	out := map[string]string{}
	p, err := baseBranchesFilePath()
	if err != nil {
		return out
	}
	if data, err := os.ReadFile(p); err == nil {
		_ = json.Unmarshal(data, &out)
	}
	return out
}

func savedBaseBranch(repoPath string) string {
	return loadBaseOverrides()[filepath.Clean(repoPath)]
}

func saveBaseBranch(repoPath, branch string) error {
	p, err := baseBranchesFilePath()
	if err != nil {
		return err
	}
	all := loadBaseOverrides()
	key := filepath.Clean(repoPath)
	if branch == "" {
		delete(all, key)
	} else {
		all[key] = branch
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0644)
}

func localBranchExists(ctx context.Context, repoPath, name string) bool {
	_, _, err := runCmd(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// pinnedBaseBranch: kullanıcının seçtiği omurga dalı; dal artık yerelde yoksa seçim yok sayılır (otomatik tespite döner).
func pinnedBaseBranch(ctx context.Context, repoPath string) string {
	if b := savedBaseBranch(repoPath); b != "" && localBranchExists(ctx, repoPath, b) {
		return b
	}
	return ""
}

// baseCandidates: seçicide gösterilecek yerel dallar (varsayılan ve uzun ömürlü adaylar öne gelir).
func baseCandidates(ctx context.Context, repoPath, current string) []string {
	out, _, err := runCmd(ctx, repoPath, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil
	}
	var names []string
	for _, n := range strings.Fields(out) {
		names = append(names, n)
	}
	sort.SliceStable(names, func(i, j int) bool {
		rank := func(n string) int {
			switch {
			case n == current:
				return 0
			case IsLongLivedBranch(n, ""):
				return 1
			}
			return 2
		}
		ri, rj := rank(names[i]), rank(names[j])
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	if len(names) > maxBaseCandidates {
		names = names[:maxBaseCandidates]
	}
	return names
}

// SetBaseBranch aktif canlı deponun omurga dalını sabitler; boş ad otomatik tespite döner.
// Simülasyon deposunda (gerçek dal seçeneği yoktur) reddedilir.
func (m *Manager) SetBaseBranch(branch string) (model.CockpitSnapshot, error) {
	live, ok := m.activeLiveRepo()
	if !ok {
		err := fmt.Errorf("simülasyon deposunda omurga dalı değiştirilemez")
		m.recordLog("error", "Omurga Dalı", branch, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	branch = strings.TrimSpace(branch)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if branch != "" {
		if err := validateBranchName(branch); err != nil {
			m.recordLog("error", "Omurga Dalı", branch, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
		if !localBranchExists(ctx, live.path, branch) {
			err := fmt.Errorf("%q adında yerel bir dal yok; önce dalı yerelde oluşturun veya indirin", branch)
			m.recordLog("error", "Omurga Dalı", branch, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
	}
	if err := saveBaseBranch(live.path, branch); err != nil {
		err = fmt.Errorf("omurga dalı kaydedilemedi: %v", err)
		m.recordLog("error", "Omurga Dalı", branch, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	InvalidateGHCache(live.path)
	snap, err := m.Snapshot()
	okMsg := "otomatik tespite dönüldü"
	if branch != "" {
		okMsg = fmt.Sprintf("omurga dalı %s olarak sabitlendi", branch)
	}
	return m.logLiveOutcome("Omurga Dalı", branch, okMsg, snap, err)
}
