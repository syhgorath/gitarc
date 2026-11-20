package mcp

import (
	"strings"

	"gitarc/internal/gitengine"
)

// Merge kuyruğu araçları. 1. aşama YALNIZCA planlar: hiçbir şey birleştirmez, dalı güncellemez, push etmez.
// (Dalı güncelleyen/birleştiren araçlar sonraki aşamalardadır ve geri alınması zor olduğu için varsayılan KAPALI olacak.)

func queueToolDefs() []toolDefinition {
	return []toolDefinition{
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
	if name != "gitarc_queue_plan" {
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
