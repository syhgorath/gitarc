package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
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
	base := []toolDefinition{
		{
			Name: "gitarc_status",
			Description: "GitArc kokpitinin anlık durumu. VARSAYILAN (detail=compact) KISA bir özet döner (birkaç yüz–birkaç bin karakter): aktif dal, kirli dosya sayısı/adları, koruma ve main CI, açık PR'lar için tek satır (CI, inceleme, geride/ileride, çakışma), yerel dallar, tanılama özeti, varsa çalışan kuyruk koşusu. " +
				"Tek bir dalın ayrıntısı (PR gövdesi, değişen dosyalar, taslak PR metni, önerilen komut) için branch=<dal adı> verin. Bugünkü tam döküm için detail=full (çok büyük olabilir; çoğu iş için gerekmez).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"detail": map[string]any{"type": "string", "enum": []string{"compact", "full"}, "description": "compact (varsayılan): kısa özet; full: tam döküm."},
					"branch": map[string]any{"type": "string", "description": "Yalnızca bu dalın ayrıntısı (ad ya da kimlik)."},
				},
			},
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
			Description: "Seçili daldaki merge çakışmasını izole git worktree içinde (union, ours, theirs veya custom) çözer, go.mod varsa go vet ile doğrular, yerel dalı günceller ve dalın uzaktaki hâline PUSH eder (açık bir PR'ı değiştirir). VARSAYILAN OLARAK KAPALIDIR: yalnızca MCP sunucusu GITARC_MCP_ALLOW_RESOLVE=1 ile başlatıldıysa çalışır. Önce gitarc_conflict_plan çağırıp kullanıcıya göstermeli ve dönen fingerprint'i confirm olarak vermelisiniz; çakışma değiştiyse reddedilir. ours/theirs bir tarafın değişikliğini ATAR. Sonuçtaki resolved yalnızca çakışma gerçekten kalktıysa true'dur.",
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
						"description": "strategy='custom' iken zorunlu: dosya yolu -> çözülmüş içerik haritası (çakışma işaretleri kalmamalı).",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
					"confirm": map[string]any{
						"type":        "string",
						"description": "gitarc_conflict_plan çıktısındaki fingerprint (aynen).",
					},
				},
				"required": []string{"branchId", "strategy", "confirm"},
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
						"description": "Commitlenecek açık dosyaların göreceli yolları listesi. Boş bırakılamaz ve en az bir dosya içermelidir.",
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
		{
			Name:        "gitarc_cleanup_candidates",
			Description: "Uzakta birleşmiş (merged) veya silinmiş (gone) olan temizlenebilir yerel ve uzak dalları kanıtlarıyla listeler. Doğrudan silme yapmaz; güvenli silme komutunu metin olarak önerir.",
			InputSchema: emptySchema,
		},
		{
			Name:        "gitarc_scan_secrets",
			Description: "Çalışma ağacındaki veya belirtilen dosyalardaki API anahtarları, özel anahtarlar (RSA/SSH), .env dosyaları ve kişisel kullanıcı yollarını (/Users/...) tarar.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "string",
						},
						"description": "Taranacak dosya yolları (opsiyonel; boşsa çalışma ağacındaki tüm kirli/değişen dosyalar taranır).",
					},
				},
			},
		},
		{
			Name:        "gitarc_audit_log",
			Description: "GitArc kokpiti ve MCP üzerinden gerçekleştirilen son işlemleri, önerileri ve onay kayıtlarını zaman damgalı olarak listeler. Her kayıt hangi projeye (repoId, repoName) ait olduğunu söyler; varsayılan yalnızca seçili projedir.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{
						"type":        "integer",
						"description": "Döndürülecek maksimum log kaydı sayısı (varsayılan: 25).",
					},
					"repoId": map[string]any{
						"type":        "string",
						"description": "Varsayılan: yalnızca seçili (etkin) proje. \"all\" = tüm projeler; ya da bir proje kimliği. Her kayıt repoId ve repoName taşır; repoId'si olmayan eski kayıtlar yalnızca \"all\" ile \"bilinmiyor\" olarak görünür.",
					},
				},
			},
		},
	}
	return append(append(append(base, conflictToolDefs()...), protectionToolDefs()...), queueToolDefs()...)
}

