package main

import (
	"os"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
)

// TestVersionConsistency keeps every hand-written copy of the version in sync with
// gitengine.AppVersion, so a release bump can never leave a stale "v0.x.y" in the UI.
func TestVersionConsistency(t *testing.T) {
	v := gitengine.AppVersion

	raw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != v {
		t.Errorf("VERSION file = %q, gitengine.AppVersion = %q", got, v)
	}

	cases := []struct{ file, want string }{
		{"web/index.html", `id="btn-update-check" title="Sürüm, güncelleme ve Team modu">v` + v + `</button>`},
		{"web/index.html", `Mevcut sürüm: v` + v},
		{"web/modules/canvas.js", `appVersion || '` + v + `'`},
		{"web/fallback.js", `currentVersion: sim.repo.appVersion || '` + v + `'`},
		{"web/fallback.js", `latestVersion: sim.repo.appVersion || '` + v + `'`},
	}
	for _, c := range cases {
		b, err := webFS.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), c.want) {
			t.Errorf("%s must contain %q (stale hard-coded version?)", c.file, c.want)
		}
	}

	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(changelog), "## ["+v+"]") {
		t.Errorf("CHANGELOG.md has no section for the current version [%s]", v)
	}
}
