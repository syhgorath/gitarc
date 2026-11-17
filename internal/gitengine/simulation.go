package gitengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitarc/internal/model"
)

var (
	reSafeBranchName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,119}$`)
	reSafeSHA        = regexp.MustCompile(`^[0-9a-fA-F]{4,40}$`)
)

// validateBranchName rejects empty names, flag-like '-' prefixes, path traversals,
// and characters outside safe git ref conventions.
func validateBranchName(name string) error {
	if name == "" {
		return fmt.Errorf("dal adı boş olamaz")
	}
	if strings.HasPrefix(name, "-") || strings.Contains(name, "..") || strings.Contains(name, "//") || strings.HasSuffix(name, "/") || strings.HasSuffix(name, ".lock") {
		return fmt.Errorf("geçersiz dal adı: %q", name)
	}
	if !reSafeBranchName.MatchString(name) {
		return fmt.Errorf("dal adı geçersiz karakter içeriyor: %q", name)
	}
	return nil
}

// Manager coordinates active workspace repositories (simulation + live local repos)
// and executes thread-safe lifecycle mutations on the cockpit graph.
type Manager struct {
	mu              sync.RWMutex
	activeRepoID    string
	simRepo         simWorkspace
	liveRepos       []repoEntry
	nextPR          int
	nextSeq         int
	logs            []model.ActionLogEntry
	logSeq          int
	teamMode        bool
	latestVer       string
	updateAvail     bool
	pendingProposal *model.CommitProposal
}

type repoEntry struct {
	id   string
	name string
	path string
}

type simWorkspace struct {
	repo     model.RepoMeta
	nodes    []model.CommitNode
	branches []model.BranchArc
}

// NewManager initializes the GitArc engine.
// Canlı mod için bir depo yolu (örn. "." veya mutlak yol) verilir.
// extraRepoPath == "sim" veya GITARC_MODE=simulation ise simülasyon moduna geçer.
// extraRepoPath == "" ise geriye dönük test uyumluluğu için varsayılan simülasyon modunda başlar.
func NewManager(extraRepoPath string) *Manager {
	m := &Manager{
		activeRepoID: "sim-sandbox",
		nextPR:       425,
		nextSeq:      10,
	}
	m.simRepo = buildSandboxSimulation()

	explicitSimulation := os.Getenv("GITARC_MODE") == "simulation" || extraRepoPath == "sim"
	forceLive := os.Getenv("GITARC_MODE") == "live"
	shouldBeLive := !explicitSimulation && (extraRepoPath != "" || forceLive)

	candidates := []repoEntry{}
	if extraRepoPath != "" && extraRepoPath != "sim" {
		abs, err := filepath.Abs(extraRepoPath)
		if err == nil && isGitDir(abs) {
			candidates = append(candidates, repoEntry{
				id:   "live-custom",
				name: filepath.Base(abs) + " (Canlı)",
				path: abs,
			})
		}
	}
	if envRepo := os.Getenv("GITARC_REPO"); envRepo != "" && envRepo != "sim" {
		if abs, err := filepath.Abs(envRepo); err == nil && isGitDir(abs) {
			already := false
			for _, c := range candidates {
				if filepath.Clean(c.path) == filepath.Clean(abs) {
					already = true
					break
				}
			}
			if !already {
				candidates = append(candidates, repoEntry{
					id:   "live-env",
					name: filepath.Base(abs) + " (Canlı)",
					path: abs,
				})
			}
		}
	}
	if wd, err := os.Getwd(); err == nil && isGitDir(wd) {
		already := false
		for _, c := range candidates {
			if filepath.Clean(c.path) == filepath.Clean(wd) {
				already = true
				break
			}
		}
		if !already {
			candidates = append(candidates, repoEntry{
				id:   "live-cwd",
				name: filepath.Base(wd) + " (Canlı)",
				path: wd,
			})
		}
	}

	// Load persisted user projects from ~/.gitarc/projects.json
	saved := loadSavedProjects()
	for _, sp := range saved {
		if isGitDir(sp.Path) {
			already := false
			for _, c := range candidates {
				if filepath.Clean(c.path) == filepath.Clean(sp.Path) {
					already = true
					break
				}
			}
			if !already {
				candidates = append(candidates, repoEntry{
					id:   sp.ID,
					name: sp.Name + " (Kayıtlı)",
					path: sp.Path,
				})
			}
		}
	}
	m.liveRepos = candidates

	// Canlı mod hedeflenmişse ve geçerli bir canlı git deposu varsa ona bağlan.
	if shouldBeLive && len(candidates) > 0 {
		for _, c := range candidates {
			if isGitDir(c.path) {
				m.activeRepoID = c.id
				break
			}
		}
	}
	return m
}

func isGitDir(p string) bool {
	if p == "" {
		return false
	}
	st, err := os.Stat(filepath.Join(p, ".git"))
	return err == nil && st.IsDir()
}

func (m *Manager) repoOptionsLocked() []model.RepoOption {
	openCount := 0
	for _, b := range m.simRepo.branches {
		if b.State == "open" && b.PR != nil {
			openCount++
		}
	}
	opts := []model.RepoOption{
		{
			ID:            "sim-sandbox",
			Name:          "GitArc Sandbox (Temel Demo)",
			Path:          "(Bellek İçi Sanal Depo)",
			Mode:          "simulation",
			DefaultBranch: "main",
			OpenPRs:       openCount,
			Ahead:         m.simRepo.repo.Ahead,
			Behind:        m.simRepo.repo.Behind,
		},
		{
			ID:            "sim-enterprise",
			Name:          "MegaCorp Platform (50 Geliştirici · Enterprise Demo)",
			Path:          "(Enterprise Çoklu Dal Simülasyonu)",
			Mode:          "simulation",
			DefaultBranch: "main",
			OpenPRs:       14,
			Ahead:         3,
			Behind:        0,
		},
	}
	for _, lr := range m.liveRepos {
		opts = append(opts, model.RepoOption{
			ID:            lr.id,
			Name:          lr.name,
			Path:          lr.path,
			Mode:          "live",
			DefaultBranch: "main",
		})
	}
	return opts
}

// Snapshot returns the current CockpitSnapshot for the active repository.
func (m *Manager) Snapshot() (model.CockpitSnapshot, error) {
	m.mu.RLock()
	active := m.activeRepoID
	opts := m.repoOptionsLocked()
	var live *repoEntry
	for i := range m.liveRepos {
		if m.liveRepos[i].id == active {
			entry := m.liveRepos[i]
			live = &entry
			break
		}
	}
	if live == nil {
		snap := m.buildSimSnapshotLocked(opts)
		m.mu.RUnlock()
		return snap, nil
	}
	m.mu.RUnlock()

	snap, err := CollectLiveRepo(live.id, live.name, live.path, opts)
	if err != nil {
		m.recordLog("error", "Snapshot", live.name, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	m.mu.RLock()
	snap.Repo.TeamMode = m.teamMode
	snap.Repo.AppVersion = AppVersion
	snap.Repo.LatestVersion = m.latestVer
	snap.Repo.UpdateAvailable = m.updateAvail
	m.mu.RUnlock()
	snap.PendingProposal = m.GetPendingProposal()
	snap.Logs = m.recentLogs()
	return snap, nil
}

// SetTeamMode toggles multi-user shared server protection (locks shared HEAD mutations).
func (m *Manager) SetTeamMode(enabled bool) {
	m.mu.Lock()
	m.teamMode = enabled
	if enabled {
		m.recordLogLocked("info", "Team Server Modu", "", "Paylaşılan sunucu modu aktif; yerel HEAD checkout/amend kilitlendi", "")
	}
	m.mu.Unlock()
}

// IsTeamMode reports whether shared server HEAD protection is active.
func (m *Manager) IsTeamMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.teamMode
}

func (m *Manager) recordLogLocked(level, action, branch, message, detail string) {
	m.logSeq++
	entry := model.ActionLogEntry{
		ID:        fmt.Sprintf("log-%d", m.logSeq),
		Timestamp: time.Now().Format("15:04:05"),
		Level:     level,
		Action:    action,
		Branch:    branch,
		Message:   strings.TrimSpace(message),
		Detail:    strings.TrimSpace(detail),
	}
	m.logs = append([]model.ActionLogEntry{entry}, m.logs...)
	if len(m.logs) > 50 {
		m.logs = m.logs[:50]
	}
}

func (m *Manager) recordLog(level, action, branch, message, detail string) {
	m.mu.Lock()
	m.recordLogLocked(level, action, branch, message, detail)
	m.mu.Unlock()
}

func (m *Manager) logLiveOutcome(action, branch, okMsg string, snap model.CockpitSnapshot, err error) (model.CockpitSnapshot, error) {
	if err != nil {
		m.recordLog("error", action, branch, err.Error(), "")
		return snap, err
	}
	m.recordLog("ok", action, branch, okMsg, "")
	snap.Logs = m.recentLogs()
	return snap, nil
}

func (m *Manager) recentLogsLocked() []model.ActionLogEntry {
	if len(m.logs) == 0 {
		return nil
	}
	out := make([]model.ActionLogEntry, len(m.logs))
	copy(out, m.logs)
	return out
}

func (m *Manager) recentLogs() []model.ActionLogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.recentLogsLocked()
}

// ClearLogs empties the in-memory cockpit operation/error log buffer.
func (m *Manager) ClearLogs() (model.CockpitSnapshot, error) {
	m.mu.Lock()
	m.logs = nil
	m.mu.Unlock()
	return m.Snapshot()
}

// SwitchRepo switches the active repository between simulation and discovered live repos.
func (m *Manager) SwitchRepo(repoID string) (model.CockpitSnapshot, error) {
	m.mu.Lock()
	if repoID == "sim-sandbox" || repoID == "sim-keylab" {
		m.activeRepoID = "sim-sandbox"
		m.simRepo = buildSandboxSimulation()
		opts := m.repoOptionsLocked()
		snap := m.buildSimSnapshotLocked(opts)
		m.mu.Unlock()
		return snap, nil
	}
	if repoID == "sim-enterprise" {
		m.activeRepoID = "sim-enterprise"
		m.simRepo = buildEnterpriseSimulation()
		opts := m.repoOptionsLocked()
		snap := m.buildSimSnapshotLocked(opts)
		m.mu.Unlock()
		return snap, nil
	}

	valid := false
	for _, lr := range m.liveRepos {
		if lr.id == repoID {
			valid = true
			break
		}
	}
	if !valid {
		m.mu.Unlock()
		return model.CockpitSnapshot{}, fmt.Errorf("bilinmeyen depo kimliği: %s", repoID)
	}
	m.activeRepoID = repoID
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) activeLiveRepo() (*repoEntry, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.liveRepos {
		if m.liveRepos[i].id == m.activeRepoID {
			e := m.liveRepos[i]
			return &e, true
		}
	}
	return nil, false
}

// CreateBranch spawns a new open branch curve from the given spine commit (or latest main).
func (m *Manager) CreateBranch(name, hemisphere, fromSHA string) (model.CockpitSnapshot, error) {
	name = strings.TrimSpace(name)
	if err := validateBranchName(name); err != nil {
		m.recordLog("error", "Dal Oluştur", name, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if hemisphere != "upper" && hemisphere != "lower" {
		hemisphere = "lower"
	}

	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.createBranchLive(live, name, hemisphere, fromSHA)
		return m.logLiveOutcome("Dal Oluştur", name, fmt.Sprintf("%s dalı (%s) açıldı", name, hemisphere), snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, b := range m.simRepo.branches {
		if strings.EqualFold(b.Name, name) {
			statusTxt := "açık"
			if b.State == "merged" {
				statusTxt = "birleşmiş/kalıntı"
			}
			err := fmt.Errorf("'%s' dalı depoda zaten mevcut (%s); lütfen benzersiz bir dal adı belirleyin", name, statusTxt)
			m.recordLogLocked("error", "Dal Oluştur", name, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
	}

	startIdx := len(m.simRepo.nodes) - 1
	baseSHA := m.simRepo.nodes[startIdx].ShortSHA
	if fromSHA != "" {
		for _, n := range m.simRepo.nodes {
			if n.ShortSHA == fromSHA || n.SHA == fromSHA {
				startIdx = n.SpineIndex
				baseSHA = n.ShortSHA
				break
			}
		}
	}

	m.nextSeq++
	newSHA := fmt.Sprintf("%07x", 0x9c4a100+m.nextSeq*17)
	endIdx := startIdx + 1

	var pr *model.PRStatus
	hasRemote := false
	remoteRef := ""
	if hemisphere == "upper" {
		hasRemote = true
		remoteRef = "origin/" + name
		prNum := m.nextPR
		m.nextPR++
		pr = &model.PRStatus{
			Number:      prNum,
			Title:       "feat: " + name,
			Author:      "ziya",
			ReviewState: "REVIEW_REQUIRED",
			CI: &model.CIStatus{
				State:    "RUNNING",
				Workflow: "ci.yml",
				Duration: "0m 14s",
			},
		}
	}

	for i := range m.simRepo.branches {
		m.simRepo.branches[i].IsHEAD = false
	}

	m.simRepo.branches = append(m.simRepo.branches, model.BranchArc{
		ID:               fmt.Sprintf("br-%d-%s", m.nextSeq, sanitizeID(name)),
		Name:             name,
		Hemisphere:       hemisphere,
		State:            "open",
		StartSpineIndex:  startIdx,
		EndSpineIndex:    endIdx,
		BaseSHA:          baseSHA,
		HasRemote:        hasRemote,
		RemoteRef:        remoteRef,
		PR:               pr,
		HasLocal:         true,
		LocalSHA:         newSHA,
		IsHEAD:           true,
		AheadMain:        1,
		BehindMain:       0,
		UncommittedCount: 0,
		LastCommitMsg:    fmt.Sprintf("wip: %s için ilk iskelet", name),
		ChangedFiles:     []string{"go/internal/gitarc/new_feature.go"},
	})

	m.recordLogLocked("ok", "Dal Oluştur", name, fmt.Sprintf("%s dalı açıldı", name), "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

	// CheckoutBranch switches HEAD to the given branch (simulation or `git switch`).
func (m *Manager) CheckoutBranch(branchID string) (model.CockpitSnapshot, error) {
	if m.IsTeamMode() {
		err := fmt.Errorf("Team Server modunda paylaşılan çalışma dizini (HEAD) kilitlidir; tüm PR merge, izole worktree çakışma çözümü ve uzak dal işlemleri aktiftir")
		m.recordLog("error", "Dala Geçiş", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.checkoutBranchLive(live, branchID)
		return m.logLiveOutcome("Dala Geçiş", branchID, "Aktif çalışma dalı değiştirildi", snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for i := range m.simRepo.branches {
		if m.simRepo.branches[i].ID == branchID {
			if m.simRepo.branches[i].State != "open" {
				err := fmt.Errorf("yalnızca açık dallara geçiş yapılabilir")
				m.recordLogLocked("error", "Dala Geçiş", branchID, err.Error(), "")
				return model.CockpitSnapshot{}, err
			}
			m.simRepo.branches[i].IsHEAD = true
			found = true
		} else {
			m.simRepo.branches[i].IsHEAD = false
		}
	}
	if !found {
		err := fmt.Errorf("dal bulunamadı: %s", branchID)
		m.recordLogLocked("error", "Dala Geçiş", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	m.recordLogLocked("ok", "Dala Geçiş", branchID, "Aktif çalışma dalı değiştirildi", "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// MergeBranch closes an open half-parabola onto a new merge commit node on the main spine,
// turning it into a complete parabolic arch. strategy can be "merge", "squash", or "rebase".
func (m *Manager) MergeBranch(branchID, strategy string) (model.CockpitSnapshot, error) {
	strategy = normalizeMergeStrategy(strategy)
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.mergeBranchLive(live, branchID, strategy)
		return m.logLiveOutcome("Merge ("+strategy+")", branchID, fmt.Sprintf("%s birleştirildi", branchID), snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	idx := -1
	for i := range m.simRepo.branches {
		if m.simRepo.branches[i].ID == branchID {
			idx = i
			break
		}
	}
	if idx < 0 {
		err := fmt.Errorf("dal bulunamadı: %s", branchID)
		m.recordLogLocked("error", "Merge ("+strategy+")", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	b := &m.simRepo.branches[idx]
	if b.State == "merged" {
		return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
	}
	if b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved {
		err := fmt.Errorf("%s dalında çözülmemiş çakışma var; önce Auto-Resolve çalıştırın", b.Name)
		m.recordLogLocked("error", "Merge ("+strategy+")", b.Name, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
		err := fmt.Errorf("%s dalında CI testleri başarısız; merge öncesi hatayı düzeltin", b.Name)
		m.recordLogLocked("error", "Merge ("+strategy+")", b.Name, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}

	m.nextSeq++
	newIdx := len(m.simRepo.nodes)
	mergeShort := fmt.Sprintf("%07x", 0xa7f1200+m.nextSeq*31)
	subject := fmt.Sprintf("Merge branch '%s' into main", b.Name)
	if b.PR != nil {
		subject = fmt.Sprintf("Merge pull request #%d from syhgorath/%s", b.PR.Number, b.Name)
		b.PR.ReviewState = "MERGED"
		if b.PR.CI != nil {
			b.PR.CI.State = "SUCCESS"
		}
	}

	for i := range m.simRepo.nodes {
		m.simRepo.nodes[i].IsHEAD = false
	}
	m.simRepo.nodes = append(m.simRepo.nodes, model.CommitNode{
		SHA:          mergeShort + "000000000000000000000000000000000",
		ShortSHA:     mergeShort,
		Subject:      subject,
		Author:       "ziya",
		RelativeTime: "az önce",
		SpineIndex:   newIdx,
		IsMerge:      true,
		IsHEAD:       true,
	})

	b.Hemisphere = "upper"
	b.State = "merged"
	b.EndSpineIndex = newIdx
	b.MergeSHA = mergeShort
	b.HasRemote = true
	b.OriginGone = true
	b.IsHEAD = false
	b.UncommittedCount = 0
	m.simRepo.repo.Ahead++
	m.recordLogLocked("ok", "Merge ("+strategy+")", b.Name, fmt.Sprintf("%s birleştirildi", b.Name), "")

	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// CommitWorkingTree stages uncommitted changes on a new or current branch, commits them,
// and optionally pushes and opens a GitHub Pull Request without touching a terminal.
func (m *Manager) CommitWorkingTree(branchName, commitMsg, prTitle, prBody string, pushAndPR bool, explicitFiles ...[]string) (model.CockpitSnapshot, error) {
	var files []string
	if len(explicitFiles) > 0 {
		files = explicitFiles[0]
	}
	if m.IsTeamMode() {
		err := fmt.Errorf("Team Server modunda paylaşılan çalışma dizini (HEAD) kilitlidir")
		m.recordLog("error", "Konsolsuz Commit & PR", branchName, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	commitMsg = strings.TrimSpace(commitMsg)
	if commitMsg == "" {
		err := fmt.Errorf("commit mesajı boş olamaz")
		m.recordLog("error", "Konsolsuz Commit & PR", branchName, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	branchName = strings.TrimSpace(branchName)
	if branchName != "" {
		if err := validateBranchName(branchName); err != nil {
			m.recordLog("error", "Konsolsuz Commit & PR", branchName, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
	}

	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.commitWorkingTreeLive(live, branchName, commitMsg, prTitle, prBody, pushAndPR, files)
		okMsg := fmt.Sprintf("Değişiklikler '%s' dalına commitlendi", branchName)
		if pushAndPR {
			okMsg = fmt.Sprintf("Dal açıldı, commitlendi, pushlandı ve PR oluşturuldu (%s)", branchName)
		}
		return m.logLiveOutcome("Konsolsuz Commit & PR", branchName, okMsg, snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, f := range files {
		if isSecret, pat := IsSecretPath(f, DefaultGuardConfig.SecretPatterns); isSecret {
			err := fmt.Errorf("GÜVENLİK ENGELİ: Gizli/korumalı dosya tespit edildi: '%s' (eşleşen kural: %s). Commit engellendi!", f, pat)
			m.recordLogLocked("error", "Konsolsuz Commit & PR", branchName, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
	}

	if pushAndPR && branchName != "" {
		for _, b := range m.simRepo.branches {
			if b.Name == branchName && (b.State == "merged" || b.OriginGone) {
				err := fmt.Errorf("'%s' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın üzerine push yapılamaz, lütfen yeni bir dal açın", branchName)
				m.recordLogLocked("error", "Konsolsuz Commit & PR", branchName, err.Error(), "")
				return model.CockpitSnapshot{}, err
			}
		}
	}

	if branchName == "" {
		branchName = "feat/working-tree"
	}
	m.simRepo.repo.DirtyCount = 0
	for i := range m.simRepo.branches {
		m.simRepo.branches[i].IsHEAD = false
	}

	startIdx := len(m.simRepo.nodes) - 1
	baseSHA := m.simRepo.nodes[startIdx].ShortSHA
	m.nextSeq++
	newSHA := fmt.Sprintf("%07x", 0x9e1c200+m.nextSeq*19)
	hemi := "lower"
	hasRemote := false
	remoteRef := ""
	var pr *model.PRStatus
	if pushAndPR {
		hemi = "upper"
		hasRemote = true
		remoteRef = "origin/" + branchName
		prNum := m.nextPR
		m.nextPR++
		t := strings.TrimSpace(prTitle)
		if t == "" {
			t = commitMsg
		}
		pr = &model.PRStatus{
			Number:      prNum,
			Title:       t,
			Body:        strings.TrimSpace(prBody),
			Author:      "ziya",
			ReviewState: "REVIEW_REQUIRED",
			CI:          &model.CIStatus{State: "RUNNING", Workflow: "ci.yml", Duration: "0m 12s"},
		}
	}

	m.simRepo.branches = append(m.simRepo.branches, model.BranchArc{
		ID:              fmt.Sprintf("br-%d-%s", m.nextSeq, sanitizeID(branchName)),
		Name:            branchName,
		Hemisphere:      hemi,
		State:           "open",
		StartSpineIndex: startIdx,
		EndSpineIndex:   startIdx + 1,
		BaseSHA:         baseSHA,
		HasRemote:       hasRemote,
		RemoteRef:       remoteRef,
		PR:              pr,
		HasLocal:        true,
		LocalSHA:        newSHA,
		Author:          "ziya",
		IsHEAD:          true,
		AheadMain:       1,
		LastCommitMsg:   commitMsg,
		ChangedFiles:    []string{"CHANGELOG.md"},
	})
	m.recordLogLocked("ok", "Konsolsuz Commit & PR", branchName, commitMsg, "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// PromoteBranch pushes a lower-hemisphere local branch to remote and opens a PR,
// flipping it into the upper hemisphere.
func (m *Manager) PromoteBranch(branchID string) (model.CockpitSnapshot, error) {
	return m.PromoteBranchWithMeta(branchID, "", "")
}

// PromoteBranchWithMeta pushes a local branch to remote and creates a PR with custom title and body.
func (m *Manager) PromoteBranchWithMeta(branchID, prTitle, prBody string) (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.promoteBranchLive(live, branchID, prTitle, prBody)
		return m.logLiveOutcome("Push & PR Aç", branchID, "Dal origin'e pushlandı ve PR açıldı", snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID {
			if b.Hemisphere == "upper" && b.PR != nil {
				return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
			}
			prNum := m.nextPR
			m.nextPR++
			b.Hemisphere = "upper"
			b.HasRemote = true
			b.RemoteRef = "origin/" + b.Name
			b.RemoteAhead = 0
			b.RemoteBehind = 0
			title := strings.TrimSpace(prTitle)
			if title == "" {
				title = "feat: " + b.Name
			}
			b.PR = &model.PRStatus{
				Number:      prNum,
				Title:       title,
				Body:        strings.TrimSpace(prBody),
				Author:      "ziya",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "SUCCESS",
					Workflow: "ci.yml",
					Duration: "1m 18s",
				},
			}
			m.recordLogLocked("ok", "Push & PR Aç", b.Name, fmt.Sprintf("PR #%d açıldı", prNum), "")
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	err := fmt.Errorf("dal bulunamadı: %s", branchID)
	m.recordLogLocked("error", "Push & PR Aç", branchID, err.Error(), "")
	return model.CockpitSnapshot{}, err
}

// UpdatePRMeta edits an open PR's title and description.
func (m *Manager) UpdatePRMeta(branchID, title, body string) (model.CockpitSnapshot, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		err := fmt.Errorf("PR başlığı boş olamaz")
		m.recordLog("error", "PR Güncelleme", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.updatePRMetaLive(live, branchID, title, body)
		return m.logLiveOutcome("PR Güncelleme", branchID, "PR başlık/açıklaması güncellendi", snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID {
			if b.PR == nil {
				err := fmt.Errorf("%s dalı için açık PR yok", b.Name)
				m.recordLogLocked("error", "PR Güncelleme", b.Name, err.Error(), "")
				return model.CockpitSnapshot{}, err
			}
			b.PR.Title = title
			b.PR.Body = strings.TrimSpace(body)
			m.recordLogLocked("ok", "PR Güncelleme", b.Name, fmt.Sprintf("PR #%d güncellendi", b.PR.Number), "")
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	err := fmt.Errorf("dal bulunamadı: %s", branchID)
	m.recordLogLocked("error", "PR Güncelleme", branchID, err.Error(), "")
	return model.CockpitSnapshot{}, err
}

// AmendLastCommit safely updates only the commit message of the active HEAD feature branch.
func (m *Manager) AmendLastCommit(branchID, message string) (model.CockpitSnapshot, error) {
	if m.IsTeamMode() {
		err := fmt.Errorf("Team Server modunda paylaşılan çalışma dizini (HEAD) kilitlidir; tüm PR merge, izole worktree çakışma çözümü ve uzak dal işlemleri aktiftir")
		m.recordLog("error", "Commit Mesajı Düzenle", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	message = strings.TrimSpace(message)
	if message == "" {
		err := fmt.Errorf("commit mesajı boş olamaz")
		m.recordLog("error", "Commit Mesajı Düzenle", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.amendLastCommitLive(live, branchID, message)
		return m.logLiveOutcome("Commit Mesajı Düzenle", branchID, "Son commit mesajı güncellendi", snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID {
			if !b.IsHEAD || b.State != "open" {
				err := fmt.Errorf("yalnızca aktif çalışma dalının (HEAD) son commit mesajı değiştirilebilir")
				m.recordLogLocked("error", "Commit Mesajı Düzenle", b.Name, err.Error(), "")
				return model.CockpitSnapshot{}, err
			}
			b.LastCommitMsg = message
			m.recordLogLocked("ok", "Commit Mesajı Düzenle", b.Name, "Son commit mesajı güncellendi", "")
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	err := fmt.Errorf("dal bulunamadı: %s", branchID)
	m.recordLogLocked("error", "Commit Mesajı Düzenle", branchID, err.Error(), "")
	return model.CockpitSnapshot{}, err
}

func normalizeMergeStrategy(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "squash", "rebase", "merge", "auto":
		return strings.ToLower(strings.TrimSpace(s))
	default:
		return "merge"
	}
}
func (m *Manager) FixCI(branchID string) (model.CockpitSnapshot, error) {
	if _, ok := m.activeLiveRepo(); ok {
		err := fmt.Errorf("canlı modda otomatik kod düzeltmesi kapalıdır; hatayı yerelde düzeltip pushlayın")
		m.recordLog("error", "CI Düzeltme", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID {
			if b.PR != nil && b.PR.CI != nil {
				b.PR.CI.State = "SUCCESS"
				b.PR.CI.FailedStep = ""
				b.PR.CI.RawLog = ""
				b.PR.CI.Extracts = nil
				b.PR.CI.Duration = "0m 52s"
				b.PR.ReviewState = "APPROVED"
			}
			b.LastCommitMsg = "fix(shellgold): guard slice upper bound & update golden"
			b.UncommittedCount = 0
			m.recordLogLocked("ok", "CI Düzeltme", b.Name, "CI testleri yeşile döndü", "")
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	err := fmt.Errorf("dal bulunamadı: %s", branchID)
	m.recordLogLocked("error", "CI Düzeltme", branchID, err.Error(), "")
	return model.CockpitSnapshot{}, err
}

// AutoResolveConflict resolves CHANGELOG.md / lockfile conflicts by preserving both sides.
func (m *Manager) AutoResolveConflict(branchID string) (model.CockpitSnapshot, error) {
	return m.ResolveBranchConflict(branchID, "union", nil, true)
}

// PruneGone cleans up branches whose remote tracking ref is gone or already merged.
func (m *Manager) PruneGone() (model.CockpitSnapshot, int, error) {
	if live, ok := m.activeLiveRepo(); ok {
		snap, removed, err := m.pruneGoneLive(live)
		snap, err = m.logLiveOutcome("Kalıntı Temizliği", "", fmt.Sprintf("%d artık dal temizlendi", removed), snap, err)
		return snap, removed, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var kept []model.BranchArc
	removed := 0
	for _, b := range m.simRepo.branches {
		if b.State == "merged" {
			if b.HasLocal || b.OriginGone {
				b.HasLocal = false
				b.OriginGone = false
				b.IsHEAD = false
				removed++
			}
			kept = append(kept, b)
			continue
		}
		if b.OriginGone || b.State == "gone" {
			removed++
			continue
		}
		kept = append(kept, b)
	}
	m.simRepo.branches = kept
	m.recordLogLocked("ok", "Kalıntı Temizliği", "", fmt.Sprintf("%d artık dal temizlendi", removed), "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), removed, nil
}

// DeleteBranch removes a single branch from the simulation graph or safely deletes
// a local merged branch in live mode (`git branch -d`).
func (m *Manager) DeleteBranch(branchID string) (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.deleteBranchLive(live, branchID)
		return m.logLiveOutcome("Dal Silme", branchID, fmt.Sprintf("%s kaldırıldı", branchID), snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []model.BranchArc
	found := false
	deletedName := branchID
	for _, b := range m.simRepo.branches {
		if b.ID == branchID {
			found = true
			deletedName = b.Name
			if b.State == "merged" && (b.HasLocal || b.OriginGone) {
				b.HasLocal = false
				b.OriginGone = false
				b.IsHEAD = false
				kept = append(kept, b)
			}
			continue
		}
		kept = append(kept, b)
	}
	if !found {
		err := fmt.Errorf("dal bulunamadı: %s", branchID)
		m.recordLogLocked("error", "Dal Silme", branchID, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	m.simRepo.branches = kept
	m.recordLogLocked("ok", "Dal Silme", deletedName, fmt.Sprintf("%s kaldırıldı", deletedName), "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// SyncSpine clears ahead/behind skew between local main and origin/main.
func (m *Manager) SyncSpine() (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		snap, err := m.syncSpineLive(live)
		return m.logLiveOutcome("Omurga Sync", "main", "main ↔ origin/main eşitlendi", snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.simRepo.repo.Behind > 0 {
		m.nextSeq++
		idx := len(m.simRepo.nodes)
		short := fmt.Sprintf("%07x", 0xb309000+m.nextSeq*19)
		m.simRepo.nodes = append(m.simRepo.nodes, model.CommitNode{
			SHA:          short + "000000000000000000000000000000000",
			ShortSHA:     short,
			Subject:      "chore(index): sync origin/main signed component lock",
			Author:       "github-actions",
			RelativeTime: "az önce",
			SpineIndex:   idx,
		})
	}
	m.simRepo.repo.Ahead = 0
	m.simRepo.repo.Behind = 0
	m.recordLogLocked("ok", "Omurga Sync", "main", "main ↔ origin/main eşitlendi", "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// ResetSimulation restores the initial curated simulation state.
func (m *Manager) ResetSimulation() model.CockpitSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID == "sim-enterprise" {
		m.simRepo = buildEnterpriseSimulation()
	} else {
		m.simRepo = buildSandboxSimulation()
		m.activeRepoID = "sim-sandbox"
	}
	m.nextPR = 425
	m.nextSeq = 10
	return m.buildSimSnapshotLocked(m.repoOptionsLocked())
}

func (m *Manager) buildSimSnapshotLocked(opts []model.RepoOption) model.CockpitSnapshot {
	branches := make([]model.BranchArc, len(m.simRepo.branches))
	copy(branches, m.simRepo.branches)
	for i := range branches {
		if branches[i].PR != nil && branches[i].PR.CI != nil && branches[i].PR.CI.RawLog != "" {
			ciCopy := *branches[i].PR.CI
			ciCopy.Extracts = ExtractCILog(ciCopy.RawLog)
			prCopy := *branches[i].PR
			prCopy.CI = &ciCopy
			branches[i].PR = &prCopy
		}
		branches[i].CanAmendCommit = branches[i].IsHEAD && branches[i].State == "open" && branches[i].AheadMain > 0
		branches[i].SuggestedCmd = BuildSuggestedCommand(branches[i], m.simRepo.repo.DefaultBranch)
	}

	ComputeCrossPRCollisions(branches)
	AssignLanes(branches)
	diags := BuildDiagnostics(branches)

	nodes := make([]model.CommitNode, len(m.simRepo.nodes))
	copy(nodes, m.simRepo.nodes)

	repoMeta := m.simRepo.repo
	repoMeta.TeamMode = m.teamMode
	repoMeta.AppVersion = AppVersion
	repoMeta.LatestVersion = m.latestVer
	repoMeta.UpdateAvailable = m.updateAvail
	repoMeta.SecretPatterns = DefaultGuardConfig.SecretPatterns
	repoMeta.NoGitAddAll = DefaultGuardConfig.NoGitAddAll

	return model.CockpitSnapshot{
		Repo:            repoMeta,
		Repos:           opts,
		Nodes:           nodes,
		Branches:        branches,
		Diagnostics:     diags,
		Logs:            m.recentLogsLocked(),
		PendingProposal: m.pendingProposal,
		GeneratedAt:     time.Now(),
	}
}

func (m *Manager) getDiffStatSimLocked() model.DiffStat {
	dirty := m.simRepo.repo.DirtyCount
	if dirty == 0 {
		return model.DiffStat{Stat: "0 files changed"}
	}
	return model.DiffStat{
		Stat:         "CHANGELOG.md | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)",
		FilesChanged: 1,
		Insertions:   1,
		Deletions:    1,
		TotalLines:   2,
		IsLargeDiff:  false,
	}
}

func buildSandboxSimulation() simWorkspace {
	nodes := []model.CommitNode{
		{SHA: "19a4c01", ShortSHA: "19a4c01", Subject: "chore(release): prepare v4.0.14 signed archive", Author: "ziya", RelativeTime: "6 gün önce", SpineIndex: 0, Tag: "v4.0.14"},
		{SHA: "2f81d12", ShortSHA: "2f81d12", Subject: "refactor(runner): unify sudo askpass gate", Author: "ziya", RelativeTime: "5 gün önce", SpineIndex: 1},
		{SHA: "3c19e44", ShortSHA: "3c19e44", Subject: "Merge PR #412 feat/vault-idle-lock", Author: "ziya", RelativeTime: "4 gün önce", SpineIndex: 2, IsMerge: true},
		{SHA: "4b70a89", ShortSHA: "4b70a89", Subject: "docs(standard): document catalog items contract", Author: "ziya", RelativeTime: "3 gün önce", SpineIndex: 3, Tag: "v4.0.15"},
		{SHA: "5e02b11", ShortSHA: "5e02b11", Subject: "Merge PR #415 fix/usb-exfat-update", Author: "ziya", RelativeTime: "2 gün önce", SpineIndex: 4, IsMerge: true},
		{SHA: "6d44f08", ShortSHA: "6d44f08", Subject: "feat(components): verify index_signers timestamp", Author: "ziya", RelativeTime: "1 gün önce", SpineIndex: 5},
		{SHA: "7a91c33", ShortSHA: "7a91c33", Subject: "Merge PR #418 feat/utility-items-catalog", Author: "ziya", RelativeTime: "18 saat önce", SpineIndex: 6, IsMerge: true, Tag: "v4.0.16"},
		{SHA: "8e35d02", ShortSHA: "8e35d02", Subject: "chore(lock): bump utility 0.2.0 sha256", Author: "ziya", RelativeTime: "6 saat önce", SpineIndex: 7},
		{SHA: "9f10b77", ShortSHA: "9f10b77", Subject: "test(csp): enforce style-src-attr none", Author: "ziya", RelativeTime: "2 saat önce", SpineIndex: 8, IsHEAD: true},
	}

	ciFailLog := `2026-11-16T07:12:04.112Z ##[group]Run cd go && go vet ./... && go test ./... -short
