package model

import "time"

// CockpitSnapshot is the primary JSON contract returned by /api/snapshot
// and consumed by the parabolic SVG cockpit frontend.
type CockpitSnapshot struct {
	Repo            RepoMeta         `json:"repo"`
	Repos           []RepoOption     `json:"repos"`
	Nodes           []CommitNode     `json:"nodes"`
	Branches        []BranchArc      `json:"branches"`
	Diagnostics     []DiagnosticItem `json:"diagnostics"`
	Logs            []ActionLogEntry `json:"logs,omitempty"`
	PendingProposal *CommitProposal  `json:"pendingProposal,omitempty"`
	GeneratedAt     time.Time        `json:"generatedAt"`
}

// CommitProposal represents a staged proposal created by an AI agent (e.g. via MCP).
// It does NOT perform any commit/push until the user explicitly reviews and confirms in the UI.
type CommitProposal struct {
	ID                string   `json:"id"`
	BranchName        string   `json:"branchName"`
	BaseBranch        string   `json:"baseBranch"`
	Message           string   `json:"message"`
	Files             []string `json:"files"`
	PRTitle           string   `json:"prTitle"`
	PRBody            string   `json:"prBody"`
	DiffStat          string   `json:"diffStat,omitempty"`
	FilesChanged      int      `json:"filesChanged"`
	TotalLines        int      `json:"totalLines"`
	IsLargeDiff       bool     `json:"isLargeDiff"`
	CreatedAt         string   `json:"createdAt"`
	ShieldResult      string   `json:"shieldResult,omitempty"`
	CountCheckMatch   bool     `json:"countCheckMatch"`
	CountCheckMessage string   `json:"countCheckMessage,omitempty"`
}

// RepoOption describes a selectable workspace repository (live or simulated).
type RepoOption struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	Mode          string `json:"mode"` // "simulation" | "live"
	DefaultBranch string `json:"defaultBranch"`
	OpenPRs       int    `json:"openPRs"`
	Ahead         int    `json:"ahead"`
	Behind        int    `json:"behind"`
}

// RepoMeta holds active repository metadata and spine sync status.
type RepoMeta struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Path              string `json:"path"`
	DefaultBranch     string `json:"defaultBranch"`
	RemoteName        string `json:"remoteName"`
	Ahead             int    `json:"ahead"`
	Behind            int    `json:"behind"`
	DirtyCount        int    `json:"dirtyCount"`
	Mode              string `json:"mode"` // "simulation" | "live"
	VersionTag        string `json:"versionTag,omitempty"`
	CurrentUser       string `json:"currentUser,omitempty"`
	TeamMode          bool   `json:"teamMode,omitempty"`
	BranchProtected   bool   `json:"branchProtected"`
	ProtectionSummary string `json:"protectionSummary,omitempty"`
	// ProtectionState: "active" | "none" | "unknown". "unknown" = durum okunamadı (yetki yok, zaman aşımı,
	// gh yok, GitHub dışı uzak depo); "Korumasız" ile karıştırılmamalıdır.
	ProtectionState string `json:"protectionState,omitempty"`
	// SyncSteps: veri toplama adımlarının (git fetch, PR listesi, koruma...) son durumu.
	SyncSteps []SyncStep `json:"syncSteps,omitempty"`
	AppVersion        string `json:"appVersion,omitempty"`
	LatestVersion     string `json:"latestVersion,omitempty"`
	UpdateAvailable   bool   `json:"updateAvailable,omitempty"`
	CacheHits         int      `json:"cacheHits,omitempty"`
	SecretPatterns    []string `json:"secretPatterns,omitempty"`
	NoGitAddAll       bool     `json:"noGitAddAll,omitempty"`
}

// DiffStat represents a summary of working tree changes (e.g. from git diff --stat).
type DiffStat struct {
	Stat         string `json:"stat"`
	FilesChanged int    `json:"filesChanged"`
	Insertions   int    `json:"insertions"`
	Deletions    int    `json:"deletions"`
	TotalLines   int    `json:"totalLines"`
	IsLargeDiff  bool   `json:"isLargeDiff"`
}

