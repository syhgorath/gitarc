package mcp

import (
	"strings"
	"testing"

	"gitarc/internal/gitengine"
)

func TestQueuePlanToolIsListedAndReadOnly(t *testing.T) {
	var found bool
	for _, d := range listTools() {
		if d.Name == "gitarc_queue_plan" {
			found = true
			if strings.Contains(strings.ToLower(d.Description), "birleştirir ") && !strings.Contains(d.Description, "hiçbir şey birleştirmez") {
				t.Fatalf("açıklama salt okunur olduğunu söylemeli: %s", d.Description)
			}
			if !strings.Contains(d.Description, "SALT OKUNUR") {
				t.Fatal("araç açıklaması SALT OKUNUR demeli")
			}
		}
	}
	if !found {
		t.Fatal("gitarc_queue_plan listede yok")
	}
}

func TestQueuePlanToolReturnsAPlanWithoutAnyPermission(t *testing.T) {
	t.Setenv(envAllowResolve, "")
	t.Setenv(envAllowProtection, "")
	mgr := gitengine.NewManager("sim")
	out, res := callTyped(t, mgr, "gitarc_queue_plan", map[string]any{})
	if res.IsError {
		t.Fatalf("plan izin gerektirmemeli: %s", res.Content[0].Text)
	}
	if out["mode"] != "simulation" || out["items"] == nil || out["baseBranch"] == "" {
		t.Fatalf("plan alanları eksik: %+v", out)
	}
	if _, ok := out["mergeable"].(float64); !ok {
		t.Fatalf("mergeable sayısı olmalı: %+v", out)
	}
}

func TestQueuePlanToolRejectsUnknownBranchAndNeverDemoWithoutProject(t *testing.T) {
	mgr := gitengine.NewManager("sim")
	if _, res := callTyped(t, mgr, "gitarc_queue_plan", map[string]any{"branchIds": []any{"yok-boyle-dal"}}); !res.IsError {
		t.Fatal("bilinmeyen dal hata vermeli")
	}
	none := noProjectManager(t)
	if _, res := callTyped(t, none, "gitarc_queue_plan", map[string]any{}); !res.IsError {
		t.Fatal("proje yokken demo veriyle çalışmamalı")
	}
}

// ---- dalı ana dala göre güncelle (uzak PR dalına push eder: varsayılan KAPALI) ----

func firstBehindBranch(t *testing.T, mgr *gitengine.Manager) string {
	t.Helper()
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range snap.Branches {
		if b.State == "open" && b.BehindMain > 0 && (b.Conflict == nil || !b.Conflict.HasConflict) {
			return b.ID
		}
	}
	t.Skip("demo verisinde geride kalmış temiz dal yok")
	return ""
}

func TestUpdateBranchToolsAreListed(t *testing.T) {
	names := map[string]bool{}
	for _, d := range listTools() {
		names[d.Name] = true
	}
	for _, want := range []string{"gitarc_update_branch_plan", "gitarc_update_branch"} {
		if !names[want] {
			t.Fatalf("%s listede yok", want)
		}
	}
}

func TestUpdateBranchPlanIsReadOnlyAndExplainsHowToEnableApply(t *testing.T) {
	t.Setenv(envAllowUpdate, "")
	mgr := gitengine.NewManager("sim")
	id := firstBehindBranch(t, mgr)
	out, res := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id})
	if res.IsError {
		t.Fatalf("plan izin gerektirmemeli: %s", res.Content[0].Text)
	}
	plan := out["plan"].(map[string]any)
	if plan["status"] != "ready" || plan["fingerprint"] == "" || out["applyEnabled"] != false || !strings.Contains(out["next"].(string), envAllowUpdate) {
		t.Fatalf("plan: %+v", out)
	}
	if again, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id}); again["plan"].(map[string]any)["status"] != "ready" {
		t.Fatal("plan hiçbir şeyi güncellememeli")
	}
}

