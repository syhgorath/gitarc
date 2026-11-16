package gitengine

import (
	"bytes"
	"context"
	"encoding/json"
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

const (
	// AppVersion identifies the current GitArc release embedded at build time.
	AppVersion        = "0.8.0"
	ghCacheTTL        = 10 * time.Second
	maxActiveBranches = 40
)

type ghCacheEntry struct {
	fetchedAt         time.Time
	openPRs           map[string]model.PRStatus
	mergedPRs         map[string]model.PRStatus
	mergedArches      []model.BranchArc
	branchProtected   bool
	protectionSummary string
}

type cachedBranchEval struct {
	mergeBaseSHA string
	aheadMain    int
	behindMain   int
	changedFiles []string
	conflict     *model.ConflictDetail
}

var (
	ghCacheMu   sync.Mutex
	ghCache     = map[string]ghCacheEntry{}
	evalCacheMu sync.RWMutex
	evalCache   = map[string]cachedBranchEval{}
)

// InvalidateGHCache clears cached GitHub PR / Actions metadata for repoPath.
func InvalidateGHCache(repoPath string) {
	ghCacheMu.Lock()
	delete(ghCache, filepath.Clean(repoPath))
	ghCacheMu.Unlock()
}

var (
	reMergeTreeConflict = regexp.MustCompile(`CONFLICT\s*\([^)]+\):\s*Merge conflict in\s+(.+)$`)
	reMergePRSubject    = regexp.MustCompile(`Merge pull request #(\d+) from [^/\s]+/([^\s]+)`)
	reMergeBranchSubj   = regexp.MustCompile(`Merge branch '([^']+)'`)
	reTrackAhead        = regexp.MustCompile(`ahead (\d+)`)
	reTrackBehind       = regexp.MustCompile(`behind (\d+)`)
	reActionsRunID      = regexp.MustCompile(`/actions/runs/(\d+)`)
)

// parseUpstreamTrack parses git %(upstream:track) values such as
// "[ahead 2, behind 1]", "[ahead 3]", "[behind 4]", or "[gone]".
func parseUpstreamTrack(track string) (ahead, behind int, gone bool) {
	if strings.Contains(track, "[gone]") {
		gone = true
	}
	if m := reTrackAhead.FindStringSubmatch(track); len(m) == 2 {
		ahead, _ = strconv.Atoi(m[1])
	}
	if m := reTrackBehind.FindStringSubmatch(track); len(m) == 2 {
		behind, _ = strconv.Atoi(m[1])
	}
	return ahead, behind, gone
}

// extractRunIDFromURL extracts the GitHub Actions run ID from a check detailsUrl.
func extractRunIDFromURL(detailsURL string) string {
	if m := reActionsRunID.FindStringSubmatch(detailsURL); len(m) == 2 {
		return m[1]
	}
	return ""
}

// ParseMergeTreeOutput inspects the stdout/stderr of `git merge-tree --write-tree`
// and builds a ConflictDetail without touching the working directory.
func ParseMergeTreeOutput(exitCode int, output string) *model.ConflictDetail {
	if exitCode == 0 {
		return nil
	}
	var files []string
	seen := map[string]bool{}
	hasSpecial := false
	hasChangelog := false
	hasVersion := false
	hasComponentsLock := false
	hasLock := false

	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if m := reMergeTreeConflict.FindStringSubmatch(line); len(m) == 2 {
			f := strings.TrimSpace(m[1])
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
				low := strings.ToLower(filepath.Base(f))
				if strings.Contains(low, "changelog") {
					hasChangelog = true
					hasSpecial = true
				} else if low == "version" {
					hasVersion = true
					hasSpecial = true
				} else if low == "components.lock" {
					hasComponentsLock = true
					hasSpecial = true
				} else if strings.Contains(low, "lock") || strings.HasSuffix(low, ".sum") || low == "go.mod" {
					hasLock = true
					hasSpecial = true
				}
			}
		}
	}
	if len(files) == 0 && strings.Contains(output, "CONFLICT") {
		files = append(files, "(working-tree-free conflict detected)")
	}
	if len(files) == 0 {
		return nil
	}

	// Madde 4: Auto-Resolve "union" yalnız CHANGELOG.md için.
	// VERSION ve components.lock için otomatik çözüm önerme; insan onayı iste.
	strategy := "manual"
	if hasVersion || hasComponentsLock {
		strategy = "manual"
	} else if hasChangelog && !hasLock && len(files) == 1 {
		strategy = "union-changelog"
	} else if hasLock {
		strategy = "lock-tidy"
	}

	return &model.ConflictDetail{
		HasConflict:       true,
		Files:             files,
		IsChangelogOrLock: hasSpecial,
		Strategy:          strategy,
		PreviewUnified:    buildConflictPreview(files, output, strategy),
	}
}

// buildConflictPreview shows actual git merge-tree conflict details without placeholder strings (Madde 3).
func buildConflictPreview(files []string, rawOutput, strategy string) string {
	var hunks []string
	for _, raw := range strings.Split(rawOutput, "\n") {
		line := strings.TrimRight(raw, "\r\n")
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "CONFLICT") || strings.HasPrefix(trimmed, "<<<<<<<") ||
			strings.HasPrefix(trimmed, "=======") || strings.HasPrefix(trimmed, ">>>>>>>") ||
			strings.HasPrefix(trimmed, "Auto-merging") {
			hunks = append(hunks, line)
		}
	}

	var b strings.Builder
	if len(hunks) > 0 {
		b.WriteString(strings.Join(hunks, "\n"))
	} else {
		b.WriteString(fmt.Sprintf("git merge-tree çakışan dosyalar (%d):\n", len(files)))
		for _, f := range files {
			b.WriteString(fmt.Sprintf("- %s (3-yollu çakışma)\n", f))
		}
	}

	switch strategy {
	case "union-changelog":
		b.WriteString("\n\n[Auto-Resolve Stratejisi: Union Merge]\nCHANGELOG girdileri alt alta güvenle birleştirilebilir.")
	case "lock-tidy":
		b.WriteString("\n\n[Auto-Resolve Stratejisi: Lock/Tidy]\nBağımlılık veya kilit bloğu birleştirilir ve bütünlük özeti yenilenir.")
	case "manual":
		b.WriteString("\n\n[Strateji: İnsan Onayı & Manuel / AI Çözüm]\nKaynak kod, VERSION veya components.lock çakışması; insan onayı gereklidir.")
	}
	return b.String()
}

