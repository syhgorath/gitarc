package gitengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gitarc/internal/model"
)

// GuardConfig defines repository security policies to prevent accidental leaks.
type GuardConfig struct {
	NoGitAddAll     bool     `json:"noGitAddAll"`
	SecretPatterns  []string `json:"secretPatterns"`
	ContentPatterns []string `json:"contentPatterns"`
}

// DefaultGuardConfig enforces strict defaults even when no .gitarc exists.
var DefaultGuardConfig = GuardConfig{
	NoGitAddAll: true,
	SecretPatterns: []string{
		"img/logoyeni.jpg",
		"referans/**",
		"referans/*",
		"referans",
		".mcp.json",
		".env*",
		"*.env",
		"*.pem",
		"*.key",
		"id_*",
		"id_rsa*",
		"id_ed25519*",
	},
	ContentPatterns: []string{
		"BEGIN OPENSSH PRIVATE KEY",
		"BEGIN RSA PRIVATE KEY",
		"BEGIN EC PRIVATE KEY",
		"BEGIN PRIVATE KEY",
		"BEGIN DSA PRIVATE KEY",
		"PRIVATE KEY-----",
	},
}

// LoadGuardConfig reads .gitarc or .gitarc.json from repoPath or returns DefaultGuardConfig.
func LoadGuardConfig(repoPath string) GuardConfig {
	cfg := GuardConfig{
		NoGitAddAll:     DefaultGuardConfig.NoGitAddAll,
		SecretPatterns:  append([]string(nil), DefaultGuardConfig.SecretPatterns...),
		ContentPatterns: append([]string(nil), DefaultGuardConfig.ContentPatterns...),
	}

	for _, fname := range []string{".gitarc", ".gitarc.json"} {
		p := filepath.Join(repoPath, fname)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var parsed GuardConfig
		if err := json.Unmarshal(data, &parsed); err == nil {
			cfg.NoGitAddAll = parsed.NoGitAddAll || cfg.NoGitAddAll
			if len(parsed.SecretPatterns) > 0 {
				cfg.SecretPatterns = mergeUniquePatterns(cfg.SecretPatterns, parsed.SecretPatterns)
			}
			if len(parsed.ContentPatterns) > 0 {
				cfg.ContentPatterns = mergeUniquePatterns(cfg.ContentPatterns, parsed.ContentPatterns)
			}
			break
		}
	}
	return cfg
}

func mergeUniquePatterns(base, custom []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range append(base, custom...) {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" && !seen[trimmed] {
			seen[trimmed] = true
			out = append(out, trimmed)
		}
	}
	return out
}

// IsSecretPath tests whether a relative file path matches any secret pattern.
func IsSecretPath(relPath string, patterns []string) (bool, string) {
	norm := filepath.ToSlash(filepath.Clean(relPath))
	norm = strings.TrimPrefix(norm, "./")
	normLower := strings.ToLower(norm)
	baseLower := strings.ToLower(filepath.Base(norm))

	for _, p := range patterns {
		patNorm := filepath.ToSlash(strings.TrimSpace(p))
		patNorm = strings.TrimPrefix(patNorm, "./")
		patLower := strings.ToLower(patNorm)

		// 1. Directory prefix match (e.g. referans/**, referans/*, referans/)
		if strings.HasSuffix(patLower, "/**") {
			dirPrefix := strings.TrimSuffix(patLower, "/**")
			if normLower == dirPrefix || strings.HasPrefix(normLower, dirPrefix+"/") {
				return true, p
			}
		} else if strings.HasSuffix(patLower, "/*") {
			dirPrefix := strings.TrimSuffix(patLower, "/*")
			if normLower == dirPrefix || strings.HasPrefix(normLower, dirPrefix+"/") {
				return true, p
			}
		} else if strings.HasSuffix(patLower, "/") {
			if strings.HasPrefix(normLower, patLower) {
				return true, p
			}
		} else if normLower == patLower {
			return true, p
		}

		// 2. Basename wildcard or exact match
		if !strings.Contains(patNorm, "/") {
			if matched, _ := filepath.Match(patLower, baseLower); matched {
				return true, p
			}
		}

		// 3. Full relative path match
		if matched, _ := filepath.Match(patLower, normLower); matched {
			return true, p
		}
	}
	return false, ""
}

// CheckFileContentForSecrets inspects file content for sensitive private key headers.
func CheckFileContentForSecrets(repoPath, relPath string, contentPatterns []string) (bool, string) {
	fullPath := filepath.Join(repoPath, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return false, ""
	}
	// Limit content scan to first 512KB for performance
	if len(data) > 512*1024 {
		data = data[:512*1024]
	}
	contentStr := string(data)
	for _, pat := range contentPatterns {
		if strings.Contains(contentStr, pat) {
			return true, pat
		}
	}
	return false, ""
}

