package mcp

import (
	"os"
	"strings"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

// Dal koruması araçları (oku → planla → [izinli] uygula).
//
// Güvenlik tasarımı:
//   - status ve plan SALT OKUNURDUR; hiçbir ayar değişmez.
//   - apply varsayılan olarak KAPALIDIR: yalnızca sunucu `GITARC_MCP_ALLOW_PROTECTION=1` ile başlatıldıysa çalışır.
//     Dal koruması bir güvenlik ayarıdır; ajana (ya da ajanın okuduğu bir metne gömülü talimata) kendiliğinden bırakılmaz.
//   - apply yalnızca ÖNCEDEN plan'dan alınmış `confirm` parmak iziyle çalışır; plan değiştiyse reddedilir.
//   - Motor yalnızca GÜÇLENDİRİR; bir kuralı gevşetmek ya da silmek bu araçlarla mümkün değildir.
const envAllowProtection = "GITARC_MCP_ALLOW_PROTECTION"

func protectionApplyEnabled() bool {
	v := strings.TrimSpace(os.Getenv(envAllowProtection))
	return v == "1" || strings.EqualFold(v, "true")
}

func protectionToolDefs() []toolDefinition {
	profileSchema := map[string]any{
		"type":        "string",
		"enum":        gitengine.ProtectionProfiles,
		"description": "temel: PR zorunlu + force-push/silme kapalı. ci: temel + dal güncel olmalı + seçilen CI kontrolleri. siki: ci + 1 onay + yöneticiler dahil (tek geliştiricide kendi PR'ını birleştirmeyi engeller).",
	}
	checksSchema := map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": "Zorunlu yapılacak CI kontrol adları (gitarc_protection_status.availableChecks içinden). ci/siki için en az biri seçilmezse CI zorunlu OLMAZ.",
	}
	return []toolDefinition{
		{
			Name:        "gitarc_protection_status",
			Description: "Varsayılan dalın GitHub dal koruması durumunu ve zorunlu yapılabilecek CI kontrol adlarını döndürür. SALT OKUNUR. state: none (korumasız) | active | ruleset (GitArc dokunmaz) | unsupported (planda koruma yok) | unknown (okunamadı/yetki yok; 'korumasız' DEĞİLDİR).",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			Name:        "gitarc_protection_plan",
			Description: "Bir koruma profilinin mevcut duruma göre NE DEĞİŞTİRECEĞİNİ döndürür (changes, notes, fingerprint). SALT OKUNUR; hiçbir şey uygulamaz. Plan yalnızca güçlendirir: mevcut sıkı bir kural gevşetilmez, mevcut zorunlu kontrol silinmez. Kullanıcıya göstermeden uygulamayın.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"profile": profileSchema, "checks": checksSchema},
				"required":   []string{"profile"},
			},
		},
		{
			Name:        "gitarc_protection_apply",
			Description: "Planı uygular. VARSAYILAN OLARAK KAPALIDIR: yalnızca MCP sunucusu GITARC_MCP_ALLOW_PROTECTION=1 ile başlatıldıysa çalışır (güvenlik ayarı; kullanıcı bilerek açar). Önce gitarc_protection_plan çağırıp kullanıcıya göstermeli ve dönen fingerprint'i confirm olarak vermelisiniz; plan değiştiyse reddedilir. Yalnızca güçlendirir, hiçbir kuralı gevşetmez.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"profile": profileSchema,
					"checks":  checksSchema,
					"confirm": map[string]any{"type": "string", "description": "gitarc_protection_plan çıktısındaki fingerprint (aynen)."},
				},
				"required": []string{"profile", "confirm"},
			},
		},
	}
}