func TestUpdateBranchApplyIsOffByDefaultEvenWithAValidFingerprint(t *testing.T) {
	t.Setenv(envAllowUpdate, "")
	mgr := gitengine.NewManager("sim")
	id := firstBehindBranch(t, mgr)
	pl, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id})
	fp := pl["plan"].(map[string]any)["fingerprint"].(string)
	_, res := callTyped(t, mgr, "gitarc_update_branch", map[string]any{"branchId": id, "confirm": fp})
	if !res.IsError || !strings.Contains(res.Content[0].Text, envAllowUpdate) {
		t.Fatalf("izinsiz güncelleme reddedilmeli ve nedenini söylemeli: %+v", res)
	}
	if after, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id}); after["plan"].(map[string]any)["status"] != "ready" {
		t.Fatal("izin yokken dal güncellenmemeli")
	}
}

func TestUpdateBranchApplyNeedsTheMatchingFingerprintWhenAllowed(t *testing.T) {
	t.Setenv(envAllowUpdate, "1")
	mgr := gitengine.NewManager("sim")
	id := firstBehindBranch(t, mgr)
	pl, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id})
	fp := pl["plan"].(map[string]any)["fingerprint"].(string)

	for i, args := range []map[string]any{
		{"branchId": id},                      // confirm yok
		{"branchId": id, "confirm": "yanlis"}, // yanlış parmak izi
		{"confirm": fp},                       // branchId yok
	} {
		if _, res := callTyped(t, mgr, "gitarc_update_branch", args); !res.IsError {
			t.Fatalf("kötü girdi #%d reddedilmeli", i)
		}
	}
	if after, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id}); after["plan"].(map[string]any)["status"] != "ready" {
		t.Fatal("reddedilen denemeler dalı güncellememeli")
	}
	out, res := callTyped(t, mgr, "gitarc_update_branch", map[string]any{"branchId": id, "confirm": fp})
	if res.IsError || out["updated"] != true {
		t.Fatalf("doğru parmak izi güncellemeli: %+v %+v", out, res)
	}
	if after, _ := callTyped(t, mgr, "gitarc_update_branch_plan", map[string]any{"branchId": id}); after["plan"].(map[string]any)["status"] != "up_to_date" {
		t.Fatalf("güncellemeden sonra dal güncel olmalı: %+v", after)
	}
}

func TestUpdateBranchToolsRefuseWithoutProjectAndUnknownBranch(t *testing.T) {
	t.Setenv(envAllowUpdate, "1")
	if _, res := callTyped(t, gitengine.NewManager("sim"), "gitarc_update_branch_plan", map[string]any{"branchId": "yok-boyle-dal"}); !res.IsError {
		t.Fatal("bilinmeyen dal hata vermeli")
	}
	none := noProjectManager(t)
	for _, name := range []string{"gitarc_update_branch_plan", "gitarc_update_branch"} {
		if _, res := callTyped(t, none, name, map[string]any{"branchId": "x", "confirm": "x"}); !res.IsError {
			t.Fatalf("%s proje yokken hata vermeli", name)
		}
	}
}

// ---- kuyruk adımı (güncelle / birleştir): varsayılan KAPALI ----

func TestQueueStepToolsAreListedAndPlanIsReadOnly(t *testing.T) {
	seen := map[string]string{}
	for _, d := range listTools() {
		seen[d.Name] = d.Description
	}
	if d, ok := seen["gitarc_queue_step_plan"]; !ok || !strings.Contains(d, "SALT OKUNUR") {
		t.Fatalf("gitarc_queue_step_plan listede ve SALT OKUNUR olmalı: %q", d)
	}
	if d, ok := seen["gitarc_queue_step"]; !ok || !strings.Contains(d, "VARSAYILAN OLARAK KAPALIDIR") || !strings.Contains(d, envAllowQueue) {
		t.Fatalf("gitarc_queue_step listede ve varsayılan kapalı olmalı: %q", d)
	}
}

