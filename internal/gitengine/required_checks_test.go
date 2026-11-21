package gitengine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// sshpocket gibi Ruleset'le korunan depolar: zorunlu kontrol adları `rules/branches/<dal>` (okuma yetkisiyle) buradan okunur.
const rulesBranchJSON = `[
 {"type":"deletion"},
 {"type":"non_fast_forward"},
 {"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"required_status_checks":[{"context":"go (ubuntu-26.04)","integration_id":15368},{"context":"web","integration_id":15368}]}},
 {"type":"pull_request","parameters":{"required_approving_review_count":0}}
]`

func TestFetchRequiredChecksFromRuleset(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_rules_branch", rulesBranchJSON, 0)
	stub.set("api_protection_get", "Branch not protected", 1) // ruleset-only depo: klasik koruma 404

	checks, strict, ok := fetchRequiredChecks(context.Background(), dir, "main")
	if !ok {
		t.Fatal("ruleset okundu: ok=true olmalı")
	}
	if strings.Join(checks, ",") != "go (ubuntu-26.04),web" {
		t.Fatalf("zorunlu kontrol adları yanlış: %v", checks)
	}
	if !strict {
		t.Fatal("ruleset strict_required_status_checks_policy=true okunmalı")
	}
}

func TestFetchRequiredChecksMergesClassicAndDeduplicates(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_rules_branch", `[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"web"}]}}]`, 0)
	stub.set("api_protection_get", `["web","lint"]`, 0)
	checks, _, ok := fetchRequiredChecks(context.Background(), dir, "main")
	if !ok || strings.Join(checks, ",") != "web,lint" {
		t.Fatalf("birleşim ve tekilleştirme bekleniyordu: ok=%v %v", ok, checks)
	}
}

// "Zorunlu kontrol yok" ile "okunamadı" farklı şeylerdir: ikisi de okunamazsa ok=false (çağıran önceki bilgiyi korur).
func TestFetchRequiredChecksUnreadableIsNotNoChecks(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_rules_branch", "HTTP 403", 1)
	stub.set("api_protection_get", "HTTP 404", 1)
	if checks, _, ok := fetchRequiredChecks(context.Background(), dir, "main"); ok || len(checks) != 0 {
		t.Fatalf("okunamadı: ok=false ve kontrol yok beklenirdi, geldi ok=%v %v", ok, checks)
	}
	// Ruleset okundu ama içinde zorunlu kontrol yok: ok=true, boş liste (gerçekten "yok").
	_ = os.Remove(filepath.Join(stub.dir, "api_rules_branch.exit")) // stub, önceki çıkış kodunu kendiliğinden silmez
	stub.set("api_rules_branch", `[{"type":"deletion"}]`, 0)
	if checks, _, ok := fetchRequiredChecks(context.Background(), dir, "main"); !ok || len(checks) != 0 {
		t.Fatalf("zorunlu kontrol yok: ok=true ve boş liste beklenirdi, geldi ok=%v %v", ok, checks)
	}
}

func TestSnapshotCarriesRequiredChecksAndRulesetStrict(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_rules_branch", rulesBranchJSON, 0)
	stub.set("api_protection_get", "Branch not protected", 1)
	stub.set("pr_list_open", "[]", 0)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	var got []string
	var strict bool
	for i := 0; i < 25 && len(got) == 0; i++ {
		snap, err := mgr.Snapshot()
		if err != nil {
			t.Fatal(err)
		}
		got, strict = snap.Repo.RequiredChecks, snap.Repo.StrictUpToDate
		if len(got) == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if strings.Join(got, ",") != "go (ubuntu-26.04),web" {
		t.Fatalf("anlık görüntü zorunlu kontrolleri taşımalı: %v", got)
	}
	if !strict {
		t.Fatal("Ruleset'teki \"dal güncel olmalı\" StrictUpToDate'e yansımalı (eskiden yalnızca klasik koruma okunuyordu)")
	}
}

// Koruma kuralı birleştirmeyi şimdilik engelledi (CI bitmedi): hiçbir şey değişmedi ve çıkış yolu var, bu yüzden günlükte "error" değil
// "info" olmalı; Hata Radarı yalnızca "error" kayıtlarından kart üretir.
func TestPolicyBlockedMergeIsLoggedAsInfoNotError(t *testing.T) {
	gh := newGHStub(t)
	mgr, _ := prSetup(t, gh, "feat/blocked")
	gh.set("pr_merge", "X Pull request o/r#7 is not mergeable: the base branch policy prohibits the merge. add the `--auto` flag ... `--admin` flag.", 1)
	_, err := mgr.MergeBranch("feat/blocked", "merge")
	if err == nil || !errors.Is(err, errMergePolicyBlocked) {
		t.Fatalf("koruma engeli tipli hata dönmeli: %v", err)
	}
	if strings.Contains(err.Error(), "onay eksik") {
		t.Fatalf("mesaj yanıltıcı (onay) içermemeli: %v", err)
	}
	var found bool
	for _, l := range mgr.recentLogs() {
		if l.Action == "Merge (merge)" && strings.Contains(l.Message, "koruma kuralı engelliyor") {
			found = true
			if l.Level != "info" {
				t.Fatalf("beklenen ön koşul hata kartı üretmemeli: level=%q", l.Level)
			}
		}
	}
	if !found {
		t.Fatal("engel günlüğe hiç yazılmadı")
	}
}