// StageSafeFiles stages only safe, non-ignored, non-secret files using explicit "git add -- <file>".
// It NEVER executes "git add -A", "git add .", or "git add -f".
func StageSafeFiles(ctx context.Context, repoPath string, requestedFiles []string, cfg GuardConfig) ([]string, error) {
	candidates := requestedFiles
	if len(candidates) == 0 {
		statusOut, _, err := runCmd(ctx, repoPath, "git", "status", "--porcelain")
		if err != nil {
			return nil, fmt.Errorf("git status okunamadı: %v", err)
		}
		candidates = parsePorcelainFiles(statusOut)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("çalışma ağacında commitlenecek değişiklik bulunamadı")
	}

	var safeFiles []string
	for _, f := range candidates {
		f = filepath.ToSlash(filepath.Clean(f))
		f = strings.TrimPrefix(f, "./")
		if f == "" {
			continue
		}

		// 1. Check if git ignores this file (e.g. via .gitignore)
		// git check-ignore returns exit code 0 if the path is ignored.
		if _, _, err := runCmd(ctx, repoPath, "git", "check-ignore", "-q", "--", f); err == nil {
			// Ignored by .gitignore -> NEVER stage!
			if len(requestedFiles) > 0 {
				return nil, fmt.Errorf("GÜVENLİK ENGELİ: '%s' dosyası .gitignore tarafından yoksayılıyor. Asla stage edilemez!", f)
			}
			continue
		}

		// 2. Secret filename pattern check
		if isSecret, pat := IsSecretPath(f, cfg.SecretPatterns); isSecret {
			return nil, fmt.Errorf("GÜVENLİK ENGELİ: Gizli/korumalı dosya tespit edildi: '%s' (eşleşen kural: %s). Commit durduruldu!", f, pat)
		}

		// 3. Secret content check
		if hasSecret, pat := CheckFileContentForSecrets(repoPath, f, cfg.ContentPatterns); hasSecret {
			return nil, fmt.Errorf("GÜVENLİK ENGELİ: '%s' dosyası içinde özel anahtar içeriği (%s) tespit edildi. Commit engellendi!", f, pat)
		}

		safeFiles = append(safeFiles, f)
	}

	if len(safeFiles) == 0 {
		return nil, fmt.Errorf("GÜVENLİK ENGELİ: Belirtilen dosyaların tümü .gitignore veya güvenlik kuralları tarafından filtrelendi")
	}

	// 4. Stage each safe file explicitly using "git add -- <file>"
	for _, sf := range safeFiles {
		if err := stagePath(ctx, repoPath, sf); err != nil {
			return nil, err
		}
	}

	return safeFiles, nil
}

// stagePath: tek bir yolu stage eder; SİLİNMİŞ dosyaları da. `git add -- <yol>` çalışma ağacında olmayan bir yol için
// ("pathspec did not match") hata verir; silinmiş izlenen dosya `git add -A -- <yol>` ile, `git rm` ile zaten stage
// edilmiş silme ise hiçbir şey yapmadan geçerli sayılır.
func stagePath(ctx context.Context, repoPath, f string) error {
	if _, err := os.Lstat(filepath.Join(repoPath, filepath.FromSlash(f))); err == nil {
		if out, _, err := runCmd(ctx, repoPath, "git", "add", "--", f); err != nil {
			return fmt.Errorf("'%s' dosyası stage edilemedi: %s", f, strings.TrimSpace(out))
		}
		return nil
	}
	// Çalışma ağacında yok: izlenen bir dosyanın silinmesi mi?
	if _, _, err := runCmd(ctx, repoPath, "git", "ls-files", "--error-unmatch", "--", f); err == nil {
		if out, _, err := runCmd(ctx, repoPath, "git", "add", "-A", "--", f); err != nil {
			return fmt.Errorf("'%s' dosyasının silinmesi stage edilemedi: %s", f, strings.TrimSpace(out))
		}
		return nil
	}
	// İndekste de yok: `git rm` ile silme zaten stage edilmiş olabilir.
	if out, _, err := runCmd(ctx, repoPath, "git", "diff", "--cached", "--name-only", "--diff-filter=D", "--", f); err == nil && strings.TrimSpace(out) != "" {
		return nil
	}
	return fmt.Errorf("'%s' dosyası çalışma ağacında yok ve git tarafından izlenmiyor; commit'e eklenemez", f)
}

var (
	reDiffSummary = regexp.MustCompile(`(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?`)
)

// ParseDiffStat converts git diff --stat text into a structured DiffStat.
func ParseDiffStat(rawStat string) model.DiffStat {
	res := model.DiffStat{
		Stat: strings.TrimSpace(rawStat),
	}
	lines := strings.Split(res.Stat, "\n")
	if len(lines) == 0 || res.Stat == "" {
		return res
	}

	lastLine := strings.TrimSpace(lines[len(lines)-1])
	if m := reDiffSummary.FindStringSubmatch(lastLine); len(m) > 1 {
		if v, err := strconv.Atoi(m[1]); err == nil {
			res.FilesChanged = v
		}
		if len(m) > 2 && m[2] != "" {
			if v, err := strconv.Atoi(m[2]); err == nil {
				res.Insertions = v
			}
		}
		if len(m) > 3 && m[3] != "" {
			if v, err := strconv.Atoi(m[3]); err == nil {
				res.Deletions = v
			}
		}
	} else {
		// Fallback: estimate from lines
		fileCount := 0
		for _, l := range lines {
			if strings.Contains(l, "|") {
				fileCount++
			}
		}
		res.FilesChanged = fileCount
	}

	res.TotalLines = res.Insertions + res.Deletions
	if res.FilesChanged > 10 || res.TotalLines >= 1000 {
		res.IsLargeDiff = true
	}
	return res
}