2026-11-16T07:12:05.840Z === RUN   TestShellGoldManifestParity
2026-11-16T07:12:05.912Z --- PASS: TestShellGoldManifestParity (0.07s)
2026-11-16T07:12:06.103Z === RUN   TestShellGoldCompare
2026-11-16T07:12:06.489Z     compare_test.go:118: golden output mismatch on key 'hardening_pve_apply': got "true", want "false"
2026-11-16T07:12:06.520Z --- FAIL: TestShellGoldCompare (0.42s)
2026-11-16T07:12:06.521Z === RUN   TestShellGoldPanicBoundary
2026-11-16T07:12:06.529Z panic: runtime error: index out of range [3] with length 3
2026-11-16T07:12:06.530Z goroutine 42 [running]:
2026-11-16T07:12:06.531Z keylab/internal/shellgold.ParseDiffRow(...)
2026-11-16T07:12:06.531Z 	go/internal/shellgold/compare.go:94 +0x1b4
2026-11-16T07:12:06.532Z FAIL	keylab/internal/shellgold	0.614s
2026-11-16T07:12:06.540Z ##[error]Process completed with exit code 1.`

	changelogPreview := `<<<<<< HEAD (main @ 8e35d02)
## [4.0.16] - 2026-11-15
- utility 0.2.0 imzalı kilit özeti güncellendi (components.lock)
- WebKit misafir mod önbellek temizliği eklendi
======
## [Unreleased] - feat/terminal-audit-export
- terminal-audit.log CSV ve JSON dışa aktarma uçları eklendi
- components.lock imzalı doğrulama satırı eklendi
>>>>>> origin/feat/terminal-audit-export

[git merge-tree --write-tree] Otomatik Union Birleştirme Hazır:
✓ main (4.0.16) + feature ([Unreleased]) kronolojik sırada birleştirilir; hiçbir satır kaybolmaz.`

	branches := []model.BranchArc{
		// Merged full parabolic arches
		{
			ID:              "br-merged-412",
			Name:            "feat/vault-idle-lock",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 0,
			EndSpineIndex:   2,
			BaseSHA:         "19a4c01",
			MergeSHA:        "3c19e44",
			HasRemote:       true,
			RemoteRef:       "origin/feat/vault-idle-lock",
			OriginGone:      false,
			PR: &model.PRStatus{
				Number:      412,
				Title:       "feat(vault): idle timeout lock & PBKDF2 hardening",
				Author:      "ziya",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "1m 24s"},
			},
			HasLocal:      false,
			LastCommitMsg: "vault: auto-lock after KEYLAB_VAULT_IDLE_MIN",
			ChangedFiles:  []string{"go/internal/vault/vault.go"},
		},
		{
			ID:              "br-merged-415",
			Name:            "fix/usb-exfat-update",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 3,
			EndSpineIndex:   4,
			BaseSHA:         "4b70a89",
			MergeSHA:        "5e02b11",
			HasRemote:       false,
			RemoteRef:       "origin/fix/usb-exfat-update",
			OriginGone:      true,
			PR: &model.PRStatus{
				Number:      415,
				Title:       "fix(selfupdate): atomic app replacement on exFAT USB drives",
				Author:      "ziya",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "1m 11s"},
			},
			HasLocal:      true,
			LocalSHA:      "5e02b11",
			LastCommitMsg: "selfupdate: avoid cross-device rename on FAT/exFAT",
			ChangedFiles:  []string{"go/internal/selfupdate/update.go"},
		},
		{
			ID:              "br-merged-418",
			Name:            "feat/utility-items-catalog",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 1,
			EndSpineIndex:   6,
			BaseSHA:         "2f81d12",
			MergeSHA:        "7a91c33",
			HasRemote:       false,
			RemoteRef:       "origin/feat/utility-items-catalog",
			OriginGone:      true,
			PR: &model.PRStatus{
				Number:      418,
				Title:       "feat(manifest): item_var & scoped requires cache",
				Author:      "ziya",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "2m 03s"},
			},
			HasLocal:      true,
			LocalSHA:      "7a91c33",
			LastCommitMsg: "manifest: scope prerequisite cache by item:mode",
			ChangedFiles:  []string{"go/internal/manifest/items.go"},
		},

		// Open Upper Hemisphere (Remote / PR / CI)
		{
			ID:              "br-open-421",
			Name:            "feat/terminal-audit-export",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 4,
			EndSpineIndex:   7,
			BaseSHA:         "5e02b11",
			HasRemote:       true,
			RemoteRef:       "origin/feat/terminal-audit-export",
			PR: &model.PRStatus{
				Number:      421,
				Title:       "feat(report): export terminal-audit.log with formula guard",
				Author:      "ziya",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "1m 38s"},
			},
			HasLocal:         true,
			LocalSHA:         "c18e420",
			Author:           "ziya",
			IsHEAD:           false,
			AheadMain:        3,
			BehindMain:       2,
			RemoteAhead:      0,
			RemoteBehind:     0,
			UncommittedCount: 0,
			LastCommitMsg:    "report: mask token columns in terminal audit CSV",
			ChangedFiles:     []string{"CHANGELOG.md", "components.lock", "go/internal/report/audit.go"},
			Conflict: &model.ConflictDetail{
				HasConflict:       true,
				Files:             []string{"CHANGELOG.md", "components.lock"},
				IsChangelogOrLock: true,
				Strategy:          "union-changelog",
				PreviewUnified:    changelogPreview,
			},
		},
		{
			ID:              "br-open-422",
			Name:            "fix/shellgold-diff-parser",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 5,
			EndSpineIndex:   8,
			BaseSHA:         "6d44f08",
			HasRemote:       true,
			RemoteRef:       "origin/fix/shellgold-diff-parser",
			PR: &model.PRStatus{
				Number:      422,
				Title:       "fix(shellgold): parse 3-column golden output diff",
				Author:      "ziya",
				ReviewState: "CHANGES_REQUESTED",
				CI: &model.CIStatus{
					State:      "FAILURE",
					Workflow:   "Go Vet & ShellGold CI",
					RunID:      "118402914",
					Duration:   "0m 49s",
					FailedStep: "Run go test ./internal/shellgold",
					RawLog:     ciFailLog,
				},
			},
			HasLocal:         true,
			LocalSHA:         "d42b991",
			Author:           "ziya",
			IsHEAD:           true,
			AheadMain:        2,
			BehindMain:       1,
			RemoteAhead:      1,
			RemoteBehind:     0,
			UncommittedCount: 2,
			LastCommitMsg:    "shellgold: guard empty column split in compare.go",
			ChangedFiles:     []string{"CHANGELOG.md", "go/internal/shellgold/compare.go", "go/internal/shellgold/compare_test.go"},
		},
		{
			ID:              "br-open-424",
			Name:            "chore/hardening-0.14-lock",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 7,
			EndSpineIndex:   9,
			BaseSHA:         "8e35d02",
			HasRemote:       true,
			RemoteRef:       "origin/chore/hardening-0.14-lock",
			PR: &model.PRStatus{
				Number:      424,
				Title:       "chore(lock): upgrade linux-hardening 0.13.0 -> 0.14.0",
				Author:      "dependabot[bot]",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "Verify Component Signatures",
					Duration: "0m 27s",
				},
			},
			HasLocal:         true,
			LocalSHA:         "e99a014",
			Author:           "dependabot[bot]",
			IsHEAD:           false,
			AheadMain:        1,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "components.lock: update sha256 for linux-hardening 0.14.0",
			ChangedFiles:     []string{"components.lock", "VERSION"},
		},

		// Open Lower Hemisphere (Local / Unpushed)
		{
			ID:               "br-local-wip",
			Name:             "wip/keychain-probe",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  6,
			EndSpineIndex:    8,
			BaseSHA:          "7a91c33",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "f03c812",
			Author:           "ziya",
			IsHEAD:           false,
			AheadMain:        3,
			BehindMain:       1,
			UncommittedCount: 4,
			LastCommitMsg:    "wip(doctor): probe macOS Keychain ACL before ssh-add",
			ChangedFiles:     []string{"go/internal/doctor/keychain.go", "go/main.go"},
		},
		{
			ID:               "br-local-exp",
			Name:             "exp/parabolic-export",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  7,
			EndSpineIndex:    9,
			BaseSHA:          "8e35d02",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "a17c504",
			Author:           "ziya",
			IsHEAD:           false,
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 1,
			LastCommitMsg:    "exp(report): export SVG fleet dependency arc",
			ChangedFiles:     []string{"go/web/js/reports.js"},
		},
	}

	return simWorkspace{
		repo: model.RepoMeta{
			ID:                "sim-sandbox",
			Name:              "GitArc Sandbox (Temel Demo)",
			Path:              "(Bellek İçi Sanal Depo)",
			DefaultBranch:     "main",
			RemoteName:        "origin",
			Ahead:             2,
			Behind:            1,
			DirtyCount:        2,
			Mode:              "simulation",
			VersionTag:        "v4.0.16",
			CurrentUser:       "ziya",
			BranchProtected:   false,
			ProtectionSummary: "Korumasız (Doğrudan push/force-push açık)",
			AppVersion:        AppVersion,
		},
		nodes:    nodes,
		branches: branches,
	}
}

