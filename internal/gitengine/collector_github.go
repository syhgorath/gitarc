package gitengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitarc/internal/model"
)

const ghCacheTTL = 10 * time.Second

type ghCacheEntry struct {
	fetchedAt         time.Time
	openPRs           map[string]model.PRStatus
	mergedPRs         map[string]model.PRStatus
	mergedArches      []model.BranchArc
	branchProtected   bool
	protectionSummary string
}

var (
	ghCacheMu sync.Mutex
	ghCache   = map[string]ghCacheEntry{}
)

// InvalidateGHCache clears cached GitHub PR / Actions metadata for repoPath.
func InvalidateGHCache(repoPath string) {
	ghCacheMu.Lock()
	delete(ghCache, filepath.Clean(repoPath))
	ghCacheMu.Unlock()
}

var runIDRegex = regexp.MustCompile(`/actions/runs/(\d+)`)

// extractRunIDFromURL extracts the GitHub Actions run ID from a check detailsUrl.
func extractRunIDFromURL(detailsURL string) string {
	m := runIDRegex.FindStringSubmatch(detailsURL)
	if len(m) > 1 {
		return m[1]
	}
	return ""
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

type ghMergedPRPayload struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
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
		"--json", "number,headRefName,title,body,author,mergeCommit,url")
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
			Body:        it.Body,
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
