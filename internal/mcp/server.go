package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

const protocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content []textContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// ServeStdio runs a zero-dependency JSON-RPC 2.0 MCP server over r/w.
func ServeStdio(r io.Reader, w io.Writer, mgr *gitengine.Manager) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(w)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(rpcResponse{
				JSONRPC: "2.0",
				Error:   &rpcError{Code: -32700, Message: "parse error"},
			})
			continue
		}

		// JSON-RPC notifications have no id and expect no response.
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue
		}

		resp := handleRequest(req, mgr)
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func handleRequest(req rpcRequest, mgr *gitengine.Manager) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": protocolVersion,
				"capabilities": map[string]any{
					"tools": map[string]any{"listChanged": false},
				},
				"serverInfo": map[string]any{
					"name":    "gitarc",
					"version": gitengine.AppVersion,
				},
			},
		}
	case "ping":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{},
		}
	case "tools/list":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": listTools(),
			},
		}
	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return rpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32602, Message: "invalid params"},
			}
		}
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  callTool(params, mgr),
		}
	default:
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("method not found: %s", req.Method)},
		}
	}
}

func listTools() []toolDefinition {
	emptySchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
	return []toolDefinition{
		{
			Name:        "gitarc_status",
			Description: "Aktif Git deposunun main omurga farkını, üst raydaki açık PR'ları, alt raydaki yerel dalları ve temizlenebilir dalları özetler.",
			InputSchema: emptySchema,
		},
		{
			Name:        "gitarc_check_conflicts",
			Description: "Çalışma dizinine dokunmadan git merge-tree --write-tree çakışmalarını ve açık PR'lar arası Cross-PR ortak dosya kesişimlerini raporlar.",
			InputSchema: emptySchema,
		},
		{
			Name:        "gitarc_ci_failure_digest",
			Description: "Başarısız GitHub Actions CI koşularından regex ile ayıklanmış gürültüsüz hata satırlarını ve yerel test repro komutlarını döndürür.",
			InputSchema: emptySchema,
		},
		{
			Name:        "gitarc_sync_main",
			Description: "Çalışma dizinini bozmadan origin/main ile yerel main referansını eşitler.",
			InputSchema: emptySchema,
		},
		{
			Name:        "gitarc_resolve_conflict",
			Description: "Seçili daldaki merge çakışmasını izole git worktree içinde (union, ours, theirs veya custom içerikle) çözer, go vet doğrulamasından geçirir ve commit/push yapar.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branchId": map[string]any{
						"type":        "string",
						"description": "Çözülecek dal kimliği (ID veya dal adı, örn. feat/v0.2.0-live-dogfooding).",
					},
					"strategy": map[string]any{
						"type":        "string",
						"enum":        []string{"union", "ours", "theirs", "custom"},
						"description": "Çakışma çözüm stratejisi: union | ours | theirs | custom.",
					},
					"customFiles": map[string]any{
						"type":        "object",
						"description": "Opsiyonel dosya yolu -> pencere içerik haritası (strategy='custom' iken kullanılır).",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
				},
				"required": []string{"branchId", "strategy"},
			},
		},
		{
			Name:        "gitarc_propose_commit",
			Description: "Hiçbir şeyi doğrudan commit/push etmez; GitArc arayüzünde kullanıcının tek tıkla inceleyip onaylayabileceği güvenli bir 'Commit & PR Öneri Kartı' oluşturur. .gitignore, özel anahtarlar ve secret kalkanları otomatik işletilir.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branch_name": map[string]any{
						"type":        "string",
						"description": "origin/main tabanlı açılacak yeni özellik dalının adı (örn. feat/safe-storage).",
					},
					"message": map[string]any{
						"type":        "string",
						"description": "Önerilen commit mesajı.",
					},
					"files": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
						"description": "Açıkça commitlenecek dosyaların göreceli yolları listesi (opsiyonel, boşsa çalışma ağacındaki güvenli değişmiş dosyalar kullanılır).",
					},
					"pr_title": map[string]any{
						"type":        "string",
						"description": "Önerilen GitHub PR başlığı (opsiyonel).",
					},
					"pr_body": map[string]any{
						"type":        "string",
						"description": "Önerilen GitHub PR gövde açıklaması (opsiyonel).",
					},
				},
				"required": []string{"branch_name", "message", "files"},
			},
		},
	}
}

