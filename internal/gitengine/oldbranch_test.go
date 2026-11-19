package gitengine

import (
	"os"
	"path/filepath"
	"testing"
)

// Eskiden başlangıç noktası omurga penceresinin dışındaysa dal sessizce omurganın ucuna (lastSpine-1) konuyordu:
// 14+ commit önce ayrılmış unutulmuş dal, sanki yeni ayrılmış gibi çiziliyordu.
func TestOldOpenBranchIsAnchoredHonestlyNotAtTheTip(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	// Eski bir dal ayır (ana dalın EN ESKİ noktasından), sonra ana dala 20 commit ekle.
	run("branch", "feat/forgotten")
	run("checkout", "feat/forgotten")
	if err := os.WriteFile(filepath.Join(dir, "forgotten.txt"), []byte("f\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-m", "feat: forgotten work")
	run("push", "-u", "origin", "feat/forgotten")
	run("checkout", "main")
	for i := 0; i < 20; i++ {
		if err := os.WriteFile(filepath.Join(dir, "m"+itoa(i)+".txt"), []byte("m\n"), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-m", "chore: main "+itoa(i))
	}
	run("push", "origin", "main")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var old, fresh bool
	for _, b := range snap.Branches {
		if b.Name != "feat/forgotten" || b.State != "open" {
			continue
		}
		old = true
		if !b.BaseOutsideWindow {
			t.Errorf("a branch cut 20 commits ago must be flagged as starting outside the window: %+v", b)
		}
		if b.StartSpineIndex != 0 {
			t.Errorf("it must start at the far left, not at the tip: start=%d", b.StartSpineIndex)
		}
		if b.BehindMain < 20 {
			t.Errorf("behind count must stay truthful: %d", b.BehindMain)
		}
	}
	if !old {
		t.Fatal("feat/forgotten missing from snapshot")
	}
	stale := false
	for _, d := range snap.Diagnostics {
		if d.Kind == "stale_branch" && d.BranchName == "feat/forgotten" {
			stale = true
		}
	}
	if !stale {
		t.Error("a forgotten branch must appear in the radar as stale_branch")
	}

	// Yeni ayrılmış bir dal bayraklanmaz ve ucun yakınında başlar.
	addFeatureBranch(t, dir, run, "feat/fresh")
	run("push", "-u", "origin", "feat/fresh")
	snap, _ = mgr.Snapshot()
	for _, b := range snap.Branches {
		if b.Name == "feat/fresh" && b.State == "open" {
			fresh = true
			if b.BaseOutsideWindow {
				t.Errorf("a fresh branch must not be flagged: %+v", b)
			}
		}
	}
	if !fresh {
		t.Fatal("feat/fresh missing")
	}
}

// Merge-base ilk-ebeveyn omurgasında değil ama yakın bir birleşmenin yan geçmişinde: dal pencerenin başına değil, o
// birleşmenin düğümüne bağlanmalı ve "pencere dışı" sayılmamalı.
func TestBranchBasedOnASideCommitAnchorsAtTheMergeThatBroughtItIn(t *testing.T) {
	dir, run := newGitHubLikeRepo(t)
	write := func(name string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		run("add", "-A")
		run("commit", "-m", "chore: "+name)
	}
	// Yan dal: iki commit; ikincisi main'e --no-ff birleşir.
	run("checkout", "-b", "side")
	write("s1.txt")
	run("branch", "feat/off-side", "HEAD") // s1'den ayrılan dal
	write("s2.txt")
	run("checkout", "main")
	run("merge", "--no-ff", "side", "-m", "Merge pull request #7 from acme/side")
	for i := 0; i < 3; i++ {
		write("later" + itoa(i) + ".txt")
	}
	run("checkout", "feat/off-side")
	write("off.txt")
	run("push", "-u", "origin", "feat/off-side")
	run("checkout", "main")
	run("push", "origin", "main")

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	mergeIdx := -1
	for _, n := range snap.Nodes {
		if n.IsMerge {
			mergeIdx = n.SpineIndex
		}
	}
	if mergeIdx < 0 {
		t.Fatal("merge node missing from the spine")
	}
	found := false
	for _, b := range snap.Branches {
		if b.Name == "feat/off-side" && b.State == "open" {
			found = true
			if b.BaseOutsideWindow {
				t.Errorf("the base is recent: it must not be flagged as outside the window: %+v", b)
			}
			if b.StartSpineIndex != mergeIdx {
				t.Errorf("must anchor at the merge that brought its base in (idx %d), got start=%d", mergeIdx, b.StartSpineIndex)
			}
		}
	}
	if !found {
		t.Fatal("feat/off-side missing")
	}
}
