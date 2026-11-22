package main

import (
	"os"
	"strings"
	"testing"
)

// TestMain isolates HOME so tests never read or write the developer's real
// ~/.gitarc (projects.json, logs.json, playground).
//
// It also lets the test binary re-run itself as the real GitArc program: when
// GITARC_TEST_RUN_MAIN=1, main() is executed with the flags in GITARC_TEST_ARGS
// (used by main_os_test.go to cover main() end to end). In that mode the HOME
// inherited from the parent test is kept.
func TestMain(m *testing.M) {
	if os.Getenv("GITARC_TEST_RUN_MAIN") == "1" {
		os.Args = append([]string{"gitarc"}, strings.Fields(os.Getenv("GITARC_TEST_ARGS"))...)
		main()
		os.Exit(0)
	}

	home, err := os.MkdirTemp("", "gitarc-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Setenv("GITARC_UPDATE_OFFLINE", "1") // sürüm denetimi testlerde gerçek GitHub'a gitmesin
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