func callTool(params toolCallParams, mgr *gitengine.Manager) toolResult {
	// Açık bir Git deposu yoksa araçlar sahte (demo) veri döndürmesin: net bir hata.
	if mgr.NoProject() {
		return errResult("Açık bir Git deposu yok: `gitarc -mcp -repo <depo-yolu>` ile bir depo klasörü verin")
	}
	if res, ok := callProtectionTool(params.Name, params.Arguments, mgr); ok {
		return res
	}
	if res, ok := callConflictTool(params.Name, params.Arguments, mgr); ok {
		return res
	}
	if res, ok := callQueueTool(params.Name, params.Arguments, mgr); ok {
		return res
	}
	switch params.Name {
	case "gitarc_propose_commit":
		return callProposeCommit(params.Arguments, mgr)

	case "gitarc_status":
		detail, _ := params.Arguments["detail"].(string)
		branch, _ := params.Arguments["branch"].(string)
		detail = strings.ToLower(strings.TrimSpace(detail))
		if detail != "" && detail != "compact" && detail != "full" {
			return errResult(`detail "compact" ya da "full" olmalıdır`)
		}
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		if strings.TrimSpace(branch) != "" {
			payload, ok := buildBranchDetailPayload(snap, strings.TrimSpace(branch))
			if !ok {
				return errResult("dal bulunamadı: " + branch + " (adları görmek için gitarc_status'u dal parametresiz çağırın)")
			}
			return jsonTextResult(payload)
		}
		if detail == "full" {
			return jsonTextResult(buildStatusPayload(snap))
		}
		return jsonTextResult(buildCompactStatusPayload(snap))

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

	case "gitarc_cleanup_candidates":
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		return jsonTextResult(buildCleanupCandidatesPayload(snap))

	case "gitarc_scan_secrets":
		var paths []string
		if rawPaths, ok := params.Arguments["paths"].([]any); ok {
			for _, p := range rawPaths {
				if s, ok := p.(string); ok && strings.TrimSpace(s) != "" {
					paths = append(paths, strings.TrimSpace(s))
				}
			}
		}
		return jsonTextResult(buildScanSecretsPayload(paths, mgr))

	case "gitarc_audit_log":
		limit := 25
		if l, ok := params.Arguments["limit"].(float64); ok && l > 0 {
			limit = int(l)
		}
		repoID, _ := params.Arguments["repoId"].(string)
		logs, scope, hidden := mgr.AuditLogs(repoID, limit)
		snap, err := mgr.Snapshot()
		if err != nil {
			return errResult(err.Error())
		}
		mode := snap.Repo.Mode
		if mode == "" {
			mode = "live"
		}
		res := map[string]any{
			"mode":  mode,
			"scope": scope, // etkin proje kimliği ya da "all"
			"repo":  snap.Repo.Name,
			"count": len(logs),
			"logs":  logs,
		}
		if hidden > 0 {
			res["hiddenUnknownRepo"] = hidden // repoId'si olmayan eski kayıtlar; repoId="all" ile görünür
		}
		return jsonTextResult(res)

	default:
		return errResult(fmt.Sprintf("bilinmeyen araç: %s", params.Name))
	}
}

