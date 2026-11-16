package gitengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gitarc/internal/model"
)

// Manager coordinates active workspace repositories (simulation + live local repos)
// and executes thread-safe lifecycle mutations on the cockpit graph.
type Manager struct {
	mu           sync.RWMutex
	activeRepoID string
	simRepo      simWorkspace
	liveRepos    []repoEntry
	nextPR       int
	nextSeq      int
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

// NewManager initializes the GitArc engine with the KeyLab ecosystem simulation
// and auto-discovers local git repositories (such as sshpocket and custom -repo).
func NewManager(extraRepoPath string) *Manager {
	m := &Manager{
		activeRepoID: "sim-keylab",
		nextPR:       425,
		nextSeq:      10,
	}
	m.simRepo = buildInitialKeyLabSimulation()

	candidates := []repoEntry{}
	if extraRepoPath != "" {
		abs, err := filepath.Abs(extraRepoPath)
		if err == nil {
			candidates = append(candidates, repoEntry{
				id:   "live-custom",
				name: filepath.Base(abs) + " (Canlı)",
				path: abs,
			})
		}
	}
	defaultKeyLab := "/Users/ziyayesilbahce/Developer/sshpocket"
	if isGitDir(defaultKeyLab) && (extraRepoPath == "" || filepath.Clean(extraRepoPath) != defaultKeyLab) {
		candidates = append(candidates, repoEntry{
			id:   "live-sshpocket",
			name: "sshpocket / KeyLab (Canlı)",
			path: defaultKeyLab,
		})
	}
	if wd, err := os.Getwd(); err == nil && isGitDir(wd) {
		already := false
		for _, c := range candidates {
			if filepath.Clean(c.path) == filepath.Clean(wd) {
				already = true
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
	m.liveRepos = candidates
	if extraRepoPath != "" && len(candidates) > 0 && isGitDir(candidates[0].path) {
		m.activeRepoID = candidates[0].id
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
		return model.CockpitSnapshot{}, err
	}
	return snap, nil
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

// CreateBranch spawns a new open branch curve from the given spine commit (or latest main).
func (m *Manager) CreateBranch(name, hemisphere, fromSHA string) (model.CockpitSnapshot, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return model.CockpitSnapshot{}, fmt.Errorf("dal adı boş olamaz")
	}
	if hemisphere != "upper" && hemisphere != "lower" {
		hemisphere = "lower"
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

	for _, b := range m.simRepo.branches {
		if b.Name == name && b.State != "merged" {
			return model.CockpitSnapshot{}, fmt.Errorf("%s dalı zaten açık", name)
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

	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// MergeBranch closes an open half-parabola onto a new merge commit node on the main spine,
// turning it into a complete parabolic arch.
func (m *Manager) MergeBranch(branchID string) (model.CockpitSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

	idx := -1
	for i := range m.simRepo.branches {
		if m.simRepo.branches[i].ID == branchID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return model.CockpitSnapshot{}, fmt.Errorf("dal bulunamadı: %s", branchID)
	}
	b := &m.simRepo.branches[idx]
	if b.State == "merged" {
		return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
	}
	if b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved {
		return model.CockpitSnapshot{}, fmt.Errorf("%s dalında çözülmemiş çakışma var; önce Auto-Resolve çalıştırın", b.Name)
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

	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

// PromoteBranch pushes a lower-hemisphere local branch to remote and opens a PR,
// flipping it into the upper hemisphere.
func (m *Manager) PromoteBranch(branchID string) (model.CockpitSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

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
			b.PR = &model.PRStatus{
				Number:      prNum,
				Title:       "feat: " + b.Name,
				Author:      "ziya",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "SUCCESS",
					Workflow: "ci.yml",
					Duration: "1m 18s",
				},
			}
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	return model.CockpitSnapshot{}, fmt.Errorf("dal bulunamadı: %s", branchID)
}

// AutoResolveConflict resolves CHANGELOG.md / lockfile conflicts by preserving both sides.
func (m *Manager) AutoResolveConflict(branchID string) (model.CockpitSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID {
			if b.Conflict == nil || !b.Conflict.HasConflict {
				return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
			}
			b.Conflict.HasConflict = false
			b.Conflict.Resolved = true
			b.Conflict.ResolvedNote = "Union merge + kilit bütünlük doğrulaması uygulandı; çalışma dizini temiz."
			b.Conflict.PreviewUnified = "✓ Her iki dalın CHANGELOG.md ve kilit girdileri sırası korunarak birleştirildi.\n✓ git merge-tree --write-tree: exit 0 (temiz)"
			b.BehindMain = 0
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	return model.CockpitSnapshot{}, fmt.Errorf("dal bulunamadı: %s", branchID)
}

// PruneGone cleans up branches whose remote tracking ref is gone or already merged.
func (m *Manager) PruneGone() (model.CockpitSnapshot, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, 0, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

	var kept []model.BranchArc
	removed := 0
	for _, b := range m.simRepo.branches {
		if b.OriginGone || b.State == "gone" {
			removed++
			continue
		}
		kept = append(kept, b)
	}
	m.simRepo.branches = kept
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), removed, nil
}

// SyncSpine clears ahead/behind skew between local main and origin/main.
func (m *Manager) SyncSpine() (model.CockpitSnapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeRepoID != "sim-keylab" {
		return model.CockpitSnapshot{}, fmt.Errorf("yazma aksiyonları güvenlik için yalnızca Kokpit Simülasyon modunda çalıştırılır")
	}

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
	}

	ComputeCrossPRCollisions(branches)
	AssignLanes(branches)
	diags := BuildDiagnostics(branches)

	nodes := make([]model.CommitNode, len(m.simRepo.nodes))
	copy(nodes, m.simRepo.nodes)

	return model.CockpitSnapshot{
		Repo:        m.simRepo.repo,
		Repos:       opts,
		Nodes:       nodes,
		Branches:    branches,
		Diagnostics: diags,
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
				Author:      "ziya",
				ReviewState: "REVIEW_REQUIRED",
				CI: &model.CIStatus{
					State:    "RUNNING",
					Workflow: "Verify Component Signatures",
					Duration: "0m 27s",
				},
			},
			HasLocal:         true,
			LocalSHA:         "e99a014",
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
			ID:            "sim-keylab",
			Name:          "keylab / sshpocket",
			Path:          "/Users/ziyayesilbahce/Developer/sshpocket",
			DefaultBranch: "main",
			RemoteName:    "origin",
			Ahead:         2,
			Behind:        1,
			DirtyCount:    2,
			Mode:          "simulation",
			VersionTag:    "v4.0.16",
		},
		nodes:    nodes,
		branches: branches,
	}
}