func TestQueueStepPlanNeedsNoPermissionButApplyIsOffByDefault(t *testing.T) {
	t.Setenv(envAllowQueue, "")
	mgr := gitengine.NewManager("sim")
	out, res := callTyped(t, mgr, "gitarc_queue_step_plan", map[string]any{"branchId": "br-open-424"})
	if res.IsError {
		t.Fatalf("plan izin gerektirmemeli: %s", res.Content[0].Text)
	}
	plan, _ := out["plan"].(map[string]any)
	if plan == nil || plan["action"] != "wait" || out["applyEnabled"] != false {
		t.Fatalf("plan alanları: %+v", out)
	}
	if _, res := callTyped(t, mgr, "gitarc_queue_step_plan", map[string]any{}); !res.IsError {
		t.Fatal("branchId zorunlu")
	}
	// uygulama KAPALI: doğru parmak izi bile işe yaramaz ve durum değişmez
	_, res = callTyped(t, mgr, "gitarc_queue_step", map[string]any{"branchId": "br-open-424", "confirm": "herhangi"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "kapalı") {
		t.Fatalf("varsayılan olarak kapalı olmalı: %+v", res)
	}
}

func TestQueueStepApplyWorksOnlyWhenEnabledAndWithFingerprint(t *testing.T) {
	t.Setenv(envAllowQueue, "1")
	mgr := gitengine.NewManager("sim")
	// demo verisinde 422 geride ve CI kırmızı: engelli; uygulanmamalı
	if _, res := callTyped(t, mgr, "gitarc_queue_step", map[string]any{"branchId": "br-open-422", "confirm": "x"}); !res.IsError {
		t.Fatal("engelli adım uygulanmamalı")
	}
	if _, res := callTyped(t, mgr, "gitarc_queue_step", map[string]any{"branchId": "br-open-422"}); !res.IsError {
		t.Fatal("confirm zorunlu")
	}
}

// ---- otomatik kuyruk koşusu (3b): varsayılan KAPALI ----

func TestQueueRunToolsAreListedWithTheRightSafety(t *testing.T) {
	seen := map[string]string{}
	for _, d := range listTools() {
		seen[d.Name] = d.Description
	}
	for _, name := range []string{"gitarc_queue_run_plan", "gitarc_queue_run_status"} {
		if d, ok := seen[name]; !ok || !strings.Contains(d, "SALT OKUNUR") {
			t.Fatalf("%s listede ve SALT OKUNUR olmalı: %q", name, d)
		}
	}
	if d, ok := seen["gitarc_queue_run"]; !ok || !strings.Contains(d, "VARSAYILAN OLARAK KAPALIDIR") || !strings.Contains(d, envAllowQueueRun) {
		t.Fatalf("gitarc_queue_run varsayılan kapalı olmalı: %q", d)
	}
	if _, ok := seen["gitarc_queue_run_stop"]; !ok {
		t.Fatal("gitarc_queue_run_stop listede olmalı")
	}
}

func TestQueueRunIsOffByDefaultEvenWithAFingerprintAndStopIsAlwaysAllowed(t *testing.T) {
	t.Setenv(envAllowQueueRun, "")
	mgr := gitengine.NewManager("sim")
	out, res := callTyped(t, mgr, "gitarc_queue_run_plan", map[string]any{})
	if res.IsError {
		t.Fatalf("plan izin gerektirmemeli: %s", res.Content[0].Text)
	}
	if out["applyEnabled"] != false {
		t.Fatalf("applyEnabled false olmalı: %+v", out)
	}
	_, res = callTyped(t, mgr, "gitarc_queue_run", map[string]any{"confirm": "herhangi"})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "kapalı") {
		t.Fatalf("varsayılan olarak kapalı olmalı: %+v", res)
	}
	if st, res := callTyped(t, mgr, "gitarc_queue_run_status", map[string]any{}); res.IsError || st["run"] != nil {
		t.Fatalf("koşu yokken run null olmalı: %+v %+v", st, res)
	}
	// durdurma kapalı bayrağından bağımsızdır; çalışan koşu yokken dürüst hata verir
	if _, res := callTyped(t, mgr, "gitarc_queue_run_stop", map[string]any{}); !res.IsError {
		t.Fatal("çalışan koşu yokken durdurma hata vermeli")
	}
}

func TestQueueRunEnabledStillNeedsTheFingerprint(t *testing.T) {
	t.Setenv(envAllowQueueRun, "1")
	mgr := gitengine.NewManager("sim")
	if _, res := callTyped(t, mgr, "gitarc_queue_run", map[string]any{}); !res.IsError {
		t.Fatal("confirm zorunlu")
	}
	// yanlış parmak izi: hiçbir şey başlamaz
	if _, res := callTyped(t, mgr, "gitarc_queue_run", map[string]any{"confirm": "yanlis", "branchIds": []any{"br-open-424"}}); !res.IsError {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if st, _ := callTyped(t, mgr, "gitarc_queue_run_status", map[string]any{}); st["run"] != nil {
		t.Fatalf("reddedilen başlatma koşu oluşturmamalı: %+v", st)
	}
}