// buildCleanupCandidatesPayload: temizlenebilir dallar. Aday kuralı arayüzdeki "🧹 Temizle" sayacıyla AYNI ve tektir
// (`gitengine.IsPrunable` → `BranchArc.Prunable`): uzun ömürlü dallar (develop, release/*), kaydedilmemiş değişikliği olan aktif dal,
// uzakta ilerlemiş dallar hariç. Ek koşul: dal GERÇEKTEN var (yerelde ya da uzakta). Yalnızca PR geçmişinden gelen birleşmiş adlar
// (ne yerelde ne uzakta) aday DEĞİLDİR; `historyOnlyIgnored` olarak sayılır ki "N aday" yalan söylemesin.
func buildCleanupCandidatesPayload(snap model.CockpitSnapshot) map[string]any {
	type cleanupCandidate struct {
		Name         string `json:"name"`
		HasLocal     bool   `json:"hasLocal"`
		HasRemote    bool   `json:"hasRemote"`
		OriginGone   bool   `json:"originGone"`
		PRNumber     int    `json:"prNumber,omitempty"`
		PRState      string `json:"prState,omitempty"`
		Proof        string `json:"proof"`
		SuggestedCmd string `json:"suggestedCmd"`
	}
	candidates := []cleanupCandidate{}
	historyOnly := 0
	for _, b := range snap.Branches {
		if !b.HasLocal && !b.HasRemote {
			if b.State == "merged" || (b.PR != nil && b.PR.ReviewState == "MERGED") {
				historyOnly++ // yalnızca PR geçmişi: temizlenecek bir şey yok
			}
			continue
		}
		if !b.Prunable || b.SuggestedCmd == "" {
			continue
		}
		proof := "Uzak dal silinmiş (origin: gone)."
		if b.PR != nil && b.PR.ReviewState == "MERGED" {
			proof = fmt.Sprintf("GitHub PR #%d birleştirildi (MERGED).", b.PR.Number)
		} else if b.State == "merged" {
			proof = "Git geçmişinde main omurgasına birleştirilmiş (merged)."
		}
		prNum, prState := 0, ""
		if b.PR != nil {
			prNum, prState = b.PR.Number, b.PR.ReviewState
		}
		candidates = append(candidates, cleanupCandidate{
			Name: b.Name, HasLocal: b.HasLocal, HasRemote: b.HasRemote, OriginGone: b.OriginGone,
			PRNumber: prNum, PRState: prState, Proof: proof, SuggestedCmd: b.SuggestedCmd,
		})
	}
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}
	summary := fmt.Sprintf("%d adet temizlenebilir birleşmiş/silinmiş dal adayı bulundu.", len(candidates))
	if len(candidates) == 0 {
		summary = "Temizlenebilir dal yok."
	}
	return map[string]any{
		"mode":               mode,
		"repo":               snap.Repo.Name,
		"count":              len(candidates),
		"summary":            summary,
		"candidates":         candidates,
		"historyOnlyIgnored": historyOnly,
		"note":               "MCP üzerinden hiçbir dal doğrudan silinmez. Temizlemek için 'suggestedCmd' komutunu terminalde çalıştırabilir veya GitArc UI üzerindeki 'Temizle' butonunu kullanabilirsiniz. Uzun ömürlü dallar (develop, release/*) ve yalnızca PR geçmişinde kalan adlar aday sayılmaz.",
	}
}

func buildScanSecretsPayload(paths []string, mgr *gitengine.Manager) map[string]any {
	snap, _ := mgr.Snapshot()
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}

	if len(paths) == 0 {
		for _, b := range snap.Branches {
			if b.ID == "local-wip-working-tree" || b.IsHEAD {
				paths = append(paths, b.ChangedFiles...)
			}
		}
	}

	type finding struct {
		Path    string `json:"path"`
		Pattern string `json:"pattern"`
		Reason  string `json:"reason"`
	}

	var findings []finding
	userHomePattern := regexp.MustCompile(`/(?:Users|home)/[a-zA-Z0-9._-]+`)

	repoPath := snap.Repo.Path
	if repoPath == "" {
		repoPath = "."
	}

	for _, p := range paths {
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".env") && !strings.HasSuffix(base, ".example") {
			findings = append(findings, finding{
				Path:    p,
				Pattern: ".env*",
				Reason:  "Ortam değişkenleri dosyası (.env) gizli parolalar/anahtarlar barındırabilir; repoya eklenmemelidir.",
			})
		}
		if strings.HasSuffix(base, ".pem") || strings.HasSuffix(base, ".key") || strings.HasSuffix(base, ".p12") {
			findings = append(findings, finding{
				Path:    p,
				Pattern: "*.pem|*.key|*.p12",
				Reason:  "Kriptografik özel anahtar / sertifika dosyası.",
			})
		}
		if base == ".mcp.json" {
			findings = append(findings, finding{
				Path:    p,
				Pattern: ".mcp.json",
				Reason:  "MCP yapılandırması hassas API anahtarları veya çevre değişkenleri içerebilir.",
			})
		}

		fullPath := filepath.Join(repoPath, p)
		if data, err := os.ReadFile(fullPath); err == nil {
			content := string(data)
			keyMarker := strings.Join([]string{"BEGIN", "PRIVATE", "KEY"}, " ")
			keySSHMarker := strings.Join([]string{"BEGIN", "OPENSSH", "PRIVATE", "KEY"}, " ")
			keyRSAMarker := strings.Join([]string{"BEGIN", "RSA", "PRIVATE", "KEY"}, " ")
			if strings.Contains(content, keySSHMarker) || strings.Contains(content, keyRSAMarker) || strings.Contains(content, keyMarker) {
				findings = append(findings, finding{
					Path:    p,
					Pattern: "BEGIN * PRIVATE KEY",
					Reason:  "Dosya içeriğinde özel SSH/RSA anahtarı tespit edildi!",
				})
			}
			if loc := userHomePattern.FindString(content); loc != "" {
				findings = append(findings, finding{
					Path:    p,
					Pattern: "User Home Path Leak",
					Reason:  fmt.Sprintf("Kişisel kullanıcı dosya yolu sızıntısı (%s). Kodda taşınabilir/göreceli yollar kullanılmalıdır.", loc),
				})
			}
		}
	}

	clean := len(findings) == 0
	summary := "✓ Taranan dosyalarda secret, özel anahtar veya kişisel dosya yolu bulunamadı (Temiz)."
	if !clean {
		summary = fmt.Sprintf("⚠️ DİKKAT: %d adet güvenlik/gizlilik bulgusu tespit edildi!", len(findings))
	}

	return map[string]any{
		"mode":     mode,
		"clean":    clean,
		"scanned":  len(paths),
		"summary":  summary,
		"findings": findings,
	}
}

