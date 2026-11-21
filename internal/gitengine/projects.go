package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitarc/internal/model"
)

// SavedProject represents a persisted user project in ~/.gitarc/projects.json
type SavedProject struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	AddedAt time.Time `json:"addedAt"`
}

func getProjectsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gitarc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "projects.json"), nil
}

func loadSavedProjects() []SavedProject {
	filePath, err := getProjectsFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var projects []SavedProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil
	}
	return projects
}

func persistSavedProjects(projects []SavedProject) error {
	filePath, err := getProjectsFilePath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// AddProject registers a local git directory into the live manager and persists it.
func (m *Manager) AddProject(repoPath, customName string) (model.CockpitSnapshot, error) {
	if strings.TrimSpace(repoPath) == "" {
		return model.CockpitSnapshot{}, fmt.Errorf("proje dizin yolu boş olamaz")
	}
	abs, err := filepath.Abs(strings.TrimSpace(repoPath))
	if err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("geçersiz dizin yolu: %v", err)
	}
	// Alt klasör, worktree ve submodule verilse de deponun köküne çözülür.
	if abs = repoRootFor(abs); abs == "" {
		return model.CockpitSnapshot{}, fmt.Errorf("belirtilen yolda geçerli bir git deposu bulunamadı: %s", strings.TrimSpace(repoPath))
	}

	name := strings.TrimSpace(customName)
	if name == "" {
		name = filepath.Base(abs)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Aynı depo farklı yol yazımlarıyla (alt klasör, sembolik bağ, /var ↔ /private/var) ikinci kez eklenmez.
	var existingID string
	for i, lr := range m.liveRepos {
		if sameRepoPath(lr.path, abs) {
			existingID = lr.id
			// -repo/ortam/klasörle açılmış geçici kimlik ("live-custom"...) kayda GEÇMEZ: yoldan türetilen kalıcı kimliğe yükseltilir, aksi
			// halde aynı proje açılış yoluna göre iki ayrı günlük/öneri kartına sahip olurdu.
			if legacySyntheticRepoIDs[existingID] {
				existingID = projectIDForPath(abs)
				m.liveRepos[i].id = existingID
				if m.activeRepoID == lr.id {
					m.activeRepoID = existingID
				}
			}
			break
		}
	}

	targetID := existingID
	if targetID == "" {
		// Kimlik yoldan türetilir: aynı depo kaldırılıp yeniden eklenince AYNI kimliği alır (arayüz son seçilen kimliği
		// tarayıcıda hatırlar; rastgele kimlik yeniden eklemede onu geçersiz kılıyordu).
		targetID = projectIDForPath(abs)
		m.liveRepos = append(m.liveRepos, repoEntry{
			id:   targetID,
			name: name + " (Canlı)",
			path: abs,
		})
	}

	saved := loadSavedProjects()
	alreadySaved := false
	for i, sp := range saved {
		if sameRepoPath(sp.Path, abs) {
			saved[i].Name = name
			saved[i].ID = targetID // kayıttaki kimlik bellektekiyle aynı kalsın (yeniden başlatınca arayüzün hatırladığı kimlik tutsun)
			alreadySaved = true
			break
		}
	}
	if !alreadySaved {
		saved = append(saved, SavedProject{
			ID:      targetID,
			Name:    name,
			Path:    abs,
			AddedAt: time.Now(),
		})
	}
	_ = persistSavedProjects(saved)

	m.activeRepoID = targetID
	opts := m.repoOptionsLocked()
	var live *repoEntry
	for i := range m.liveRepos {
		if m.liveRepos[i].id == targetID {
			live = &m.liveRepos[i]
			break
		}
	}
	if live == nil {
		return m.buildSimSnapshotLocked(opts), nil
	}

	snap, err := CollectLiveRepo(live.id, live.name, live.path, opts)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	return snap, nil
}

// RemoveProject unregisters a project from the manager and removes it from projects.json.
func (m *Manager) RemoveProject(repoID string) (model.CockpitSnapshot, error) {
	if strings.HasPrefix(repoID, "sim-") {
		return model.CockpitSnapshot{}, fmt.Errorf("demo simülasyon deposu listeden kaldırılamaz")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var removedPath string
	newLive := []repoEntry{}
	for _, lr := range m.liveRepos {
		if lr.id == repoID {
			removedPath = lr.path
		} else {
			newLive = append(newLive, lr)
		}
	}
	m.liveRepos = newLive

	if removedPath != "" {
		saved := loadSavedProjects()
		updated := []SavedProject{}
		for _, sp := range saved {
			if !sameRepoPath(sp.Path, removedPath) {
				updated = append(updated, sp)
			}
		}
		_ = persistSavedProjects(updated)
	}

	if m.activeRepoID == repoID {
		switch {
		case len(m.liveRepos) > 0:
			m.activeRepoID = m.liveRepos[0].id
		case m.demoAllowed:
			m.activeRepoID = "sim-sandbox"
		default:
			m.activeRepoID = "" // son proje kaldırıldı: sessizce demoya dönme, "proje yok" durumu
		}
	}

	opts := m.repoOptionsLocked()
	var live *repoEntry
	for i := range m.liveRepos {
		if m.liveRepos[i].id == m.activeRepoID {
			live = &m.liveRepos[i]
			break
		}
	}
	if m.activeRepoID == "" {
		return m.noProjectSnapshotLocked(opts), nil
	}
	if live == nil {
		return m.buildSimSnapshotLocked(opts), nil
	}
	return CollectLiveRepo(live.id, live.name, live.path, opts)
}

// DiscardWorkingTree discards all uncommitted/staged/untracked changes in the active repository.
func (m *Manager) DiscardWorkingTree() (model.CockpitSnapshot, error) {
	m.mu.RLock()
	activeID := m.activeRepoID
	var live *repoEntry
	for i := range m.liveRepos {
		if m.liveRepos[i].id == activeID {
			entry := m.liveRepos[i]
			live = &entry
			break
		}
	}
	m.mu.RUnlock()

	if live == nil {
		m.mu.Lock()
		m.simRepo.repo.DirtyCount = 0
		var cleanBranches []model.BranchArc
		for _, b := range m.simRepo.branches {
			if !strings.HasPrefix(b.ID, "local-wip") {
				b.UncommittedCount = 0
				cleanBranches = append(cleanBranches, b)
			}
		}
		m.simRepo.branches = cleanBranches
		opts := m.repoOptionsLocked()
		snap := m.buildSimSnapshotLocked(opts)
		m.mu.Unlock()
		return snap, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, _, _ = runCmd(ctx, live.path, "git", "restore", "--staged", "--worktree", ".")
	_, _, _ = runCmd(ctx, live.path, "git", "clean", "-fd")

	InvalidateGHCache(live.path)
	return m.Snapshot()
}

// projectIDForPath: depo yolundan kararlı proje kimliği ("live-proj-" + yolun özetinin ilk 8 hex'i). Sembolik bağlar çözülür,
// yani aynı depo hangi yazımla eklenirse eklensin aynı kimliği alır.
func projectIDForPath(p string) string {
	canon := filepath.Clean(p)
	if r, err := filepath.EvalSymlinks(canon); err == nil {
		canon = r
	}
	sum := sha256.Sum256([]byte(canon))
	return "live-proj-" + hex.EncodeToString(sum[:4])
}
