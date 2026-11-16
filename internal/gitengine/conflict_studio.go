package gitengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitarc/internal/model"
)

func runCmdStdout(ctx context.Context, dir, bin string, args ...string) (string, string, int, error) {
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
	return stdout.String(), stderr.String(), exitCode, err
}

func normalizeStudioStrategy(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "union", "union-changelog", "lock-tidy":
		return "union"
	case "ours", "main":
		return "ours"
	case "theirs", "branch":
		return "theirs"
	case "custom", "manual":
		return "custom"
	default:
		return "union"
	}
}

func splitRawLines(raw string) ([]string, bool) {
	hadTrailingNL := strings.HasSuffix(raw, "\n")
	trimmed := strings.TrimSuffix(raw, "\n")
	if trimmed == "" {
		return []string{}, hadTrailingNL
	}
	lines := strings.Split(trimmed, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines, hadTrailingNL
}

func conflictWindowBounds(lines []string) (int, int) {
	firstMarker := -1
	lastMarker := -1
	for i, line := range lines {
		if firstMarker == -1 && strings.HasPrefix(line, "<<<<<<< ") {
			firstMarker = i
		}
		if strings.HasPrefix(line, ">>>>>>> ") {
			lastMarker = i
		}
	}
	if firstMarker == -1 || lastMarker == -1 || lastMarker < firstMarker {
		return 0, len(lines)
	}
	start := firstMarker - 6
	if start < 0 {
		start = 0
	}
	end := lastMarker + 7
	if end > len(lines) {
		end = len(lines)
	}
	return start, end
}

func resolveConflictLines(lines []string, strategy string) string {
	var out []string
	state := "normal" // "normal" | "ours" | "base" | "theirs"
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "<<<<<<< "):
			state = "ours"
		case state == "ours" && strings.HasPrefix(line, "||||||| "):
			state = "base"
		case (state == "ours" || state == "base") && line == "=======":
			state = "theirs"
		case state != "normal" && strings.HasPrefix(line, ">>>>>>> "):
			state = "normal"
		default:
			switch state {
			case "normal":
				out = append(out, line)
			case "ours":
				if strategy == "ours" || strategy == "union" {
					out = append(out, line)
				}
			case "base":
				// common ancestor lines omitted unless explicitly kept; none needed here
			case "theirs":
				if strategy == "theirs" || strategy == "union" {
					out = append(out, line)
				}
			}
		}
	}
	return strings.Join(out, "\n")
}

func buildConflictFileDiff(path, rawConflict string) model.ConflictFileDiff {
	lines, _ := splitRawLines(rawConflict)
	start, end := conflictWindowBounds(lines)
	windowLines := lines[start:end]

	startLine := start + 1
	endLine := end
	if len(windowLines) == 0 {
		startLine = 0
		endLine = 0
	}

	return model.ConflictFileDiff{
		Path:          path,
		StartLine:     startLine,
		EndLine:       endLine,
		ConflictHunks: strings.Join(windowLines, "\n"),
		UnionPreview:  resolveConflictLines(windowLines, "union"),
		OursPreview:   resolveConflictLines(windowLines, "ours"),
		TheirsPreview: resolveConflictLines(windowLines, "theirs"),
	}
}

func applyWindowResolution(rawConflict, strategy, customWindow string) string {
	lines, hadTrailingNL := splitRawLines(rawConflict)
	var resolved string
	if customWindow != "" {
		start, end := conflictWindowBounds(lines)
		customTrimmed := strings.TrimSuffix(strings.ReplaceAll(customWindow, "\r\n", "\n"), "\n")
		var customLines []string
		if customTrimmed != "" {
			customLines = strings.Split(customTrimmed, "\n")
		}
		merged := make([]string, 0, len(lines)-(end-start)+len(customLines))
		merged = append(merged, lines[:start]...)
		merged = append(merged, customLines...)
		merged = append(merged, lines[end:]...)
		resolved = strings.Join(merged, "\n")
	} else {
		norm := normalizeStudioStrategy(strategy)
		if norm == "custom" {
			norm = "union"
		}
		resolved = resolveConflictLines(lines, norm)
	}
	if hadTrailingNL && resolved != "" && !strings.HasSuffix(resolved, "\n") {
		resolved += "\n"
	}
	return resolved
}