func buildStatusPayload(snap model.CockpitSnapshot) map[string]any {
	type branchBrief struct {
		Name             string   `json:"name"`
		Hemisphere       string   `json:"hemisphere"`
		State            string   `json:"state"`
		IsHEAD           bool     `json:"isHead"`
		PRNumber         int      `json:"prNumber,omitempty"`
		PRTitle          string   `json:"prTitle,omitempty"`
		PRBody           string   `json:"prBody,omitempty"`
		ReviewState      string   `json:"reviewState,omitempty"`
		CIState          string   `json:"ciState,omitempty"`
		LastCommitMsg    string   `json:"lastCommitMsg,omitempty"`
		DraftPRTitle     string   `json:"draftPrTitle,omitempty"`
		DraftPRBody      string   `json:"draftPrBody,omitempty"`
		AheadMain        int      `json:"aheadMain,omitempty"`
		BehindMain       int      `json:"behindMain,omitempty"`
		UncommittedCount int      `json:"uncommittedCount,omitempty"`
		ChangedFiles     []string `json:"changedFiles,omitempty"`
		OriginGone       bool     `json:"originGone,omitempty"`
		HasConflict      bool     `json:"hasConflict,omitempty"`
		CrossPRCount     int      `json:"crossPrCount,omitempty"`
		SuggestedCmd     string   `json:"suggestedCmd,omitempty"`
	}

	type openPRItem struct {
		Number       int      `json:"number"`
		Branch       string   `json:"branch"`
		Title        string   `json:"title"`
		Body         string   `json:"body,omitempty"`
		Author       string   `json:"author,omitempty"`
		ReviewState  string   `json:"reviewState,omitempty"`
		CIState      string   `json:"ciState,omitempty"`
		URL          string   `json:"url,omitempty"`
		ChangedFiles []string `json:"changedFiles,omitempty"`
	}

	type localBranchItem struct {
		Name             string   `json:"name"`
		IsHEAD           bool     `json:"isHead"`
		AheadMain        int      `json:"aheadMain"`
		BehindMain       int      `json:"behindMain"`
		UncommittedCount int      `json:"uncommittedCount"`
		LastCommitMsg    string   `json:"lastCommitMsg,omitempty"`
		DraftPRTitle     string   `json:"draftPrTitle,omitempty"`
		DraftPRBody      string   `json:"draftPrBody,omitempty"`
		ChangedFiles     []string `json:"changedFiles,omitempty"`
	}

	type mergedPRItem struct {
		Number      int    `json:"number"`
		Branch      string `json:"branch"`
		Title       string `json:"title"`
		Body        string `json:"body,omitempty"`
		Author      string `json:"author,omitempty"`
		ReviewState string `json:"reviewState,omitempty"`
		URL         string `json:"url,omitempty"`
	}

	var briefs []branchBrief
	var openPRs []openPRItem
	var localBranches []localBranchItem
	var recentMergedPRs []mergedPRItem

	openPRCount := 0
	localBranchCount := 0
	mergedCount := 0

	for _, b := range snap.Branches {
		br := branchBrief{
			Name:             b.Name,
			Hemisphere:       b.Hemisphere,
			State:            b.State,
			IsHEAD:           b.IsHEAD,
			LastCommitMsg:    b.LastCommitMsg,
			DraftPRTitle:     b.DraftPRTitle,
			DraftPRBody:      b.DraftPRBody,
			AheadMain:        b.AheadMain,
			BehindMain:       b.BehindMain,
			UncommittedCount: b.UncommittedCount,
			ChangedFiles:     b.ChangedFiles,
			OriginGone:       b.OriginGone,
			HasConflict:      b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved,
			CrossPRCount:     len(b.CrossPR),
			SuggestedCmd:     b.SuggestedCmd,
		}
		if b.PR != nil {
			br.PRNumber = b.PR.Number
			br.PRTitle = b.PR.Title
			br.PRBody = b.PR.Body
			br.ReviewState = b.PR.ReviewState
			if b.PR.CI != nil {
				br.CIState = b.PR.CI.State
			}
			if b.State == "open" && b.PR.Number > 0 && b.PR.ReviewState != "MERGED" {
				openPRCount++
				ciSt := ""
				if b.PR.CI != nil {
					ciSt = b.PR.CI.State
				}
				openPRs = append(openPRs, openPRItem{
					Number:       b.PR.Number,
					Branch:       b.Name,
					Title:        b.PR.Title,
					Body:         b.PR.Body,
					Author:       b.PR.Author,
					ReviewState:  b.PR.ReviewState,
					CIState:      ciSt,
					URL:          b.PR.URL,
					ChangedFiles: b.ChangedFiles,
				})
			} else if b.State == "merged" || b.PR.ReviewState == "MERGED" {
				if len(recentMergedPRs) < 15 {
					recentMergedPRs = append(recentMergedPRs, mergedPRItem{
						Number:      b.PR.Number,
						Branch:      b.Name,
						Title:       b.PR.Title,
						Body:        b.PR.Body,
						Author:      b.PR.Author,
						ReviewState: b.PR.ReviewState,
						URL:         b.PR.URL,
					})
				}
			}
		}
		if b.HasLocal && !strings.HasPrefix(b.ID, "gh-merged-") {
			localBranchCount++
			if b.ID != "local-wip-working-tree" {
				localBranches = append(localBranches, localBranchItem{
					Name:             b.Name,
					IsHEAD:           b.IsHEAD,
					AheadMain:        b.AheadMain,
					BehindMain:       b.BehindMain,
					UncommittedCount: b.UncommittedCount,
					LastCommitMsg:    b.LastCommitMsg,
					DraftPRTitle:     b.DraftPRTitle,
					DraftPRBody:      b.DraftPRBody,
					ChangedFiles:     b.ChangedFiles,
				})
			}
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

	currentBranch := snap.Repo.DefaultBranch
	var dirtyFiles []string
	for _, b := range snap.Branches {
		if b.IsHEAD {
			currentBranch = b.Name
		}
		if b.ID == "local-wip-working-tree" || (b.IsHEAD && b.UncommittedCount > 0) {
			if len(dirtyFiles) == 0 {
				dirtyFiles = b.ChangedFiles
			}
		}
	}

	activeContext := map[string]any{
		"currentBranch":      currentBranch,
		"defaultBranch":      snap.Repo.DefaultBranch,
		"isWorkingTreeClean": snap.Repo.DirtyCount == 0,
		"dirtyCount":         snap.Repo.DirtyCount,
		"dirtyFiles":         dirtyFiles,
		"openPRs":            openPRs,
		"localBranches":      localBranches,
		"recentMergedPRs":    recentMergedPRs,
	}
	if snap.PendingProposal != nil {
		activeContext["pendingProposal"] = snap.PendingProposal
	}

	return map[string]any{
		"mode":             mode,
		"openPRCount":      openPRCount,
		"localBranchCount": localBranchCount,
		"mergedCount":      mergedCount,
		"activeContext":    activeContext,
		"metricsGlossary": map[string]string{
			"openPRCount":      "GitHub üzerinde açık (state=open) ve henüz birleştirilmemiş Pull Request sayısı.",
			"localBranchCount": "Yerel Git deposunda fiziksel olarak var olan dalların sayısı (hasLocal=true).",
			"mergedCount":      "GitHub'da birleşmiş (merged) veya remote dalı silinmiş (origin: gone) temizleme adayı dal sayısı.",
		},
		"summary":     fmt.Sprintf("%s (%s): %d açık PR, %d yerel dal, %d birleşmiş/temizleme adayı", snap.Repo.Name, mode, openPRCount, localBranchCount, mergedCount),
		"repo":        snap.Repo,
		"branches":    briefs,
		"diagnostics": snap.Diagnostics,
		"logs":        snap.Logs,
	}
}

// ---- gitarc_status: kısa özet (varsayılan) ve tek dal ayrıntısı ----
// Tam döküm bir ajanın bağlamına ~14–66 bin karakter yazıyordu (PR gövdeleri, değişen dosyalar, günlükler, tekrarlı listeler).
// Kısa özet, "şu an durum ne?" sorusunun cevabıdır; ayrıntı isteğe bağlıdır (branch / detail=full).

const (
	compactMaxDirtyFiles  = 20
	compactMaxDiagnostics = 10
	compactMaxTitle       = 80
)

func clip(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n-1]) + "…"
}

