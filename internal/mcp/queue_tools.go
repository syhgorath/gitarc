package mcp

import (
	"os"
	"strings"

	"gitarc/internal/gitengine"
)

// Merge kuyruğu araçları. gitarc_queue_plan ve gitarc_update_branch_plan SALT OKUNURDUR. gitarc_update_branch uzak PR dalına
// PUSH eder (açık bir PR'ı değiştirir, CI'ı yeniden başlatır): geri alınması zor olduğu için varsayılan KAPALIDIR
// (GITARC_MCP_ALLOW_UPDATE=1), plandan alınmış parmak izi ister, çakışmada hiçbir şey yapmaz, force-push etmez.
const envAllowUpdate = "GITARC_MCP_ALLOW_UPDATE"

func updateApplyEnabled() bool {
	v := strings.TrimSpace(os.Getenv(envAllowUpdate))
	return v == "1" || strings.EqualFold(v, "true")
}

func queueToolDefs() []toolDefinition {
	branchProp := map[string]any{"type": "string", "description": "Dal kimliği (ID) ya da dal adı."}
	return []toolDefinition{
		{
			Name: "gitarc_update_branch_plan",
			Description: "Bir PR dalını ana dala göre güncellemenin (ana dalı dalın İÇİNE merge etmek) planı. SALT OKUNUR: hiçbir şey yazmaz. status: ready (temiz merge edilebilir) | up_to_date | conflict (çakışan dosyalar; güncelleme yapılmaz, Çakışma Stüdyosu gerekir) | blocked (neden reason'da; örn. dal uzakta yok, fork PR'ı). " +
				"Dönen fingerprint (dalın ve ana dalın ucu) uygulamada confirm olarak verilir; uzak dal ya da ana dal ilerlediyse değişir.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"branchId": branchProp}, "required": []string{"branchId"}},
		},
		{
			Name: "gitarc_update_branch",
			Description: "PR dalını ana dala göre günceller: ana dalı dalın içine merge eder (merge commit; rebase/force-push YOK) ve UZAK PR dalına PUSH eder (açık bir PR'ı değiştirir, CI'ı yeniden başlatır). VARSAYILAN OLARAK KAPALIDIR: yalnızca MCP sunucusu " + envAllowUpdate + "=1 ile başlatıldıysa çalışır. " +
				"Önce gitarc_update_branch_plan çağırıp kullanıcıya göstermeli ve fingerprint'i confirm olarak vermelisiniz; uzak dal planla uygulama arasında değiştiyse ya da çakışma varsa HİÇBİR ŞEY yapılmaz. Kullanıcının çalışma ağacına dokunulmaz (izole worktree).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branchId": branchProp,
					"confirm":  map[string]any{"type": "string", "description": "gitarc_update_branch_plan çıktısındaki fingerprint (aynen)."},
				},
				"required": []string{"branchId", "confirm"},
			},
		},
		{
			Name: "gitarc_queue_plan",
			Description: "Merge kuyruğu planı (SALT OKUNUR; hiçbir şey birleştirmez/güncellemez/push etmez). Seçilen dalları/PR'ları verilen SIRAYLA main'e birleştirilmiş gibi bellekte simüle eder: her dal, kuyruktaki ÖNCEKİLER birleşmiş olarak denenir " +
				"(tek başına temiz bir dal, bir öncekinden sonra çakışabilir). Dönenler: her dal için status (clean | conflict | unknown), çakışan dosyalar, çakışmaya sebep olan önceki PR'lar, " +
				"main ile tek başına çakışıp çakışmadığı, CI/inceleme durumu; kuyruğun nerede durduğu (stopsAt), kaç dalın sorunsuz birleşebildiği (mergeable) ve tahmini CI tur sayısı. " +
				"CI durumu bilinmiyorsa yeşil varsayılmaz. Plan metin düzeyindeki çakışmayı gösterir; anlamsal uyumsuzluğu güncellenmiş dalın CI'ı yakalar. branchIds boşsa tüm açık PR'lar (PR numarasına göre).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branchIds": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Kuyruğa girecek dal kimlikleri ya da adları, birleşme SIRASIYLA. Boşsa tüm açık PR'lar PR numarasına göre.",
					},
				},
			},
		},
	}
}

func callQueueTool(name string, args map[string]any, mgr *gitengine.Manager) (toolResult, bool) {
	switch name {
	case "gitarc_update_branch_plan":
		id, _ := args["branchId"].(string)
		if strings.TrimSpace(id) == "" {
			return errResult("branchId zorunludur"), true
		}
		plan, err := mgr.BranchUpdatePlan(strings.TrimSpace(id))
		if err != nil {
			return errResult(err.Error()), true
		}
		next := "Planı kullanıcıya gösterin. Uygulamak için MCP sunucusu " + envAllowUpdate + "=1 ile başlatılmalı; ardından gitarc_update_branch'i bu fingerprint ile çağırın."
		if updateApplyEnabled() {
			next = "Planı kullanıcıya gösterip onayını alın; ardından gitarc_update_branch'i fingerprint'i confirm vererek çağırın. Bu, uzak PR dalına push eder."
		}
		switch plan.Status {
		case "conflict":
			next = "Dal ana dalla çakışıyor; güncelleme yapılamaz. Önce gitarc_conflict_plan ile çakışmayı inceleyin."
		case "up_to_date":
			next = "Dal zaten güncel; yapılacak bir şey yok."
		case "blocked":
			next = "Bu dal güncellenemez (reason'a bakın); kullanıcıya nedenini aktarın."
		}
		return jsonTextResult(map[string]any{"plan": plan, "applyEnabled": updateApplyEnabled(), "next": next}), true
	case "gitarc_update_branch":
		if !updateApplyEnabled() {
			return errResult("Dal güncelleme MCP'de kapalı: uzak PR dalına push eder (açık bir PR'ı değiştirir, CI'ı yeniden başlatır). Kullanıcı MCP sunucusunu " + envAllowUpdate + "=1 ile başlatırsa açılır; ya da güncelleme GitArc arayüzündeki 🚦 Kuyruk penceresinden yapılabilir. Önce gitarc_update_branch_plan ile durumu görebilirsiniz."), true
		}
		id, _ := args["branchId"].(string)
		confirm, _ := args["confirm"].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(confirm) == "" {
			return errResult("branchId ve confirm (gitarc_update_branch_plan fingerprint'i) zorunludur"), true
		}
		snap, err := mgr.UpdateBranchFromBase(strings.TrimSpace(id), strings.TrimSpace(confirm))
		if err != nil {
			return errResult(err.Error() + " (Hiçbir şey değiştirilmediyse mesaj bunu söyler; emin olmak için gitarc_update_branch_plan'ı yeniden çağırın.)"), true
		}
		return jsonTextResult(map[string]any{"mode": snap.Repo.Mode, "updated": true, "branchId": id, "note": "Ana dal dalın içine merge edildi ve uzak PR dalına push edildi; CI yeniden başlar."}), true
	case "gitarc_queue_plan":
	default:
		return toolResult{}, false
	}
	var ids []string
	for _, id := range stringList(args["branchIds"]) {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	plan, err := mgr.QueuePlan(ids)
	if err != nil {
		return errResult(err.Error()), true
	}
	return jsonTextResult(plan), true
}