func hasStrayConflictMarkers(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(l, "<<<<<<< ") || l == "=======" || strings.HasPrefix(l, ">>>>>>> ") {
			return true
		}
	}
	return false
}

func buildConflictAIPrompt(branchName, baseRef string, files []model.ConflictFileDiff) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("GitArc Çakışma Çözümü İsteği (%s ↔ %s)\n", branchName, baseRef))
	sb.WriteString("Aşağıdaki çakışan blok(ları) bozulmayacak şekilde birleştir. Yalnızca temizlenmiş kodu üret veya MCP `gitarc_resolve_conflict` aracını çağır.\n\n")
	for _, f := range files {
		sb.WriteString(fmt.Sprintf("--- Dosya: %s (Satır %d-%d) ---\n%s\n\n", f.Path, f.StartLine, f.EndLine, f.ConflictHunks))
	}
	return strings.TrimSpace(sb.String())
}

// InspectConflictStudio extracts focused 3-way conflict hunks and Union/Ours/Theirs previews
// without mutating the repository working directory.
func (m *Manager) InspectConflictStudio(branchID string) (model.ConflictStudioPayload, error) {
	if live, ok := m.activeLiveRepo(); ok {
		return m.inspectConflictStudioLive(live, branchID)
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, b := range m.simRepo.branches {
		if b.ID == branchID || b.Name == branchID {
			if b.Conflict == nil || !b.Conflict.HasConflict {
				return model.ConflictStudioPayload{}, fmt.Errorf("%s dalında açık çakışma yok", b.Name)
			}
			var files []model.ConflictFileDiff
			for _, f := range b.Conflict.Files {
				raw := simRawConflictSample(f, b.Name)
				files = append(files, buildConflictFileDiff(f, raw))
			}
			rec := "ours"
			if b.Conflict.IsChangelogOrLock {
				rec = "union"
			}
			return model.ConflictStudioPayload{
				BranchID:    b.ID,
				BranchName:  b.Name,
				BaseRef:     "origin/" + m.simRepo.repo.DefaultBranch,
				Recommended: rec,
				AIPrompt:    buildConflictAIPrompt(b.Name, "origin/"+m.simRepo.repo.DefaultBranch, files),
				Files:       files,
			}, nil
		}
	}
	return model.ConflictStudioPayload{}, fmt.Errorf("dal bulunamadı: %s", branchID)
}

func simRawConflictSample(path, branchName string) string {
	low := strings.ToLower(filepath.Base(path))
	if strings.Contains(low, "changelog") {
		return fmt.Sprintf("<<<<<<< origin/main\n## [4.0.16] - 2026-11-15\n- utility 0.2.0 imzalı kilit özeti güncellendi (components.lock)\n- WebKit misafir mod önbellek temizliği eklendi\n=======\n## [Unreleased] - %s\n- terminal-audit.log CSV ve JSON dışa aktarma uçları eklendi\n- formül enjeksiyonu koruması eklendi\n>>>>>>> %s\n", branchName, branchName)
	}
	if strings.Contains(low, "lock") {
		return fmt.Sprintf("<<<<<<< origin/main\nutility 0.2.0 sha256:9f82c10d\n=======\naudit-exporter 0.1.0 sha256:44a1e8b2\n>>>>>>> %s\n", branchName)
	}
	return fmt.Sprintf("<<<<<<< origin/main\n// origin/main üzerindeki güncel uygulama\nreturn nil\n=======\n// %s dalındaki yeni özellik mantığı\nreturn applyFeature()\n>>>>>>> %s\n", branchName, branchName)
}

func (m *Manager) inspectConflictStudioLive(live *repoEntry, branchID string) (model.ConflictStudioPayload, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.ConflictStudioPayload{}, err
	}
	if b.Conflict == nil || !b.Conflict.HasConflict || len(b.Conflict.Files) == 0 {
		return model.ConflictStudioPayload{}, fmt.Errorf("%s dalında aktif merge çakışması bulunamadı", b.Name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	baseRef, branchRef, mbSHA, err := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, b)
	if err != nil {
		return model.ConflictStudioPayload{}, err
	}

	tmpDir, err := os.MkdirTemp("", "gitarc-studio-*")
	if err != nil {
		return model.ConflictStudioPayload{}, err
	}
	defer os.RemoveAll(tmpDir)

	var files []model.ConflictFileDiff
	for _, relPath := range b.Conflict.Files {
		rawConflict, err := generateThreeWayMergeText(ctx, live.path, tmpDir, mbSHA, baseRef, branchRef, relPath, b.Name)
		if err != nil {
			return model.ConflictStudioPayload{}, err
		}
		files = append(files, buildConflictFileDiff(relPath, rawConflict))
	}

	rec := "ours"
	if b.Conflict.IsChangelogOrLock {
		rec = "union"
	}
	return model.ConflictStudioPayload{
		BranchID:    b.ID,
		BranchName:  b.Name,
		BaseRef:     baseRef,
		Recommended: rec,
		AIPrompt:    buildConflictAIPrompt(b.Name, baseRef, files),
		Files:       files,
	}, nil
}

func resolveConflictRefs(ctx context.Context, repoPath, defBranch, remoteName string, b model.BranchArc) (baseRef, branchRef, mbSHA string, err error) {
	if defBranch == "" {
		defBranch = "main"
	}
	if remoteName == "" {
		remoteName = "origin"
	}
	baseRef = defBranch
	remoteDefault := remoteName + "/" + defBranch
	if _, _, _, e := runCmdStdout(ctx, repoPath, "git", "show-ref", "--verify", "--quiet", "refs/remotes/"+remoteDefault); e == nil {
		baseRef = remoteDefault
	}

	branchRef = b.Name
	if !b.HasLocal {
		if b.RemoteRef != "" {
			branchRef = b.RemoteRef
		} else {
			branchRef = remoteName + "/" + b.Name
		}
	}

	mbOut, mbErr, _, e := runCmdStdout(ctx, repoPath, "git", "merge-base", baseRef, branchRef)
	if e != nil {
		return "", "", "", fmt.Errorf("merge-base hesaplanamadı (%s vs %s): %s", baseRef, branchRef, strings.TrimSpace(mbErr))
	}
	return baseRef, branchRef, strings.TrimSpace(mbOut), nil
}

func gitShowOrEmpty(ctx context.Context, repoPath, rev, relPath string) string {
	if rev == "" {
		return ""
	}
	out, _, _, err := runCmdStdout(ctx, repoPath, "git", "show", rev+":"+relPath)
	if err != nil {
		return ""
	}
	return out
}

func generateThreeWayMergeText(ctx context.Context, repoPath, tmpDir, mbSHA, baseRef, branchRef, relPath, branchLabel string) (string, error) {
	baseTxt := gitShowOrEmpty(ctx, repoPath, mbSHA, relPath)
	oursTxt := gitShowOrEmpty(ctx, repoPath, baseRef, relPath)
	theirsTxt := gitShowOrEmpty(ctx, repoPath, branchRef, relPath)

	baseFile := filepath.Join(tmpDir, "base.tmp")
	oursFile := filepath.Join(tmpDir, "ours.tmp")
	theirsFile := filepath.Join(tmpDir, "theirs.tmp")

	if err := os.WriteFile(baseFile, []byte(baseTxt), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(oursFile, []byte(oursTxt), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(theirsFile, []byte(theirsTxt), 0600); err != nil {
		return "", err
	}

	stdout, stderr, code, err := runCmdStdout(ctx, repoPath, "git", "merge-file", "-p",
		"-L", baseRef, "-L", "base", "-L", branchLabel,
		oursFile, baseFile, theirsFile)
	if err != nil && code < 0 {
		return "", fmt.Errorf("git merge-file başarısız (%s): %s", relPath, strings.TrimSpace(stderr))
	}
	if err != nil && code == 128 {
		return "", fmt.Errorf("git merge-file hatası (%s): %s", relPath, strings.TrimSpace(stderr))
	}
	return stdout, nil
}

// ResolveBranchConflict resolves conflicts on branchID using one of four strategies:
// "union", "ours", "theirs", or "custom" (via customFiles window overrides),
// executed inside an isolated temporary git worktree without modifying the user's active tree.
func (m *Manager) ResolveBranchConflict(branchID, strategy string, customFiles map[string]string, runVet bool) (model.CockpitSnapshot, error) {
	strategy = normalizeStudioStrategy(strategy)
	if live, ok := m.activeLiveRepo(); ok {
		return m.resolveConflictLive(live, branchID, strategy, customFiles, runVet)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for i := range m.simRepo.branches {
		b := &m.simRepo.branches[i]
		if b.ID == branchID || b.Name == branchID {
			if b.Conflict == nil || !b.Conflict.HasConflict {
				return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
			}
			note := "Union merge + kilit bütünlük doğrulaması uygulandı; çalışma dizini temiz."
			switch strategy {
			case "ours":
				note = "origin/main sürümü baz alınarak çakışma çözüldü; çalışma dizini temiz."
			case "theirs":
				note = "Özellik dalı kodu korunarak çakışma çözüldü; çalışma dizini temiz."
			case "custom":
				note = "Özel / AI editör çözümü uygulandı; go vet doğrulandı."
			}
			b.Conflict.HasConflict = false
			b.Conflict.Resolved = true
			b.Conflict.Strategy = strategy
			b.Conflict.ResolvedNote = note
			b.Conflict.PreviewUnified = fmt.Sprintf("✓ Çakışma Çözüm Stüdyosu (%s) ile temizlendi.\n✓ git merge-tree --write-tree: exit 0 (temiz)", strategy)
			b.BehindMain = 0
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	return model.CockpitSnapshot{}, fmt.Errorf("dal bulunamadı: %s", branchID)
}

func (m *Manager) resolveConflictLive(live *repoEntry, branchID, strategy string, customFiles map[string]string, runVet bool) (model.CockpitSnapshot, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if b.Conflict == nil || !b.Conflict.HasConflict || len(b.Conflict.Files) == 0 {
		return m.Snapshot()
	}
	if b.IsHEAD && snap.Repo.DirtyCount > 0 {
		return model.CockpitSnapshot{}, fmt.Errorf("aktif dal (%s) üzerinde %d uncommitted değişiklik varken çakışma uygulanamaz; önce commit veya stash yapın", b.Name, snap.Repo.DirtyCount)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	baseRef, branchRef, mbSHA, err := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, b)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}

	wtDir, err := os.MkdirTemp("", "gitarc-wt-*")
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_, _, _, _ = runCmdStdout(cleanCtx, live.path, "git", "worktree", "remove", "--force", wtDir)
		_ = os.RemoveAll(wtDir)
	}()

	if _, errOut, _, err := runCmdStdout(ctx, live.path, "git", "worktree", "add", "--detach", wtDir, branchRef); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("izole worktree oluşturulamadı: %s", strings.TrimSpace(errOut))
	}

	gitIdentityArgs := buildGitIdentityArgs(ctx, live.path)

	mergeArgs := append([]string{}, gitIdentityArgs...)
	mergeArgs = append(mergeArgs, "merge", "--no-commit", "--no-ff", baseRef)
	_, mergeErrOut, mergeCode, mergeErr := runCmdStdout(ctx, wtDir, "git", mergeArgs...)
	if mergeErr != nil && mergeCode != 0 {
		if _, _, _, mhErr := runCmdStdout(ctx, wtDir, "git", "rev-parse", "-q", "--verify", "MERGE_HEAD"); mhErr != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("worktree içinde merge başlatılamadı: %s", strings.TrimSpace(mergeErrOut))
		}
	}

	tmpDir, err := os.MkdirTemp("", "gitarc-mergefiles-*")
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	defer os.RemoveAll(tmpDir)

	for _, relPath := range b.Conflict.Files {
		rawConflict, err := generateThreeWayMergeText(ctx, live.path, tmpDir, mbSHA, baseRef, branchRef, relPath, b.Name)
		if err != nil {
			return model.CockpitSnapshot{}, err
		}
		customWin := ""
		if customFiles != nil {
			customWin = customFiles[relPath]
		}
		resolvedContent := applyWindowResolution(rawConflict, strategy, customWin)
		if hasStrayConflictMarkers(resolvedContent) {
			return model.CockpitSnapshot{}, fmt.Errorf("%s için sağlanan çözümde hâlâ <<<<<<< / >>>>>>> işaretleri var", relPath)
		}
		targetPath := filepath.Join(wtDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return model.CockpitSnapshot{}, err
		}
		if err := os.WriteFile(targetPath, []byte(resolvedContent), 0644); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("çözüm yazılamadı (%s): %w", relPath, err)
		}
		if _, addErr, _, err := runCmdStdout(ctx, wtDir, "git", "add", "--", relPath); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("dosya stage edilemedi (%s): %s", relPath, strings.TrimSpace(addErr))
		}
	}

	if unmergedOut, _, _, _ := runCmdStdout(ctx, wtDir, "git", "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(unmergedOut) != "" {
		return model.CockpitSnapshot{}, fmt.Errorf("çözülmemiş başka dosyalar kaldı: %s", strings.TrimSpace(unmergedOut))
	}

	if runVet {
		if _, statErr := os.Stat(filepath.Join(wtDir, "go.mod")); statErr == nil {
			vetOut, vetErrOut, _, vetErr := runCmdStdout(ctx, wtDir, "go", "vet", "./...")
			if vetErr != nil {
				combined := strings.TrimSpace(vetOut + "\n" + vetErrOut)
				return model.CockpitSnapshot{}, fmt.Errorf("go vet doğrulaması başarısız oldu, bozuk kod pushlanmadı:\n%s", combined)
			}
		}
	}

	commitMsg := fmt.Sprintf("merge: resolve conflict with %s via GitArc (%s)", baseRef, strategy)
	commitArgs := append([]string{}, gitIdentityArgs...)
	commitArgs = append(commitArgs, "commit", "--no-edit", "-m", commitMsg)
	if _, cErrOut, _, err := runCmdStdout(ctx, wtDir, "git", commitArgs...); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("worktree merge commit atılamadı: %s", strings.TrimSpace(cErrOut))
	}

	shaOut, _, _, err := runCmdStdout(ctx, wtDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("yeni commit SHA alınamadı: %w", err)
	}
	newSHA := strings.TrimSpace(shaOut)

	if b.HasLocal {
		if b.IsHEAD {
			if _, rErr, _, err := runCmdStdout(ctx, live.path, "git", "reset", "--hard", newSHA); err != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("aktif dal güncellenemedi: %s", strings.TrimSpace(rErr))
			}
		} else {
			if _, bErr, _, err := runCmdStdout(ctx, live.path, "git", "branch", "-f", b.Name, newSHA); err != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("yerel dal güncellenemedi: %s", strings.TrimSpace(bErr))
			}
		}
	}

	if b.HasRemote {
		remoteName := snap.Repo.RemoteName
		if remoteName == "" {
			remoteName = "origin"
		}
		if _, pErr, _, err := runCmdStdout(ctx, wtDir, "git", "push", remoteName, newSHA+":refs/heads/"+b.Name); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("çözüm pushlanamadı: %s", strings.TrimSpace(pErr))
		}
		_, _, _, _ = runCmdStdout(ctx, live.path, "git", "fetch", remoteName)
	}

	InvalidateGHCache(live.path)
	return m.Snapshot()
}

func buildGitIdentityArgs(ctx context.Context, repoPath string) []string {
	var args []string
	if out, _, _, err := runCmdStdout(ctx, repoPath, "git", "config", "user.name"); err != nil || strings.TrimSpace(out) == "" {
		args = append(args, "-c", "user.name=GitArc")
	}
	if out, _, _, err := runCmdStdout(ctx, repoPath, "git", "config", "user.email"); err != nil || strings.TrimSpace(out) == "" {
		args = append(args, "-c", "user.email=gitarc@local")
	}
	return args
}
