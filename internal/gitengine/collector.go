package gitengine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitarc/internal/model"
)

var (
	reMergeTreeConflict = regexp.MustCompile(`CONFLICT\s*\([^)]+\):\s*Merge conflict in\s+(.+)$`)
	reMergePRSubject    = regexp.MustCompile(`Merge pull request #(\d+) from \S+/([^\s]+)`)
	reMergeBranchSubj   = regexp.MustCompile(`Merge branch '([^']+)'`)
)

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
	hasLock := false

	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if m := reMergeTreeConflict.FindStringSubmatch(line); len(m) == 2 {
			f := strings.TrimSpace(m[1])
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
				if IsSpecialAutoMergeFile(f) {
					hasSpecial = true
					low := strings.ToLower(filepath.Base(f))
					if strings.Contains(low, "changelog") {
						hasChangelog = true
					} else {
						hasLock = true
					}
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

	strategy := "manual"
	if hasChangelog && !hasLock {
		strategy = "union-changelog"
	} else if hasLock {
		strategy = "lock-tidy"
	}

	return &model.ConflictDetail{
		HasConflict:       true,
		Files:             files,
		IsChangelogOrLock: hasSpecial,
		Strategy:          strategy,
		PreviewUnified:    buildConflictPreview(files, strategy),
	}
}

func buildConflictPreview(files []string, strategy string) string {
	var b strings.Builder
	b.WriteString("<<<<<< HEAD (main)\n")
	b.WriteString(fmt.Sprintf("// main üzerinde %s güncellendi\n", strings.Join(files, ", ")))
	b.WriteString("======\n")
	b.WriteString("// feature dalındaki yeni girdiler\n")
	b.WriteString(">>>>>> feature\n")
	switch strategy {
	case "union-changelog":
		b.WriteString("\n[Auto-Resolve Stratejisi: Union Merge]\nHer iki dalın CHANGELOG girdileri kronolojik sırada korunur.")
	case "lock-tidy":
		b.WriteString("\n[Auto-Resolve Stratejisi: Lock/Tidy]\nBağımlılık veya kilit bloğu birleştirilir ve bütünlük özeti yenilenir.")
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
	if out, _, err := runCmd(ctx, repoPath, "git", "status", "--porcelain"); err == nil && strings.TrimSpace(out) != "" {
		dirtyCount = len(strings.Split(strings.TrimRight(out, "\n"), "\n"))
	}

	ahead, behind := 0, 0
	if remoteName != "" {
		remoteRef := remoteName + "/" + defaultBranch
		if out, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--left-right", "--count", remoteRef+"..."+defaultBranch); err == nil {
			fields := strings.Fields(out)
			if len(fields) == 2 {
				behind, _ = strconv.Atoi(fields[0])
				ahead, _ = strconv.Atoi(fields[1])
			}
		}
	}

	headSHA := ""
	if out, _, err := runCmd(ctx, repoPath, "git", "rev-parse", "HEAD"); err == nil {
		headSHA = strings.TrimSpace(out)
	}

	nodes, shaToIdx := collectSpineNodes(ctx, repoPath, defaultBranch, headSHA)
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

	ghPRs := map[string]model.PRStatus{}
	if remoteName != "" {
		ghPRs = fetchGHPRs(ctx, repoPath)
	}
	branches := collectLocalBranches(ctx, repoPath, defaultBranch, nodes, shaToIdx, dirtyCount, ghPRs)
	mergedArches := extractMergedArchesFromSpine(ctx, repoPath, defaultBranch, nodes, shaToIdx)
	branches = append(mergedArches, branches...)

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
			ID:            repoID,
			Name:          repoName,
			Path:          repoPath,
			DefaultBranch: defaultBranch,
			RemoteName:    remoteName,
			Ahead:         ahead,
			Behind:        behind,
			DirtyCount:    dirtyCount,
			Mode:          "live",
			VersionTag:    versionTag,
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
			HasRemote:       true,
			RemoteRef:       "origin/" + name,
			OriginGone:      true,
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
	ReviewDecision string `json:"reviewDecision"`
	URL            string `json:"url"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	StatusCheckRollup []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"statusCheckRollup"`
}

func fetchGHPRs(ctx context.Context, repoPath string) map[string]model.PRStatus {
	res := map[string]model.PRStatus{}
	if _, err := exec.LookPath("gh"); err != nil {
		return res
	}
	out, _, err := runCmd(ctx, repoPath, "gh", "pr", "list", "--state", "open", "--limit", "20",
		"--json", "number,headRefName,title,author,reviewDecision,statusCheckRollup,url")
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
		failedStep := ""
		for _, chk := range it.StatusCheckRollup {
			if strings.EqualFold(chk.Conclusion, "FAILURE") || strings.EqualFold(chk.Conclusion, "TIMED_OUT") {
				ciState = "FAILURE"
				failedStep = chk.Name
				break
			}
			if !strings.EqualFold(chk.Status, "COMPLETED") && chk.Status != "" {
				ciState = "RUNNING"
			}
		}
		res[it.HeadRefName] = model.PRStatus{
			Number:      it.Number,
			Title:       it.Title,
			Author:      it.Author.Login,
			URL:         it.URL,
			ReviewState: rev,
			CI: &model.CIStatus{
				State:      ciState,
				Workflow:   "ci.yml",
				Duration:   "—",
				FailedStep: failedStep,
			},
		}
	}
	return res
}

