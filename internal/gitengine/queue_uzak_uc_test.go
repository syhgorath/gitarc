package gitengine

import (
	"os"
	"path/filepath"
	"testing"
)

// Olayın birebir hâli: PR dalı main'le ÇAKIŞIYOR, çözüm yerelde yapıldı ama pushlanmadı. GitHub PR'ın UZAK ucunu birleştirir ("çakışıyor"
// der); plan eskiden dalın YEREL ucunu simüle edip "temiz" diyordu ve sonra koşu "ana dalla çakışıyor" diye duruyordu.
func TestQueuePlanSimulatesTheRemotePRHeadNotTheUnpushedLocalFix(t *testing.T) {
	isoHome(t)
	gh := newGHStub(t)
	gh.set("pr_list_open", `[{"number":1,"headRefName":"feat/x","title":"x","body":"","url":"https://github.com/acme/demo/pull/1","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]}]`, 0)
	dir, run := newGitHubLikeRepo(t)
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ortak.txt", "temel\n")
	run("add", "-A")
	run("commit", "-qm", "ortak")
	run("push", "-q", "origin", "main")

	run("checkout", "-q", "-b", "feat/x")
	write("ortak.txt", "pr degisikligi\n")
	run("add", "-A")
	run("commit", "-qm", "pr")
	run("push", "-q", "-u", "origin", "feat/x")

	run("checkout", "-q", "main")
	write("ortak.txt", "main degisikligi\n")
	run("add", "-A")
	run("commit", "-qm", "main ilerledi")
	run("push", "-q", "origin", "main")

	run("checkout", "-q", "feat/x")
	run("merge", "-q", "-X", "ours", "main", "-m", "cakisma yerelde cozuldu") // çözüm YEREL, pushlanmadı
	run("checkout", "-q", "main")

	mgr := switchLive(t, dir)
	waitPR(t, mgr, "feat/x")
	planFor := func() (status string, onMain bool, unpushed int) {
		t.Helper()
		snap, err := mgr.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		var id string
		for _, b := range snap.Branches {
			if b.Name == "feat/x" && b.State == "open" && b.PR != nil {
				id = b.ID
			}
		}
		if id == "" {
			t.Fatal("feat/x PR'lı açık dal olarak listelenmeli")
		}
		plan, err := mgr.QueuePlan([]string{id})
		if err != nil || len(plan.Items) != 1 {
			t.Fatalf("plan: %+v %v", plan, err)
		}
		it := plan.Items[0]
		return it.Status, it.ConflictsOnMain, it.UnpushedCommits
	}

	status, onMain, unpushed := planFor()
	if status != "conflict" || !onMain {
		t.Fatalf("GitHub'daki (uzak) PR ucu main'le çakışıyor: plan 'conflict' demeli (yerel pushlanmamış çözüm sayılmaz); durum=%q onMain=%v", status, onMain)
	}
	if unpushed < 1 {
		t.Fatalf("pushlanmamış yerel commit'ler bildirilmeli: %d", unpushed)
	}

	// çözüm pushlanınca plan gerçeği görür: temiz, ve pushlanmamış commit kalmaz
	run("push", "-q", "origin", "feat/x")
	status, _, unpushed = planFor()
	if status != "clean" || unpushed != 0 {
		t.Fatalf("pushlandıktan sonra plan temiz olmalı ve uyarı kalkmalı: durum=%q unpushed=%d", status, unpushed)
	}
}
