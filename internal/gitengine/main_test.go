package gitengine

import (
	"os"
	"testing"
)

// TestMain isolates HOME so tests never read or write the developer's real
// ~/.gitarc (projects.json, logs.json, playground).
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "gitarc-test-home-*")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	os.Setenv("GITARC_UPDATE_OFFLINE", "1") // sürüm denetimi testlerde gerçek GitHub'a hiç gitmesin (testler kendi sahte güncelleyicisini verir)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