// buildEnterpriseSimulation creates a large-scale repository simulation representing
// an enterprise engineering organization with 4 primary tracks and 50 active developers.
func buildEnterpriseSimulation() simWorkspace {
	// 50 Developers Roster:
	// Alice Chen, Mehmet Yilmaz, David Kim, Elena Rostova, Carlos Silva, Sarah Jenkins,
	// Kenji Sato, Fatima Zahra, Lucas Muller, Ayse Kaya, Priya Patel, Marcus Vance,
	// Emily Blunt, Tarik Ozkan, Olga Ivanova, Chen Wei, Diego Torres, Anna Kowalski,
	// Hassan Ali, Kevin Duque, Rachel Green, Zeynep Demir, Brian Cox, Sophie Martin,
	// Alex Turner, Cem Yilmaz, Victoria Beck, Leo Messi, Elizabeth Swan, Mustafa Ak,
	// John Doe, Jane Smith, Can Erik, Gita Rao, Tom Hanks, Deniz Oz,
	// Charlie Brown, Selin Erdogan, Robert De, Alperen Sen, Clara Oswald, Baris Manco,
	// Emma Watson, Serdar Oz, Natalie Portman, Onur Bay, Yuki Tsunoda, Burak Oz,
	// Mia Wallace, Hakan Sukru.

	nodes := []model.CommitNode{
		{SHA: "a101f01", ShortSHA: "a101f01", Subject: "chore(release): cut v3.0.0 enterprise lts baseline", Author: "alice.chen", RelativeTime: "14 gün önce", SpineIndex: 0, Tag: "v3.0.0"},
		{SHA: "b202e12", ShortSHA: "b202e12", Subject: "feat(gateway): envoy proxy mesh ingress integration", Author: "david.kim", RelativeTime: "12 gün önce", SpineIndex: 1},
		{SHA: "c303d23", ShortSHA: "c303d23", Subject: "Merge PR #790 feat/distributed-cache-redis7", Author: "mehmet.yilmaz", RelativeTime: "11 gün önce", SpineIndex: 2, IsMerge: true},
		{SHA: "d404c34", ShortSHA: "d404c34", Subject: "chore(security): apply cve-2026-4190 kernel patch", Author: "elena.rostova", RelativeTime: "10 gün önce", SpineIndex: 3, Tag: "v3.1.0"},
		{SHA: "e505b45", ShortSHA: "e505b45", Subject: "Merge PR #794 fix/jwt-rsa-key-rotation", Author: "carlos.silva", RelativeTime: "9 gün önce", SpineIndex: 4, IsMerge: true},
		{SHA: "f606a56", ShortSHA: "f606a56", Subject: "feat(observability): opentelemetry trace collector", Author: "sarah.jenkins", RelativeTime: "8 gün önce", SpineIndex: 5},
		{SHA: "0707967", ShortSHA: "0707967", Subject: "Merge PR #798 chore/upgrade-go123", Author: "kenji.sato", RelativeTime: "7 gün önce", SpineIndex: 6, IsMerge: true},
		{SHA: "1808878", ShortSHA: "1808878", Subject: "chore(release): v3.2.0 platform core rollout", Author: "fatima.zahra", RelativeTime: "6 gün önce", SpineIndex: 7, Tag: "v3.2.0"},
		{SHA: "2909789", ShortSHA: "2909789", Subject: "feat(billing): multi-currency stripe webhooks handler", Author: "priya.patel", RelativeTime: "5 gün önce", SpineIndex: 8},
		{SHA: "3a0a69a", ShortSHA: "3a0a69a", Subject: "Merge PR #802 feat/kafka-event-streaming", Author: "lucas.muller", RelativeTime: "4 gün önce", SpineIndex: 9, IsMerge: true},
		{SHA: "4b0b5ab", ShortSHA: "4b0b5ab", Subject: "fix(db): postgres deadlocks on concurrent audit writes", Author: "ayse.kaya", RelativeTime: "3 gün önce", SpineIndex: 10},
		{SHA: "5c0c4bc", ShortSHA: "5c0c4bc", Subject: "Merge PR #807 perf/graphql-query-batching", Author: "marcus.vance", RelativeTime: "2 gün önce", SpineIndex: 11, IsMerge: true},
		{SHA: "6d0d3cd", ShortSHA: "6d0d3cd", Subject: "feat(auth): webauthn passkey biometric authentication", Author: "emily.blunt", RelativeTime: "1 gün önce", SpineIndex: 12},
		{SHA: "7e0e2de", ShortSHA: "7e0e2de", Subject: "Merge PR #810 fix/k8s-pod-disruption-budget", Author: "tarik.ozkan", RelativeTime: "18 saat önce", SpineIndex: 13, IsMerge: true, Tag: "v3.3.0-rc2"},
		{SHA: "8f0f1ef", ShortSHA: "8f0f1ef", Subject: "chore(config): bump production cluster replicas to 64", Author: "olga.ivanova", RelativeTime: "8 saat önce", SpineIndex: 14},
		{SHA: "90100f0", ShortSHA: "90100f0", Subject: "ci(security): enforce codeql & gitleaks scanner", Author: "chen.wei", RelativeTime: "2 saat önce", SpineIndex: 15, IsHEAD: true},
	}

	entCiFailLog := `2026-11-17T06:14:02.100Z ##[group]Testing service mesh tracing spans
2026-11-17T06:14:04.300Z === RUN   TestDistributedTracePropagation
2026-11-17T06:14:05.120Z --- PASS: TestDistributedTracePropagation (0.82s)
2026-11-17T06:14:05.121Z === RUN   TestOpenTelemetrySpanExporter
2026-11-17T06:14:07.450Z     exporter_test.go:88: context deadline exceeded after 2000ms waiting for OTLP gRPC endpoint: 10.240.12.9:4317
2026-11-17T06:14:07.452Z --- FAIL: TestOpenTelemetrySpanExporter (2.33s)
2026-11-17T06:14:07.455Z === RUN   TestSpanPanicRecovery
2026-11-17T06:14:07.460Z panic: nil pointer dereference evaluating spanContext.TraceFlags()
2026-11-17T06:14:07.461Z goroutine 148 [running]:
2026-11-17T06:14:07.462Z platform/internal/telemetry.ExportSpan(...)
2026-11-17T06:14:07.463Z 	services/telemetry/exporter.go:142 +0x2a8
2026-11-17T06:14:07.470Z FAIL	platform/services/telemetry	3.150s
2026-11-17T06:14:07.480Z ##[error]Process completed with exit code 1.`

	entConflictPreview := `<<<<<< HEAD (main @ 8f0f1ef)
replicas: 64
minReplicas: 16
autoscaling:
  targetCPUUtilizationPercentage: 70
  targetMemoryUtilizationPercentage: 75
======
replicas: 32
minReplicas: 8
autoscaling:
  targetCPUUtilizationPercentage: 80
  targetMemoryUtilizationPercentage: 85
>>>>>> origin/feat/auth-v2-jwt-rotation

[git merge-tree --write-tree] Otomatik Çözüm Önerisi:
⚠ Çakışan dosya: deploy/helm/values.yaml (Üretim küme autoscaling eşikleri)`

	branches := []model.BranchArc{
		// 1. DÖRT ANA DAL (Primary Long-Running Tracks)
		// Track 1: develop (Ana geliştirme entegrasyon dalı)
		{
			ID:              "br-track-develop",
			Name:            "develop",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 10,
			EndSpineIndex:   15,
			BaseSHA:         "4b0b5ab",
			HasRemote:       true,
			RemoteRef:       "origin/develop",
			PR: &model.PRStatus{
				Number:      770,
				Title:       "track(develop): active platform integration line",
				Author:      "diego.torres",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Nightly Integration Test", Duration: "4m 12s"},
			},
			HasLocal:         true,
			LocalSHA:         "fa0192b",
			Author:           "diego.torres",
			AheadMain:        5,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "develop: sync nightly service builds across all clusters",
			ChangedFiles:     []string{"services/develop-manifest.yaml"},
		},
		// Track 2: staging (Ön-canlı doğrulama dalı)
		{
			ID:              "br-track-staging",
			Name:            "staging",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/staging",
			PR: &model.PRStatus{
				Number:      775,
				Title:       "track(staging): pre-production validation track",
				Author:      "anna.kowalski",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "E2E Smoke Tests", Duration: "3m 45s"},
			},
			HasLocal:         true,
			LocalSHA:         "fb1283c",
			Author:           "anna.kowalski",
			AheadMain:        3,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "staging: promote RC candidates to stg-us-east-2 cluster",
			ChangedFiles:     []string{"deploy/staging/kustomization.yaml"},
		},
		// Track 3: release/v3.3 (Sürüm hazırlık dalı)
		{
			ID:              "br-track-release-33",
			Name:            "release/v3.3",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 13,
			EndSpineIndex:   15,
			BaseSHA:         "7e0e2de",
			HasRemote:       true,
			RemoteRef:       "origin/release/v3.3",
			PR: &model.PRStatus{
				Number:      780,
				Title:       "track(release): v3.3.0 stabilization and release gate",
				Author:      "hassan.ali",
				ReviewState: "REVIEW_REQUIRED",
				CI:          &model.CIStatus{State: "RUNNING", Workflow: "Release Candidate Sanity", Duration: "1m 15s"},
			},
			HasLocal:         true,
			LocalSHA:         "fc2374d",
			Author:           "hassan.ali",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "release: lock dependency freeze for v3.3.0 final tag",
			ChangedFiles:     []string{"VERSION", "CHANGELOG.md"},
		},
		// Track 4: support/lts-2.x (Uzun vadeli destek dalı)
		{
			ID:              "br-track-lts-2x",
			Name:            "support/lts-2.x",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 0,
			EndSpineIndex:   7,
			BaseSHA:         "a101f01",
			HasRemote:       true,
			RemoteRef:       "origin/support/lts-2.x",
			PR: &model.PRStatus{
				Number:      750,
				Title:       "track(lts): long-term maintenance branch for 2.x customers",
				Author:      "kevin.duque",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "LTS Security Regression", Duration: "5m 20s"},
			},
			HasLocal:         false,
			Author:           "kevin.duque",
			AheadMain:        6,
			BehindMain:       8,
			UncommittedCount: 0,
			LastCommitMsg:    "lts: backport openssl vulnerability fix to 2.8 patchline",
			ChangedFiles:     []string{"crypto/openssl_compat.c"},
		},

		// 2. GELİŞTİRİCİ DALLARI VE PR'LARI (50 Developer Contributions)
		// Merged Pull Requests (Geçmiş başarılar)
		{
			ID:              "br-ent-merged-802",
			Name:            "feat/kafka-event-streaming",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 7,
			EndSpineIndex:   9,
			BaseSHA:         "1808878",
			MergeSHA:        "3a0a69a",
			HasRemote:       true,
			RemoteRef:       "origin/feat/kafka-event-streaming",
			PR: &model.PRStatus{
				Number:      802,
				Title:       "feat(events): partitioned kafka message broker with zookeeper-less kraft",
				Author:      "alice.chen",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Broker Load Test", Duration: "2m 45s"},
			},
			HasLocal:      false,
			Author:        "alice.chen",
			LastCommitMsg: "kafka: add producer ack retry with exponential backoff",
			ChangedFiles:  []string{"services/events/kafka.go"},
		},
		{
			ID:              "br-ent-merged-807",
			Name:            "perf/graphql-query-batching",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 9,
			EndSpineIndex:   11,
			BaseSHA:         "3a0a69a",
			MergeSHA:        "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/perf/graphql-query-batching",
			PR: &model.PRStatus{
				Number:      807,
				Title:       "perf(graphql): dataloader N+1 batch caching across microservices",
				Author:      "lucas.muller",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "GraphQL Benchmark", Duration: "1m 55s"},
			},
			HasLocal:      false,
			Author:        "lucas.muller",
			LastCommitMsg: "graphql: reduce redis roundtrips using bulk dataloader",
			ChangedFiles:  []string{"gateway/graphql/batcher.go"},
		},

		// Açık PR'lar (CI: RUNNING - Nabız animasyonlu)
		{
			ID:              "br-ent-open-815",
			Name:            "feat/grpc-service-mesh",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 11,
			EndSpineIndex:   14,
			BaseSHA:         "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/feat/grpc-service-mesh",
			PR: &model.PRStatus{
				Number:      815,
				Title:       "feat(mesh): http/2 multiplexing and mTLS mutual authentication",
				Author:      "david.kim",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "mTLS Certificate Exchange Suite",
					Duration: "0m 42s",
				},
			},
			HasLocal:         true,
			LocalSHA:         "ea1109a",
			Author:           "david.kim",
			AheadMain:        3,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "mesh: establish strict mTLS boundary on ingress",
			ChangedFiles:     []string{"services/mesh/tls.go", "deploy/helm/values.yaml"},
		},
		{
			ID:              "br-ent-open-818",
			Name:            "feat/billing-stripe-webhooks",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/feat/billing-stripe-webhooks",
			PR: &model.PRStatus{
				Number:      818,
				Title:       "feat(billing): idempotency keys for payment intent hooks",
				Author:      "priya.patel",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "Payment Gateway Simulation",
					Duration: "0m 22s",
				},
			},
			HasLocal:         false,
			Author:           "priya.patel",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "billing: lock transaction table during webhook replay",
			ChangedFiles:     []string{"services/billing/stripe.go"},
		},

		// Açık PR (CI: FAILURE - Kırmızı uyarı, log çıkarma ve panik analizi)
		{
			ID:              "br-ent-open-820",
			Name:            "fix/distributed-tracing-span",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 10,
			EndSpineIndex:   14,
			BaseSHA:         "4b0b5ab",
			HasRemote:       true,
			RemoteRef:       "origin/fix/distributed-tracing-span",
			PR: &model.PRStatus{
				Number:      820,
				Title:       "fix(telemetry): eliminate nil dereference in otel span export",
				Author:      "mehmet.yilmaz",
				ReviewState: "CHANGES_REQUESTED",
				CI: &model.CIStatus{
					State:      "FAILURE",
					Workflow:   "OpenTelemetry Integration CI",
					RunID:      "994182410",
					Duration:   "0m 58s",
					FailedStep: "Testing service mesh tracing spans",
					RawLog:     entCiFailLog,
				},
			},
			HasLocal:         true,
			LocalSHA:         "eb2210b",
			Author:           "mehmet.yilmaz",
			IsHEAD:           true,
			AheadMain:        4,
			BehindMain:       2,
			UncommittedCount: 3,
			LastCommitMsg:    "telemetry: check nil span context before flag evaluation",
			ChangedFiles:     []string{"services/telemetry/exporter.go", "services/telemetry/exporter_test.go"},
		},

		// Açık PR (ÇAKIŞMA VE CROSS-PR COLLISION: Auth ve Helm dosyaları)
		{
			ID:              "br-ent-open-825",
			Name:            "feat/auth-v2-jwt-rotation",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 11,
			EndSpineIndex:   15,
			BaseSHA:         "5c0c4bc",
			HasRemote:       true,
			RemoteRef:       "origin/feat/auth-v2-jwt-rotation",
			PR: &model.PRStatus{
				Number:      825,
				Title:       "feat(auth): dynamic asymmetric jwt rotation with jwks endpoint",
				Author:      "elena.rostova",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Security & Auth Test", Duration: "1m 30s"},
			},
			HasLocal:         true,
			LocalSHA:         "ec3321c",
			Author:           "elena.rostova",
			AheadMain:        3,
			BehindMain:       1,
			UncommittedCount: 0,
			LastCommitMsg:    "auth: rotate public jwks keys every 6 hours",
			ChangedFiles:     []string{"services/auth/jwt.go", "api/v1/auth.go", "deploy/helm/values.yaml"},
			Conflict: &model.ConflictDetail{
				HasConflict:       true,
				Files:             []string{"deploy/helm/values.yaml"},
				IsChangelogOrLock: false,
				Strategy:          "merge-editor",
				PreviewUnified:    entConflictPreview,
			},
		},
		{
			ID:              "br-ent-open-828",
			Name:            "feat/oauth-pkce-flow",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 12,
			EndSpineIndex:   15,
			BaseSHA:         "6d0d3cd",
			HasRemote:       true,
			RemoteRef:       "origin/feat/oauth-pkce-flow",
			PR: &model.PRStatus{
				Number:      828,
				Title:       "feat(oauth): pkce challenge verification for native mobile apps",
				Author:      "carlos.silva",
				ReviewState: "REVIEW_REQUIRED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "OAuth Conformance Suite", Duration: "1m 18s"},
			},
			HasLocal:         false,
			Author:           "carlos.silva",
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "oauth: verify sha256 code challenge on token exchange",
			ChangedFiles:     []string{"api/v1/auth.go", "services/auth/pkce.go"}, // Cross-PR collision with 825!
		},

		// Açık PR (Onaylı ve Hazır)
		{
			ID:              "br-ent-open-830",
			Name:            "fix/postgres-connection-leak",
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: 13,
			EndSpineIndex:   15,
			BaseSHA:         "7e0e2de",
			HasRemote:       true,
			RemoteRef:       "origin/fix/postgres-connection-leak",
			PR: &model.PRStatus{
				Number:      830,
				Title:       "fix(database): defer pgxpool.Conn.Release on error paths",
				Author:      "ayse.kaya",
				ReviewState: "APPROVED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Postgres Stress Benchmark", Duration: "2m 10s"},
			},
			HasLocal:         false,
			Author:           "ayse.kaya",
			AheadMain:        1,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "db: guarantee connection release in batch execution loops",
			ChangedFiles:     []string{"storage/postgres/pool.go"},
		},

		// 3. KALINTI VE YEREL DALLAR (Remnants, Fresh Local & WIP)
		// Squash Remnant / Local Remnant (origin: gone, aheadMain > 0)
		{
			ID:              "br-ent-remnant-gone",
			Name:            "feat/legacy-elasticsearch-drop",
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: 8,
			EndSpineIndex:   11,
			BaseSHA:         "2909789",
			MergeSHA:        "5c0c4bc",
			HasRemote:       false,
			RemoteRef:       "origin/feat/legacy-elasticsearch-drop",
			OriginGone:      true,
			PR: &model.PRStatus{
				Number:      795,
				Title:       "refactor(search): migrate query cluster from es7 to opensearch",
				Author:      "marcus.vance",
				ReviewState: "MERGED",
				CI:          &model.CIStatus{State: "SUCCESS", Workflow: "Search Ingestion Tests", Duration: "3m 05s"},
			},
			HasLocal:      true,
			LocalSHA:      "5c0c4bc",
			Author:        "marcus.vance",
			AheadMain:     3,
			LastCommitMsg: "search: deprecate old elasticsearch index clients",
			ChangedFiles:  []string{"services/search/client.go"},
		},

		// Taze Yerel Dal (🌱 Yeni Yerel Dal, aheadMain == 0, remote yok)
		{
			ID:               "br-ent-fresh-local",
			Name:             "feat/edge-wasm-worker",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  15,
			EndSpineIndex:    16,
			BaseSHA:          "90100f0",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "90100f0",
			Author:           "rachel.green",
			IsHEAD:           false,
			AheadMain:        0,
			BehindMain:       0,
			UncommittedCount: 0,
			LastCommitMsg:    "edge: bootstrap webassembly micro-worker scaffolding",
			ChangedFiles:     []string{"workers/wasm/worker.go"},
		},

		// Alt Ray Yerel WIP ve Deney Dalları (Lower Hemisphere)
		{
			ID:               "br-ent-local-ebpf",
			Name:             "wip/ebpf-tc-bandwidth-shaper",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  13,
			EndSpineIndex:    15,
			BaseSHA:          "7e0e2de",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "ed4432d",
			Author:           "tarik.ozkan",
			IsHEAD:           false,
			AheadMain:        2,
			BehindMain:       0,
			UncommittedCount: 5,
			LastCommitMsg:    "wip(ebpf): attach tc ingress filter to veth pairs for rate limiting",
			ChangedFiles:     []string{"kernel/bpf/shaper.c", "kernel/bpf/loader.go"},
		},
		{
			ID:               "br-ent-local-wasm",
			Name:             "exp/v8-isolated-sandbox",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  14,
			EndSpineIndex:    16,
			BaseSHA:          "8f0f1ef",
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         "ee5543e",
			Author:           "olga.ivanova",
			IsHEAD:           false,
			AheadMain:        1,
			BehindMain:       0,
			UncommittedCount: 2,
			LastCommitMsg:    "exp(sandbox): evaluate v8 isolate heap snapshot limits",
			ChangedFiles:     []string{"runtime/v8/isolate.cc"},
		},
	}

	return simWorkspace{
		repo: model.RepoMeta{
			ID:                "sim-enterprise",
			Name:              "MegaCorp Cloud Platform (50 Geliştirici)",
			Path:              "(Enterprise Çoklu Dal Simülasyonu)",
			DefaultBranch:     "main",
			RemoteName:        "origin",
			Ahead:             3,
			Behind:            0,
			DirtyCount:        3,
			Mode:              "simulation",
			VersionTag:        "v3.3.0-rc2",
			CurrentUser:       "mehmet.yilmaz",
			BranchProtected:   true,
			ProtectionSummary: "Korumalı (PR Zorunlu + 2 Onay + Strict CI + Force-Push Kilitli)",
			AppVersion:        AppVersion,
		},
		nodes:    nodes,
		branches: branches,
	}
}

