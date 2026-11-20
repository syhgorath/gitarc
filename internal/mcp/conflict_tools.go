package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"sort"
	"strings"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

// Çakışma araçları (planla → [izinli] çöz).
//
// Neden izne bağlı: çözüm yalnızca bir dosya düzenlemesi değildir. Yerel dalı yeniden yazar (aktif dalsa
// `git reset --hard`), dalın uzaktaki hâline `git push` yapar (açık bir PR'ı değiştirir) ve `ours`/`theirs` stratejileri
// bir tarafın değişikliğini ATAR. Bu yüzden:
//   - gitarc_conflict_plan SALT OKUNURDUR (hangi dosyalar, hangi stratejiler, ne risk).
//   - gitarc_resolve_conflict varsayılan KAPALIDIR: yalnızca `GITARC_MCP_ALLOW_RESOLVE=1` ile çalışır.
//   - Açıkken bile plandan alınmış `confirm` parmak iziyle çalışır; çakışma içeriği değiştiyse reddedilir.
//   - Sonuç dürüsttür: çakışma gerçekte kalktıysa `resolved: true`; yoksa false ve nedeni.
const envAllowResolve = "GITARC_MCP_ALLOW_RESOLVE"

func resolveApplyEnabled() bool {
	v := strings.TrimSpace(os.Getenv(envAllowResolve))
	return v == "1" || strings.EqualFold(v, "true")
}

// conflictFingerprint: dal + taban + çakışan her dosyanın yolu ve içeriği. Çakışma değiştiyse parmak izi değişir.
func conflictFingerprint(p model.ConflictStudioPayload) string {
	h := sha256.New()
	h.Write([]byte(p.BranchName + "\x00" + p.BaseRef + "\x00"))
	files := append([]model.ConflictFileDiff(nil), p.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		h.Write([]byte(f.Path + "\x00" + f.ConflictHunks + "\x00"))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

const maxHunkChars = 4000

func conflictToolDefs() []toolDefinition {
	branchProp := map[string]any{"type": "string", "description": "Dal kimliği (ID) ya da dal adı."}
	return []toolDefinition{
		{
			Name:        "gitarc_conflict_plan",
			Description: "Bir daldaki merge çakışmasını SALT OKUNUR inceler: çakışan dosyalar ve çakışma metni, önerilen strateji ve riskleri, ve çözüm için gereken fingerprint. Hiçbir şeyi değiştirmez. Çakışma yoksa bunu söyler (hata değil).",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"branchId": branchProp},
				"required":   []string{"branchId"},
			},
		},
	}
}

func callConflictTool(name string, args map[string]any, mgr *gitengine.Manager) (toolResult, bool) {
	switch name {
	case "gitarc_conflict_plan":
		branchID, _ := args["branchId"].(string)
		if strings.TrimSpace(branchID) == "" {
			return errResult("branchId zorunludur"), true
		}
		plan, err := mgr.InspectConflictStudio(strings.TrimSpace(branchID))
		if err != nil {
			// Çakışma bulunamaması bir arıza değil, bir sonuçtur: ajan "çözülecek bir şey yok" demeli.
			return jsonTextResult(map[string]any{
				"mode":        modeOf(mgr),
				"branchId":    branchID,
				"hasConflict": false,
				"note":        err.Error(),
			}), true
		}
		type fileOut struct {
			Path      string `json:"path"`
			StartLine int    `json:"startLine"`
			EndLine   int    `json:"endLine"`
			Hunks     string `json:"conflictHunks"`
			Truncated bool   `json:"truncated,omitempty"`
		}
		files := make([]fileOut, 0, len(plan.Files))
		for _, f := range plan.Files {
			o := fileOut{Path: f.Path, StartLine: f.StartLine, EndLine: f.EndLine, Hunks: f.ConflictHunks}
			if len(o.Hunks) > maxHunkChars {
				o.Hunks, o.Truncated = o.Hunks[:maxHunkChars], true
			}
			files = append(files, o)
		}
		next := "Planı kullanıcıya gösterin. Çözmek için MCP sunucusu " + envAllowResolve + "=1 ile başlatılmalı; ardından gitarc_resolve_conflict'ı bu fingerprint'i confirm vererek çağırın."
		if resolveApplyEnabled() {
			next = "Planı ve stratejinin sonucunu kullanıcıya gösterip onayını alın; ardından gitarc_resolve_conflict'ı fingerprint'i confirm vererek çağırın."
		}
		return jsonTextResult(map[string]any{
			"mode":                 modeOf(mgr),
			"hasConflict":          true,
			"branchId":             plan.BranchID,
			"branchName":           plan.BranchName,
			"baseRef":              plan.BaseRef,
			"recommended":          plan.Recommended,
			"recommendationReason": plan.RecommendationReason,
			"strategies":           plan.Strategies,
			"files":                files,
			"fingerprint":          conflictFingerprint(plan),
			"resolveEnabled":       resolveApplyEnabled(),
			"warnings": []string{
				"ours: dalın (PR'ın) bu dosyalardaki değişikliklerini ATAR; theirs: tabandaki (main) değişiklikleri atar.",
				"union iki tarafı alt alta koyar; kod dosyalarında çift tanım/derleme hatası riski vardır (go.mod varsa go vet ile doğrulanır).",
				"Çözüm dalın uzaktaki hâline push edilir (açık bir PR'ı değiştirir) ve yerel dalı günceller.",
			},
			"next": next,
		}), true

	case "gitarc_resolve_conflict":
		return callResolveConflict(args, mgr), true
	}
	return toolResult{}, false
}

