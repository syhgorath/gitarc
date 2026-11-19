package main

import (
	"os/exec"
	"testing"
)

// TestCanvasGeometryJS runs the zero-dependency Node tests in tests/js so
// `go test ./...` also covers the SVG layout math. Skipped when node is missing.
func TestCanvasGeometryJS(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	out, err := exec.Command(node, "--test", "tests/js/").CombinedOutput()
	if err != nil {
		t.Fatalf("node tests failed: %v\n%s", err, out)
	}
}