// CollectLiveRepo inspects an actual local git repository (read-only) and optionally
// enriches open branches using `gh` CLI when available.
func CollectLiveRepo(repoID, repoName, repoPath string, repos []model.RepoOption) (model.CockpitSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	if _, _, err := runCmd(ctx, repoPath, "git", "rev-parse", "--is-inside-work-tree"); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("%s geçerli bir git deposu değil: %w", repoPath, err)
	}

	defaultBranch := detectDefaultBranch(ctx, repoPath)
	remoteName := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "remote"); err == nil {
		lines := strings.Fields(out)
		if len(lines) > 0 {
			remoteName = lines[0]
		}
	}

	dirtyCount := 0
	var dirtyFiles []string
	if out, _, err := runCmd(ctx, repoPath, "git", "status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
		dirtyFiles = parsePorcelainFiles(out)
		dirtyCount = len(dirtyFiles)
	}

	ahead, behind := 0, 0
	baseRef := defaultBranch
	if remoteName != "" {
		remoteRef := remoteName + "/" + defaultBranch
		if _, _, err := runCmd(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/remotes/"+remoteRef); err == nil {
			baseRef = remoteRef
		}
		if out, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--left-right", "--count", remoteRef+"..."+defaultBranch); err == nil {
			fields := strings.Fields(out)
			if len(fields) == 2 {
				behind, _ = strconv.Atoi(fields[0])
				ahead, _ = strconv.Atoi(fields[1])
			}
		}
	}

	currentUser := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "config", "user.name"); err == nil {
		currentUser = strings.TrimSpace(out)
	}

	baseTipSHA := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "rev-parse", "--short", baseRef); err == nil {
		baseTipSHA = strings.TrimSpace(out)
	}

	curBranchName := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "branch", "--show-current"); err == nil {
		curBranchName = strings.TrimSpace(out)
	}

	headSHA := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "rev-parse", "HEAD"); err == nil {
		headSHA = strings.TrimSpace(out)
	}

	spineRef := defaultBranch
	if behind > 0 && baseRef != defaultBranch {
		spineRef = baseRef
	}
	nodes, shaToIdx := collectSpineNodes(ctx, repoPath, spineRef, headSHA)
	if len(nodes) == 0 {
		nodes = []model.CommitNode{{
			SHA:          "0000000",
			ShortSHA:     "0000000",
			Subject:      "initial repository state",
			Author:       "git",
			RelativeTime: "now",
			SpineIndex:   0,
			IsHEAD:       true,
		}}
		shaToIdx["0000000"] = 0
	}

	mergedArches := extractMergedArchesFromSpine(ctx, repoPath, spineRef, nodes, shaToIdx)
	ghPRs := map[string]model.PRStatus{}
	mergedPRs := map[string]model.PRStatus{}
	branchProtected := false
	protectionSummary := "Yerel depo (koruma yok)"
	if remoteName != "" {
		// Madde 6: Uzak dal listesini git fetch --prune sonrası güncel tut
		fetchCtx, fCancel := context.WithTimeout(ctx, 3*time.Second)
		_, _, _ = runCmd(fetchCtx, repoPath, "git", "fetch", "--prune", remoteName)
		fCancel()

		var ghMergedArches []model.BranchArc
		ghPRs, mergedPRs, ghMergedArches, branchProtected, protectionSummary = fetchGHMetadataCached(ctx, repoPath, remoteName, defaultBranch, nodes, shaToIdx, mergedArches)
		mergedArches = append(mergedArches, ghMergedArches...)
	}
	branches, localHits := collectLocalBranches(ctx, repoPath, defaultBranch, baseRef, baseTipSHA, curBranchName, currentUser, nodes, shaToIdx, dirtyCount, dirtyFiles, ghPRs, mergedPRs, mergedArches)
	remoteBranches, remoteHits := collectRemoteOnlyBranches(ctx, repoPath, remoteName, defaultBranch, baseRef, baseTipSHA, nodes, shaToIdx, branches, ghPRs, mergedPRs, mergedArches)
	cacheHits := localHits + remoteHits
	branches = append(branches, remoteBranches...)
	branches = append(mergedArches, branches...)

	for i := range branches {
		if branches[i].SuggestedCmd == "" {
			branches[i].SuggestedCmd = BuildSuggestedCommand(branches[i], defaultBranch)
		}
	}

	ComputeCrossPRCollisions(branches)
	AssignLanes(branches)
	diags := BuildDiagnostics(branches)

	versionTag := ""
	for i := len(nodes) - 1; i >= 0; i-- {
		if nodes[i].Tag != "" {
			versionTag = nodes[i].Tag
			break
		}
	}

	return model.CockpitSnapshot{
		Repo: model.RepoMeta{
			ID:                repoID,
			Name:              repoName,
			Path:              repoPath,
			DefaultBranch:     defaultBranch,
			RemoteName:        remoteName,
			Ahead:             ahead,
			Behind:            behind,
			DirtyCount:        dirtyCount,
			Mode:              "live",
			VersionTag:        versionTag,
			CurrentUser:       currentUser,
			BranchProtected:   branchProtected,
			ProtectionSummary: protectionSummary,
			AppVersion:        AppVersion,
			CacheHits:         cacheHits,
		},
		Repos:       repos,
		Nodes:       nodes,
		Branches:    branches,
		Diagnostics: diags,
		GeneratedAt: time.Now(),
	}, nil
}

func detectDefaultBranch(ctx context.Context, repoPath string) string {
	for _, cand := range []string{"main", "master", "develop"} {
		if _, _, err := runCmd(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+cand); err == nil {
			return cand
		}
	}
	if out, _, err := runCmd(ctx, repoPath, "git", "branch", "--show-current"); err == nil {
		if b := strings.TrimSpace(out); b != "" {
			return b
		}
	}
	return "main"
}

func collectSpineNodes(ctx context.Context, repoPath, defaultBranch, headSHA string) ([]model.CommitNode, map[string]int) {
	out, _, err := runCmd(ctx, repoPath, "git", "log", defaultBranch, "-n", "12", "--first-parent", "--format=%H%x1f%h%x1f%s%x1f%an%x1f%cr%x1f%P%x1f%D")
	if err != nil {
		return nil, map[string]int{}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	var rawNodes []model.CommitNode
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 7 {
			continue
		}
		parents := strings.Fields(parts[5])
		tag := ""
		for _, deco := range strings.Split(parts[6], ",") {
			deco = strings.TrimSpace(deco)
			if strings.HasPrefix(deco, "tag: ") {
				tag = strings.TrimPrefix(deco, "tag: ")
				break
			}
		}
		rawNodes = append(rawNodes, model.CommitNode{
			SHA:          parts[0],
			ShortSHA:     parts[1],
			Subject:      parts[2],
			Author:       parts[3],
			RelativeTime: parts[4],
			IsMerge:      len(parents) > 1,
			Tag:          tag,
			IsHEAD:       parts[0] == headSHA,
		})
	}
	// Reverse to chronological left-to-right order (0 = oldest, N-1 = newest)
	n := len(rawNodes)
	nodes := make([]model.CommitNode, n)
	shaToIdx := make(map[string]int, n)
	for i := 0; i < n; i++ {
		node := rawNodes[n-1-i]
		node.SpineIndex = i
		nodes[i] = node
		shaToIdx[node.SHA] = i
		shaToIdx[node.ShortSHA] = i
	}
	return nodes, shaToIdx
}