func callTool(params toolCallParams, mgr *gitengine.Manager) toolResult {
	switch params.Name {
	case "gitarc_propose_commit":
		return callProposeCommit(params.Arguments, mgr)

	case "gitarc_status":
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		return jsonTextResult(buildStatusPayload(snap))

	case "gitarc_check_conflicts":
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		return jsonTextResult(buildConflictsPayload(snap, mgr))

	case "gitarc_ci_failure_digest":
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		return jsonTextResult(buildCIDigestPayload(snap))

	case "gitarc_sync_main":
		snap, err := mgr.SyncSpine()
		if err != nil {
			return errResult(err.Error())
		}
		mode := snap.Repo.Mode
		if mode == "" {
			mode = "live"
		}
		return jsonTextResult(map[string]any{
			"mode":          mode,
			"repo":          snap.Repo.Name,
			"defaultBranch": snap.Repo.DefaultBranch,
			"ahead":         snap.Repo.Ahead,
			"behind":        snap.Repo.Behind,
			"nodeCount":     len(snap.Nodes),
		})

	case "gitarc_resolve_conflict":
		branchID, _ := params.Arguments["branchId"].(string)
		strategy, _ := params.Arguments["strategy"].(string)
		if strings.TrimSpace(branchID) == "" || strings.TrimSpace(strategy) == "" {
			return errResult("branchId ve strategy zorunludur")
		}
		var customFiles map[string]string
		if rawMap, ok := params.Arguments["customFiles"].(map[string]any); ok && len(rawMap) > 0 {
			customFiles = make(map[string]string, len(rawMap))
			for k, v := range rawMap {
				if s, ok := v.(string); ok {
					customFiles[k] = s
				}
			}
		}
		snap, err := mgr.ResolveBranchConflict(branchID, strategy, customFiles, true)
		if err != nil {
			return errResult(err.Error())
		}
		mode := snap.Repo.Mode
		if mode == "" {
			mode = "live"
		}
		return jsonTextResult(map[string]any{
			"mode":     mode,
			"resolved": true,
			"branchId": branchID,
			"strategy": strategy,
			"repo":     snap.Repo.Name,
			"branches": buildStatusPayload(snap)["branches"],
		})

	default:
		return errResult(fmt.Sprintf("bilinmeyen araç: %s", params.Name))
	}
}

func buildStatusPayload(snap model.CockpitSnapshot) map[string]any {
	type branchBrief struct {
		Name         string `json:"name"`
		Hemisphere   string `json:"hemisphere"`
		State        string `json:"state"`
		IsHEAD       bool   `json:"isHead"`
		PRNumber     int    `json:"prNumber,omitempty"`
		CIState      string `json:"ciState,omitempty"`
		OriginGone   bool   `json:"originGone,omitempty"`
		HasConflict  bool   `json:"hasConflict,omitempty"`
		CrossPRCount int    `json:"crossPrCount,omitempty"`
		SuggestedCmd string `json:"suggestedCmd,omitempty"`
	}
	var briefs []branchBrief
	openPRCount := 0
	localBranchCount := 0
	mergedCount := 0

	for _, b := range snap.Branches {
		br := branchBrief{
			Name:         b.Name,
			Hemisphere:   b.Hemisphere,
			State:        b.State,
			IsHEAD:       b.IsHEAD,
			OriginGone:   b.OriginGone,
			HasConflict:  b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved,
			CrossPRCount: len(b.CrossPR),
			SuggestedCmd: b.SuggestedCmd,
		}
		if b.PR != nil {
			br.PRNumber = b.PR.Number
			if b.PR.CI != nil {
				br.CIState = b.PR.CI.State
			}
			if b.State == "open" && b.PR.Number > 0 && b.PR.ReviewState != "MERGED" {
				openPRCount++
			}
		}
		if b.HasLocal && !strings.HasPrefix(b.ID, "gh-merged-") {
			localBranchCount++
		}
		if b.State == "merged" || b.OriginGone {
			mergedCount++
		}
		briefs = append(briefs, br)
	}
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}
	return map[string]any{
		"mode":              mode,
		"openPRCount":      openPRCount,
		"localBranchCount": localBranchCount,
		"mergedCount":      mergedCount,
		"summary":          fmt.Sprintf("%s (%s): %d açık PR, %d yerel dal, %d birleşmiş/temizleme adayı", snap.Repo.Name, mode, openPRCount, localBranchCount, mergedCount),
		"repo":             snap.Repo,
		"branches":         briefs,
		"diagnostics":      snap.Diagnostics,
		"logs":             snap.Logs,
	}
}

