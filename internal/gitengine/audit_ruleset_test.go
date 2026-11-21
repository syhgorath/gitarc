package gitengine

import (
	"context"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// Ruleset'le korunan depoda koruma "ruleset" olarak bilinir ama içi görünmezdi (current boş). Artık dala uygulanan kurallar salt okunur okunur.
func TestProtectionPlanReadsRulesetRulesReadOnly(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_protection_get", "Branch not protected", 1) // klasik koruma yok
	stub.set("api_rulesets", `[{"id":1,"name":"main","target":"branch","enforcement":"active"}]`, 0)
	stub.set("api_rules_branch", rulesBranchJSON, 0)

	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	plan, err := mgr.ProtectionPlan("temel", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ruleset" || plan.Source != "ruleset" {
		t.Fatalf("state=ruleset, source=ruleset beklenirdi: %+v", plan)
	}
	c := plan.Current
	if !c.RequirePR || !c.BlockForcePush || !c.BlockDeletion || !c.StrictUpToDate || strings.Join(c.Checks, ",") != "go (ubuntu-26.04),web" {
		t.Fatalf("Ruleset kuralları Current'a yansımalı: %+v", c)
	}
	if plan.Applicable {
		t.Fatal("Ruleset'i GitArc değiştirmez: plan uygulanabilir olmamalı")
	}
	if stub.called("-X PUT") || stub.called("-X POST") {
		t.Fatal("okuma sırasında hiçbir yazma çağrısı olmamalı")
	}
}

// Ruleset kuralları okunamazsa Source boş kalır ve neden yazılır ("okunamadı" ≠ "kural yok").
func TestProtectionPlanRulesetUnreadableSaysSo(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_protection_get", "Branch not protected", 1)
	stub.set("api_rulesets", `[{"id":1,"name":"main","target":"branch","enforcement":"active"}]`, 0)
	stub.set("api_rules_branch", "HTTP 403", 1)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	plan, err := mgr.ProtectionPlan("temel", nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "ruleset" || plan.Source != "" {
		t.Fatalf("kurallar okunamadı: state=ruleset, source boş beklenirdi: %+v", plan)
	}
	joined := strings.Join(plan.Notes, " | ")
	if !strings.Contains(joined, "okunamadı") {
		t.Fatalf("neden yazılmalı: %s", joined)
	}
}

func TestReadRulesetSettingsParsesApprovals(t *testing.T) {
	stub := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	stub.set("api_rules_branch", `[{"type":"pull_request","parameters":{"required_approving_review_count":2,"dismiss_stale_reviews_on_push":true}}]`, 0)
	s, ok := readRulesetSettings(context.Background(), dir, "main")
	if !ok || !s.RequirePR || s.Approvals != 2 || !s.DismissStale || s.BlockForcePush {
		t.Fatalf("pull_request kuralı okunmalı, diğerleri kapalı: ok=%v %+v", ok, s)
	}
}

// Denetim günlüğü: her kayıt proje kimliği + adıyla döner; varsayılan yalnızca etkin proje; repoId'siz eski kayıtlar gizli (sayılır), yalnızca
// "all" ile "bilinmiyor" olarak görünür. Başka projenin kaydı ("PR #102 ...") bu projede görünmez.
func TestAuditLogsScopeAndLegacyHandling(t *testing.T) {
	dir, _ := newTempRepo(t)
	mgr := NewManager(dir)
	if _, err := mgr.SwitchRepo("live-custom"); err != nil {
		t.Fatal(err)
	}
	mgr.mu.Lock()
	mgr.logs = []model.ActionLogEntry{
		{ID: "l1", RepoID: "live-custom", Action: "A", Message: "bu proje"},
		{ID: "l2", RepoID: "baska-proje", Action: "B", Message: "PR #102 başka projeye ait"},
		{ID: "l3", RepoID: "", Action: "C", Message: "eski kayıt, projesi bilinmiyor"},
	}
	mgr.mu.Unlock()

	logs, scope, hidden := mgr.AuditLogs("", 25)
	if scope != "live-custom" || len(logs) != 1 || logs[0].ID != "l1" || logs[0].RepoName == "" || logs[0].RepoID != "live-custom" {
		t.Fatalf("varsayılan: yalnızca etkin proje, kimlik+ad ile: scope=%s %+v", scope, logs)
	}
	if hidden != 1 {
		t.Fatalf("repoId'siz eski kayıt gizlenip sayılmalı: hidden=%d", hidden)
	}
	all, scope2, _ := mgr.AuditLogs("all", 25)
	if scope2 != "all" || len(all) != 3 {
		t.Fatalf("all: tüm kayıtlar beklenirdi: %d", len(all))
	}
	for _, e := range all {
		if e.ID == "l3" && (e.RepoID != "bilinmiyor" || e.RepoName != "bilinmiyor") {
			t.Fatalf("eski kayıt 'bilinmiyor' işaretlenmeli: %+v", e)
		}
		if e.ID == "l2" && e.RepoName != "baska-proje" {
			t.Fatalf("listede olmayan proje için kimlik gösterilmeli: %+v", e)
		}
	}
	other, _, _ := mgr.AuditLogs("baska-proje", 25)
	if len(other) != 1 || other[0].ID != "l2" {
		t.Fatalf("belirli proje: yalnızca onun kaydı: %+v", other)
	}
}
