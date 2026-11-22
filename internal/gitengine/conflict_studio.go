package gitengine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func buildStrategyAdvice(b model.BranchArc, files []model.ConflictFileDiff) (recommended, reason string, advices []model.StrategyAdvice) {
	hasChangelog := false
	hasVersion := false
	hasLock := false
	hasSourceCode := false
	sourcePath := ""

	for _, f := range files {
		base := strings.ToLower(filepath.Base(f.Path))
		if strings.Contains(base, "changelog") {
			hasChangelog = true
		} else if base == "version" {
			hasVersion = true
		} else if strings.Contains(base, "lock") || strings.HasSuffix(base, ".sum") {
			hasLock = true
		} else {
			hasSourceCode = true
			if sourcePath == "" {
				sourcePath = f.Path
			}
		}
	}

	// Madde 4: Auto-Resolve "union" YALNIZ CHANGELOG.md için.
	// VERSION ve components.lock için otomatik çözüm önerme; insan onayı iste ve nedenini yaz.
	if hasVersion {
		recommended = "custom"
		reason = "Çakışan dosyalarda VERSION bulunmaktadır. Sürüm dosyaları tekil sürüm etiketi içerir; iki sürüm numarası alt alta eklenemez. İnsan onayı ile doğru hedef sürüm manuel/AI editöründe seçilmelidir."
		advices = []model.StrategyAdvice{
			{
				Strategy: "custom",
				Title:    "★ İnsan Onayı & Elle Seçim (VERSION)",
				Subtitle: "Sürüm dosyasında hedef sürümü manuel veya AI ile belirleyin.",
				Badge:    "★ İNSAN ONAYI GEREKLİ",
				Tone:     "ok",
			},
			{
				Strategy: "ours",
				Title:    "2. 🛡️ Main'deki Sürümü Koru (Ours)",
				Subtitle: "origin/main üzerindeki mevcut sürüm numarasını korur.",
				Badge:    "Main Sürümü",
				Tone:     "warn",
			},
			{
				Strategy: "theirs",
				Title:    "3. 🚀 Daldaki Yeni Sürümü Kullan (Theirs)",
				Subtitle: "Bu dalda tanımlanan sürüm numarasını kullanır.",
				Badge:    "Dal Sürümü",
				Tone:     "accent",
			},
			{
				Strategy: "union",
				Title:    "4. ✦ İkisini Alt Alta Koru (Union)",
				Subtitle: "Geçersiz sürüm formatı üretir (önerilmez).",
				Badge:    "⛔ Sürüm Formatı Bozulur",
				Tone:     "danger",
			},
		}
		return recommended, reason, advices
	}

	if hasLock {
		recommended = "custom"
		reason = "Çakışan dosyalarda kilit dosyası (components.lock / go.sum) bulunmaktadır. Kilit dosyaları paket bağımlılıkları ve SHA256 bütünlük imzaları içerir; union (alt alta ekleme) dosya formatını ve imza doğrulamasını bozar. İnsan onayı ile paket yöneticisi çalıştırılmalı veya doğru blok seçilmelidir."
		advices = []model.StrategyAdvice{
			{
				Strategy: "custom",
				Title:    "★ İnsan Onayı & Kilit Yenileme",
				Subtitle: "Kilit dosyasını paket yöneticisiyle veya editörde inceleyerek çözün.",
				Badge:    "★ İNSAN ONAYI GEREKLİ",
				Tone:     "ok",
			},
			{
				Strategy: "ours",
				Title:    "2. 🛡️ Main'deki Kilidi Kullan (Ours)",
				Subtitle: "origin/main üzerindeki doğrulanmış kilit özetini korur.",
				Badge:    "Main Kilidi",
				Tone:     "warn",
			},
			{
				Strategy: "theirs",
				Title:    "3. 🚀 Daldaki Kilidi Kullan (Theirs)",
				Subtitle: "Daldaki yeni bağımlılık özetlerini kullanır.",
				Badge:    "Dal Kilidi",
				Tone:     "accent",
			},
			{
				Strategy: "union",
				Title:    "4. ✦ İkisini Alt Alta Koru (Union)",
				Subtitle: "Bozuk JSON/YAML veya çift kayıt riski taşır.",
				Badge:    "⚠ İmza / Format Bozulma Riski",
				Tone:     "danger",
			},
		}
		return recommended, reason, advices
	}

	if hasChangelog && !hasSourceCode {
		recommended = "union"
		reason = "Çakışan dosya değişiklik günlüğüdür (CHANGELOG.md). '1. İkisini Alt Alta Koru (Union)' seçeneği hem main hem daldaki sürüm notlarını veri kaybı olmadan alt alta birleştirir."
		advices = []model.StrategyAdvice{
			{
				Strategy: "union",
				Title:    "1. ✦ İkisini Alt Alta Koru (Union)",
				Subtitle: "Hem main hem daldaki değişiklik notlarını korur.",
				Badge:    "★ ÖNERİLEN · Veri Kaybı Yok",
				Tone:     "ok",
			},
			{
				Strategy: "ours",
				Title:    "2. 🛡️ Main'deki Güncel Notları Koru (Ours)",
				Subtitle: "Çakışan kısımda yalnızca origin/main notlarını tutar.",
				Badge:    "Daldaki Notları Atlar",
				Tone:     "warn",
			},
			{
				Strategy: "theirs",
				Title:    "3. 🚀 Sadece Bu Dalın Notlarını Kullan (Theirs)",
				Subtitle: "Çakışan kısımda yalnızca bu dalın notlarını bırakır.",
				Badge:    "Main'deki Notları Atlar",
				Tone:     "warn",
			},
			{
				Strategy: "custom",
				Title:    "4. 🤖 AI / Elle Düzenle",
				Subtitle: "Sağdaki editörde veya MCP üzerinden düzenleyin.",
				Badge:    "Tam Kontrol",
				Tone:     "accent",
			},
		}
		return recommended, reason, advices
	}

	recommended = "ours"
	reason = fmt.Sprintf("Çakışan dosya kaynak kodudur (%s). Kaynak kodda 'İkisini Alt Alta Koru (Union)' seçilirse aynı değişken/fonksiyon iki kez yazılıp derleme hatası (go vet) üretebilir. '2. Main'deki Güncel Kodu Kullan (Ours)' veya '4. AI / Elle Düzenle' önerilir.", sourcePath)
	advices = []model.StrategyAdvice{
		{
			Strategy: "union",
			Title:    "1. ✦ İkisini Alt Alta Koru (Union)",
			Subtitle: "İki bloğu alt alta yazar (kodda tekrar tanım yapabilir).",
			Badge:    "⚠ Kodda Çift Tanım / Hata Riski",
			Tone:     "warn",
		},
		{
			Strategy: "ours",
			Title:    "2. 🛡️ Main'deki Güncel Kodu Kullan (Ours)",
			Subtitle: "Çakışan blokta origin/main güncel kodunu korur.",
			Badge:    "★ ÖNERİLEN · Main Güncel Kodu",
			Tone:     "ok",
		},
		{
			Strategy: "theirs",
			Title:    "3. 🚀 Sadece Bu Dalın Kodunu Kullan (Theirs)",
			Subtitle: "Çakışan blokta yalnızca bu daldaki kodu kullanır.",
			Badge:    "✓ Sadece Dalın Kodunu Yazar",
			Tone:     "accent",
		},
		{
			Strategy: "custom",
			Title:    "4. 🤖 AI / Elle Düzenle",
			Subtitle: "İki bloğu harmanlamak için sağdaki editörü veya AI'ı kullan.",
			Badge:    "✓ Özgün Birleştirme",
			Tone:     "accent",
		},
	}
	return recommended, reason, advices
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
			rec, reason, advices := buildStrategyAdvice(b, files)
			return model.ConflictStudioPayload{
				BranchID:             b.ID,
				BranchName:           b.Name,
				BaseRef:              "origin/" + m.simRepo.repo.DefaultBranch,
				Recommended:          rec,
				RecommendationReason: reason,
				Strategies:           advices,
				AIPrompt:             buildConflictAIPrompt(b.Name, "origin/"+m.simRepo.repo.DefaultBranch, files),
				Files:                files,
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

// freshConflict: dalın taban dalla GÜNCEL (önbelleksiz) merge-tree sonucu. Hata/zaman aşımı "çakışma yok" sayılmaz (nil
// döner ama çağıran bunu "belirsiz" olarak ele almalıdır); yalnızca çıkış kodu 1 (çakışma) ya da 0 (temiz) güvenilirdir.
func freshConflict(ctx context.Context, repoPath, baseRef, branchRef string) *model.ConflictDetail {
	out, code, _ := runCmd(ctx, repoPath, "git", "merge-tree", "--write-tree", baseRef, branchRef)
	if ctx.Err() != nil || (code != 0 && code != 1) {
		return nil
	}
	return ParseMergeTreeOutput(code, out)
}

func (m *Manager) inspectConflictStudioLive(live *repoEntry, branchID string) (model.ConflictStudioPayload, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.ConflictStudioPayload{}, err
	}
	notFound := fmt.Errorf("%s dalında aktif merge çakışması bulunamadı", b.Name)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	baseRef, branchRef, mbSHA, refsErr := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, b)

	// Anlık görüntüdeki çakışma bayrağı önbellekten/bayat olabilir: kokpit çakışma gösterirken Stüdyo "yok" demesin diye
	// bayrak yoksa dal güncel merge-tree ile bir kez daha denetlenir.
	if b.Conflict == nil || !b.Conflict.HasConflict || len(b.Conflict.Files) == 0 {
		if refsErr != nil {
			return model.ConflictStudioPayload{}, notFound
		}
		c := freshConflict(ctx, live.path, baseRef, branchRef)
		if c == nil || !c.HasConflict || len(c.Files) == 0 {
			return model.ConflictStudioPayload{}, notFound
		}
		b.Conflict = c
	}
	if refsErr != nil {
		return model.ConflictStudioPayload{}, refsErr
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

	rec, reason, advices := buildStrategyAdvice(b, files)
	return model.ConflictStudioPayload{
		BranchID:             b.ID,
		BranchName:           b.Name,
		BaseRef:              baseRef,
		Recommended:          rec,
		RecommendationReason: reason,
		Strategies:           advices,
		AIPrompt:             buildConflictAIPrompt(b.Name, baseRef, files),
		Files:                files,
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
	return m.ResolveBranchConflictFiles(branchID, strategy, nil, customFiles, runVet)
}

// strategyLabel: günlük/commit için strateji adı. Dosya başına seçim varsa ve hepsi aynıysa o, farklıysa "karma".
func strategyLabel(strategy string, fileStrategies map[string]string) string {
	if len(fileStrategies) == 0 {
		return strategy
	}
	distinct := map[string]bool{}
	for _, v := range fileStrategies {
		distinct[normalizeStudioStrategy(v)] = true
	}
	if len(distinct) == 1 {
		for k := range distinct {
			return k
		}
	}
	return "karma"
}

// strategyDetail: "karma" çözümün commit mesajı için dosya başına özet ("a.go=ours, b.md=union"); karma değilse boş.
func strategyDetail(fileStrategies map[string]string) string {
	if strategyLabel("", fileStrategies) != "karma" {
		return ""
	}
	paths := make([]string, 0, len(fileStrategies))
	for p := range fileStrategies {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, p+"="+normalizeStudioStrategy(fileStrategies[p]))
	}
	return strings.Join(parts, ", ")
}

// ResolveBranchConflictFiles: ResolveBranchConflict'ın dosya başına strateji destekleyen hâli. fileStrategies (dosya yolu → union|ours|theirs|custom)
// içinde olmayan dosyalar `strategy`'yi kullanır. Anahtarlar yalnızca çakışan dosya listesinde ARANIR (hiçbir zaman yol olarak kullanılmaz).
func (m *Manager) ResolveBranchConflictFiles(branchID, strategy string, fileStrategies, customFiles map[string]string, runVet bool) (model.CockpitSnapshot, error) {
	strategy = normalizeStudioStrategy(strategy)
	if live, ok := m.activeLiveRepo(); ok {
		return m.resolveConflictLive(live, branchID, strategy, fileStrategies, customFiles, runVet)
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
			label := strategyLabel(strategy, fileStrategies)
			if label == "karma" {
				note = "Dosya başına seçilen stratejilerle çakışma çözüldü; çalışma dizini temiz."
			}
			b.Conflict.HasConflict = false
			b.Conflict.Resolved = true
			b.Conflict.Strategy = label
			b.Conflict.ResolvedNote = note
			b.Conflict.PreviewUnified = fmt.Sprintf("✓ Çakışma Çözüm Stüdyosu (%s) ile temizlendi.\n✓ git merge-tree --write-tree: exit 0 (temiz)", label)
			b.BehindMain = 0
			m.recordLogLocked("ok", "Çakışma Çözümü ("+label+")", b.Name, note, "")
			return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
		}
	}
	err := fmt.Errorf("dal bulunamadı: %s", branchID)
	m.recordLogLocked("error", "Çakışma Çözümü ("+strategy+")", branchID, err.Error(), "")
	return model.CockpitSnapshot{}, err
}