func extractMergedArchesFromSpine(ctx context.Context, repoPath, defaultBranch string, nodes []model.CommitNode, shaToIdx map[string]int) []model.BranchArc {
	var arches []model.BranchArc
	for _, node := range nodes {
		if !node.IsMerge {
			continue
		}
		prNum := 0
		name := ""
		if m := reMergePRSubject.FindStringSubmatch(node.Subject); len(m) == 3 {
			prNum, _ = strconv.Atoi(m[1])
			name = m[2]
		} else if m := reMergeBranchSubj.FindStringSubmatch(node.Subject); len(m) == 2 {
			name = m[1]
		} else {
			name = "merged-" + node.ShortSHA
		}

		startIdx := node.SpineIndex - 2
		if startIdx < 0 {
			startIdx = 0
		}
		baseSHA := nodes[startIdx].ShortSHA
		if out, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--parents", "-n", "1", node.SHA); err == nil {
			fields := strings.Fields(out)
			if len(fields) >= 3 {
				if mb, _, err := runCmd(ctx, repoPath, "git", "merge-base", fields[1], fields[2]); err == nil {
					mb = strings.TrimSpace(mb)
					if idx, ok := shaToIdx[mb]; ok && idx < node.SpineIndex {
						startIdx = idx
						baseSHA = nodes[idx].ShortSHA
					}
				}
			}
		}

		var pr *model.PRStatus
		if prNum > 0 {
			pr = &model.PRStatus{
				Number:      prNum,
				Title:       node.Subject,
				Author:      node.Author,
				ReviewState: "MERGED",
				CI: &model.CIStatus{
					State:    "SUCCESS",
					Workflow: "ci.yml",
					Duration: "1m 42s",
				},
			}
		}

		arches = append(arches, model.BranchArc{
			ID:              "merged-" + node.ShortSHA,
			Name:            name,
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: startIdx,
			EndSpineIndex:   node.SpineIndex,
			BaseSHA:         baseSHA,
			MergeSHA:        node.ShortSHA,
			HasRemote:       false,
			RemoteRef:       "",
			OriginGone:      false,
			PR:              pr,
			HasLocal:        false,
			LastCommitMsg:   node.Subject,
		})
	}
	return arches
}

type ghPRPayload struct {
	Number         int    `json:"number"`
	HeadRefName    string `json:"headRefName"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ReviewDecision string `json:"reviewDecision"`
	URL            string `json:"url"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	StatusCheckRollup []struct {
		Name         string `json:"name"`
		Context      string `json:"context"`
		Status       string `json:"status"`
		State        string `json:"state"`
		Conclusion   string `json:"conclusion"`
		DetailsURL   string `json:"detailsUrl"`
		TargetURL    string `json:"targetUrl"`
		WorkflowName string `json:"workflowName"`
	} `json:"statusCheckRollup"`
}

func fetchGHMetadataCached(ctx context.Context, repoPath, remoteName, defaultBranch string, nodes []model.CommitNode, shaToIdx map[string]int, existing []model.BranchArc) (map[string]model.PRStatus, map[string]model.PRStatus, []model.BranchArc, bool, string) {
	key := filepath.Clean(repoPath)
	ghCacheMu.Lock()
	cached, ok := ghCache[key]
	if ok && time.Since(cached.fetchedAt) < ghCacheTTL {
		openCopy := make(map[string]model.PRStatus, len(cached.openPRs))
		for k, v := range cached.openPRs {
			openCopy[k] = v
		}
		mergedCopy := make(map[string]model.PRStatus, len(cached.mergedPRs))
		for k, v := range cached.mergedPRs {
			mergedCopy[k] = v
		}
		archesCopy := make([]model.BranchArc, len(cached.mergedArches))
		copy(archesCopy, cached.mergedArches)
		ghCacheMu.Unlock()
		return openCopy, mergedCopy, archesCopy, cached.branchProtected, cached.protectionSummary
	}
	ghCacheMu.Unlock()

	if urlOut, _, err := runCmd(ctx, repoPath, "git", "remote", "get-url", remoteName); err != nil || !strings.Contains(strings.ToLower(urlOut), "github.com") {
		summary := "GitHub dışı uzak depo (Saf Git modu)"
		ghCacheMu.Lock()
		ghCache[key] = ghCacheEntry{
			fetchedAt:         time.Now(),
			openPRs:           map[string]model.PRStatus{},
			mergedPRs:         map[string]model.PRStatus{},
			protectionSummary: summary,
		}
		ghCacheMu.Unlock()
		return map[string]model.PRStatus{}, map[string]model.PRStatus{}, nil, false, summary
	}

	openPRs := fetchGHPRs(ctx, repoPath)
	mergedPRs, mergedArches := fetchGHMergedPRs(ctx, repoPath, nodes, shaToIdx, existing)
	branchProtected, protectionSummary := fetchGHBranchProtection(ctx, repoPath, defaultBranch)

	ghCacheMu.Lock()
	ghCache[key] = ghCacheEntry{
		fetchedAt:         time.Now(),
		openPRs:           openPRs,
		mergedPRs:         mergedPRs,
		mergedArches:      mergedArches,
		branchProtected:   branchProtected,
		protectionSummary: protectionSummary,
	}
	ghCacheMu.Unlock()

	return openPRs, mergedPRs, mergedArches, branchProtected, protectionSummary
}

