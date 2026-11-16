package model

import "time"

// CockpitSnapshot is the primary JSON contract returned by /api/snapshot
// and consumed by the parabolic SVG cockpit frontend.
type CockpitSnapshot struct {
	Repo        RepoMeta         `json:"repo"`
	Repos       []RepoOption     `json:"repos"`
	Nodes       []CommitNode     `json:"nodes"`
	Branches    []BranchArc      `json:"branches"`
	Diagnostics []DiagnosticItem `json:"diagnostics"`
	GeneratedAt time.Time        `json:"generatedAt"`
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
	ID            string `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteName    string `json:"remoteName"`
	Ahead         int    `json:"ahead"`
	Behind        int    `json:"behind"`
	DirtyCount    int    `json:"dirtyCount"`
	Mode          string `json:"mode"` // "simulation" | "live"
	VersionTag    string `json:"versionTag,omitempty"`
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
	ColorToken      string `json:"colorToken,omitempty"`

	// Upper layer of Branch Pair Capsule (GitHub Remote / PR / Review / CI)
	HasRemote  bool      `json:"hasRemote"`
	RemoteRef  string    `json:"remoteRef,omitempty"`
	OriginGone bool      `json:"originGone"`
	PR         *PRStatus `json:"pr,omitempty"`

	// Lower layer of Branch Pair Capsule (Local Branch / HEAD / Working Tree)
	HasLocal         bool   `json:"hasLocal"`
	LocalSHA         string `json:"localSha,omitempty"`
	IsHEAD           bool   `json:"isHead"`
	AheadMain        int    `json:"aheadMain"`
	BehindMain       int    `json:"behindMain"`
	RemoteAhead      int    `json:"remoteAhead"`
	RemoteBehind     int    `json:"remoteBehind"`
	UncommittedCount int    `json:"uncommittedCount"`
	LastCommitMsg    string `json:"lastCommitMsg,omitempty"`

	// Smart Diagnostics & Conflict Radar
	ChangedFiles []string         `json:"changedFiles"`
	Conflict     *ConflictDetail  `json:"conflict,omitempty"`
	CrossPR      []CrossPRWarning `json:"crossPr,omitempty"`
}

// PRStatus models GitHub Pull Request review & Actions CI status.
type PRStatus struct {
	Number      int       `json:"number"`
	Title       string    `json:"title"`
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