func callResolveConflict(args map[string]any, mgr *gitengine.Manager) toolResult {
	if !resolveApplyEnabled() {
		return errResult("Çakışma çözümü MCP'de kapalı: çözüm yerel dalı yeniden yazar ve dalın uzaktaki hâline push eder (açık bir PR'ı değiştirir). Kullanıcı MCP sunucusunu " + envAllowResolve + "=1 ile başlatırsa açılır; ya da çözüm GitArc arayüzündeki Çakışma Stüdyosu'ndan yapılabilir. Önce gitarc_conflict_plan ile durumu görebilirsiniz.")
	}
	branchID, _ := args["branchId"].(string)
	strategy, _ := args["strategy"].(string)
	confirm, _ := args["confirm"].(string)
	branchID, strategy, confirm = strings.TrimSpace(branchID), strings.TrimSpace(strategy), strings.TrimSpace(confirm)
	if branchID == "" || strategy == "" || confirm == "" {
		return errResult("branchId, strategy ve confirm (gitarc_conflict_plan fingerprint'i) zorunludur")
	}
	switch strategy {
	case "union", "ours", "theirs", "custom":
	default:
		return errResult("strategy union, ours, theirs veya custom olmalıdır")
	}
	var customFiles map[string]string
	if rawMap, ok := args["customFiles"].(map[string]any); ok && len(rawMap) > 0 {
		customFiles = make(map[string]string, len(rawMap))
		for k, v := range rawMap {
			if s, ok := v.(string); ok {
				customFiles[k] = s
			}
		}
	}
	if strategy == "custom" && len(customFiles) == 0 {
		return errResult("strategy=custom için customFiles (dosya yolu -> çözülmüş içerik) zorunludur")
	}

	// Çözümden ÖNCE güncel plan: çakışma yoksa hiçbir şey yapılmaz; içerik değiştiyse onay geçersizdir.
	plan, err := mgr.InspectConflictStudio(branchID)
	if err != nil {
		return errResult("Çözülecek aktif bir çakışma bulunamadı; hiçbir şey değiştirilmedi: " + err.Error())
	}
	if confirm != conflictFingerprint(plan) {
		return errResult("Çakışma planı değişti veya onaylanmadı (fingerprint uyuşmuyor); gitarc_conflict_plan'ı yeniden çağırıp kullanıcıya gösterin. Hiçbir şey değiştirilmedi.")
	}

	snap, err := mgr.ResolveBranchConflict(branchID, strategy, customFiles, true)
	if err != nil {
		return errResult(err.Error())
	}

	// Dürüst sonuç: çakışma gerçekten kalktı mı?
	resolved, pushed := true, false
	for _, b := range snap.Branches {
		if b.ID == plan.BranchID || b.Name == plan.BranchName {
			if b.Conflict != nil && b.Conflict.HasConflict {
				resolved = false
			}
			pushed = b.HasRemote
			break
		}
	}
	note := "Çözüm izole worktree'de uygulandı; go.mod varsa go vet ile doğrulandı."
	if !resolved {
		note = "Çözüm uygulandı ama dal hâlâ çakışma gösteriyor; GitArc'ta durumu kontrol edin."
	}
	return jsonTextResult(map[string]any{
		"mode":           snap.Repo.Mode,
		"resolved":       resolved,
		"branchId":       plan.BranchID,
		"branchName":     plan.BranchName,
		"strategy":       strategy,
		"pushedToRemote": pushed,
		"note":           note,
	})
}