// CheckForUpdates inspects remote git tags to detect whether a newer GitArc release exists.
func (m *Manager) CheckForUpdates() (map[string]any, error) {
	latest := AppVersion
	updateCmd := "git pull --ff-only origin main && go build -o gitarc ."

	if live, ok := m.activeLiveRepo(); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		remoteName := "origin"
		if rOut, _, err := runCmd(ctx, live.path, "git", "remote"); err == nil && strings.TrimSpace(rOut) != "" {
			remoteName = strings.Fields(rOut)[0]
		}
		if out, _, err := runCmd(ctx, live.path, "git", "ls-remote", "--tags", "--sort=-v:refname", remoteName); err == nil {
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				if len(fields) < 2 {
					continue
				}
				ref := strings.TrimSuffix(fields[1], "^{}")
				if strings.HasPrefix(ref, "refs/tags/v") {
					latest = strings.TrimPrefix(ref, "refs/tags/v")
					break
				}
			}
		}
	}

	avail := compareSemver(latest, AppVersion) > 0
	m.mu.Lock()
	m.latestVer = latest
	m.updateAvail = avail
	m.recordLogLocked("ok", "Sürüm Kontrolü", "", fmt.Sprintf("Mevcut v%s · Uzak v%s (Güncelleme: %v)", AppVersion, latest, avail), "")
	m.mu.Unlock()

	return map[string]any{
		"currentVersion":  AppVersion,
		"latestVersion":   latest,
		"updateAvailable": avail,
		"updateCommand":   updateCmd,
		"teamMode":        m.IsTeamMode(),
	}, nil
}

