package gitengine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultPlaygroundPath returns the standard path for the user playground repository (~/.gitarc/playground).
func DefaultPlaygroundPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gitarc", "playground")
	return dir, nil
}

// EnsurePlaygroundRepo initializes or resets a real, lightweight Git repository
// with sample branches, commits, and a merge conflict scenario for demonstration.
func EnsurePlaygroundRepo(targetDir string, forceReset bool) (string, error) {
	if strings.TrimSpace(targetDir) == "" {
		p, err := DefaultPlaygroundPath()
		if err != nil {
			return "", err
		}
		targetDir = p
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return "", err
	}

	gitDir := filepath.Join(absDir, ".git")
	exists := false
	if fi, err := os.Stat(gitDir); err == nil && fi.IsDir() {
		exists = true
	}

	if exists && !forceReset {
		return absDir, nil
	}

	// Remove existing and rebuild
	_ = os.RemoveAll(absDir)
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return "", fmt.Errorf("playground dizini oluşturulamadı: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// 1. git init -b main
	if _, _, err := runCmd(ctx, absDir, "git", "init", "-b", "main"); err != nil {
		if _, _, err := runCmd(ctx, absDir, "git", "init"); err != nil {
			return "", fmt.Errorf("git init başarısız: %w", err)
		}
		_, _, _ = runCmd(ctx, absDir, "git", "checkout", "-b", "main")
	}

	// 2. Local author config
	_, _, _ = runCmd(ctx, absDir, "git", "config", "user.name", "GitArc Playground")
	_, _, _ = runCmd(ctx, absDir, "git", "config", "user.email", "playground@gitarc.dev")

	// 3. Initial commit on main
	_ = os.WriteFile(filepath.Join(absDir, "README.md"), []byte("# GitArc Playground\n\nBu depo, GitArc özelliklerini denemeniz için oluşturulmuş gerçek bir yerel Git kum havuzudur.\n"), 0644)
	_ = os.WriteFile(filepath.Join(absDir, "app.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"GitArc Playground\")\n}\n"), 0644)
	_, _, _ = runCmd(ctx, absDir, "git", "add", "-A")
	_, _, _ = runCmd(ctx, absDir, "git", "commit", "-m", "chore: initial playground commit")

	// 4. Second commit on main
	_ = os.WriteFile(filepath.Join(absDir, "config.json"), []byte("{\n  \"env\": \"playground\",\n  \"version\": \"1.0.0\"\n}\n"), 0644)
	_, _, _ = runCmd(ctx, absDir, "git", "add", "-A")
	_, _, _ = runCmd(ctx, absDir, "git", "commit", "-m", "feat(config): add playground configuration")

	// 5. Branch 1: feat/auth-service
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "-b", "feat/auth-service")
	_ = os.WriteFile(filepath.Join(absDir, "auth.go"), []byte("package main\n\n// AuthService handles login tokens\ntype AuthService struct{}\n"), 0644)
	_, _, _ = runCmd(ctx, absDir, "git", "add", "-A")
	_, _, _ = runCmd(ctx, absDir, "git", "commit", "-m", "feat(auth): implement token authentication service")

	// 6. Branch 2: feat/payment-gateway
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "main")
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "-b", "feat/payment-gateway")
	_ = os.WriteFile(filepath.Join(absDir, "payment.go"), []byte("package main\n\n// PaymentGateway processes card payments\ntype PaymentGateway struct{}\n"), 0644)
	_, _, _ = runCmd(ctx, absDir, "git", "add", "-A")
	_, _, _ = runCmd(ctx, absDir, "git", "commit", "-m", "feat(payment): add stripe provider interface")

	// 7. Branch 3: fix/config-staging
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "main")
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "-b", "fix/config-staging")
	_ = os.WriteFile(filepath.Join(absDir, "config.json"), []byte("{\n  \"env\": \"staging\",\n  \"version\": \"1.0.1-rc1\"\n}\n"), 0644)
	_, _, _ = runCmd(ctx, absDir, "git", "add", "-A")
	_, _, _ = runCmd(ctx, absDir, "git", "commit", "-m", "fix(config): update staging environment target")

	// Return to main as default HEAD
	_, _, _ = runCmd(ctx, absDir, "git", "checkout", "main")

	return absDir, nil
}
