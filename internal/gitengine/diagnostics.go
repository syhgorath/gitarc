package gitengine

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gitarc/internal/model"
)

var (
	reISOTimestamp = regexp.MustCompile(`^(?:.*?\t)?\d{4}-\d{2}-\d{2}T[0-9:.]+Z\s*`)
	reGoFailTest   = regexp.MustCompile(`---\s+FAIL:\s+([A-Za-z0-9_./-]+)\s+\(([^)]+)\)`)
	rePanic        = regexp.MustCompile(`(?i)^.*panic:\s+(.+)$`)
	reFileLine     = regexp.MustCompile(`([a-zA-Z0-9_./-]+\.(?:go|js|ts|py|sh|yml|yaml)):(\d+)`)
	reGoCompError  = regexp.MustCompile(`^([a-zA-Z0-9_./-]+\.go:\d+:\d+):\s+(.+)$`)
	reCrossMarker  = regexp.MustCompile(`(?:❌|FAIL\s+[a-zA-Z0-9_./-]+)`)
)

func cleanCILogLine(raw string) string {
	s := reISOTimestamp.ReplaceAllString(raw, "")
	return strings.TrimSpace(s)
}

// ExtractCILog parses noisy CI / Go test logs into concise actionable error extracts.
func ExtractCILog(rawLog string) []model.CIFailExtract {
	if strings.TrimSpace(rawLog) == "" {
		return nil
	}

	rawLines := strings.Split(rawLog, "\n")
	lines := make([]string, len(rawLines))
	for i, r := range rawLines {
		lines[i] = cleanCILogLine(r)
	}

	var extracts []model.CIFailExtract
	seen := make(map[string]bool)

	var lastFileLine string
	var lastPkg string

	findUpcomingPkg := func(from int) string {
		for k := from; k < len(lines) && k <= from+15; k++ {
			l := lines[k]
			if strings.HasPrefix(l, "FAIL\t") || strings.HasPrefix(l, "FAIL ") {
				fields := strings.Fields(l)
				if len(fields) >= 2 && strings.Contains(fields[1], "/") {
					return normalizeGoPkg(fields[1])
				}
			}
		}
		return lastPkg
	}

	for i, line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "FAIL\t") || strings.HasPrefix(line, "FAIL ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.Contains(fields[1], "/") {
				lastPkg = normalizeGoPkg(fields[1])
			}
			continue
		}

		if m := reFileLine.FindStringSubmatch(line); len(m) == 3 {
			lastFileLine = m[1] + ":" + m[2]
		}

		if m := rePanic.FindStringSubmatch(line); len(m) == 2 {
			key := "panic:" + m[1]
			if !seen[key] {
				seen[key] = true
				fl := lastFileLine
				// Peek ahead a few lines for the first source file:line in stack trace
				for k := i + 1; k < len(lines) && k <= i+8; k++ {
					if fm := reFileLine.FindStringSubmatch(lines[k]); len(fm) == 3 {
						fl = fm[1] + ":" + fm[2]
						break
					}
				}
				pkg := inferPackageFromFileLine(fl, findUpcomingPkg(i+1))
				extracts = append(extracts, model.CIFailExtract{
					Kind:     "panic",
					Symbol:   "panic",
					FileLine: fl,
					Message:  strings.TrimSpace(m[1]),
					ReproCmd: buildGoReproCmd(pkg, ""),
				})
			}
			continue
		}

		if m := reGoFailTest.FindStringSubmatch(line); len(m) == 3 {
			testName := m[1]
			dur := m[2]
			msg := fmt.Sprintf("%s failed (%s)", testName, dur)
			fl := lastFileLine
			// Inspect preceding lines for the assertion location & error message
			for k := i - 1; k >= 0 && k >= i-5; k-- {
				prev := lines[k]
				if prev == "" || strings.HasPrefix(prev, "===") {
					continue
				}
				if fm := reFileLine.FindStringSubmatch(prev); len(fm) == 3 {
					fl = fm[1] + ":" + fm[2]
					msg = prev
					break
				}
				if !strings.HasPrefix(prev, "---") {
					msg = prev
				}
			}
			key := "test:" + testName
			if !seen[key] {
				seen[key] = true
				pkg := inferPackageFromFileLine(fl, findUpcomingPkg(i+1))
				extracts = append(extracts, model.CIFailExtract{
					Kind:     "test_fail",
					Symbol:   testName,
					FileLine: fl,
					Message:  msg,
					ReproCmd: buildGoReproCmd(pkg, testName),
				})
			}
			continue
		}

		if m := reGoCompError.FindStringSubmatch(line); len(m) == 3 {
			key := "compile:" + m[1]
			if !seen[key] {
				seen[key] = true
				pkg := inferPackageFromFileLine(m[1], findUpcomingPkg(i+1))
				extracts = append(extracts, model.CIFailExtract{
					Kind:     "compile_error",
					Symbol:   filepath.Base(strings.Split(m[1], ":")[0]),
					FileLine: m[1],
					Message:  m[2],
					ReproCmd: buildGoReproCmd(pkg, ""),
				})
			}
			continue
		}

		if reCrossMarker.MatchString(line) {
			key := "marker:" + line
			if !seen[key] && len(extracts) < 6 {
				seen[key] = true
				extracts = append(extracts, model.CIFailExtract{
					Kind:     "lint_fail",
					Symbol:   "ci-check",
					FileLine: lastFileLine,
					Message:  line,
				})
			}
		}
	}

	return extracts
}