// DiffFileSummary represents changes for a single file within a diff.
type DiffFileSummary struct {
	Path      string `json:"path"`
	OldPath   string `json:"oldPath,omitempty"`
	Status    string `json:"status"` // "M", "A", "D", "R"
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// DiffResult represents a full diff inspection result for a commit, branch, or working tree.
type DiffResult struct {
	Target       string            `json:"target"`
	Type         string            `json:"type"` // "branch", "commit", "working-tree", "pr"
	Title        string            `json:"title"`
	RawDiff      string            `json:"rawDiff"`
	Files        []DiffFileSummary `json:"files"`
	FilesChanged int               `json:"filesChanged"`
	TotalAdds    int               `json:"totalAdds"`
	TotalDels    int               `json:"totalDels"`
}

// CommitNode represents a commit along the central main/develop spine.
type CommitNode struct {
	SHA          string `json:"sha"`
	ShortSHA     string `json:"shortSha"`
	Subject      string `json:"subject"`
	Author       string `json:"author"`
	RelativeTime string `json:"relativeTime"`
	SpineIndex   int    `json:"spineIndex"`
	IsMerge      bool   `json:"isMerge"`
	Tag          string `json:"tag,omitempty"`
	IsHEAD       bool   `json:"isHead,omitempty"`
}

// BranchArc represents a single branch lifecycle curve (half parabola or full arch)
// along with its unified remote (PR/CI) and local (HEAD/dirty) capsule metadata.
type BranchArc struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Hemisphere      string `json:"hemisphere"` // "upper" (Remote/PR) | "lower" (Local/Unpushed)
	State           string `json:"state"`      // "open" | "merged" | "gone"
	Lane            int    `json:"lane"`       // 1..K assigned by interval packing
	StartSpineIndex int    `json:"startSpineIndex"`
	EndSpineIndex   int    `json:"endSpineIndex"`
	BaseSHA         string `json:"baseSha"`
	MergeSHA        string `json:"mergeSha,omitempty"`
	// HistoryOnly: birleşmiş ama merge noktası omurga penceresinde (son N commit) bulunamayan dal.
	// Gerçek bir konumu olmadığı için parabol olarak çizilmez; arayüzde "geçmiş" çipinde listelenir.
	HistoryOnly bool `json:"historyOnly,omitempty"`
	// LongLived: develop/release/* gibi uzun ömürlü dal; kalıntı temizliğinden ve silmeden korunur.
	LongLived bool `json:"longLived,omitempty"`
	ColorToken      string `json:"colorToken,omitempty"`

	// Upper layer of Branch Pair Capsule (GitHub Remote / PR / Review / CI)
	HasRemote  bool      `json:"hasRemote"`
	RemoteRef  string    `json:"remoteRef,omitempty"`
	OriginGone bool      `json:"originGone"`
	PR         *PRStatus `json:"pr,omitempty"`

	// Lower layer of Branch Pair Capsule (Local Branch / HEAD / Working Tree)
	HasLocal         bool   `json:"hasLocal"`
	LocalSHA         string `json:"localSha,omitempty"`
	Author           string `json:"author,omitempty"`
	IsHEAD           bool   `json:"isHead"`
	AheadMain        int    `json:"aheadMain"`
	BehindMain       int    `json:"behindMain"`
	RemoteAhead      int    `json:"remoteAhead"`
	RemoteBehind     int    `json:"remoteBehind"`
	UncommittedCount int    `json:"uncommittedCount"`
	LastCommitMsg    string `json:"lastCommitMsg,omitempty"`

	// Smart Diagnostics & Conflict Radar
	ChangedFiles   []string         `json:"changedFiles"`
	Conflict       *ConflictDetail  `json:"conflict,omitempty"`
	CrossPR        []CrossPRWarning `json:"crossPr,omitempty"`
	CanAmendCommit bool             `json:"canAmendCommit,omitempty"`
	SuggestedCmd   string           `json:"suggestedCmd,omitempty"`
	DraftPRTitle   string           `json:"draftPrTitle,omitempty"`
	DraftPRBody    string           `json:"draftPrBody,omitempty"`
}

// PRStatus models GitHub Pull Request review & Actions CI status.
type PRStatus struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
	Body        string    `json:"body,omitempty"`
	Author      string    `json:"author"`
	URL         string    `json:"url,omitempty"`
	ReviewState string    `json:"reviewState"` // "APPROVED" | "CHANGES_REQUESTED" | "REVIEW_REQUIRED" | "MERGED"
	CI          *CIStatus `json:"ci,omitempty"`
}

// CIStatus holds GitHub Actions run status and extracted failure summaries.
type CIStatus struct {
	State      string          `json:"state"` // "SUCCESS" | "FAILURE" | "RUNNING"
	Workflow   string          `json:"workflow"`
	RunID      string          `json:"runId,omitempty"`
	Duration   string          `json:"duration"`
	FailedStep string          `json:"failedStep,omitempty"`
	Extracts   []CIFailExtract `json:"extracts,omitempty"`
	RawLog     string          `json:"rawLog,omitempty"`
	Checks     []CICheckStep   `json:"checks,omitempty"`
}

// CICheckStep models an individual GitHub Actions status check or job.
type CICheckStep struct {
	Name       string `json:"name"`
	Workflow   string `json:"workflow,omitempty"`
	Status     string `json:"status"`     // "COMPLETED" | "IN_PROGRESS" | "QUEUED"
	Conclusion string `json:"conclusion"` // "SUCCESS" | "FAILURE" | "SKIPPED" | "CANCELLED"
	DetailsURL string `json:"detailsUrl,omitempty"`
}

// CIFailExtract is a noise-free parsed line from raw CI logs.
type CIFailExtract struct {
	Kind     string `json:"kind"` // "test_fail" | "panic" | "compile_error" | "lint_fail"
	Symbol   string `json:"symbol"`
	FileLine string `json:"fileLine,omitempty"`
	Message  string `json:"message"`
	ReproCmd string `json:"reproCmd,omitempty"`
}

