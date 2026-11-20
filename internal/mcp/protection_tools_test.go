package mcp

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
)

func callTyped(t *testing.T, mgr *gitengine.Manager, name string, args map[string]any) (map[string]any, toolResult) {
	t.Helper()
	res := callTool(toolCallParams{Name: name, Arguments: args}, mgr)
	var out map[string]any
	if !res.IsError && len(res.Content) > 0 {
		if err := json.Unmarshal([]byte(res.Content[0].Text), &out); err != nil {
			t.Fatalf("%s çıktısı JSON değil: %v\n%s", name, err, res.Content[0].Text)
		}
	}
	return out, res
}

func TestProtectionToolsAreListed(t *testing.T) {
	names := map[string]bool{}
	for _, d := range listTools() {
		names[d.Name] = true
	}
	for _, want := range []string{"gitarc_protection_status", "gitarc_protection_plan", "gitarc_protection_apply"} {
		if !names[want] {
			t.Fatalf("%s listede yok", want)
		}
	}
}

func TestProtectionStatusAndPlanAreReadOnly(t *testing.T) {
	t.Setenv(envAllowProtection, "")
	mgr := gitengine.NewManager("sim")

	st, res := callTyped(t, mgr, "gitarc_protection_status", nil)
	if res.IsError || st["state"] != "none" || st["applyEnabled"] != false {
		t.Fatalf("status: %+v %+v", st, res)
	}
	if checks, _ := st["availableChecks"].([]any); len(checks) == 0 {
		t.Fatalf("seçilebilir kontroller listelenmeli: %+v", st)
	}

	pl, res := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "ci", "checks": []any{"test"}})
	if res.IsError {
		t.Fatalf("plan: %s", res.Content[0].Text)
	}
	plan := pl["plan"].(map[string]any)
	if plan["fingerprint"] == "" || len(plan["changes"].([]any)) == 0 || !strings.Contains(pl["next"].(string), envAllowProtection) {
		t.Fatalf("plan eksik: %+v", pl)
	}
	after, _ := callTyped(t, mgr, "gitarc_protection_status", nil)
	if after["state"] != "none" {
		t.Fatal("plan korumayı uygulamamalı")
	}
}

func TestProtectionApplyIsOffByDefault(t *testing.T) {
	t.Setenv(envAllowProtection, "")
	mgr := gitengine.NewManager("sim")
	pl, _ := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "temel"})
	fp := pl["plan"].(map[string]any)["fingerprint"].(string)

	_, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"profile": "temel", "confirm": fp})
	if !res.IsError || !strings.Contains(res.Content[0].Text, envAllowProtection) {
		t.Fatalf("izinsiz uygulama reddedilmeli ve nedenini söylemeli: %+v", res)
	}
	st, _ := callTyped(t, mgr, "gitarc_protection_status", nil)
	if st["state"] != "none" {
		t.Fatal("izin yokken koruma değişmemeli")
	}
}

func TestProtectionApplyWithPermissionNeedsMatchingFingerprint(t *testing.T) {
	t.Setenv(envAllowProtection, "1")
	mgr := gitengine.NewManager("sim")

	if _, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"profile": "temel"}); !res.IsError {
		t.Fatal("confirm olmadan uygulama reddedilmeli")
	}
	if _, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"profile": "temel", "confirm": "yanlis"}); !res.IsError {
		t.Fatal("yanlış parmak izi reddedilmeli")
	}
	if st, _ := callTyped(t, mgr, "gitarc_protection_status", nil); st["state"] != "none" || st["applyEnabled"] != true {
		t.Fatalf("başarısız denemeler korumayı değiştirmemeli: %+v", st)
	}

	pl, _ := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "ci", "checks": []any{"test"}})
	fp := pl["plan"].(map[string]any)["fingerprint"].(string)
	// Plan alındıktan sonra başka seçimle uygulama (plan değişti) reddedilir.
	if _, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"profile": "ci", "checks": []any{"lint"}, "confirm": fp}); !res.IsError {
		t.Fatal("farklı kontrol seçimiyle eski parmak izi reddedilmeli")
	}
	ap, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"profile": "ci", "checks": []any{"test"}, "confirm": fp})
	if res.IsError || ap["applied"] != true || ap["protectionState"] != "active" || !strings.Contains(ap["summary"].(string), "CI: test") {
		t.Fatalf("uygulama: %+v %+v", ap, res)
	}
	again, _ := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "ci", "checks": []any{"test"}})
	if n := len(again["plan"].(map[string]any)["changes"].([]any)); n != 0 {
		t.Fatalf("uygulamadan sonra değişecek bir şey kalmamalı: %d", n)
	}
}

// MCP üzerinden de koruma yalnızca güçlenir: kurumsal demo 2 onaylı; "temel" profil bunu düşürmez.
func TestProtectionViaMCPNeverRelaxes(t *testing.T) {
	t.Setenv(envAllowProtection, "1")
	mgr := gitengine.NewManager("sim")
	if _, err := mgr.SwitchRepo("sim-enterprise"); err != nil {
		t.Fatal(err)
	}
	pl, res := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "temel"})
	if res.IsError {
		t.Fatal(res.Content[0].Text)
	}
	plan := pl["plan"].(map[string]any)
	if got := plan["target"].(map[string]any)["approvals"]; got != float64(2) {
		t.Fatalf("mevcut 2 onay düşürülmemeli, hedef: %v", got)
	}
	if n := len(plan["changes"].([]any)); n != 0 {
		t.Fatalf("temel profil zaten karşılanıyor, değişiklik olmamalı: %d", n)
	}
}

func TestProtectionToolsRejectBadInput(t *testing.T) {
	t.Setenv(envAllowProtection, "1")
	mgr := gitengine.NewManager("sim")
	if _, res := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "hepsi"}); !res.IsError {
		t.Fatal("bilinmeyen profil reddedilmeli")
	}
	if _, res := callTyped(t, mgr, "gitarc_protection_plan", map[string]any{"profile": "ci", "checks": []any{"a\nb"}}); !res.IsError {
		t.Fatal("satır sonu içeren kontrol adı reddedilmeli")
	}
	if _, res := callTyped(t, mgr, "gitarc_protection_apply", map[string]any{"confirm": "x"}); !res.IsError {
		t.Fatal("profil olmadan uygulama reddedilmeli")
	}
}

// noProjectManager: depo olmayan bir klasörde (ve çalışma dizininde) açılmış, projesi olmayan yönetici.
func noProjectManager(t *testing.T) *gitengine.Manager {
	t.Helper()
	empty := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return gitengine.NewManager(empty)
}

func TestProtectionToolsRefuseWithoutProject(t *testing.T) {
	t.Setenv(envAllowProtection, "1")
	mgr := noProjectManager(t)
	for _, name := range []string{"gitarc_protection_status", "gitarc_protection_plan", "gitarc_protection_apply"} {
		if _, res := callTyped(t, mgr, name, map[string]any{"profile": "temel", "confirm": "x"}); !res.IsError {
			t.Fatalf("%s proje yokken hata vermeli (demo veriyle çalışmamalı)", name)
		}
	}
}