func (m *Manager) resolveConflictLive(live *repoEntry, branchID, strategy string, fileStrategies, customFiles map[string]string, runVet bool) (model.CockpitSnapshot, error) {
	label := strategyLabel(strategy, fileStrategies)
	failStep := func(branchName, cmdDesc string, err error) (model.CockpitSnapshot, error) {
		m.recordLog("error", "Çakışma Çözümü ("+label+")", branchName, err.Error(), cmdDesc)
		return model.CockpitSnapshot{}, err
	}

	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return failStep(branchID, "findLiveBranch", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	baseRef, branchRef, mbSHA, refsErr := resolveConflictRefs(ctx, live.path, snap.Repo.DefaultBranch, snap.Repo.RemoteName, b)
	// Anlık görüntüdeki çakışma bayrağı önbellekten/bayat olabilir; yokken dal güncel merge-tree ile bir kez denetlenir.
	if b.Conflict == nil || !b.Conflict.HasConflict || len(b.Conflict.Files) == 0 {
		if refsErr != nil {
			return m.Snapshot()
		}
		c := freshConflict(ctx, live.path, baseRef, branchRef)
		if c == nil || !c.HasConflict || len(c.Files) == 0 {
			return m.Snapshot()
		}
		b.Conflict = c
	}
	if refsErr != nil {
		return failStep(b.Name, "git merge-base", refsErr)
	}
	if b.IsHEAD && snap.Repo.DirtyCount > 0 {
		return failStep(b.Name, "dirty working tree guard", fmt.Errorf("aktif dal (%s) üzerinde %d uncommitted değişiklik varken çakışma uygulanamaz; önce commit veya stash yapın", b.Name, snap.Repo.DirtyCount))
	}

	wtDir, err := os.MkdirTemp("", "gitarc-wt-*")
	if err != nil {
		return failStep(b.Name, "os.MkdirTemp(gitarc-wt)", err)
	}
	defer func() {
		cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanCancel()
		_, _, _, _ = runCmdStdout(cleanCtx, live.path, "git", "worktree", "remove", "--force", wtDir)
		_ = os.RemoveAll(wtDir)
	}()

	if _, errOut, _, err := runCmdStdout(ctx, live.path, "git", "worktree", "add", "--detach", wtDir, branchRef); err != nil {
		return failStep(b.Name, fmt.Sprintf("git worktree add --detach %s %s", wtDir, branchRef), fmt.Errorf("izole worktree oluşturulamadı: %s", strings.TrimSpace(errOut)))
	}

	gitIdentityArgs := buildGitIdentityArgs(ctx, live.path)

	mergeArgs := append([]string{}, gitIdentityArgs...)
	mergeArgs = append(mergeArgs, "merge", "--no-commit", "--no-ff", baseRef)
	_, mergeErrOut, mergeCode, mergeErr := runCmdStdout(ctx, wtDir, "git", mergeArgs...)
	if mergeErr != nil && mergeCode != 0 {
		if _, _, _, mhErr := runCmdStdout(ctx, wtDir, "git", "rev-parse", "-q", "--verify", "MERGE_HEAD"); mhErr != nil {
			return failStep(b.Name, "git merge --no-commit --no-ff "+baseRef, fmt.Errorf("worktree içinde merge başlatılamadı: %s", strings.TrimSpace(mergeErrOut)))
		}
	}

	tmpDir, err := os.MkdirTemp("", "gitarc-mergefiles-*")
	if err != nil {
		return failStep(b.Name, "os.MkdirTemp(gitarc-mergefiles)", err)
	}
	defer os.RemoveAll(tmpDir)

	for _, relPath := range b.Conflict.Files {
		rawConflict, err := generateThreeWayMergeText(ctx, live.path, tmpDir, mbSHA, baseRef, branchRef, relPath, b.Name)
		if err != nil {
			return failStep(b.Name, "git merge-file "+relPath, err)
		}
		customWin := ""
		if customFiles != nil {
			customWin = customFiles[relPath]
		}
		fileStrat := strategy
		if fs, ok := fileStrategies[relPath]; ok {
			fileStrat = normalizeStudioStrategy(fs)
			if fileStrat != "custom" {
				customWin = "" // bu dosya için "özel" seçilmedi: eskiden yazılmış bir özel metin uygulanmaz
			}
		}
		resolvedContent := applyWindowResolution(rawConflict, fileStrat, customWin)
		if hasStrayConflictMarkers(resolvedContent) {
			return failStep(b.Name, "stray marker check ("+relPath+")", fmt.Errorf("%s için sağlanan çözümde hâlâ <<<<<<< / >>>>>>> işaretleri var", relPath))
		}
		targetPath := filepath.Join(wtDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return failStep(b.Name, "mkdir "+filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, []byte(resolvedContent), 0644); err != nil {
			return failStep(b.Name, "write "+relPath, fmt.Errorf("çözüm yazılamadı (%s): %w", relPath, err))
		}
		if _, addErr, _, err := runCmdStdout(ctx, wtDir, "git", "add", "--", relPath); err != nil {
			return failStep(b.Name, "git add -- "+relPath, fmt.Errorf("dosya stage edilemedi (%s): %s", relPath, strings.TrimSpace(addErr)))
		}
	}

	if unmergedOut, _, _, _ := runCmdStdout(ctx, wtDir, "git", "diff", "--name-only", "--diff-filter=U"); strings.TrimSpace(unmergedOut) != "" {
		return failStep(b.Name, "git diff --diff-filter=U", fmt.Errorf("çözülmemiş başka dosyalar kaldı: %s", strings.TrimSpace(unmergedOut)))
	}

	if runVet {
		if _, statErr := os.Stat(filepath.Join(wtDir, "go.mod")); statErr == nil {
			vetOut, vetErrOut, _, vetErr := runCmdStdout(ctx, wtDir, "go", "vet", "./...")
			if vetErr != nil {
				combined := strings.TrimSpace(vetOut + "\n" + vetErrOut)
				return failStep(b.Name, "go vet ./...", fmt.Errorf("go vet doğrulaması başarısız oldu, bozuk kod pushlanmadı:\n%s", combined))
			}
		}
	}

	commitLabel := label
	if d := strategyDetail(fileStrategies); d != "" {
		commitLabel = "karma: " + d
	}
	commitMsg := fmt.Sprintf("merge: resolve conflict with %s via GitArc (%s)", baseRef, commitLabel)
	commitArgs := append([]string{}, gitIdentityArgs...)
	commitArgs = append(commitArgs, "commit", "--no-edit", "-m", commitMsg)
	if _, cErrOut, _, err := runCmdStdout(ctx, wtDir, "git", commitArgs...); err != nil {
		return failStep(b.Name, "git commit --no-edit", fmt.Errorf("worktree merge commit atılamadı: %s", strings.TrimSpace(cErrOut)))
	}

	shaOut, _, _, err := runCmdStdout(ctx, wtDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return failStep(b.Name, "git rev-parse HEAD", fmt.Errorf("yeni commit SHA alınamadı: %w", err))
	}
	newSHA := strings.TrimSpace(shaOut)

	if b.HasLocal {
		if b.IsHEAD {
			if _, rErr, _, err := runCmdStdout(ctx, live.path, "git", "reset", "--hard", newSHA); err != nil {
				return failStep(b.Name, "git reset --hard "+newSHA, fmt.Errorf("aktif dal güncellenemedi: %s", strings.TrimSpace(rErr)))
			}
		} else {
			if _, bErr, _, err := runCmdStdout(ctx, live.path, "git", "branch", "-f", b.Name, newSHA); err != nil {
				return failStep(b.Name, "git branch -f "+b.Name, fmt.Errorf("yerel dal güncellenemedi: %s", strings.TrimSpace(bErr)))
			}
		}
	}

	if b.HasRemote {
		remoteName := snap.Repo.RemoteName
		if remoteName == "" {
			remoteName = "origin"
		}
		if _, pErr, _, err := runCmdStdout(ctx, wtDir, "git", "push", remoteName, newSHA+":refs/heads/"+b.Name); err != nil {
			return failStep(b.Name, fmt.Sprintf("git push %s %s:refs/heads/%s", remoteName, newSHA, b.Name), fmt.Errorf("çözüm pushlanamadı: %s", strings.TrimSpace(pErr)))
		}
		_, _, _, _ = runCmdStdout(ctx, live.path, "git", "fetch", remoteName)
	}

	shortNew := newSHA
	if len(shortNew) > 7 {
		shortNew = shortNew[:7]
	}
	m.recordLog("ok", "Çakışma Çözümü ("+label+")", b.Name, fmt.Sprintf("İzole git worktree içinde çözüldü (%s)", shortNew), fmt.Sprintf("base=%s files=%s", baseRef, strings.Join(b.Conflict.Files, ", ")))
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
