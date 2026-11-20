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

// gitarc_queue_step_plan SALT OKUNURDUR. gitarc_queue_step kuyruğun TEK adımını uygular (dalı güncelle = uzak PR dalına push,
// ya da PR'ı main'e birleştir): geri alınması zor olduğu için varsayılan KAPALIDIR (GITARC_MCP_ALLOW_QUEUE=1).
const envAllowQueue = "GITARC_MCP_ALLOW_QUEUE"

// gitarc_queue_run_plan ve gitarc_queue_run_status SALT OKUNURDUR; gitarc_queue_run_stop her zaman serbesttir (güvenliği artırır).
// gitarc_queue_run kuyruğu OTOMATİK çalıştırır (PR'ları sırayla günceller + birleştirir): geri alınması zor olduğu için
// varsayılan KAPALIDIR (GITARC_MCP_ALLOW_QUEUE_RUN=1) ve plan parmak izi ister. Koşu MCP sunucusunun sürecinde çalışır.
const envAllowQueueRun = "GITARC_MCP_ALLOW_QUEUE_RUN"

func queueRunEnabled() bool {
	v := strings.TrimSpace(os.Getenv(envAllowQueueRun))
	return v == "1" || strings.EqualFold(v, "true")
}

func queueStepEnabled() bool {
	v := strings.TrimSpace(os.Getenv(envAllowQueue))
	return v == "1" || strings.EqualFold(v, "true")
}

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
			Name: "gitarc_queue_step_plan",
			Description: "Merge kuyruğunun BİR dalı için sıradaki adımın planı. SALT OKUNUR: hiçbir şey yazmaz. action: update (main dalın içine merge edilip uzak PR dalına push edilir) | merge (PR birleştirilebilir: main'e göre güncel + CI yeşil) | wait (CI sürüyor ya da BİLİNMİYOR; yeşil varsayılmaz) | blocked (çakışma, kırmızı CI, inceleme engeli, taslak PR...; reason'a bakın). " +
				"update/merge için fingerprint (dal ucu + main ucu + eylem + CI) döner; uygulamada confirm olarak verilir.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"branchId": branchProp}, "required": []string{"branchId"}},
		},
		{
			Name: "gitarc_queue_step",
			Description: "Kuyruğun TEK adımını uygular: planın action'ı update ise ana dalı dalın içine merge edip UZAK PR dalına push eder (CI yeniden başlar); merge ise PR'ı `--match-head-commit` ile main'e birleştirir (koruma atlatma yok). VARSAYILAN OLARAK KAPALIDIR: yalnızca MCP sunucusu " + envAllowQueue + "=1 ile başlatıldıysa çalışır. " +
				"Önce gitarc_queue_step_plan çağırıp kullanıcıya göstermeli ve fingerprint'i confirm olarak vermelisiniz; durum değiştiyse (dal/main ilerledi, CI değişti) HİÇBİR ŞEY yapılmaz. Her çağrı tek adımdır; sonraki adım için planı yeniden alın.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branchId": branchProp,
					"confirm":  map[string]any{"type": "string", "description": "gitarc_queue_step_plan çıktısındaki fingerprint (aynen)."},
				},
				"required": []string{"branchId", "confirm"},
			},
		},
		{
			Name: "gitarc_queue_run_plan",
			Description: "Kuyruğu OTOMATİK çalıştırmanın planı. SALT OKUNUR: hiçbir şey başlatmaz. Seçilen PR'ların (boşsa tüm açık PR'lar) yalnızca SORUNSUZ ÖNEKİ koşuya girer (items); ilk sorunlu PR ve sonrakiler excluded'dadır. " +
				"startable true ise fingerprint (sıralı dal + uç listesi + ana dal ucu) gitarc_queue_run'a confirm olarak verilir. Koşu her PR için: gerekirse dalı main'e göre güncelle → CI yeşil olana kadar bekle (zaman aşımı timeoutMinutes) → birleştir; ilk sorunda durur.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"branchIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Birleşme SIRASIYLA dal kimlikleri/adları. Boşsa tüm açık PR'lar PR numarasına göre."}}},
		},
		{
			Name: "gitarc_queue_run",
			Description: "Kuyruğu OTOMATİK çalıştırır (arka planda): PR'ları sırayla işler; her biri için dalı main'e göre günceller (uzak PR dalına push), CI yeşil olana kadar bekler, PR'ı `--match-head-commit` ile main'e birleştirir (koruma atlatma yok). İlk sorunda (çakışma, kırmızı CI, inceleme engeli, dışarıdan değişen dal ucu, zaman aşımı) DURUR. " +
				"VARSAYILAN OLARAK KAPALIDIR: yalnızca MCP sunucusu " + envAllowQueueRun + "=1 ile başlatıldıysa çalışır. Önce gitarc_queue_run_plan çağırıp kullanıcıya göstermeli ve fingerprint'i confirm olarak vermelisiniz; durum değiştiyse HİÇBİR ŞEY başlamaz. Başladıktan sonra gitarc_queue_run_status ile izleyin; gitarc_queue_run_stop ile durdurun.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"branchIds": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "gitarc_queue_run_plan'a verdiğiniz AYNI sıra."},
					"confirm":   map[string]any{"type": "string", "description": "gitarc_queue_run_plan çıktısındaki fingerprint (aynen)."},
				},
				"required": []string{"confirm"},
			},
		},
		{
			Name:        "gitarc_queue_run_status",
			Description: "Etkin projenin son kuyruk koşusunun durumu (SALT OKUNUR): status (running | completed | stopped | blocked | timeout | interrupted), kaç PR birleşti, durma nedeni ve olay günlüğü. Koşu yoksa run null. interrupted: uygulama kapanmıştı; koşu kendiliğinden devam ETMEZ.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "gitarc_queue_run_stop",
			Description: "Çalışan kuyruk koşusunu durdurur. Her zaman serbesttir (güvenliği artırır): süren adım tamamlanır, sonrası yapılmaz. Koşu yoksa hata döner.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
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

func trimmedList(v any) []string {
	var out []string
	for _, id := range stringList(v) {
		if id = strings.TrimSpace(id); id != "" {
			out = append(out, id)
		}
	}
	return out
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
	case "gitarc_queue_step_plan":
		id, _ := args["branchId"].(string)
		if strings.TrimSpace(id) == "" {
			return errResult("branchId zorunludur"), true
		}
		plan, err := mgr.QueueStepPlan(strings.TrimSpace(id))
		if err != nil {
			return errResult(err.Error()), true
		}
		next := "Planı kullanıcıya gösterin. Uygulamak için MCP sunucusu " + envAllowQueue + "=1 ile başlatılmalı; ardından gitarc_queue_step'i bu fingerprint ile çağırın."
		if queueStepEnabled() {
			next = "Planı kullanıcıya gösterip onayını alın; ardından gitarc_queue_step'i fingerprint'i confirm vererek çağırın."
		}
		switch plan.Action {
		case "wait":
			next = "Şimdilik yapılacak bir şey yok (CI sürüyor ya da bilinmiyor). CI bitince planı yeniden alın; yeşil varsaymayın."
		case "blocked":
			next = "Bu adım engelli (reason'a bakın); kullanıcıya nedenini aktarın. Çakışmaysa gitarc_conflict_plan ile inceleyin."
		}
		return jsonTextResult(map[string]any{"plan": plan, "applyEnabled": queueStepEnabled(), "next": next}), true
	case "gitarc_queue_step":
		if !queueStepEnabled() {
			return errResult("Kuyruk adımı MCP'de kapalı: dalı günceller (uzak PR dalına push) ya da PR'ı main'e birleştirir; geri alınması zor. Kullanıcı MCP sunucusunu " + envAllowQueue + "=1 ile başlatırsa açılır; ya da adım GitArc arayüzündeki 🚦 Kuyruk penceresinden uygulanabilir. Önce gitarc_queue_step_plan ile durumu görebilirsiniz."), true
		}
		id, _ := args["branchId"].(string)
		confirm, _ := args["confirm"].(string)
		if strings.TrimSpace(id) == "" || strings.TrimSpace(confirm) == "" {
			return errResult("branchId ve confirm (gitarc_queue_step_plan fingerprint'i) zorunludur"), true
		}
		snap, err := mgr.QueueStepApply(strings.TrimSpace(id), strings.TrimSpace(confirm))
		if err != nil {
			return errResult(err.Error() + " (Hiçbir şey değiştirilmediyse mesaj bunu söyler; emin olmak için gitarc_queue_step_plan'ı yeniden çağırın.)"), true
		}
		return jsonTextResult(map[string]any{"mode": snap.Repo.Mode, "applied": true, "branchId": id, "note": "Tek adım uygulandı. Sonraki adım için gitarc_queue_step_plan'ı yeniden çağırın."}), true
	case "gitarc_queue_run_plan":
		plan, err := mgr.QueueRunPlan(trimmedList(args["branchIds"]))
		if err != nil {
			return errResult(err.Error()), true
		}
		next := "Planı kullanıcıya gösterin. Başlatmak için MCP sunucusu " + envAllowQueueRun + "=1 ile başlatılmalı; ardından gitarc_queue_run'ı bu fingerprint ve AYNI branchIds ile çağırın."
		if queueRunEnabled() {
			next = "Planı kullanıcıya gösterip onayını alın; ardından gitarc_queue_run'ı fingerprint'i confirm vererek ve AYNI branchIds ile çağırın."
		}
		if !plan.Startable {
			next = "Koşu başlatılamaz (reason'a bakın); kullanıcıya nedenini aktarın."
		}
		return jsonTextResult(map[string]any{"plan": plan, "applyEnabled": queueRunEnabled(), "next": next}), true
	case "gitarc_queue_run":
		if !queueRunEnabled() {
			return errResult("Otomatik kuyruk koşusu MCP'de kapalı: PR'ları sırayla günceller (uzak dallara push) ve birleştirir; geri alınması zor. Kullanıcı MCP sunucusunu " + envAllowQueueRun + "=1 ile başlatırsa açılır; ya da koşu GitArc arayüzündeki 🚦 Kuyruk penceresinden başlatılabilir. Önce gitarc_queue_run_plan ile planı görebilirsiniz."), true
		}
		confirm, _ := args["confirm"].(string)
		if strings.TrimSpace(confirm) == "" {
			return errResult("confirm (gitarc_queue_run_plan fingerprint'i) zorunludur"), true
		}
		run, err := mgr.StartQueueRun(trimmedList(args["branchIds"]), strings.TrimSpace(confirm))
		if err != nil {
			return errResult(err.Error() + " (Hiçbir şey başlatılmadıysa mesaj bunu söyler; emin olmak için gitarc_queue_run_status'u çağırın.)"), true
		}
		return jsonTextResult(map[string]any{"started": true, "run": run, "note": "Koşu arka planda sürüyor; gitarc_queue_run_status ile izleyin, gitarc_queue_run_stop ile durdurun."}), true
	case "gitarc_queue_run_status":
		run, err := mgr.QueueRunStatus()
		if err != nil {
			return errResult(err.Error()), true
		}
		return jsonTextResult(map[string]any{"run": run}), true
	case "gitarc_queue_run_stop":
		run, err := mgr.StopQueueRun()
		if err != nil {
			return errResult(err.Error()), true
		}
		return jsonTextResult(map[string]any{"stopped": true, "run": run}), true
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
