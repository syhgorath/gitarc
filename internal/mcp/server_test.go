package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
)

func TestMCPServeStdioTools(t *testing.T) {
	mgr := gitengine.NewManager("sim")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"gitarc_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"gitarc_check_conflicts","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"gitarc_ci_failure_digest","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"gitarc_sync_main","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"gitarc_resolve_conflict","arguments":{"branchId":"br-open-421","strategy":"ours"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"gitarc_cleanup_candidates","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"gitarc_scan_secrets","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"gitarc_audit_log","arguments":{"limit":10}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := ServeStdio(strings.NewReader(input), &out, mgr); err != nil {
		t.Fatalf("ServeStdio failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 10 {
		t.Fatalf("expected 10 JSON-RPC responses (notification skipped), got %d:\n%s", len(lines), out.String())
	}

	// Verify initialize
	var r1 struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name string `json:"name"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &r1); err != nil {
		t.Fatal(err)
	}
	if r1.Result.ServerInfo.Name != "gitarc" || r1.Result.ProtocolVersion == "" {
		t.Fatalf("unexpected initialize response: %s", lines[0])
	}

	// Verify tools/list
	var r2 struct {
		Result struct {
			Tools []toolDefinition `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &r2); err != nil {
		t.Fatal(err)
	}
	if len(r2.Result.Tools) != 12 {
		t.Fatalf("expected 12 MCP tools, got %d", len(r2.Result.Tools))
	}

	// Verify gitarc_status includes activeContext and openPRs for AI assistance
	if !strings.Contains(lines[2], "activeContext") || !strings.Contains(lines[2], "openPRs") {
		t.Fatalf("expected gitarc_status to contain activeContext and openPRs, got: %s", lines[2])
	}

	// Verify gitarc_check_conflicts mentions CHANGELOG.md and conflictHunks
	if !strings.Contains(lines[3], "CHANGELOG.md") || !strings.Contains(lines[3], "conflictHunks") {
		t.Fatalf("expected gitarc_check_conflicts output to mention CHANGELOG.md and conflictHunks, got: %s", lines[3])
	}

	// Verify gitarc_ci_failure_digest contains concise TestShellGoldCompare without noisy group headers
	if !strings.Contains(lines[4], "TestShellGoldCompare") || strings.Contains(lines[4], "##[group]") {
		t.Fatalf("expected clean CI failure digest, got: %s", lines[4])
	}

	// Verify gitarc_resolve_conflict resolved br-open-421
	if !strings.Contains(lines[6], `\"resolved\": true`) || strings.Contains(lines[6], `"isError":true`) {
		t.Fatalf("expected gitarc_resolve_conflict to resolve cleanly, got: %s", lines[6])
	}

	// Verify gitarc_cleanup_candidates
	if !strings.Contains(lines[7], "candidates") || strings.Contains(lines[7], `"isError":true`) {
		t.Fatalf("expected clean candidates payload, got: %s", lines[7])
	}

	// Verify gitarc_scan_secrets
	if !strings.Contains(lines[8], "clean") || strings.Contains(lines[8], `"isError":true`) {
		t.Fatalf("expected scan secrets output, got: %s", lines[8])
	}

	// Verify gitarc_audit_log
	if !strings.Contains(lines[9], "logs") || strings.Contains(lines[9], `"isError":true`) {
		t.Fatalf("expected audit log output, got: %s", lines[9])
	}
}

func TestMCPServeStdioLiveModeAndSimulationToggle(t *testing.T) {
	// 1. Live mode: Pass a real git repo path
	liveMgr := gitengine.NewManager("../..")
	snap, err := liveMgr.Snapshot()
	if err != nil {
		t.Fatalf("live manager snapshot failed: %v", err)
	}
	if snap.Repo.Mode != "live" {
		t.Fatalf("expected mode=live for real git repo, got %q", snap.Repo.Mode)
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"gitarc_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"gitarc_check_conflicts","arguments":{}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := ServeStdio(strings.NewReader(input), &out, liveMgr); err != nil {
		t.Fatalf("ServeStdio failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(lines))
	}

	if !strings.Contains(lines[0], `\"mode\": \"live\"`) {
		t.Fatalf("expected mode: live in status output, got: %s", lines[0])
	}
	if !strings.Contains(lines[0], `\"openPRCount\"`) {
		t.Fatalf("expected openPRCount in status output, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], `\"mode\": \"live\"`) {
		t.Fatalf("expected mode: live in conflicts output, got: %s", lines[1])
	}

	// 2. Explicit simulation mode via GITARC_MODE=simulation
	t.Setenv("GITARC_MODE", "simulation")
	simMgr := gitengine.NewManager("")
	simSnap, err := simMgr.Snapshot()
	if err != nil {
		t.Fatalf("sim snapshot failed: %v", err)
	}
	if simSnap.Repo.Mode != "simulation" {
		t.Fatalf("expected mode=simulation when GITARC_MODE=simulation, got %q", simSnap.Repo.Mode)
	}
}

func TestMCPProposeCommit(t *testing.T) {
	t.Setenv("GITARC_MODE", "simulation")
	mgr := gitengine.NewManager("")

	// 1. Temiz bir dal ve dosya ile öneri oluşturma
	reqProposal := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"gitarc_propose_commit","arguments":{"branch_name":"feat/safe-ai-flow","message":"feat: ai flow test","files":["CHANGELOG.md"]}}}` + "\n"
	var out bytes.Buffer
	if err := ServeStdio(strings.NewReader(reqProposal), &out, mgr); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	respStr := out.String()
	if !strings.Contains(respStr, "proposed") || !strings.Contains(respStr, "feat/safe-ai-flow") {
		t.Fatalf("expected proposed status in response, got: %s", respStr)
	}

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if snap.PendingProposal == nil {
		t.Fatalf("expected pendingProposal in snapshot, got nil")
	}
	if snap.PendingProposal.BranchName != "feat/safe-ai-flow" {
		t.Errorf("expected branchName feat/safe-ai-flow, got %s", snap.PendingProposal.BranchName)
	}

	// 2. Gizli dosya içeren öneri reddedilmeli
	reqSecret := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"gitarc_propose_commit","arguments":{"branch_name":"feat/secret-leak","message":"feat: leak","files":[".mcp.json"]}}}` + "\n"
	out.Reset()
	if err := ServeStdio(strings.NewReader(reqSecret), &out, mgr); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	if !strings.Contains(out.String(), "GÜVENLİK ENGELİ") {
		t.Fatalf("expected GÜVENLİK ENGELİ for secret file .mcp.json, got: %s", out.String())
	}

	// 3. Dismiss öneriyi temizlemeli
	dismissSnap, errDismiss := mgr.DismissProposal()
	if errDismiss != nil {
		t.Fatalf("dismiss failed: %v", errDismiss)
	}
	if dismissSnap.PendingProposal != nil {
		t.Fatalf("expected pendingProposal to be nil after dismiss")
	}

	// 4. files boş olduğunda HATA dönmeli (Kural 1)
	reqEmptyFiles := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"gitarc_propose_commit","arguments":{"branch_name":"feat/empty-files","message":"feat: empty","files":[]}}}` + "\n"
	out.Reset()
	if err := ServeStdio(strings.NewReader(reqEmptyFiles), &out, mgr); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	if !strings.Contains(out.String(), "files parametresi zorunludur") {
		t.Fatalf("expected error for empty files, got: %s", out.String())
	}

	// 5. Co-Authored-By içeren çok satırlı mesaj tam olarak korunmalı (Kural 4)
	reqCoAuthor := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"gitarc_propose_commit","arguments":{"branch_name":"feat/co-author-check","message":"feat: add co-author\n\nCo-Authored-By: Claude <noreply@anthropic.com>","files":["CHANGELOG.md"]}}}` + "\n"
	out.Reset()
	if err := ServeStdio(strings.NewReader(reqCoAuthor), &out, mgr); err != nil {
		t.Fatalf("ServeStdio error: %v", err)
	}
	if !strings.Contains(out.String(), "Co-Authored-By: Claude") {
		t.Fatalf("expected message to preserve Co-Authored-By line, got: %s", out.String())
	}

	// 6. Dosya sayısı eşleşmesi doğrulanmalı (Kural 3)
	snapCoAuth, _ := mgr.Snapshot()
	if snapCoAuth.PendingProposal == nil || !snapCoAuth.PendingProposal.CountCheckMatch {
		t.Fatalf("expected countCheckMatch to be true, got: %+v", snapCoAuth.PendingProposal)
	}
	if !strings.Contains(snapCoAuth.PendingProposal.ShieldResult, "Güvenlik Kalkanı") {
		t.Errorf("expected shieldResult to contain Güvenlik Kalkanı, got: %s", snapCoAuth.PendingProposal.ShieldResult)
	}
}


func TestToolsRefuseToServeDemoDataWhenThereIsNoRepo(t *testing.T) {
	empty := t.TempDir()
	old, _ := os.Getwd()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	mgr := gitengine.NewManager(empty)
	res := callTool(toolCallParams{Name: "gitarc_status"}, mgr)
	if !res.IsError || len(res.Content) == 0 || !strings.Contains(res.Content[0].Text, "Git deposu yok") {
		t.Fatalf("without a repo the tools must say so instead of returning fake data: %+v", res)
	}
}