// ConflictDetail reports in-memory `git merge-tree --write-tree` results.
type ConflictDetail struct {
	HasConflict       bool     `json:"hasConflict"`
	Files             []string `json:"files,omitempty"`
	IsChangelogOrLock bool     `json:"isChangelogOrLock"`
	Strategy          string   `json:"strategy,omitempty"` // "union-changelog" | "lock-tidy" | "manual"
	PreviewUnified    string   `json:"previewUnified,omitempty"`
	Resolved          bool     `json:"resolved"`
	ResolvedNote      string   `json:"resolvedNote,omitempty"`
}

// CrossPRWarning signals file overlap with another open PR/branch.
type CrossPRWarning struct {
	OtherBranchID string   `json:"otherBranchId"`
	OtherBranch   string   `json:"otherBranch"`
	OtherPRNumber int      `json:"otherPrNumber,omitempty"`
	SharedFiles   []string `json:"sharedFiles"`
}

// DiagnosticItem is a prioritized entry in the right-hand smart radar panel.
type DiagnosticItem struct {
	ID         string   `json:"id"`
	Severity   string   `json:"severity"` // "critical" | "warning" | "info"
	Kind       string   `json:"kind"`     // "ci_failure" | "conflict" | "cross_pr" | "prune_candidate"
	BranchID   string   `json:"branchId"`
	BranchName string   `json:"branchName"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	ActionHint string   `json:"actionHint,omitempty"`
	Files      []string `json:"files,omitempty"`
}

// ConflictFileDiff holds the 3-way conflict hunk window and the 3 automatic resolution previews.
type ConflictFileDiff struct {
	Path          string `json:"path"`
	StartLine     int    `json:"startLine"`
	EndLine       int    `json:"endLine"`
	ConflictHunks string `json:"conflictHunks"` // Sol Panel: <<<<<<< origin/main ... ======= ... >>>>>>> dal
	UnionPreview  string `json:"unionPreview"`  // 1. Adım: Her İkisini Koru (Union)
	OursPreview   string `json:"oursPreview"`   // 2. Adım: Main'i Baz Al (Ours)
	TheirsPreview string `json:"theirsPreview"` // 3. Adım: Dalı Baz Al (Theirs)
}

// StrategyAdvice describes a conflict resolution strategy recommendation and risk badge.
type StrategyAdvice struct {
	Strategy string `json:"strategy"` // "union" | "ours" | "theirs" | "custom"
	Title    string `json:"title"`
	Subtitle string `json:"subtitle"`
	Badge    string `json:"badge"` // "★ ÖNERİLEN" | "✓ Güvenli" | "⚠ Kodda Çift Tanım Riski"
	Tone     string `json:"tone"`  // "ok" | "warn" | "accent"
}

// ConflictStudioPayload is returned by /api/conflict/inspect for the side-by-side Conflict Studio modal.
type ConflictStudioPayload struct {
	BranchID             string             `json:"branchId"`
	BranchName           string             `json:"branchName"`
	BaseRef              string             `json:"baseRef"`
	Recommended          string             `json:"recommended"` // "union" | "ours" | "theirs" | "custom"
	RecommendationReason string             `json:"recommendationReason"`
	Strategies           []StrategyAdvice   `json:"strategies,omitempty"`
	AIPrompt             string             `json:"aiPrompt"` // 4. Adım: Kota dostu hazır AI / MCP çözüm istemi
	Files                []ConflictFileDiff `json:"files"`
}

// ActionLogEntry records recent cockpit operations and detailed error diagnostics.
type ActionLogEntry struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`  // "info" | "ok" | "error"
	Action    string `json:"action"` // örn. "Çakışma Çözümü", "Dal Silme", "PR Merge"
	Branch    string `json:"branch,omitempty"`
	Message   string `json:"message"`
	Detail    string `json:"detail,omitempty"`
}

// SyncStep, bir veri toplama adımının (git fetch, gh PR listesi, koruma sorgusu) son durumudur.
// Arayüz bunu "main ↔ origin/main (Synced)" rozetinin altında gösterir; böylece "Synced" yazarken
// aslında hangi verinin eski/eksik olduğu görünür.
type SyncStep struct {
	ID         string `json:"id"`                   // "fetch" | "prs" | "merged" | "protection"
	Label      string `json:"label"`                // kullanıcıya gösterilen kısa ad
	State      string `json:"state"`                // "ok" | "pending" | "stale" | "failed" | "unknown" | "skipped"
	At         string `json:"at,omitempty"`         // son tamamlanma, RFC3339
	DurationMs int64  `json:"durationMs,omitempty"` // son denemenin süresi
	Count      int    `json:"count,omitempty"`      // örn. bulunan PR sayısı
	Detail     string `json:"detail,omitempty"`     // hata / açıklama
}