func fetchGHBranchProtection(ctx context.Context, repoPath, defaultBranch string) (bool, string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return false, "gh CLI yok; koruma okunamadı"
	}
	ghCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 1. Klasik branch protection denetimi
	out, _, err := runCmd(ghCtx, repoPath, "gh", "api", fmt.Sprintf("repos/:owner/:repo/branches/%s/protection", defaultBranch), "--jq", ".required_status_checks.strict")
	if err == nil {
		return true, fmt.Sprintf("Aktif (strict checks: %s)", strings.TrimSpace(out))
	}

	// 2. Modern GitHub Ruleset denetimi (Madde 5)
	rulesetCtx, rCancel := context.WithTimeout(ctx, 3*time.Second)
	defer rCancel()
	rOut, _, rErr := runCmd(rulesetCtx, repoPath, "gh", "api", "repos/:owner/:repo/rulesets")
	if rErr == nil && strings.TrimSpace(rOut) != "" && strings.TrimSpace(rOut) != "[]" {
		type ghRulesetItem struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Target      string `json:"target"`
			Enforcement string `json:"enforcement"`
		}
		var rItems []ghRulesetItem
		if json.Unmarshal([]byte(rOut), &rItems) == nil {
			for _, r := range rItems {
				enf := strings.ToLower(r.Enforcement)
				if (enf == "active" || enf == "evaluate") && (r.Target == "" || r.Target == "branch") {
					return true, fmt.Sprintf("Aktif (GitHub Ruleset: %s)", r.Name)
				}
			}
		}
	}

	low := strings.ToLower(out)
	if strings.Contains(low, "upgrade to github pro") {
		return false, "Korumasız (GitHub Free özel repo / Pro gerekli)"
	}
	return false, "Korumasız (Doğrudan push/force-push açık)"
}

type ghMergedPRPayload struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

func fetchGHPRs(ctx context.Context, repoPath string) map[string]model.PRStatus {
	res := map[string]model.PRStatus{}
	if _, err := exec.LookPath("gh"); err != nil {
		return res
	}
	ghCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, _, err := runCmd(ghCtx, repoPath, "gh", "pr", "list", "--state", "open", "--limit", "20",
		"--json", "number,headRefName,title,body,author,reviewDecision,statusCheckRollup,url")
	if err != nil || strings.TrimSpace(out) == "" {
		return res
	}
	var items []ghPRPayload
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return res
	}
	for _, it := range items {
		rev := it.ReviewDecision
		if rev == "" {
			rev = "REVIEW_REQUIRED"
		}
		ciState := "SUCCESS"
		if len(it.StatusCheckRollup) == 0 {
			ciState = "RUNNING"
		}
		failedStep := ""
		runID := ""
		workflow := "ci.yml"
		for _, chk := range it.StatusCheckRollup {
			chkName := chk.Name
			if chkName == "" {
				chkName = chk.Context
			}
			if chk.WorkflowName != "" {
				workflow = chk.WorkflowName
			}
			conc := strings.ToUpper(chk.Conclusion)
			st := strings.ToUpper(chk.Status)
			ctxSt := strings.ToUpper(chk.State)
			if conc == "FAILURE" || conc == "TIMED_OUT" || conc == "CANCELLED" || ctxSt == "FAILURE" || ctxSt == "ERROR" {
				ciState = "FAILURE"
				failedStep = chkName
				runID = extractRunIDFromURL(chk.DetailsURL)
				if runID == "" {
					runID = extractRunIDFromURL(chk.TargetURL)
				}
				break
			}
			if (st != "" && st != "COMPLETED") || ctxSt == "PENDING" {
				ciState = "RUNNING"
			}
		}

		var rawLog string
		var extracts []model.CIFailExtract
		if ciState == "FAILURE" && runID != "" {
			logCtx, logCancel := context.WithTimeout(ctx, 3*time.Second)
			if logOut, _, err := runCmd(logCtx, repoPath, "gh", "run", "view", runID, "--log-failed"); err == nil && strings.TrimSpace(logOut) != "" {
				if len(logOut) > 16000 {
					logOut = logOut[len(logOut)-16000:]
				}
				rawLog = logOut
				extracts = ExtractCILog(rawLog)
			}
			logCancel()
		}

		res[it.HeadRefName] = model.PRStatus{
			Number:      it.Number,
			Title:       it.Title,
			Body:        it.Body,
			Author:      it.Author.Login,
			URL:         it.URL,
			ReviewState: rev,
			CI: &model.CIStatus{
				State:      ciState,
				Workflow:   workflow,
				RunID:      runID,
				Duration:   "—",
				FailedStep: failedStep,
				Extracts:   extracts,
				RawLog:     rawLog,
			},
		}
	}
	return res
}

func fetchGHMergedPRs(ctx context.Context, repoPath string, nodes []model.CommitNode, shaToIdx map[string]int, existing []model.BranchArc) (map[string]model.PRStatus, []model.BranchArc) {
	mergedMap := map[string]model.PRStatus{}
	if _, err := exec.LookPath("gh"); err != nil {
		return mergedMap, nil
	}
	seenMergeIdx := map[int]bool{}
	for _, a := range existing {
		seenMergeIdx[a.EndSpineIndex] = true
	}

	ghCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, _, err := runCmd(ghCtx, repoPath, "gh", "pr", "list", "--state", "merged", "--limit", "60",
		"--json", "number,headRefName,title,author,mergeCommit,url")
	if err != nil || strings.TrimSpace(out) == "" {
		return mergedMap, nil
	}
	var items []ghMergedPRPayload
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return mergedMap, nil
	}

	var arches []model.BranchArc
	for _, it := range items {
		prObj := model.PRStatus{
			Number:      it.Number,
			Title:       it.Title,
			Author:      it.Author.Login,
			URL:         it.URL,
			ReviewState: "MERGED",
			CI:          &model.CIStatus{State: "SUCCESS", Workflow: "ci.yml", Duration: "—"},
		}
		if it.HeadRefName != "" {
			if _, exists := mergedMap[it.HeadRefName]; !exists {
				mergedMap[it.HeadRefName] = prObj
			}
		}
		if it.MergeCommit == nil || it.MergeCommit.OID == "" {
			continue
		}
		endIdx, ok := shaToIdx[it.MergeCommit.OID]
		if !ok || seenMergeIdx[endIdx] || endIdx == 0 {
			continue
		}
		seenMergeIdx[endIdx] = true
		startIdx := endIdx - 1
		if startIdx < 0 {
			startIdx = 0
		}
		arches = append(arches, model.BranchArc{
			ID:              fmt.Sprintf("gh-merged-%d", it.Number),
			Name:            it.HeadRefName,
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: startIdx,
			EndSpineIndex:   endIdx,
			BaseSHA:         nodes[startIdx].ShortSHA,
			MergeSHA:        nodes[endIdx].ShortSHA,
			HasRemote:       false,
			RemoteRef:       "",
			PR:              &prObj,
			LastCommitMsg:   it.Title,
		})
	}
	return mergedMap, arches
}