func normalizeGoPkg(rawPkg string) string {
	// Convert "keylab/internal/shellgold" or "github.com/org/repo/internal/x" into "./internal/..."
	if idx := strings.Index(rawPkg, "/internal/"); idx >= 0 {
		return "." + rawPkg[idx:]
	}
	if idx := strings.Index(rawPkg, "/cmd/"); idx >= 0 {
		return "." + rawPkg[idx:]
	}
	return rawPkg
}

func inferPackageFromFileLine(fl, fallback string) string {
	if fl == "" {
		if fallback != "" {
			return fallback
		}
		return "./..."
	}
	clean := strings.Split(fl, ":")[0]
	dir := filepath.Dir(clean)
	if dir == "." || dir == "" {
		if fallback != "" {
			return fallback
		}
		return "./..."
	}
	dir = strings.TrimPrefix(dir, "go/")
	if !strings.HasPrefix(dir, "./") {
		dir = "./" + dir
	}
	return dir
}

func buildGoReproCmd(pkg, testName string) string {
	target := pkg
	if target == "" {
		target = "./..."
	}
	if testName != "" {
		rootTest := strings.Split(testName, "/")[0]
		return fmt.Sprintf("go test -v %s -run '^%s$'", target, rootTest)
	}
	return fmt.Sprintf("go test -v %s", target)
}

// ComputeCrossPRCollisions finds shared modified files across open branches/PRs
// using an O(N * F) inverted index so mega-repos with many open branches stay fast.
func ComputeCrossPRCollisions(branches []model.BranchArc) {
	for i := range branches {
		branches[i].CrossPR = nil
	}

	fileToBranches := map[string][]int{}
	for i := range branches {
		// Madde 2: Cross-PR çarpışması YALNIZCA açık PR'lar arasında hesaplanır.
		if branches[i].State != "open" || branches[i].PR == nil || branches[i].PR.Number <= 0 || branches[i].PR.ReviewState == "MERGED" || len(branches[i].ChangedFiles) == 0 {
			continue
		}
		seenFile := map[string]bool{}
		for _, f := range branches[i].ChangedFiles {
			if seenFile[f] {
				continue
			}
			seenFile[f] = true
			fileToBranches[f] = append(fileToBranches[f], i)
		}
	}

	pairShared := map[[2]int][]string{}
	for f, idxs := range fileToBranches {
		if len(idxs) < 2 {
			continue
		}
		for a := 0; a < len(idxs); a++ {
			for b := a + 1; b < len(idxs); b++ {
				i, j := idxs[a], idxs[b]
				if i > j {
					i, j = j, i
				}
				key := [2]int{i, j}
				pairShared[key] = append(pairShared[key], f)
			}
		}
	}

	keys := make([][2]int, 0, len(pairShared))
	for k := range pairShared {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(a, b int) bool {
		if keys[a][0] != keys[b][0] {
			return keys[a][0] < keys[b][0]
		}
		return keys[a][1] < keys[b][1]
	})

	for _, key := range keys {
		i, j := key[0], key[1]
		shared := pairShared[key]
		sort.Strings(shared)

		prNumA := 0
		if branches[i].PR != nil {
			prNumA = branches[i].PR.Number
		}
		prNumB := 0
		if branches[j].PR != nil {
			prNumB = branches[j].PR.Number
		}

		branches[i].CrossPR = append(branches[i].CrossPR, model.CrossPRWarning{
			OtherBranchID: branches[j].ID,
			OtherBranch:   branches[j].Name,
			OtherPRNumber: prNumB,
			SharedFiles:   shared,
		})
		branches[j].CrossPR = append(branches[j].CrossPR, model.CrossPRWarning{
			OtherBranchID: branches[i].ID,
			OtherBranch:   branches[i].Name,
			OtherPRNumber: prNumA,
			SharedFiles:   shared,
		})
	}
}

