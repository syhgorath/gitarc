package gitengine

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gitarc/internal/model"
)

var diffGitHeaderRe = regexp.MustCompile(`^diff --git a/(.*?) b/(.*)$`)

// ParseDiffOutput parses unified git diff output into a structured DiffResult.
func ParseDiffOutput(raw, target, diffType, title string) model.DiffResult {
	res := model.DiffResult{
		Target:  target,
		Type:    diffType,
		Title:   title,
		RawDiff: raw,
		Files:   make([]model.DiffFileSummary, 0),
	}

	if strings.TrimSpace(raw) == "" {
		return res
	}

	var curFile *model.DiffFileSummary
	scanner := bufio.NewScanner(strings.NewReader(raw))
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if m := diffGitHeaderRe.FindStringSubmatch(line); len(m) == 3 {
			if curFile != nil {
				res.Files = append(res.Files, *curFile)
			}
			curFile = &model.DiffFileSummary{
				Path:    m[2],
				OldPath: m[1],
				Status:  "M",
			}
			continue
		}

		if curFile == nil {
			continue
		}

		if strings.HasPrefix(line, "new file mode") {
			curFile.Status = "A"
		} else if strings.HasPrefix(line, "deleted file mode") {
			curFile.Status = "D"
		} else if strings.HasPrefix(line, "similarity index") || strings.HasPrefix(line, "rename from") {
			curFile.Status = "R"
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			curFile.Additions++
			res.TotalAdds++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			curFile.Deletions++
			res.TotalDels++
		}
	}

	if curFile != nil {
		res.Files = append(res.Files, *curFile)
	}

	res.FilesChanged = len(res.Files)
	return res
}

func isHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// GetDiff computes a unified diff for target (commit, branch, or working tree).
func (m *Manager) GetDiff(target, diffType string) (model.DiffResult, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.getDiffLive(live, target, diffType)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.getDiffSimLocked(target, diffType)
}

func (m *Manager) getDiffLive(live *repoEntry, target, diffType string) (model.DiffResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	target = strings.TrimSpace(target)
	diffType = strings.TrimSpace(diffType)

	if target == "" || diffType == "working-tree" || target == "local-wip-working-tree" || target == "wip/working-tree" {
		title := "Çalışma Alanı (Kaydedilmemiş Değişiklikler)"
		out, _, err := runCmd(ctx, live.path, "git", "diff", "-M", "--color=never", "HEAD")
		if err != nil || strings.TrimSpace(out) == "" {
			out, _, _ = runCmd(ctx, live.path, "git", "diff", "-M", "--color=never")
		}
		res := ParseDiffOutput(out, "working-tree", "working-tree", title)
		return res, nil
	}

	isCommit := diffType == "commit" || (len(target) >= 7 && len(target) <= 40 && isHex(target))
	if isCommit {
		title := "Commit " + target
		subjectOut, _, err := runCmd(ctx, live.path, "git", "log", "-1", "--format=%s (%h · %an)", target)
		if err == nil && strings.TrimSpace(subjectOut) != "" {
			title = strings.TrimSpace(subjectOut)
		}
		out, _, err := runCmd(ctx, live.path, "git", "show", "--color=never", "-M", target)
		if err != nil {
			return model.DiffResult{}, fmt.Errorf("commit diff alınamadı (%s): %w", target, err)
		}
		res := ParseDiffOutput(out, target, "commit", title)
		return res, nil
	}

	// Branch or PR
	title := "Dal " + target
	out, _, err := runCmd(ctx, live.path, "git", "diff", "-M", "--color=never", "origin/main..."+target)
	if err != nil || strings.TrimSpace(out) == "" {
		out, _, err = runCmd(ctx, live.path, "git", "diff", "-M", "--color=never", "main..."+target)
	}
	if err != nil || strings.TrimSpace(out) == "" {
		out, _, _ = runCmd(ctx, live.path, "git", "show", "--color=never", "-M", target)
	}
	res := ParseDiffOutput(out, target, "branch", title)
	return res, nil
}

func (m *Manager) getDiffSimLocked(target, diffType string) (model.DiffResult, error) {
	mockDiff := `diff --git a/internal/gitengine/diff_service.go b/internal/gitengine/diff_service.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/internal/gitengine/diff_service.go
@@ -0,0 +1,5 @@
+package gitengine
+
+// Simülasyon diff içeriği
+func SimulatedDiff() string {
+	return "ok"
+}
`
	title := "Simülasyon: " + target
	if target == "" || diffType == "working-tree" {
		title = "Çalışma Alanı (Simülasyon)"
	}
	return ParseDiffOutput(mockDiff, target, diffType, title), nil
}