func stringList(v any) []string {
	var out []string
	if raw, ok := v.([]any); ok {
		for _, x := range raw {
			if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

func modeOf(mgr *gitengine.Manager) string {
	snap, err := mgr.Snapshot()
	if err != nil || snap.Repo.Mode == "" {
		return "live"
	}
	return snap.Repo.Mode
}

func callProtectionTool(name string, args map[string]any, mgr *gitengine.Manager) (toolResult, bool) {
	switch name {
	case "gitarc_protection_status":
		plan, err := mgr.ProtectionPlan("temel", nil)
		if err != nil {
			return errResult(err.Error()), true
		}
		return jsonTextResult(protectionStatusPayload(modeOf(mgr), plan, protectionApplyEnabled())), true

	case "gitarc_protection_plan":
		profile, _ := args["profile"].(string)
		plan, err := mgr.ProtectionPlan(strings.TrimSpace(profile), stringList(args["checks"]))
		if err != nil {
			return errResult(err.Error()), true
		}
		next := "Planı kullanıcıya gösterin. Uygulamak için MCP sunucusu " + envAllowProtection + "=1 ile başlatılmalı; ardından gitarc_protection_apply'ı bu fingerprint ile çağırın."
		if protectionApplyEnabled() {
			next = "Planı kullanıcıya gösterip onayını alın; ardından gitarc_protection_apply'ı bu fingerprint'i confirm vererek çağırın."
		}
		if !plan.Applicable {
			next = "Bu plan uygulanamaz (notes'a bakın); kullanıcıya nedenini aktarın."
		}
		return jsonTextResult(map[string]any{
			"mode":         modeOf(mgr),
			"plan":         plan,
			"applyEnabled": protectionApplyEnabled(),
			"next":         next,
		}), true

	case "gitarc_protection_apply":
		if !protectionApplyEnabled() {
			return errResult("Dal koruması uygulaması MCP'de kapalı: bu bir güvenlik ayarıdır. Kullanıcı MCP sunucusunu " + envAllowProtection + "=1 ile başlatırsa açılır; ya da koruma GitArc arayüzünden (🛡️ Koruma) uygulanabilir."), true
		}
		profile, _ := args["profile"].(string)
		confirm, _ := args["confirm"].(string)
		if strings.TrimSpace(profile) == "" || strings.TrimSpace(confirm) == "" {
			return errResult("profile ve confirm (gitarc_protection_plan fingerprint'i) zorunludur"), true
		}
		snap, err := mgr.ApplyProtection(strings.TrimSpace(profile), stringList(args["checks"]), strings.TrimSpace(confirm))
		if err != nil {
			return errResult(err.Error()), true
		}
		return jsonTextResult(map[string]any{
			"mode":            snap.Repo.Mode,
			"applied":         true,
			"profile":         profile,
			"branch":          snap.Repo.DefaultBranch,
			"protectionState": snap.Repo.ProtectionState,
			"summary":         snap.Repo.ProtectionSummary,
		}), true
	}
	return toolResult{}, false
}

// firstNote: plan notlarından "okunamadı/yok" ile ilgili ilkini ya da varsayılanı döndürür.
func firstNote(notes []string, def string) string {
	for _, n := range notes {
		if strings.Contains(n, "okunamadı") || strings.Contains(n, "Ruleset") || strings.Contains(n, "planında") {
			return n
		}
	}
	return def
}

// protectionStatusPayload: gitarc_protection_status çıktısı (saf; sınanabilir). "current" yalnızca GERÇEKTEN okunduysa verilir.
func protectionStatusPayload(mode string, plan model.ProtectionPlan, applyEnabled bool) map[string]any {
	res := map[string]any{
		"mode":            mode,
		"branch":          plan.Branch,
		"state":           plan.State,
		"availableChecks": plan.AvailableChecks,
		"notes":           plan.Notes,
		"applyEnabled":    applyEnabled,
	}
	// "current" yalnızca GERÇEKTEN okunduysa verilir (klasik koruma ya da Ruleset kuralları). Okunamadıysa/korumasızsa sıfır değerli
	// bir nesne ("checks: null") "korumasız" gibi okunurdu: bunun yerine neden yazılır.
	switch {
	case plan.State == "unknown":
		res["current"] = nil
		res["reason"] = firstNote(plan.Notes, "Koruma durumu okunamadı (yetki yok ya da gh hatası); bu \"korumasız\" demek değildir.")
	case plan.State == "none":
		res["current"] = nil
		res["reason"] = "Dal korumasız: zorunlu PR/CI kuralı yok."
	case plan.State == "unsupported":
		res["current"] = nil
		res["reason"] = firstNote(plan.Notes, "Bu depo planında dal koruması yok.")
	case plan.State == "ruleset" && plan.Source != "ruleset":
		res["current"] = nil
		res["reason"] = firstNote(plan.Notes, "Dal Ruleset ile korunuyor ama kurallar okunamadı.")
	default:
		res["current"] = plan.Current
		if plan.Source != "" {
			res["source"] = plan.Source // classic | ruleset (ruleset salt okunur: GitArc değiştirmez)
		}
	}
	return res
}