// IsSpecialAutoMergeFile returns true for CHANGELOG, VERSION, and lock/module files
// that GitArc can intelligently diagnose and auto-resolve.
func IsSpecialAutoMergeFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	switch base {
	case "changelog.md", "changelog", "components.lock", "go.mod", "go.sum", "version", "package-lock.json", "yarn.lock", "pnpm-lock.yaml", "cargo.lock":
		return true
	default:
		return false
	}
}

// BuildDiagnostics aggregates prioritized smart radar entries from branches.
func BuildDiagnostics(branches []model.BranchArc) []model.DiagnosticItem {
	var items []model.DiagnosticItem

	for _, b := range branches {
		if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
			summary := b.PR.CI.FailedStep
			if len(b.PR.CI.Extracts) > 0 {
				e := b.PR.CI.Extracts[0]
				summary = fmt.Sprintf("%s (%s)", e.Symbol, e.FileLine)
				if e.FileLine == "" {
					summary = e.Message
				}
			}
			items = append(items, model.DiagnosticItem{
				ID:         "ci-" + b.ID,
				Severity:   "critical",
				Kind:       "ci_failure",
				BranchID:   b.ID,
				BranchName: b.Name,
				Title:      fmt.Sprintf("PR #%d CI Hatası: %s", b.PR.Number, b.Name),
				Detail:     summary,
				ActionHint: "Ayıklanmış logu aç ve yerelde yeniden çalıştır",
			})
		}

		if b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved {
			title := "Merge Çakışması Tespit Edildi"
			hint := "git merge-tree --write-tree önizlemesini incele"
			if b.Conflict.IsChangelogOrLock {
				title = "CHANGELOG / Lockfile Çakışması"
				hint = "Her iki tarafı koruyarak Auto-Resolve çalıştır"
			}
			items = append(items, model.DiagnosticItem{
				ID:         "conf-" + b.ID,
				Severity:   "warning",
				Kind:       "conflict",
				BranchID:   b.ID,
				BranchName: b.Name,
				Title:      title,
				Detail:     strings.Join(b.Conflict.Files, ", "),
				ActionHint: hint,
				Files:      b.Conflict.Files,
			})
		}

		if len(b.CrossPR) > 0 {
			var peers []string
			var files []string
			seenF := map[string]bool{}
			for _, w := range b.CrossPR {
				label := w.OtherBranch
				if w.OtherPRNumber > 0 {
					label = fmt.Sprintf("#%d", w.OtherPRNumber)
				}
				peers = append(peers, label)
				for _, f := range w.SharedFiles {
					if !seenF[f] {
						seenF[f] = true
						files = append(files, f)
					}
				}
			}
			items = append(items, model.DiagnosticItem{
				ID:         "cross-" + b.ID,
				Severity:   "warning",
				Kind:       "cross_pr",
				BranchID:   b.ID,
				BranchName: b.Name,
				Title:      fmt.Sprintf("Cross-PR Çarpışma (%s)", strings.Join(peers, ", ")),
				Detail:     fmt.Sprintf("Ortak dosyalar: %s", strings.Join(files, ", ")),
				ActionHint: "Sıralı merge veya erken rebase önerilir",
				Files:      files,
			})
		}

		if (b.OriginGone || b.State == "gone") && !b.LongLived {
			items = append(items, model.DiagnosticItem{
				ID:         "gone-" + b.ID,
				Severity:   "info",
				Kind:       "prune_candidate",
				BranchID:   b.ID,
				BranchName: b.Name,
				Title:      "Uzakta Silinmiş Dal (origin: gone)",
				Detail:     fmt.Sprintf("%s uzakta merge edilip silinmiş; yerelde temizlenebilir", b.Name),
				ActionHint: "Tek Tıkla Temizle (Prune Gone)",
			})
		}
	}

	sevRank := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(items, func(i, j int) bool {
		if sevRank[items[i].Severity] != sevRank[items[j].Severity] {
			return sevRank[items[i].Severity] < sevRank[items[j].Severity]
		}
		return items[i].BranchName < items[j].BranchName
	})

	return items
}

// mainCIFailureDiag: omurga dalının son CI çalıştırması başarısızsa kritik uyarı (merge sonrası testler kırık).
func mainCIFailureDiag(mc *model.MainCIStatus, defaultBranch string) model.DiagnosticItem {
	detail := strings.TrimSpace(strings.Join([]string{mc.SHA, mc.Title}, " · "))
	if strings.Trim(detail, " ·") == "" {
		detail = "Son çalıştırma başarısız"
	}
	return model.DiagnosticItem{
		ID:         "main-ci-failure",
		Severity:   "critical",
		Kind:       "main_ci_failure",
		BranchName: defaultBranch,
		Title:      fmt.Sprintf("%s CI kırık: %s", defaultBranch, mc.Workflow),
		Detail:     detail,
		ActionHint: "Başarısız çalıştırmayı GitHub Actions'ta açıp inceleyin",
		URL:        mc.URL,
	}
}
