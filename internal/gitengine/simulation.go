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
	mu           sync.RWMutex
	activeRepoID string
	simRepo      simWorkspace
	liveRepos    []repoEntry
	nextPR       int
	nextSeq      int
	logs         []model.ActionLogEntry
	logSeq       int
	teamMode     bool
	latestVer    string
	updateAvail  bool
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
		activeRepoID: "sim-keylab",
		nextPR:       425,
		nextSeq:      10,
	}
	m.simRepo = buildInitialKeyLabSimulation()

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
	defaultKeyLab := "/Users/ziyayesilbahce/Developer/sshpocket"
	if isGitDir(defaultKeyLab) {
		already := false
		for _, c := range candidates {
			if filepath.Clean(c.path) == filepath.Clean(defaultKeyLab) {
				already = true
				break
			}
		}
		if !already {
			candidates = append(candidates, repoEntry{
				id:   "live-sshpocket",
				name: "sshpocket / KeyLab (Canlı)",
				path: defaultKeyLab,
			})
		}
	}
	m.liveRepos = candidates

	// Canlı mod hedeflenmişse ve geçerli bir canlı git deposu varsa ona bağlan.
	// extraRepoPath geçersiz/silinmiş olsa bile (örn. keylab) doğrudan simülasyona düşmek yerine
	// mevcut geçerli canlı depoya (cwd veya sshpocket) bağlanır.
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
			ID:            "sim-keylab",
			Name:          "keylab / sshpocket (Kokpit Simülasyon)",
			Path:          "/Users/ziyayesilbahce/Developer/sshpocket",
			Mode:          "simulation",
			DefaultBranch: m.simRepo.repo.DefaultBranch,
			OpenPRs:       openCount,
			Ahead:         m.simRepo.repo.Ahead,
			Behind:        m.simRepo.repo.Behind,
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
	valid := repoID == "sim-keylab"
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
		if b.Name == name && b.State != "merged" {
			err := fmt.Errorf("%s dalı zaten açık", name)
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
func (m *Manager) CommitWorkingTree(branchName, commitMsg, prTitle, prBody string, pushAndPR bool) (model.CockpitSnapshot, error) {
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
		snap, err := m.commitWorkingTreeLive(live, branchName, commitMsg, prTitle, prBody, pushAndPR)
		okMsg := fmt.Sprintf("Değişiklikler '%s' dalına commitlendi", branchName)
		if pushAndPR {
			okMsg = fmt.Sprintf("Dal açıldı, commitlendi, pushlandı ve PR oluşturuldu (%s)", branchName)
		}
		return m.logLiveOutcome("Konsolsuz Commit & PR", branchName, okMsg, snap, err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

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

// ResetSimulation restores the initial curated KeyLab simulation state.
func (m *Manager) ResetSimulation() model.CockpitSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.simRepo = buildInitialKeyLabSimulation()
	m.activeRepoID = "sim-keylab"
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

	return model.CockpitSnapshot{
		Repo:        repoMeta,
		Repos:       opts,
		Nodes:       nodes,
		Branches:    branches,
		Diagnostics: diags,
		Logs:        m.recentLogsLocked(),
		GeneratedAt: time.Now(),
	}
}

func buildInitialKeyLabSimulation() simWorkspace {
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
			ID:                "sim-keylab",
			Name:              "keylab / sshpocket",
			Path:              "/Users/ziyayesilbahce/Developer/sshpocket",
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