// BuildSuggestedCommand produces a ready-to-copy git/gh CLI chain for the branch's current state.
func BuildSuggestedCommand(b model.BranchArc, defBranch string) string {
	if defBranch == "" {
		defBranch = "main"
	}
	if b.ID == "local-wip-working-tree" || strings.HasPrefix(b.ID, "local-wip-") {
		return "git status -s && git add -A && git commit -m \"wip: save local changes\""
	}
	if !b.HasLocal && b.HasRemote {
		if b.State == "merged" || b.OriginGone {
			return fmt.Sprintf("git push origin --delete %s && git fetch --prune origin", b.Name)
		}
		if b.PR != nil && b.PR.Number > 0 {
			return fmt.Sprintf("gh pr close %d --delete-branch", b.PR.Number)
		}
		return fmt.Sprintf("git push origin --delete %s", b.Name)
	}
	if b.State == "merged" || b.OriginGone || b.State == "gone" {
		if b.IsHEAD {
			if b.UncommittedCount > 0 {
				return fmt.Sprintf("# ⚠️ DİKKAT: Aktif dalda (%s) kaydedilmemiş %d dosya var! Silmeden önce commit veya stash yapın.\ngit switch %s && git branch -D %s && git fetch --prune origin", b.Name, b.UncommittedCount, defBranch, b.Name)
			}
			return fmt.Sprintf("git switch %s && git branch -D %s && git fetch --prune origin", defBranch, b.Name)
		}
		return fmt.Sprintf("git branch -D %s && git fetch --prune origin", b.Name)
	}
	if b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved {
		return fmt.Sprintf("git switch %s && git merge origin/%s", b.Name, defBranch)
	}
	if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
		if len(b.PR.CI.Extracts) > 0 && b.PR.CI.Extracts[0].ReproCmd != "" {
			return fmt.Sprintf("git switch %s && %s", b.Name, b.PR.CI.Extracts[0].ReproCmd)
		}
		return fmt.Sprintf("gh pr checks %d", b.PR.Number)
	}
	if b.PR != nil && b.PR.Number > 0 {
		return fmt.Sprintf("gh pr merge %d --merge --delete-branch && git fetch --prune origin %s:%s", b.PR.Number, defBranch, defBranch)
	}
	if b.Hemisphere == "lower" {
		return fmt.Sprintf("git push -u origin %s && gh pr create --fill", b.Name)
	}
	return fmt.Sprintf("git switch %s", b.Name)
}

func cloneConflict(c *model.ConflictDetail) *model.ConflictDetail {
	if c == nil {
		return nil
	}
	cp := *c
	if len(c.Files) > 0 {
		cp.Files = append([]string(nil), c.Files...)
	}
	return &cp
}

func evaluateBranchVsBase(ctx context.Context, repoPath, baseRef, baseTipSHA, branchRef, branchSHA string, nodes []model.CommitNode, shaToIdx map[string]int, lastSpine int) (startIdx int, baseSHA string, aheadMain, behindMain int, changed []string, conflict *model.ConflictDetail, cacheHit bool) {
	startIdx = lastSpine - 1
	if startIdx < 0 {
		startIdx = 0
	}
	baseSHA = nodes[startIdx].ShortSHA

	cacheKey := ""
	if baseTipSHA != "" && branchSHA != "" {
		cacheKey = fmt.Sprintf("%s|%s|%s|%s", filepath.Clean(repoPath), baseRef, baseTipSHA, branchSHA)
		evalCacheMu.RLock()
		cached, ok := evalCache[cacheKey]
		evalCacheMu.RUnlock()
		if ok {
			if cached.mergeBaseSHA != "" {
				if idx, exists := shaToIdx[cached.mergeBaseSHA]; exists {
					startIdx = idx
					baseSHA = nodes[idx].ShortSHA
				}
			}
			var changedCopy []string
			if len(cached.changedFiles) > 0 {
				changedCopy = append([]string(nil), cached.changedFiles...)
			}
			return startIdx, baseSHA, cached.aheadMain, cached.behindMain, changedCopy, cloneConflict(cached.conflict), true
		}
	}

	mb := ""
	if mbOut, _, err := runCmd(ctx, repoPath, "git", "merge-base", baseRef, branchRef); err == nil {
		mb = strings.TrimSpace(mbOut)
		if idx, ok := shaToIdx[mb]; ok {
			startIdx = idx
			baseSHA = nodes[idx].ShortSHA
		}
	}

	if cntOut, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--left-right", "--count", baseRef+"..."+branchRef); err == nil {
		fields := strings.Fields(cntOut)
		if len(fields) == 2 {
			behindMain, _ = strconv.Atoi(fields[0])
			aheadMain, _ = strconv.Atoi(fields[1])
		}
	}

	if diffOut, _, err := runCmd(ctx, repoPath, "git", "diff", "--name-only", baseRef+"..."+branchRef); err == nil {
		for _, f := range strings.Split(strings.TrimSpace(diffOut), "\n") {
			if strings.TrimSpace(f) != "" {
				changed = append(changed, strings.TrimSpace(f))
			}
		}
	}

	if aheadMain > 0 {
		mtOut, mtCode, _ := runCmd(ctx, repoPath, "git", "merge-tree", "--write-tree", baseRef, branchRef)
		conflict = ParseMergeTreeOutput(mtCode, mtOut)
	}

	if cacheKey != "" {
		evalCacheMu.Lock()
		if len(evalCache) > 2000 {
			evalCache = map[string]cachedBranchEval{}
		}
		evalCache[cacheKey] = cachedBranchEval{
			mergeBaseSHA: mb,
			aheadMain:    aheadMain,
			behindMain:   behindMain,
			changedFiles: append([]string(nil), changed...),
			conflict:     cloneConflict(conflict),
		}
		evalCacheMu.Unlock()
	}

	return startIdx, baseSHA, aheadMain, behindMain, changed, conflict, false
}

func parsePorcelainFiles(out string) []string {
	var files []string
	seen := map[string]bool{}
	for _, rawLine := range strings.Split(strings.TrimRight(out, "\r\n"), "\n") {
		if len(rawLine) < 4 {
			continue
		}
		pathPart := strings.TrimSpace(rawLine[3:])
		if idx := strings.Index(pathPart, " -> "); idx >= 0 {
			pathPart = pathPart[idx+4:]
		}
		pathPart = strings.Trim(pathPart, "\"")
		if pathPart != "" && !seen[pathPart] {
			seen[pathPart] = true
			files = append(files, pathPart)
		}
	}
	return files
}

