package gitengine

import (
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// Taze depo (hiç commit'i/dalı olmayan `git init`): GitArc çökmemeli ve "proje yok"a düşmemeli; boş ama geçerli bir canlı depo gösterir.
func TestSnapshotOfFreshEmptyRepoDoesNotFail(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	mgr := NewManager(dir)
	if mgr.NoProject() {
		t.Fatal("taze depo 'proje yok' sayılmamalı")
	}
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("taze depoda anlık görüntü hata vermemeli: %v", err)
	}
	if snap.Repo.Mode != "live" || snap.Repo.DefaultBranch != "main" {
		t.Fatalf("canlı mod ve omurga 'main' beklenirdi: %+v", snap.Repo)
	}
	if len(snap.Branches) != 0 {
		t.Fatalf("hiç dal yok: %d", len(snap.Branches))
	}
	// Kuyruk planı da boş depoda hata vermeden "aday yok" demeli.
	if _, err := mgr.QueuePlan(nil); err != nil {
		t.Fatalf("boş depoda kuyruk planı hata vermemeli: %v", err)
	}
}

// Açık PR listesi sınıra (openPRListLimit) dayandıysa bu SÖYLENİR; aksi halde 20'den fazla açık PR sessizce görünmez olurdu.
func TestOpenPRListHitsLimitIsReportedAndLimitIsPassedToGH(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	var items []string
	for i := 1; i <= openPRListLimit; i++ {
		items = append(items, fmt.Sprintf(`{"number":%d,"headRefName":"feat/b%d","title":"t%d","body":"","author":{"login":"u"},"reviewDecision":"","statusCheckRollup":[],"url":"https://github.com/acme/demo/pull/%d"}`, i, i, i, i))
	}
	gh.set("pr_list_open", "["+strings.Join(items, ",")+"]", 0)

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	var truncated bool
	for i := 0; i < 25 && !truncated; i++ {
		snap, err := mgr.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		truncated = snap.Repo.OpenPRsTruncated
		if !truncated {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if !truncated {
		t.Fatal("liste sınıra dayandı: openPRsTruncated true olmalı")
	}
	if !gh.called("pr list", "--state open", fmt.Sprintf("--limit %d", openPRListLimit)) {
		t.Fatalf("sınır gh'ye bu sabitle verilmeli: %v", gh.calls())
	}

	// Sınırın altında: uyarı yok.
	gh2 := newGHStub(t)
	dir2, _ := newGitHubLikeRepo(t)
	gh2.set("pr_list_open", "["+items[0]+"]", 0)
	mgr2 := NewManager(dir2)
	if _, err := mgr2.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1500 * time.Millisecond)
	snap2, err := mgr2.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap2.Repo.OpenPRsTruncated {
		t.Fatal("tek açık PR: kesilme uyarısı olmamalı")
	}
}
