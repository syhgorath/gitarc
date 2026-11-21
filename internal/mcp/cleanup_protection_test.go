package mcp

import (
	"encoding/json"
	"testing"

	"gitarc/internal/model"
)

func branch(name string, f func(*model.BranchArc)) model.BranchArc {
	b := model.BranchArc{ID: "br-" + name, Name: name, State: "merged"}
	f(&b)
	return b
}

// cleanup_candidates: arayüzdeki 🧹 sayacıyla AYNI kural (Prunable) + dal gerçekten var; yalnızca PR geçmişinden gelen adlar ve uzun ömürlü
// dallar aday DEĞİL. Gerçek hata: 13 "aday"ın 10'u hiçbir yerde olmayan adlardı, `release/*` için `git branch -D` önerilmişti.
func TestCleanupCandidatesOnlyRealPrunableBranches(t *testing.T) {
	merged := &model.PRStatus{Number: 9, ReviewState: "MERGED"}
	snap := model.CockpitSnapshot{
		Repo: model.RepoMeta{Mode: "live", Name: "demo"},
		Branches: []model.BranchArc{
			// uzakta duran, PR'ı MERGED dal → aday
			branch("feat/uzakta", func(b *model.BranchArc) { b.HasRemote, b.PR, b.Prunable, b.SuggestedCmd = true, merged, true, "git push origin --delete feat/uzakta && git fetch --prune origin" }),
			// squash ile birleşmiş, uzakta silinmiş yerel kopya → aday
			branch("feat/squash", func(b *model.BranchArc) { b.HasLocal, b.OriginGone, b.PR, b.Prunable, b.SuggestedCmd = true, true, merged, true, "git branch -D feat/squash && git fetch --prune origin" }),
			// yalnızca PR geçmişi: ne yerelde ne uzakta → aday DEĞİL (yalnızca sayılır)
			branch("feat/gecmis-1", func(b *model.BranchArc) { b.PR, b.Prunable = merged, true }),
			branch("feat/gecmis-2", func(b *model.BranchArc) { b.PR = merged }),
			// uzun ömürlü dal: Prunable false → aday DEĞİL
			branch("release/0.10.0", func(b *model.BranchArc) { b.HasLocal, b.HasRemote, b.OriginGone, b.LongLived, b.PR = true, true, true, true, merged }),
			// açık, birleşmemiş dal
			{ID: "br-acik", Name: "feat/acik", State: "open", HasLocal: true, HasRemote: true},
		},
	}
	p := buildCleanupCandidatesPayload(snap)
	raw, _ := json.Marshal(p)
	var got struct {
		Mode, Repo         string
		Count              int
		Candidates         []struct{ Name, SuggestedCmd string }
		HistoryOnlyIgnored int
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Count != 2 || len(got.Candidates) != 2 {
		t.Fatalf("yalnızca gerçekten var olan 2 aday beklenirdi: %+v", got)
	}
	for _, c := range got.Candidates {
		if c.Name == "release/0.10.0" || c.Name == "feat/gecmis-1" || c.Name == "feat/gecmis-2" {
			t.Fatalf("aday olmamalı: %s", c.Name)
		}
		if c.SuggestedCmd == "" {
			t.Fatalf("var olan dal için silme komutu önerilmeli: %s", c.Name)
		}
	}
	if got.HistoryOnlyIgnored != 2 || got.Mode != "live" {
		t.Fatalf("PR geçmişi adları sayılmalı ve mode yazılmalı: %+v", got)
	}
}

func TestCleanupCandidatesEmptyIsZeroAndNotNull(t *testing.T) {
	p := buildCleanupCandidatesPayload(model.CockpitSnapshot{Repo: model.RepoMeta{Mode: "live"}})
	if p["count"].(int) != 0 {
		t.Fatalf("aday yoksa count=0: %v", p["count"])
	}
	raw, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if arr, ok := m["candidates"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("candidates null değil boş dizi olmalı: %v", m["candidates"])
	}
}

// protection_status: okunamayan koruma "korumasız" (sıfır değerli current, checks: null) gibi görünmemeli; neden yazılmalı.
func TestProtectionStatusPayloadHonestStates(t *testing.T) {
	read := model.ProtectionSettings{RequirePR: true, Checks: []string{"web", "go"}, StrictUpToDate: true}
	cases := []struct {
		name       string
		plan       model.ProtectionPlan
		wantCur    bool
		wantReason bool
		wantSource string
	}{
		{"classic", model.ProtectionPlan{State: "active", Source: "classic", Current: read}, true, false, "classic"},
		{"ruleset okundu", model.ProtectionPlan{State: "ruleset", Source: "ruleset", Current: read}, true, false, "ruleset"},
		{"ruleset okunamadı", model.ProtectionPlan{State: "ruleset", Notes: []string{"Ruleset kuralları okunamadı"}}, false, true, ""},
		{"unknown (yetki yok)", model.ProtectionPlan{State: "unknown", Notes: []string{"Koruma durumu okunamadı (yetki yok)"}}, false, true, ""},
		{"korumasız", model.ProtectionPlan{State: "none"}, false, true, ""},
	}
	for _, c := range cases {
		p := protectionStatusPayload("live", c.plan, false)
		cur, has := p["current"]
		if c.wantCur && (!has || cur == nil) {
			t.Errorf("%s: current dolu olmalı", c.name)
		}
		if !c.wantCur && cur != nil {
			t.Errorf("%s: current boş (null) olmalı, sıfır değerli nesne 'korumasız' gibi okunur: %v", c.name, cur)
		}
		if _, ok := p["reason"]; ok != c.wantReason {
			t.Errorf("%s: reason beklentisi %v", c.name, c.wantReason)
		}
		if src, _ := p["source"].(string); src != c.wantSource {
			t.Errorf("%s: source=%q, beklenen %q", c.name, src, c.wantSource)
		}
		if p["mode"] != "live" || p["state"] != c.plan.State {
			t.Errorf("%s: mode/state eksik: %v", c.name, p)
		}
	}
}
