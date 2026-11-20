package gitengine

import (
	"encoding/json"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// applyProfile: planı alıp parmak iziyle onaylayarak uygular (testler için kısayol).
func applyProfile(t *testing.T, mgr *Manager, profile string, checks []string) model.CockpitSnapshot {
	t.Helper()
	plan, err := mgr.ProtectionPlan(profile, checks)
	if err != nil {
		t.Fatalf("ProtectionPlan(%s): %v", profile, err)
	}
	snap, err := mgr.ApplyProtection(profile, checks, plan.Fingerprint)
	if err != nil {
		t.Fatalf("ApplyProtection(%s): %v", profile, err)
	}
	return snap
}

func TestStrengthenNeverRelaxes(t *testing.T) {
	strict := model.ProtectionSettings{
		RequirePR: true, Approvals: 3, DismissStale: true, StrictUpToDate: true,
		Checks: []string{"a", "z"}, EnforceAdmins: true, BlockForcePush: true, BlockDeletion: true,
	}
	for _, p := range ProtectionProfiles {
		want, err := profileTarget(p, []string{"b"})
		if err != nil {
			t.Fatal(err)
		}
		got := strengthen(strict, want)
		if got.Approvals != 3 || !got.DismissStale || !got.StrictUpToDate || !got.EnforceAdmins || !got.BlockForcePush || !got.BlockDeletion {
			t.Fatalf("profil %s mevcut sıkı kuralı gevşetti: %+v", p, got)
		}
		if strings.Join(got.Checks, ",") != "a,b,z" && (p == "ci" || p == "siki") {
			t.Fatalf("mevcut kontroller korunmalı, yenisi eklenmeli: %v", got.Checks)
		}
	}
	// Mevcut kontrol listesi profilin kontrol listesi boşken de silinmez.
	want, _ := profileTarget("ci", nil)
	if got := strengthen(strict, want); len(got.Checks) != 2 {
		t.Fatalf("boş seçim mevcut kontrolleri silmemeli: %v", got.Checks)
	}
}

func TestProfileTargets(t *testing.T) {
	temel, _ := profileTarget("temel", []string{"x"})
	if !temel.RequirePR || temel.StrictUpToDate || len(temel.Checks) != 0 || !temel.BlockForcePush || !temel.BlockDeletion {
		t.Fatalf("temel: %+v", temel)
	}
	ci, _ := profileTarget("ci", []string{"x"})
	if !ci.StrictUpToDate || len(ci.Checks) != 1 || ci.Approvals != 0 || ci.EnforceAdmins {
		t.Fatalf("ci: %+v", ci)
	}
	siki, _ := profileTarget("siki", []string{"x"})
	if siki.Approvals != 1 || !siki.DismissStale || !siki.EnforceAdmins || !siki.StrictUpToDate {
		t.Fatalf("siki: %+v", siki)
	}
	if _, err := profileTarget("hepsi", nil); err == nil {
		t.Fatal("bilinmeyen profil reddedilmeli")
	}
}

func TestCleanChecksRejectsBadNames(t *testing.T) {
	got, err := cleanChecks([]string{" test ", "lint", "test", ""})
	if err != nil || strings.Join(got, ",") != "lint,test" {
		t.Fatalf("temizleme: %v %v", got, err)
	}
	if _, err := cleanChecks([]string{"a\nb"}); err == nil {
		t.Fatal("satır sonu içeren ad reddedilmeli")
	}
	if _, err := cleanChecks([]string{strings.Repeat("x", 101)}); err == nil {
		t.Fatal("çok uzun ad reddedilmeli")
	}
}

func TestPlanNotesAndApplicability(t *testing.T) {
	for state, applicable := range map[string]bool{"none": true, "active": true, "ruleset": false, "unsupported": false, "unknown": false} {
		plan, err := buildPlan("main", "ci", []string{"test"}, model.ProtectionSettings{}, state, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Applicable != applicable || (applicable && plan.Fingerprint == "") || (!applicable && (plan.Fingerprint != "" || len(plan.Notes) == 0)) {
			t.Fatalf("durum %s: %+v", state, plan)
		}
	}
	// CI profili ama kontrol seçilmedi: dürüst uyarı (CI zorunlu OLMAZ).
	plan, _ := buildPlan("main", "ci", nil, model.ProtectionSettings{}, "none", nil)
	if !strings.Contains(strings.Join(plan.Notes, " "), "seçilmedi") {
		t.Fatalf("kontrolsüz ci profili uyarı vermeli: %v", plan.Notes)
	}
	// Aynı hedef aynı parmak izi, farklı hedef farklı parmak izi.
	a, _ := buildPlan("main", "ci", []string{"test"}, model.ProtectionSettings{}, "none", nil)
	b, _ := buildPlan("main", "ci", []string{"test"}, model.ProtectionSettings{}, "none", nil)
	c, _ := buildPlan("main", "ci", []string{"lint"}, model.ProtectionSettings{}, "none", nil)
	if a.Fingerprint != b.Fingerprint || a.Fingerprint == c.Fingerprint {
		t.Fatalf("parmak izi: %s %s %s", a.Fingerprint, b.Fingerprint, c.Fingerprint)
	}
}

func TestSimulationProtectionPlanAndApply(t *testing.T) {
	mgr := NewManager("")
	plan, err := mgr.ProtectionPlan("ci", []string{"test"})
	if err != nil || plan.State != "none" || !plan.Applicable || len(plan.Changes) == 0 {
		t.Fatalf("sandbox planı: %+v %v", plan, err)
	}
	// Plan hiçbir şey uygulamaz.
	if snap, _ := mgr.Snapshot(); snap.Repo.BranchProtected {
		t.Fatal("plan korumayı uygulamamalı")
	}
	if _, err := mgr.ApplyProtection("ci", []string{"test"}, ""); err == nil {
		t.Fatal("onaysız uygulama reddedilmeli")
	}
	if _, err := mgr.ApplyProtection("ci", []string{"lint"}, plan.Fingerprint); err == nil {
		t.Fatal("başka kontrol seçimiyle eski plan onayı reddedilmeli")
	}
	snap := applyProfile(t, mgr, "ci", []string{"test"})
	if !snap.Repo.BranchProtected || snap.Repo.ProtectionState != "active" || !strings.Contains(snap.Repo.ProtectionSummary, "CI: test") {
		t.Fatalf("uygulama sonrası: %+v", snap.Repo)
	}
	// Aynı profil tekrar: değişecek bir şey yok.
	again, _ := mgr.ProtectionPlan("ci", []string{"test"})
	if len(again.Changes) != 0 {
		t.Fatalf("ikinci plan boş olmalı: %+v", again.Changes)
	}
	if _, err := mgr.ApplyProtection("ci", []string{"test"}, again.Fingerprint); err == nil || !strings.Contains(err.Error(), "zaten karşılıyor") {
		t.Fatalf("değişiklik yokken uygulama reddedilmeli: %v", err)
	}
}

func TestSimulationEnterpriseOnlyStrengthens(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.SwitchRepo("sim-enterprise"); err != nil {
		t.Fatal(err)
	}
	plan, err := mgr.ProtectionPlan("siki", []string{"lint"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target.Approvals != 2 {
		t.Fatalf("mevcut 2 onay 1'e düşürülmemeli: %+v", plan.Target)
	}
	if strings.Join(plan.Target.Checks, ",") != "build,lint,test" {
		t.Fatalf("mevcut kontroller korunup yenisi eklenmeli: %v", plan.Target.Checks)
	}
}

// ---- canlı (sahte gh) ----

const liveProtectedJSON = `{
 "required_status_checks":{"strict":false,"contexts":["legacy"],"checks":[{"context":"legacy","app_id":1}]},
 "required_pull_request_reviews":{"dismiss_stale_reviews":false,"require_code_owner_reviews":true,"required_approving_review_count":2,
   "dismissal_restrictions":{"users":[{"login":"carol"}],"teams":[{"slug":"leads"}],"apps":[]}},
 "enforce_admins":{"enabled":false},
 "restrictions":{"users":[{"login":"alice"}],"teams":[],"apps":[{"slug":"deploybot"}]},
 "required_linear_history":{"enabled":true},
 "allow_force_pushes":{"enabled":true},
 "allow_deletions":{"enabled":false},
 "required_conversation_resolution":{"enabled":true}
}`

func TestLiveProtectionPlanFromUnprotectedRepo(t *testing.T) {
	gh := newGHStub(t)
	gh.set("api_protection_get", `{"message":"Branch not protected"}`, 1)
	gh.set("api_check_runs", `["test","lint","test"]`, 0)
	dir, _ := newGitHubLikeRepo(t)
	mgr := switchLive(t, dir)

	plan, err := mgr.ProtectionPlan("ci", []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "none" || !plan.Applicable || strings.Join(plan.AvailableChecks, ",") != "lint,test" {
		t.Fatalf("plan: %+v", plan)
	}
	if gh.called("-X PUT") {
		t.Fatal("plan asla PUT göndermemeli")
	}

	if _, err := mgr.ApplyProtection("ci", []string{"test"}, ""); err == nil || gh.called("-X PUT") {
		t.Fatalf("onaysız uygulama PUT göndermemeli: %v", err)
	}
	if _, err := mgr.ApplyProtection("ci", []string{"test"}, "yanlis"); err == nil || gh.called("-X PUT") {
		t.Fatalf("yanlış parmak izi PUT göndermemeli: %v", err)
	}

	if _, err := mgr.ApplyProtection("ci", []string{"test"}, plan.Fingerprint); err != nil {
		t.Fatalf("uygulama: %v", err)
	}
	if !gh.called("api -X PUT", "branches/main/protection", "--input -") {
		t.Fatalf("PUT gönderilmedi: %v", gh.calls())
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(gh.stdin("api_protection_put")), &p); err != nil {
		t.Fatal(err)
	}
	rs := p["required_status_checks"].(map[string]any)
	if rs["strict"] != true || len(rs["contexts"].([]any)) != 1 || rs["contexts"].([]any)[0] != "test" {
		t.Fatalf("zorunlu kontrol yanlış: %v", rs)
	}
	if p["allow_force_pushes"] != false || p["allow_deletions"] != false || p["enforce_admins"] != false {
		t.Fatalf("temel kilitler: %v", p)
	}
	if pr := p["required_pull_request_reviews"].(map[string]any); pr["required_approving_review_count"] != float64(0) {
		t.Fatalf("ci profili onay sayısını 0 tutmalı: %v", pr)
	}
}

func TestLiveProtectionPreservesUnmanagedSettings(t *testing.T) {
	gh := newGHStub(t)
	gh.set("api_protection_get", liveProtectedJSON, 0)
	dir, _ := newGitHubLikeRepo(t)
	mgr := switchLive(t, dir)

	plan, err := mgr.ProtectionPlan("ci", []string{"test"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "active" || plan.Current.Approvals != 2 || !plan.Current.RequirePR {
		t.Fatalf("mevcut ayarlar yanlış okundu: %+v", plan.Current)
	}
	if _, err := mgr.ApplyProtection("ci", []string{"test"}, plan.Fingerprint); err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(gh.stdin("api_protection_put")), &p); err != nil {
		t.Fatal(err)
	}
	pr := p["required_pull_request_reviews"].(map[string]any)
	if pr["required_approving_review_count"] != float64(2) {
		t.Fatalf("2 onay korunmalı: %v", pr)
	}
	if pr["require_code_owner_reviews"] != true {
		t.Fatalf("kod sahibi onayı korunmalı: %v", pr)
	}
	dr := pr["dismissal_restrictions"].(map[string]any)
	if dr["users"].([]any)[0] != "carol" || dr["teams"].([]any)[0] != "leads" {
		t.Fatalf("onay düşürme kısıtı korunmalı: %v", dr)
	}
	rest := p["restrictions"].(map[string]any)
	if rest["users"].([]any)[0] != "alice" || rest["apps"].([]any)[0] != "deploybot" {
		t.Fatalf("push kısıtlaması korunmalı: %v", rest)
	}
	if p["required_linear_history"] != true || p["required_conversation_resolution"] != true {
		t.Fatalf("doğrusal tarih ve konuşma çözümü korunmalı: %v", p)
	}
	rs := p["required_status_checks"].(map[string]any)
	ctx := rs["contexts"].([]any)
	if len(ctx) != 2 || ctx[0] != "legacy" || ctx[1] != "test" {
		t.Fatalf("mevcut kontrol korunup yenisi eklenmeli: %v", ctx)
	}
	// Force-push açıktı: temel kilit artık kapatır (bu bilinçli güçlendirme).
	if p["allow_force_pushes"] != false {
		t.Fatalf("force-push kapatılmalı: %v", p)
	}
}

func TestLiveProtectionRulesetAndUnsupportedAreNotApplicable(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	mgr := switchLive(t, dir)

	gh.set("api_protection_get", `{"message":"Branch not protected"}`, 1)
	gh.set("api_rulesets", `[{"id":1,"name":"main-guard","target":"branch","enforcement":"active"}]`, 0)
	plan, err := mgr.ProtectionPlan("temel", nil)
	if err != nil || plan.State != "ruleset" || plan.Applicable {
		t.Fatalf("ruleset: %+v %v", plan, err)
	}
	if _, err := mgr.ApplyProtection("temel", nil, "x"); err == nil || gh.called("-X PUT") {
		t.Fatalf("ruleset'li depoya klasik koruma yazılmamalı: %v", err)
	}

	gh.set("api_rulesets", "[]", 0)
	gh.set("api_protection_get", "Upgrade to GitHub Pro or make this repository public", 1)
	plan, _ = mgr.ProtectionPlan("temel", nil)
	if plan.State != "unsupported" || plan.Applicable {
		t.Fatalf("unsupported: %+v", plan)
	}

	gh.set("api_protection_get", `{"message":"Not Found","status":"404"}`, 1)
	plan, _ = mgr.ProtectionPlan("temel", nil)
	if plan.State != "unknown" || plan.Applicable {
		t.Fatalf("yetkisiz okuma 'korumasız' sayılmamalı: %+v", plan)
	}
}

func TestLiveProtectionAPIFailureIsFriendly(t *testing.T) {
	gh := newGHStub(t)
	gh.set("api_protection_get", `{"message":"Branch not protected"}`, 1)
	dir, _ := newGitHubLikeRepo(t)
	mgr := switchLive(t, dir)
	plan, err := mgr.ProtectionPlan("temel", nil)
	if err != nil {
		t.Fatal(err)
	}
	gh.set("api_protection_put", `{"message":"Resource not accessible by integration"}`, 1)
	if _, err := mgr.ApplyProtection("temel", nil, plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "yöneticisi yetkisi") {
		t.Fatalf("yetki hatası anlaşılır olmalı: %v", err)
	}
	gh.set("api_protection_put", `boom`, 1)
	if _, err := mgr.ApplyProtection("temel", nil, plan.Fingerprint); err == nil || !strings.Contains(err.Error(), "Branch Protection API hatası") {
		t.Fatalf("genel API hatası bildirilmeli: %v", err)
	}
}

func TestProtectionBadProfileRejected(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.ProtectionPlan("hepsi", nil); err == nil {
		t.Fatal("bilinmeyen profil reddedilmeli")
	}
	if _, err := mgr.ApplyProtection("", nil, "x"); err == nil {
		t.Fatal("boş profil reddedilmeli")
	}
}