func buildCompactStatusPayload(snap model.CockpitSnapshot) map[string]any {
	type prLine struct {
		Number       int    `json:"number"`
		Branch       string `json:"branch"`
		Title        string `json:"title,omitempty"`
		CI           string `json:"ci"`
		Review       string `json:"review,omitempty"`
		Ahead        int    `json:"ahead,omitempty"`
		Behind       int    `json:"behind,omitempty"`
		Conflict     bool   `json:"conflict,omitempty"`
		CrossPRs     int    `json:"crossPrs,omitempty"`
		ChangedFiles int    `json:"changedFiles,omitempty"`
	}
	type localLine struct {
		Name        string `json:"name"`
		IsHEAD      bool   `json:"isHead,omitempty"`
		Ahead       int    `json:"ahead,omitempty"`
		Behind      int    `json:"behind,omitempty"`
		Uncommitted int    `json:"uncommitted,omitempty"`
		LastCommit  string `json:"lastCommit,omitempty"`
	}
	type diagLine struct {
		Kind     string `json:"kind"`
		Severity string `json:"severity,omitempty"`
		Branch   string `json:"branch,omitempty"`
		Title    string `json:"title"`
	}
	openPRs := []prLine{}
	locals := []localLine{}
	localCount, mergedCount := 0, 0
	current := snap.Repo.DefaultBranch
	var dirtyFiles []string
	for _, b := range snap.Branches {
		if b.IsHEAD {
			current = b.Name
		}
		if (b.ID == "local-wip-working-tree" || (b.IsHEAD && b.UncommittedCount > 0)) && len(dirtyFiles) == 0 {
			dirtyFiles = b.ChangedFiles
		}
		if b.State == "merged" || b.OriginGone {
			mergedCount++
		}
		if b.State == "open" && b.PR != nil && b.PR.Number > 0 && b.PR.ReviewState != "MERGED" {
			ci := "bilinmiyor"
			if b.PR.CI != nil && b.PR.CI.State != "" {
				ci = b.PR.CI.State
			}
			openPRs = append(openPRs, prLine{
				Number: b.PR.Number, Branch: b.Name, Title: clip(b.PR.Title, compactMaxTitle), CI: ci, Review: b.PR.ReviewState,
				Ahead: b.AheadMain, Behind: b.BehindMain, Conflict: b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved,
				CrossPRs: len(b.CrossPR), ChangedFiles: len(b.ChangedFiles),
			})
		}
		if b.HasLocal && !strings.HasPrefix(b.ID, "gh-merged-") {
			localCount++
			if b.ID != "local-wip-working-tree" && b.State == "open" {
				locals = append(locals, localLine{Name: b.Name, IsHEAD: b.IsHEAD, Ahead: b.AheadMain, Behind: b.BehindMain, Uncommitted: b.UncommittedCount, LastCommit: clip(b.LastCommitMsg, compactMaxTitle)})
			}
		}
	}
	more := 0
	if len(dirtyFiles) > compactMaxDirtyFiles {
		more = len(dirtyFiles) - compactMaxDirtyFiles
		dirtyFiles = dirtyFiles[:compactMaxDirtyFiles]
	}
	diags := []diagLine{}
	for i, d := range snap.Diagnostics {
		if i >= compactMaxDiagnostics {
			break
		}
		diags = append(diags, diagLine{Kind: d.Kind, Severity: d.Severity, Branch: d.BranchName, Title: clip(d.Title, 120)})
	}
	mode := snap.Repo.Mode
	if mode == "" {
		mode = "live"
	}
	r := snap.Repo
	repo := map[string]any{
		"name": r.Name, "defaultBranch": r.DefaultBranch, "ahead": r.Ahead, "behind": r.Behind, "dirtyCount": r.DirtyCount,
		"branchProtected": r.BranchProtected, "protectionState": r.ProtectionState, "strictUpToDate": r.StrictUpToDate,
	}
	if r.MainCI != nil {
		repo["mainCI"] = map[string]string{"state": r.MainCI.State, "sha": r.MainCI.SHA, "workflow": r.MainCI.Workflow}
	}
	if r.QueueRun != nil {
		repo["queueRun"] = r.QueueRun
	}
	active := map[string]any{
		"currentBranch": current, "defaultBranch": r.DefaultBranch, "isWorkingTreeClean": r.DirtyCount == 0, "dirtyCount": r.DirtyCount,
		"dirtyFiles": dirtyFiles, "openPRs": openPRs, "localBranches": locals,
	}
	if more > 0 {
		active["dirtyFilesMore"] = more
	}
	if snap.PendingProposal != nil {
		active["pendingProposal"] = map[string]any{"id": snap.PendingProposal.ID, "branch": snap.PendingProposal.BranchName, "title": clip(snap.PendingProposal.Message, compactMaxTitle)}
	}
	return map[string]any{
		"mode":             mode,
		"detail":           "compact",
		"summary":          fmt.Sprintf("%s (%s): %d açık PR, %d yerel dal, %d birleşmiş/temizleme adayı", r.Name, mode, len(openPRs), localCount, mergedCount),
		"openPRCount":      len(openPRs),
		"localBranchCount": localCount,
		"mergedCount":      mergedCount,
		"repo":             repo,
		"activeContext":    active,
		"diagnostics":      diags,
		"diagnosticsTotal": len(snap.Diagnostics),
		"next":             "Kısa özet. Bir dalın ayrıntısı (PR gövdesi, değişen dosyalar, taslak PR, önerilen komut) için gitarc_status'u branch=<ad> ile çağırın; tam döküm için detail=full.",
	}
}