func buildConflictsPayload(snap model.CockpitSnapshot, mgr *gitengine.Manager) map[string]any {
	type conflictReport struct {
		BranchID          string                   `json:"branchId"`
		Branch            string                   `json:"branch"`
		PRNumber          int                      `json:"prNumber,omitempty"`
		MergeTreeFiles    []string                 `json:"mergeTreeFiles,omitempty"`
		IsChangelogOrLock bool                     `json:"isChangelogOrLock,omitempty"`
		Strategy          string                   `json:"strategy,omitempty"`
		Files             []model.ConflictFileDiff `json:"files,omitempty"`
		CrossPR           []model.CrossPRWarning   `json:"crossPr,omitempty"`
	}
	var reports []conflictReport
	for _, b := range snap.Branches {
		hasConf := b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved
		if !hasConf && len(b.CrossPR) == 0 {
			continue
		}
		r := conflictReport{
			BranchID: b.ID,
			Branch:   b.Name,
			CrossPR:  b.CrossPR,
		}
		if b.PR != nil {
			r.PRNumber = b.PR.Number
		}
		if hasConf {
			r.MergeTreeFiles = b.Conflict.Files
			r.IsChangelogOrLock = b.Conflict.IsChangelogOrLock
			r.Strategy = b.Conflict.Strategy
			if mgr != nil {
				if studio, err := mgr.InspectConflictStudio(b.ID); err == nil {
					r.Files = studio.Files
				}
			}
		}
		reports = append(reports, r)
	}
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}
	return map[string]any{
		"mode":          mode,
		"repo":          snap.Repo.Name,
		"conflictCount": len(reports),
		"items":         reports,
	}
}

func buildCIDigestPayload(snap model.CockpitSnapshot) map[string]any {
	type ciFailItem struct {
		Branch     string                `json:"branch"`
		PRNumber   int                   `json:"prNumber"`
		Workflow   string                `json:"workflow"`
		FailedStep string                `json:"failedStep"`
		Extracts   []model.CIFailExtract `json:"extracts"`
	}
	var items []ciFailItem
	for _, b := range snap.Branches {
		if b.PR == nil || b.PR.CI == nil || b.PR.CI.State != "FAILURE" {
			continue
		}
		items = append(items, ciFailItem{
			Branch:     b.Name,
			PRNumber:   b.PR.Number,
			Workflow:   b.PR.CI.Workflow,
			FailedStep: b.PR.CI.FailedStep,
			Extracts:   b.PR.CI.Extracts,
		})
	}
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}
	return map[string]any{
		"mode":     mode,
		"repo":     snap.Repo.Name,
		"failures": items,
	}
}

func callProposeCommit(args map[string]any, mgr *gitengine.Manager) toolResult {
	branchName, _ := args["branch_name"].(string)
	if branchName == "" {
		branchName, _ = args["branchName"].(string)
	}
	message, _ := args["message"].(string)
	if message == "" {
		message, _ = args["commitMsg"].(string)
	}
	prTitle, _ := args["pr_title"].(string)
	if prTitle == "" {
		prTitle, _ = args["prTitle"].(string)
	}
	prBody, _ := args["pr_body"].(string)
	if prBody == "" {
		prBody, _ = args["prBody"].(string)
	}

	var files []string
	if rawFiles, ok := args["files"].([]any); ok {
		for _, f := range rawFiles {
			if s, ok := f.(string); ok && strings.TrimSpace(s) != "" {
				files = append(files, strings.TrimSpace(s))
			}
		}
	} else if strFiles, ok := args["files"].([]string); ok {
		for _, s := range strFiles {
			if strings.TrimSpace(s) != "" {
				files = append(files, strings.TrimSpace(s))
			}
		}
	}

	if len(files) == 0 {
		return errResult("files parametresi zorunludur ve en az bir geçerli dosya yolu içermelidir; boş bırakılamaz")
	}

	prop, err := mgr.ProposeCommit(branchName, message, files, prTitle, prBody)
	if err != nil {
		return errResult(err.Error())
	}

	snap, _ := mgr.Snapshot()
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}

	return jsonTextResult(map[string]any{
		"mode":     mode,
		"status":   "proposed",
		"proposal": prop,
		"summary":  fmt.Sprintf("Öneri GitArc kokpitine iletildi (%s dalı, %d dosya). Kullanıcının arayüzdeki 'Öneri Kartı' üzerinden inceleyip onaylaması bekleniyor.", prop.BranchName, prop.FilesChanged),
	})
}

func jsonTextResult(v any) toolResult {
	b, _ := json.MarshalIndent(v, "", "  ")
	return toolResult{
		Content: []textContent{{Type: "text", Text: string(b)}},
	}
}

func errResult(msg string) toolResult {
	return toolResult{
		Content: []textContent{{Type: "text", Text: msg}},
		IsError: true,
	}
}
