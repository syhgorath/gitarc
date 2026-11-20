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
