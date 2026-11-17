package gitengine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsurePlaygroundRepo(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gitarc-playground-test-*")
	if err != nil {
		t.Fatalf("temp dir error: %v", err)
	}
	defer os.RemoveAll(tempDir)

	target := filepath.Join(tempDir, "playground")
	p, err := EnsurePlaygroundRepo(target, false)
	if err != nil {
		t.Fatalf("EnsurePlaygroundRepo error: %v", err)
	}

	gitDir := filepath.Join(p, ".git")
	if fi, err := os.Stat(gitDir); err != nil || !fi.IsDir() {
		t.Fatalf("expected .git directory to exist at %s", gitDir)
	}

	readmeFile := filepath.Join(p, "README.md")
	if fi, err := os.Stat(readmeFile); err != nil || fi.Size() == 0 {
		t.Fatalf("expected README.md to exist and be non-empty")
	}

	// Test force reset
	p2, err := EnsurePlaygroundRepo(target, true)
	if err != nil {
		t.Fatalf("force reset error: %v", err)
	}
	if p2 != p {
		t.Fatalf("expected path %s, got %s", p, p2)
	}
}
