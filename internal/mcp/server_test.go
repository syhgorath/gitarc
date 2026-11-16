package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
)

func TestMCPServeStdioTools(t *testing.T) {
	mgr := gitengine.NewManager("")

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"gitarc_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"gitarc_check_conflicts","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"gitarc_ci_failure_digest","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"gitarc_sync_main","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"gitarc_resolve_conflict","arguments":{"branchId":"br-open-421","strategy":"ours"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := ServeStdio(strings.NewReader(input), &out, mgr); err != nil {
		t.Fatalf("ServeStdio failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("expected 7 JSON-RPC responses (notification skipped), got %d:\n%s", len(lines), out.String())
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
	if len(r2.Result.Tools) != 5 {
		t.Fatalf("expected 5 MCP tools (including gitarc_resolve_conflict), got %d", len(r2.Result.Tools))
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
}