func mergeUniqueFiles(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, f := range a {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	for _, f := range b {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

func buildSuggestedCommitAndPR(repoPath, branchHint string, files []string) (commitMsg, prTitle, prBody string) {
	ver := ""
	if rawVer, err := os.ReadFile(filepath.Join(repoPath, "VERSION")); err == nil {
		ver = strings.TrimSpace(string(rawVer))
	}
	var bullets []string
	if rawCL, err := os.ReadFile(filepath.Join(repoPath, "CHANGELOG.md")); err == nil {
		inFirst := false
		for _, line := range strings.Split(string(rawCL), "\n") {
			t := strings.TrimSpace(line)
			if strings.HasPrefix(t, "## [") {
				if inFirst {
					break
				}
				inFirst = true
				continue
			}
			if inFirst && strings.HasPrefix(t, "- ") && len(bullets) < 5 {
				bullets = append(bullets, t)
			}
		}
	}

	prefix := "feat"
	if ver != "" {
		prefix = fmt.Sprintf("feat(v%s)", ver)
	} else if branchHint != "" && branchHint != "wip/working-tree" {
		prefix = fmt.Sprintf("feat(%s)", branchHint)
	}

	summary := fmt.Sprintf("update %d files", len(files))
	if len(files) == 1 {
		summary = "update " + filepath.Base(files[0])
	}
	commitMsg = fmt.Sprintf("%s: %s", prefix, summary)
	prTitle = commitMsg
	if len(bullets) > 0 {
		firstClean := strings.TrimPrefix(bullets[0], "- ")
		firstClean = strings.ReplaceAll(firstClean, "**", "")
		if idx := strings.Index(firstClean, ":"); idx > 6 && idx < 72 {
			firstClean = strings.TrimSpace(firstClean[:idx])
		}
		if len(firstClean) > 10 && len(firstClean) <= 72 {
			prTitle = fmt.Sprintf("%s: %s", prefix, firstClean)
		}
	}

	var body strings.Builder
	body.WriteString("## Neler Değişti?\n")
	if len(bullets) > 0 {
		for _, b := range bullets {
			body.WriteString(b + "\n")
		}
	} else {
		for i, f := range files {
			if i >= 12 {
				body.WriteString(fmt.Sprintf("- ... ve %d dosya daha\n", len(files)-12))
				break
			}
			body.WriteString("- `" + f + "`\n")
		}
	}
	if len(files) > 0 {
		body.WriteString(fmt.Sprintf("\n## Etkilenen Dosyalar (%d)\n", len(files)))
		for i, f := range files {
			if i >= 10 {
				body.WriteString(fmt.Sprintf("- ... toplam %d dosya\n", len(files)))
				break
			}
			body.WriteString("- `" + f + "`\n")
		}
	}
	prBody = strings.TrimSpace(body.String())
	return commitMsg, prTitle, prBody
}

// isSquashMerged checks if a branch was merged into main via squash merge (Madde 1).
func isSquashMerged(ctx context.Context, repoPath, branchName, defaultBranch, baseRef string) bool {
	// 1. git cherry: Eğer daldaki commit'ler ana hatta zaten varsa (- ile işaretlenir)
	if cherryOut, _, err := runCmd(ctx, repoPath, "git", "cherry", baseRef, branchName); err == nil && strings.TrimSpace(cherryOut) != "" {
		lines := strings.Split(strings.TrimSpace(cherryOut), "\n")
		hasOnlyMerged := true
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "+") {
				hasOnlyMerged = false
				break
			}
		}
		if hasOnlyMerged && len(lines) > 0 {
			return true
		}
	}

	// 2. gh pr list --state merged --head <branchName>
	if _, err := exec.LookPath("gh"); err == nil {
		qCtx, qCancel := context.WithTimeout(ctx, 2*time.Second)
		defer qCancel()
		if ghOut, _, err := runCmd(qCtx, repoPath, "gh", "pr", "list", "--state", "merged", "--head", branchName, "--limit", "1", "--json", "number"); err == nil {
			trimmed := strings.TrimSpace(ghOut)
			if trimmed != "" && trimmed != "[]" {
				return true
			}
		}
	}

	// 3. main logunda dal adı veya squash PR izi
	if logOut, _, err := runCmd(ctx, repoPath, "git", "log", "-n", "35", "--oneline", baseRef); err == nil {
		lowBranch := strings.ToLower(branchName)
		if strings.Contains(strings.ToLower(logOut), lowBranch) {
			return true
		}
	}

	return false
}

func collectLocalBranches(ctx context.Context, repoPath, defaultBranch, baseRef, baseTipSHA, curBranchName, currentUser string, nodes []model.CommitNode, shaToIdx map[string]int, dirtyCount int, dirtyFiles []string, ghPRs, mergedPRs map[string]model.PRStatus, mergedArches []model.BranchArc) ([]model.BranchArc, int) {
	out, _, err := runCmd(ctx, repoPath, "git", "for-each-ref",
		"--sort=-committerdate",
		fmt.Sprintf("--count=%d", maxActiveBranches),
		"--format=%(refname:short)\x1f%(objectname:short)\x1f%(upstream:short)\x1f%(upstream:track)\x1f%(HEAD)\x1f%(contents:subject)\x1f%(authorname)",
		"refs/heads")
	if err != nil {
		return nil, 0
	}

	mergedArchByName := map[string]int{}
	for idx, a := range mergedArches {
		if a.Name != "" {
			mergedArchByName[a.Name] = idx
		}
	}

	var branches []model.BranchArc
	cacheHits := 0
	lastSpine := len(nodes) - 1
	headOnDefault := curBranchName == defaultBranch
	for _, line := range strings.Split(strings.TrimRight(out, "\r\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 7 {
			continue
		}
		name := parts[0]
		localSHA := parts[1]
		upstream := parts[2]
		track := parts[3]
		isHead := strings.TrimSpace(parts[4]) == "*"
		subject := parts[5]
		author := strings.TrimSpace(parts[6])
		if author == "" {
			author = currentUser
		}
		if name == defaultBranch {
			if isHead {
				headOnDefault = true
			}
			continue
		}

		startIdx, baseSHA, aheadMain, behindMain, changed, conflict, hit := evaluateBranchVsBase(
			ctx, repoPath, baseRef, baseTipSHA, name, localSHA, nodes, shaToIdx, lastSpine,
		)
		if hit {
			cacheHits++
		}

		remoteAhead, remoteBehind, originGone := parseUpstreamTrack(track)
		hasRemote := upstream != "" && !originGone
		var prPtr *model.PRStatus
		if prVal, ok := ghPRs[name]; ok {
			p := prVal
			prPtr = &p
			hasRemote = true
			if p.Author != "" {
				author = p.Author
			}
		}

		uncommitted := 0
		if isHead {
			uncommitted = dirtyCount
		}

		// Kural 1 (Madde 1 & Squash Merge Algılama):
		// Repo PR'ları squash ile birleştirmişse dalın commit'leri main'de ayrı SHA ile görünse de
		// (aheadMain > 0 olsa bile) PR 'merged' ise dalı "birleşmiş (squash)" say:
		// çakışma, Cross-PR ve uyarı hesaplarına katma; yalnız temizleme adayı göster.
		mergedPR, isGHMerged := mergedPRs[name]
		if prPtr == nil && (isGHMerged || (originGone && isSquashMerged(ctx, repoPath, name, defaultBranch, baseRef)) || (aheadMain == 0 && uncommitted == 0 && !isHead)) {
			if archIdx, exists := mergedArchByName[name]; exists {
				mergedArches[archIdx].HasLocal = true
				mergedArches[archIdx].LocalSHA = localSHA
				mergedArches[archIdx].Author = author
				mergedArches[archIdx].IsHEAD = isHead
				mergedArches[archIdx].OriginGone = true
				mergedArches[archIdx].ID = "local-" + sanitizeID(name)
				mergedArches[archIdx].Conflict = nil
				mergedArches[archIdx].CrossPR = nil
				mergedArches[archIdx].UncommittedCount = uncommitted
				if uncommitted > 0 && len(dirtyFiles) > 0 {
					mergedArches[archIdx].ChangedFiles = mergeUniqueFiles(mergedArches[archIdx].ChangedFiles, dirtyFiles)
				}
				continue
			}
			var mp *model.PRStatus
			if isGHMerged {
				p := mergedPR
				mp = &p
			}
			branchChanged := changed
			if uncommitted > 0 && len(dirtyFiles) > 0 {
				branchChanged = mergeUniqueFiles(branchChanged, dirtyFiles)
			}
			branches = append(branches, model.BranchArc{
				ID:               "local-" + sanitizeID(name),
				Name:             name,
				Hemisphere:       "upper",
				State:            "merged",
				StartSpineIndex:  startIdx,
				EndSpineIndex:    lastSpine,
				BaseSHA:          baseSHA,
				MergeSHA:         localSHA,
				HasRemote:        hasRemote,
				RemoteRef:        upstream,
				OriginGone:       true,
				PR:               mp,
				HasLocal:         true,
				LocalSHA:         localSHA,
				Author:           author,
				IsHEAD:           isHead,
				AheadMain:        aheadMain,
				BehindMain:       behindMain,
				UncommittedCount: uncommitted,
				LastCommitMsg:    subject,
				ChangedFiles:     branchChanged,
				Conflict:         nil, // Birleşmiş dalda çakışma aranmaz (Madde 1)
			})
			continue
		}

		// Lower hemisphere rule: PR'ı olmayan yerel dallar alt raya (lower) yerleşir.
		// Yalnızca açık PR'ı olan dallar üst raya (upper) geçer (Madde 2).
		hemisphere := "lower"
		if prPtr != nil && prPtr.ReviewState != "MERGED" {
			hemisphere = "upper"
		}

		state := "open"
		if originGone {
			state = "gone"
		}

		endIdx := startIdx + 2
		if endIdx > lastSpine {
			endIdx = lastSpine
		}
		if endIdx <= startIdx {
			endIdx = startIdx + 1
		}

		canAmend := isHead && state == "open" && name != defaultBranch && aheadMain > 0
		branchChanged := changed
		draftTitle := ""
		draftBody := ""
		if isHead && uncommitted > 0 && len(dirtyFiles) > 0 {
			branchChanged = mergeUniqueFiles(branchChanged, dirtyFiles)
		}
		if hemisphere == "lower" && state == "open" {
			_, draftTitle, draftBody = buildSuggestedCommitAndPR(repoPath, name, branchChanged)
			if subject != "" && (draftTitle == "" || uncommitted == 0) {
				draftTitle = subject
			}
		}

		branches = append(branches, model.BranchArc{
			ID:               "local-" + sanitizeID(name),
			Name:             name,
			Hemisphere:       hemisphere,
			State:            state,
			StartSpineIndex:  startIdx,
			EndSpineIndex:    endIdx,
			BaseSHA:          baseSHA,
			HasRemote:        hasRemote,
			RemoteRef:        upstream,
			OriginGone:       originGone,
			PR:               prPtr,
			HasLocal:         true,
			LocalSHA:         localSHA,
			Author:           author,
			IsHEAD:           isHead,
			AheadMain:        aheadMain,
			BehindMain:       behindMain,
			RemoteAhead:      remoteAhead,
			RemoteBehind:     remoteBehind,
			UncommittedCount: uncommitted,
			LastCommitMsg:    subject,
			ChangedFiles:     branchChanged,
			Conflict:         conflict,
			CanAmendCommit:   canAmend,
			DraftPRTitle:     draftTitle,
			DraftPRBody:      draftBody,
		})

		// If an open PR branch also has unpushed local commits or uncommitted files,
		// spawn a companion lower-hemisphere local parabola to visualize local divergence.
		if hemisphere == "upper" && state == "open" && (remoteAhead > 0 || uncommitted > 0) {
			branches = append(branches, model.BranchArc{
				ID:               "local-wip-" + sanitizeID(name),
				Name:             name + " (yerel)",
				Hemisphere:       "lower",
				State:            "open",
				StartSpineIndex:  startIdx,
				EndSpineIndex:    endIdx,
				BaseSHA:          baseSHA,
				HasRemote:        false,
				HasLocal:         true,
				LocalSHA:         localSHA,
				Author:           author,
				IsHEAD:           isHead,
				AheadMain:        remoteAhead,
				BehindMain:       remoteBehind,
				RemoteAhead:      remoteAhead,
				RemoteBehind:     remoteBehind,
				UncommittedCount: uncommitted,
				LastCommitMsg:    fmt.Sprintf("Pushlanmamış yerel çalışma (%d commit, %d dirty)", remoteAhead, uncommitted),
			})
		}
	}

	if headOnDefault && dirtyCount > 0 && lastSpine >= 0 {
		commitMsg, prTitle, prBody := buildSuggestedCommitAndPR(repoPath, "wip/working-tree", dirtyFiles)
		branches = append(branches, model.BranchArc{
			ID:               "local-wip-working-tree",
			Name:             "wip/working-tree",
			Hemisphere:       "lower",
			State:            "open",
			StartSpineIndex:  lastSpine,
			EndSpineIndex:    lastSpine + 1,
			BaseSHA:          nodes[lastSpine].ShortSHA,
			HasRemote:        false,
			HasLocal:         true,
			LocalSHA:         nodes[lastSpine].ShortSHA,
			Author:           currentUser,
			IsHEAD:           true,
			AheadMain:        0,
			BehindMain:       0,
			UncommittedCount: dirtyCount,
			LastCommitMsg:    commitMsg,
			ChangedFiles:     dirtyFiles,
			DraftPRTitle:     prTitle,
			DraftPRBody:      prBody,
		})
	}
	return branches, cacheHits
}

func collectRemoteOnlyBranches(ctx context.Context, repoPath, remoteName, defaultBranch, baseRef, baseTipSHA string, nodes []model.CommitNode, shaToIdx map[string]int, localBranches []model.BranchArc, ghPRs, mergedPRs map[string]model.PRStatus, mergedArches []model.BranchArc) ([]model.BranchArc, int) {
	if remoteName == "" {
		return nil, 0
	}

	localByName := map[string]bool{
		defaultBranch: true,
	}
	for _, b := range localBranches {
		localByName[b.Name] = true
	}

	mergedArchByName := map[string]int{}
	for idx, a := range mergedArches {
		if a.Name != "" {
			mergedArchByName[a.Name] = idx
		}
		if a.HasLocal && a.Name != "" {
			localByName[a.Name] = true
		}
	}

	out, _, err := runCmd(ctx, repoPath, "git", "for-each-ref",
		"--sort=-committerdate",
		fmt.Sprintf("--count=%d", maxActiveBranches),
		"--format=%(refname:short)\x1f%(objectname:short)\x1f%(contents:subject)\x1f%(authorname)",
		"refs/remotes/"+remoteName)

	var branches []model.BranchArc
	cacheHits := 0
	lastSpine := len(nodes) - 1
	seenRemote := map[string]bool{}
	prefix := remoteName + "/"

	if err == nil {
		for _, line := range strings.Split(strings.TrimRight(out, "\r\n"), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Split(line, "\x1f")
			if len(parts) < 4 {
				continue
			}
			remoteRef := parts[0]
			remoteSHA := parts[1]
			subject := parts[2]
			author := strings.TrimSpace(parts[3])
			if !strings.HasPrefix(remoteRef, prefix) {
				continue
			}
			name := strings.TrimPrefix(remoteRef, prefix)
			if name == "" || name == "HEAD" || name == defaultBranch {
				continue
			}
			seenRemote[name] = true

			if archIdx, ok := mergedArchByName[name]; ok {
				mergedArches[archIdx].HasRemote = true
				mergedArches[archIdx].RemoteRef = remoteRef
				mergedArches[archIdx].OriginGone = true
			}

			if localByName[name] {
				continue
			}

			startIdx, baseSHA, aheadMain, behindMain, changed, conflict, hit := evaluateBranchVsBase(
				ctx, repoPath, baseRef, baseTipSHA, remoteRef, remoteSHA, nodes, shaToIdx, lastSpine,
			)
			if hit {
				cacheHits++
			}

			var prPtr *model.PRStatus
			if prVal, ok := ghPRs[name]; ok {
				p := prVal
				prPtr = &p
				if p.Author != "" {
					author = p.Author
				}
			}
			mergedPR, isGHMerged := mergedPRs[name]
			isSquash := isSquashMerged(ctx, repoPath, name, defaultBranch, baseRef)

			// Kural A: Merge edilmiş uzak kalıntı dal (Madde 1 & 6)
			if (prPtr == nil && (isGHMerged || isSquash || aheadMain == 0)) || (prPtr != nil && prPtr.ReviewState == "MERGED") {
				if _, exists := mergedArchByName[name]; exists {
					continue
				}
				var mp *model.PRStatus
				if isGHMerged {
					p := mergedPR
					mp = &p
				} else if prPtr != nil && prPtr.ReviewState == "MERGED" {
					mp = prPtr
				}
				branches = append(branches, model.BranchArc{
					ID:              "remote-" + sanitizeID(name),
					Name:            name,
					Hemisphere:      "upper",
					State:           "merged",
					StartSpineIndex: startIdx,
					EndSpineIndex:   lastSpine,
					BaseSHA:         baseSHA,
					MergeSHA:        remoteSHA,
					HasRemote:       true,
					RemoteRef:       remoteRef,
					OriginGone:      true,
					PR:              mp,
					HasLocal:        false,
					LocalSHA:        remoteSHA,
					Author:          author,
					AheadMain:       aheadMain,
					BehindMain:      behindMain,
					LastCommitMsg:   subject,
					Conflict:        nil, // Birleşmiş dalda çakışma aranmaz
				})
				continue
			}

			// Kural B: Açık remote-only dal veya açık PR
			endIdx := startIdx + 2
			if endIdx > lastSpine {
				endIdx = lastSpine
			}
			if endIdx <= startIdx {
				endIdx = startIdx + 1
			}

			branches = append(branches, model.BranchArc{
				ID:              "remote-" + sanitizeID(name),
				Name:            name,
				Hemisphere:      "upper",
				State:           "open",
				StartSpineIndex: startIdx,
				EndSpineIndex:   endIdx,
				BaseSHA:         baseSHA,
				HasRemote:       true,
				RemoteRef:       remoteRef,
				PR:              prPtr,
				HasLocal:        false,
				LocalSHA:        remoteSHA,
				Author:          author,
				AheadMain:       aheadMain,
				BehindMain:      behindMain,
				LastCommitMsg:   subject,
				ChangedFiles:    changed,
				Conflict:        conflict,
			})
		}
	}

	// Henüz local fetch edilmemiş açık GitHub PR'larını da üst yarımküreye ekle
	for name, prVal := range ghPRs {
		if localByName[name] || seenRemote[name] || name == "" || name == defaultBranch {
			continue
		}
		p := prVal
		startIdx := lastSpine - 1
		if startIdx < 0 {
			startIdx = 0
		}
		endIdx := lastSpine + 1
		if endIdx <= startIdx {
			endIdx = startIdx + 1
		}
		branches = append(branches, model.BranchArc{
			ID:              "remote-" + sanitizeID(name),
			Name:            name,
			Hemisphere:      "upper",
			State:           "open",
			StartSpineIndex: startIdx,
			EndSpineIndex:   endIdx,
			BaseSHA:         nodes[startIdx].ShortSHA,
			HasRemote:       true,
			RemoteRef:       remoteName + "/" + name,
			HasLocal:        false,
			Author:          p.Author,
			PR:              &p,
			AheadMain:       1,
			LastCommitMsg:   p.Title,
		})
	}

	return branches, cacheHits
}

func sanitizeID(name string) string {
	r := strings.NewReplacer("/", "-", ".", "-", " ", "-")
	return r.Replace(name)
}

func runCmd(ctx context.Context, dir, bin string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	combined := stdout.String()
	if stderr.Len() > 0 {
		if combined != "" {
			combined += "\n"
		}
		combined += stderr.String()
	}
	return combined, exitCode, err
}