// buildBranchDetailPayload: tek dalın tam bilgisi (ad ya da kimlik; aynı adlı birden çok kayıtta açık olan tercih edilir).
func buildBranchDetailPayload(snap model.CockpitSnapshot, name string) (map[string]any, bool) {
	var pick *model.BranchArc
	for i := range snap.Branches {
		b := &snap.Branches[i]
		if b.ID == name || b.Name == name {
			if pick == nil || (b.State == "open" && pick.State != "open") {
				pick = b
			}
		}
	}
	if pick == nil {
		return nil, false
	}
	b := *pick
	diags := []model.DiagnosticItem{}
	for _, d := range snap.Diagnostics {
		if d.BranchID == b.ID || d.BranchName == b.Name {
			diags = append(diags, d)
		}
	}
	out := map[string]any{
		"detail": "branch", "name": b.Name, "id": b.ID, "state": b.State, "hemisphere": b.Hemisphere, "isHead": b.IsHEAD,
		"hasLocal": b.HasLocal, "hasRemote": b.HasRemote, "originGone": b.OriginGone,
		"aheadMain": b.AheadMain, "behindMain": b.BehindMain, "uncommittedCount": b.UncommittedCount,
		"lastCommitMsg": b.LastCommitMsg, "changedFiles": b.ChangedFiles, "draftPrTitle": b.DraftPRTitle, "draftPrBody": b.DraftPRBody,
		"suggestedCmd": b.SuggestedCmd, "hasConflict": b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved,
		"crossPR": b.CrossPR, "diagnostics": diags,
	}
	if b.Conflict != nil && len(b.Conflict.Files) > 0 {
		out["conflictFiles"] = b.Conflict.Files
	}
	if b.PR != nil {
		pr := map[string]any{"number": b.PR.Number, "title": b.PR.Title, "body": b.PR.Body, "author": b.PR.Author, "url": b.PR.URL, "reviewState": b.PR.ReviewState}
		if b.PR.CI != nil {
			pr["ci"] = map[string]any{"state": b.PR.CI.State, "workflow": b.PR.CI.Workflow, "failedStep": b.PR.CI.FailedStep}
		}
		out["pr"] = pr
	}
	return out, true
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