func collectLocalBranches(ctx context.Context, repoPath, defaultBranch string, nodes []model.CommitNode, shaToIdx map[string]int, dirtyCount int, ghPRs map[string]model.PRStatus) []model.BranchArc {
	out, _, err := runCmd(ctx, repoPath, "git", "for-each-ref",
		"--format=%(refname:short)\x1f%(objectname:short)\x1f%(upstream:short)\x1f%(upstream:track)\x1f%(HEAD)\x1f%(contents:subject)",
		"refs/heads")
	if err != nil {
		return nil
	}

	var branches []model.BranchArc
	lastSpine := len(nodes) - 1
	for _, line := range strings.Split(strings.TrimRight(out, "\r\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 6 {
			continue
		}
		name := parts[0]
		if name == defaultBranch {
			continue
		}
		localSHA := parts[1]
		upstream := parts[2]
		track := parts[3]
		isHead := strings.TrimSpace(parts[4]) == "*"
		subject := parts[5]

		startIdx := lastSpine - 1
		if startIdx < 0 {
			startIdx = 0
		}
		baseSHA := nodes[startIdx].ShortSHA
		if mbOut, _, err := runCmd(ctx, repoPath, "git", "merge-base", defaultBranch, name); err == nil {
			mb := strings.TrimSpace(mbOut)
			if idx, ok := shaToIdx[mb]; ok {
				startIdx = idx
				baseSHA = nodes[idx].ShortSHA
			}
		}

		aheadMain, behindMain := 0, 0
		if cntOut, _, err := runCmd(ctx, repoPath, "git", "rev-list", "--left-right", "--count", defaultBranch+"..."+name); err == nil {
			fields := strings.Fields(cntOut)
			if len(fields) == 2 {
				behindMain, _ = strconv.Atoi(fields[0])
				aheadMain, _ = strconv.Atoi(fields[1])
			}
		}

		var changed []string
		if diffOut, _, err := runCmd(ctx, repoPath, "git", "diff", "--name-only", defaultBranch+"..."+name); err == nil {
			for _, f := range strings.Split(strings.TrimSpace(diffOut), "\n") {
				if strings.TrimSpace(f) != "" {
					changed = append(changed, strings.TrimSpace(f))
				}
			}
		}

		originGone := strings.Contains(track, "[gone]")
		hasRemote := upstream != "" && !originGone
		hemisphere := "lower"
		if hasRemote || originGone {
			hemisphere = "upper"
		}
		var prPtr *model.PRStatus
		if prVal, ok := ghPRs[name]; ok {
			p := prVal
			prPtr = &p
			hemisphere = "upper"
			hasRemote = true
		}

		state := "open"
		if originGone {
			state = "gone"
		}

		uncommitted := 0
		if isHead {
			uncommitted = dirtyCount
		}

		// Working-tree-free conflict check via git merge-tree --write-tree
		var conflict *model.ConflictDetail
		if aheadMain > 0 {
			mtOut, mtCode, _ := runCmd(ctx, repoPath, "git", "merge-tree", "--write-tree", defaultBranch, name)
			conflict = ParseMergeTreeOutput(mtCode, mtOut)
		}

		endIdx := startIdx + 2
		if endIdx > lastSpine {
			endIdx = lastSpine
		}
		if endIdx <= startIdx {
			endIdx = startIdx + 1
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
			IsHEAD:           isHead,
			AheadMain:        aheadMain,
			BehindMain:       behindMain,
			UncommittedCount: uncommitted,
			LastCommitMsg:    subject,
			ChangedFiles:     changed,
			Conflict:         conflict,
		})
	}
	return branches
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