func compareSemver(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	pb := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	for i := 0; i < 3; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va, _ = strconv.Atoi(strings.Split(pa[i], "-")[0])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(strings.Split(pb[i], "-")[0])
		}
		if va > vb {
			return 1
		}
		if va < vb {
			return -1
		}
	}
	return 0
}

// ProtectDefaultBranch enables main/default branch protection in simulation or via GitHub API.
func (m *Manager) ProtectDefaultBranch() (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		defBranch := detectDefaultBranch(ctx, live.path)
		if _, err := exec.LookPath("gh"); err != nil {
			err := fmt.Errorf("Branch Protection etkinleştirmek için gh CLI gereklidir")
			m.recordLog("error", "Branch Protection", defBranch, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
		payload := `{"required_status_checks":{"strict":true,"contexts":[]},"enforce_admins":false,"required_pull_request_reviews":{"dismiss_stale_reviews":true,"required_approving_review_count":1},"restrictions":null}`
		cmd := exec.CommandContext(ctx, "gh", "api", "-X", "PUT", fmt.Sprintf("repos/:owner/:repo/branches/%s/protection", defBranch), "--input", "-")
		cmd.Dir = live.path
		cmd.Stdin = strings.NewReader(payload)
		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &outBuf
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(outBuf.String())
			errOut := fmt.Errorf("Branch Protection API hatası (%s): %s", defBranch, msg)
			m.recordLog("error", "Branch Protection", defBranch, errOut.Error(), fmt.Sprintf("gh api -X PUT repos/:owner/:repo/branches/%s/protection", defBranch))
			return model.CockpitSnapshot{}, errOut
		}
		InvalidateGHCache(live.path)
		m.recordLog("ok", "Branch Protection", defBranch, fmt.Sprintf("%s dalı korumaya alındı (PR + strict status checks)", defBranch), "")
		return m.Snapshot()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.simRepo.repo.BranchProtected = true
	m.simRepo.repo.ProtectionSummary = "PR Zorunlu + CI Status Checks + Force-Push Kilitli"
	m.recordLogLocked("ok", "Branch Protection", m.simRepo.repo.DefaultBranch, "main dalı korumaya alındı (PR + CI + Force-Push kilitli)", "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}
