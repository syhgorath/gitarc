package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// Olayın birebir hâli: açık PR'ı olan dalda pushlanmamış commit varsa GitArc "dal (yerel)" adlı eşlik kartı üretir. Kartın adı git dal adı
// DEĞİLDİR; eskiden "Push & PR Aç" bu adı git'e/doğrulamaya verip "dal adı geçersiz karakter içeriyor" hatası alıyordu (ve kart arayüzde
// "Çalışma Alanı, 0 dosya, Commit & PR" gibi yanlış görünüyordu).
func TestCompanionCardPushesTheRealBranchAndUpdatesTheExistingPR(t *testing.T) {
	isoHome(t)
	gh := newGHStub(t)
	gh.set("pr_list_open", openPRsJSON, 0)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green") // PR #7 (openPRsJSON)
	run("push", "-u", "origin", "feat/green")
	run("checkout", "-q", "feat/green")
	if err := os.WriteFile(filepath.Join(dir, "yeni.txt"), []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "pushlanmamis commit")
	run("checkout", "-q", "main")

	mgr := switchLive(t, dir)
	waitPR(t, mgr, "feat/green")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var comp *struct{ id, name, parent string }
	for _, b := range snap.Branches {
		if strings.HasPrefix(b.ID, "local-wip-") && b.ID != "local-wip-working-tree" {
			comp = &struct{ id, name, parent string }{b.ID, b.Name, b.ParentBranch}
		}
	}
	if comp == nil || comp.name != "feat/green (yerel)" || comp.parent != "feat/green" {
		t.Fatalf("eşlik kartı gerçek dalını (ParentBranch) taşımalı: %+v", comp)
	}

	localTip := strings.TrimSpace(mustGit(t, dir, "rev-parse", "feat/green"))
	if remote := strings.TrimSpace(mustGit(t, dir, "rev-parse", "origin/feat/green")); remote == localTip {
		t.Fatal("önkoşul: pushlanmamış commit olmalı")
	}
	if _, err := mgr.PromoteBranch(comp.id); err != nil {
		t.Fatalf("eşlik kartından push çalışmalı (eskiden 'dal adı geçersiz karakter içeriyor'): %v", err)
	}
	if remote := strings.TrimSpace(mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/green")); !strings.HasPrefix(remote, localTip) {
		t.Fatalf("gerçek dal origin'e pushlanmalı: %q (yerel uç %s)", remote, localTip)
	}
	if gh.called("pr", "create") {
		t.Fatal("PR zaten var: yeni PR açılmamalı")
	}

	// kart üzerinde YIKICI işlemler gerçek dalı etkilememeli ve nedenini söylemeli (geçersiz-karakter değil)
	snap2, _ := mgr.Snapshot()
	for _, b := range snap2.Branches {
		if strings.HasPrefix(b.ID, "local-wip-") && b.ID != "local-wip-working-tree" {
			if _, err := mgr.DeleteBranch(b.ID); err == nil || strings.Contains(err.Error(), "geçersiz karakter") {
				t.Fatalf("eşlik kartı silinememeli ve neden açık olmalı: %v", err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatal(err)
	}
	if out := mustGit(t, dir, "branch", "--list", "feat/green"); !strings.Contains(out, "feat/green") {
		t.Fatal("gerçek dal silinmemeli")
	}
}

func TestRealBranchNamePrefersTheParent(t *testing.T) {
	if got := realBranchName(model.BranchArc{Name: "a (yerel)", ParentBranch: "a"}); got != "a" {
		t.Fatalf("eşlik kartı gerçek adı vermeli: %q", got)
	}
	if got := realBranchName(model.BranchArc{Name: "b"}); got != "b" {
		t.Fatalf("normal dal kendi adı: %q", got)
	}
	if validateBranchName("a (yerel)") == nil {
		t.Fatal("görünen ad geçerli bir git dal adı DEĞİLDİR (bu yüzden ParentBranch var)")
	}
}
