package mcp

import (
	"strings"
	"testing"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

const simConflictBranch = "br-open-421"

func conflictPlan(t *testing.T, mgr *gitengine.Manager, id string) map[string]any {
	t.Helper()
	out, res := callTyped(t, mgr, "gitarc_conflict_plan", map[string]any{"branchId": id})
	if res.IsError {
		t.Fatalf("conflict_plan: %s", res.Content[0].Text)
	}
	return out
}

func TestConflictPlanIsReadOnlyAndHonest(t *testing.T) {
	t.Setenv(envAllowResolve, "")
	mgr := gitengine.NewManager("sim")

	p := conflictPlan(t, mgr, simConflictBranch)
	if p["hasConflict"] != true || p["fingerprint"] == "" || p["resolveEnabled"] != false {
		t.Fatalf("plan: %+v", p)
	}
	if files, _ := p["files"].([]any); len(files) == 0 {
		t.Fatalf("çakışan dosyalar listelenmeli: %+v", p)
	}
	if w, _ := p["warnings"].([]any); len(w) == 0 || !strings.Contains(p["next"].(string), envAllowResolve) {
		t.Fatalf("riskler ve nasıl açılacağı söylenmeli: %+v", p)
	}
	// Plan hiçbir şeyi çözmez; aynı plan aynı parmak izini verir.
	again := conflictPlan(t, mgr, simConflictBranch)
	if again["hasConflict"] != true || again["fingerprint"] != p["fingerprint"] {
		t.Fatalf("plan çakışmayı çözmemeli ve kararlı olmalı: %+v", again)
	}
	// Çakışma bulunamaması arıza değil, sonuçtur.
	none := conflictPlan(t, mgr, "yok-boyle-bir-dal")
	if none["hasConflict"] != false || none["note"] == "" {
		t.Fatalf("çakışma yoksa bunu söylemeli: %+v", none)
	}
}

func TestResolveConflictIsOffByDefault(t *testing.T) {
	t.Setenv(envAllowResolve, "")
	mgr := gitengine.NewManager("sim")
	p := conflictPlan(t, mgr, simConflictBranch)

	_, res := callTyped(t, mgr, "gitarc_resolve_conflict", map[string]any{
		"branchId": simConflictBranch, "strategy": "ours", "confirm": p["fingerprint"],
	})
	if !res.IsError || !strings.Contains(res.Content[0].Text, envAllowResolve) {
		t.Fatalf("izinsiz çözüm reddedilmeli ve nedenini söylemeli: %+v", res)
	}
	if after := conflictPlan(t, mgr, simConflictBranch); after["hasConflict"] != true {
		t.Fatal("izin yokken çakışma çözülmemeli")
	}
}

func TestResolveConflictNeedsMatchingFingerprintAndReportsTruthfully(t *testing.T) {
	t.Setenv(envAllowResolve, "1")
	mgr := gitengine.NewManager("sim")
	p := conflictPlan(t, mgr, simConflictBranch)
	fp := p["fingerprint"].(string)

	bad := []map[string]any{
		{"branchId": simConflictBranch, "strategy": "ours"},                      // confirm yok
		{"branchId": simConflictBranch, "strategy": "ours", "confirm": "yanlis"}, // yanlış parmak izi
		{"branchId": simConflictBranch, "strategy": "hepsi", "confirm": fp},      // bilinmeyen strateji
		{"branchId": simConflictBranch, "strategy": "custom", "confirm": fp},     // customFiles yok
		{"strategy": "ours", "confirm": fp},                                      // branchId yok
	}
	for i, args := range bad {
		if _, res := callTyped(t, mgr, "gitarc_resolve_conflict", args); !res.IsError {
			t.Fatalf("kötü girdi #%d reddedilmeli", i)
		}
	}
	if conflictPlan(t, mgr, simConflictBranch)["hasConflict"] != true {
		t.Fatal("reddedilen denemeler çakışmayı çözmemeli")
	}

	out, res := callTyped(t, mgr, "gitarc_resolve_conflict", map[string]any{"branchId": simConflictBranch, "strategy": "ours", "confirm": fp})
	if res.IsError || out["resolved"] != true || out["strategy"] != "ours" {
		t.Fatalf("çözüm: %+v %+v", out, res)
	}
	if conflictPlan(t, mgr, simConflictBranch)["hasConflict"] != false {
		t.Fatal("çözümden sonra çakışma kalkmış olmalı")
	}

	// Artık çakışma yok: "başarılı" demek yerine hiçbir şey değiştirmeden reddeder.
	_, res = callTyped(t, mgr, "gitarc_resolve_conflict", map[string]any{"branchId": simConflictBranch, "strategy": "ours", "confirm": fp})
	if !res.IsError || !strings.Contains(res.Content[0].Text, "hiçbir şey değiştirilmedi") {
		t.Fatalf("çakışma yokken çözüm başarılı sayılmamalı: %+v", res)
	}
}

func TestConflictFingerprintChangesWithContent(t *testing.T) {
	base := model.ConflictStudioPayload{BranchName: "feat", BaseRef: "origin/main", Files: []model.ConflictFileDiff{{Path: "a.go", ConflictHunks: "<<<<<<< x\nA\n=======\nB\n>>>>>>> y"}}}
	same := base
	if conflictFingerprint(base) != conflictFingerprint(same) {
		t.Fatal("aynı çakışma aynı parmak izini vermeli")
	}
	changed := base
	changed.Files = []model.ConflictFileDiff{{Path: "a.go", ConflictHunks: "<<<<<<< x\nA\n=======\nC\n>>>>>>> y"}}
	if conflictFingerprint(base) == conflictFingerprint(changed) {
		t.Fatal("çakışma içeriği değişince parmak izi değişmeli")
	}
	other := base
	other.BaseRef = "origin/develop"
	if conflictFingerprint(base) == conflictFingerprint(other) {
		t.Fatal("taban değişince parmak izi değişmeli")
	}
}

func TestConflictToolsRefuseWithoutProject(t *testing.T) {
	t.Setenv(envAllowResolve, "1")
	mgr := noProjectManager(t)
	for _, name := range []string{"gitarc_conflict_plan", "gitarc_resolve_conflict"} {
		if _, res := callTyped(t, mgr, name, map[string]any{"branchId": "x", "strategy": "ours", "confirm": "x"}); !res.IsError {
			t.Fatalf("%s proje yokken hata vermeli", name)
		}
	}
}
