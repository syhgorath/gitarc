package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Dal koruması: oku → planla → (onayla) → uygula.
//
// Kurallar:
//   - Plan HİÇBİR ŞEY uygulamaz; yalnızca mevcut durumla hedef arasındaki farkı üretir.
//   - Plan yalnızca GÜÇLENDİRİR: mevcut bir kural hedeften sıkıysa (ör. 2 onay) korunur, asla gevşetilmez.
//   - Uygulama, gösterilen planın parmak iziyle (Fingerprint) onaylanmadan çalışmaz; arada ayar değişirse reddedilir.
//   - GitArc'ın yönetmediği klasik koruma alanları (kod sahibi onayı, kısıtlamalar, doğrusal tarih...) PUT'ta korunur;
//     GitHub PUT tüm koruma nesnesini değiştirdiği için bunlar taşınmazsa sessizce silinirdi.

const protectionTimeout = 12 * time.Second

// ProtectionProfiles: sıralı profil adları.
var ProtectionProfiles = []string{"temel", "ci", "siki"}

// profileTarget: bir profilin "en az bu kadar" hedefi. checks yalnızca ci/siki'de kullanılır.
func profileTarget(profile string, checks []string) (model.ProtectionSettings, error) {
	t := model.ProtectionSettings{RequirePR: true, BlockForcePush: true, BlockDeletion: true}
	switch profile {
	case "temel":
	case "ci":
		t.StrictUpToDate = true
		t.Checks = checks
	case "siki":
		t.StrictUpToDate = true
		t.Checks = checks
		t.Approvals = 1
		t.DismissStale = true
		t.EnforceAdmins = true
	default:
		return t, fmt.Errorf("bilinmeyen koruma profili %q (temel, ci veya siki olmalı)", profile)
	}
	return t, nil
}

// cleanChecks: kontrol adlarını temizler, tekilleştirir, sıralar. Boş/uzun/kontrol karakteri içerenler reddedilir.
func cleanChecks(in []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range in {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if len(c) > 100 || strings.ContainsAny(c, "\r\n\x00") {
			return nil, fmt.Errorf("geçersiz kontrol adı %q", truncateRunes(c, 40))
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out, nil
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// strengthen: mevcut ve hedef ayarlardan "daha sıkı olanı" seçer. Hiçbir kural gevşemez.
func strengthen(cur, want model.ProtectionSettings) model.ProtectionSettings {
	out := cur
	out.RequirePR = cur.RequirePR || want.RequirePR
	if want.Approvals > out.Approvals {
		out.Approvals = want.Approvals
	}
	out.DismissStale = cur.DismissStale || want.DismissStale
	out.StrictUpToDate = cur.StrictUpToDate || want.StrictUpToDate
	out.EnforceAdmins = cur.EnforceAdmins || want.EnforceAdmins
	out.BlockForcePush = cur.BlockForcePush || want.BlockForcePush
	out.BlockDeletion = cur.BlockDeletion || want.BlockDeletion
	set := map[string]bool{}
	for _, c := range cur.Checks {
		set[c] = true
	}
	for _, c := range want.Checks {
		set[c] = true
	}
	out.Checks = make([]string, 0, len(set))
	for c := range set {
		out.Checks = append(out.Checks, c)
	}
	sort.Strings(out.Checks)
	if !out.RequirePR {
		out.Approvals, out.DismissStale = 0, false
	}
	return out
}

func yesNo(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}

func checksText(c []string) string {
	if len(c) == 0 {
		return "yok"
	}
	return strings.Join(c, ", ")
}

// diffProtection: iki ayar arasındaki farkı insan okunur satırlara çevirir (yalnızca değişenler).
func diffProtection(cur, tgt model.ProtectionSettings) []model.ProtectionChange {
	var ch []model.ProtectionChange
	add := func(field, label, from, to string) {
		if from != to {
			ch = append(ch, model.ProtectionChange{Field: field, Label: label, From: from, To: to})
		}
	}
	add("requirePR", "PR zorunlu", yesNo(cur.RequirePR, "evet", "hayır"), yesNo(tgt.RequirePR, "evet", "hayır"))
	add("approvals", "Gereken onay", fmt.Sprint(cur.Approvals), fmt.Sprint(tgt.Approvals))
	add("dismissStale", "Yeni commit eski onayı düşürür", yesNo(cur.DismissStale, "evet", "hayır"), yesNo(tgt.DismissStale, "evet", "hayır"))
	add("strictUpToDate", "Dal güncel olmalı", yesNo(cur.StrictUpToDate, "evet", "hayır"), yesNo(tgt.StrictUpToDate, "evet", "hayır"))
	add("checks", "Zorunlu CI kontrolleri", checksText(cur.Checks), checksText(tgt.Checks))
	add("enforceAdmins", "Yöneticiler için de geçerli", yesNo(cur.EnforceAdmins, "evet", "hayır"), yesNo(tgt.EnforceAdmins, "evet", "hayır"))
	add("blockForcePush", "Force-push kapalı", yesNo(cur.BlockForcePush, "evet", "hayır"), yesNo(tgt.BlockForcePush, "evet", "hayır"))
	add("blockDeletion", "Dal silme kapalı", yesNo(cur.BlockDeletion, "evet", "hayır"), yesNo(tgt.BlockDeletion, "evet", "hayır"))
	return ch
}

func protectionFingerprint(branch string, t model.ProtectionSettings) string {
	b, _ := json.Marshal(struct {
		B string
		T model.ProtectionSettings
	}{branch, t})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// protectionSummaryOf: ayarlardan kısa özet (simülasyonda ve uygulama günlüğünde).
func protectionSummaryOf(s model.ProtectionSettings) string {
	var p []string
	if s.RequirePR {
		if s.Approvals > 0 {
			p = append(p, fmt.Sprintf("PR zorunlu (%d onay)", s.Approvals))
		} else {
			p = append(p, "PR zorunlu")
		}
	}
	if s.StrictUpToDate {
		p = append(p, "dal güncel olmalı")
	}
	if len(s.Checks) > 0 {
		p = append(p, "CI: "+strings.Join(s.Checks, ", "))
	}
	if s.EnforceAdmins {
		p = append(p, "yöneticiler dahil")
	}
	if s.BlockForcePush {
		p = append(p, "force-push kapalı")
	}
	if s.BlockDeletion {
		p = append(p, "silme kapalı")
	}
	if len(p) == 0 {
		return "Koruma var ama kısıtlama yok"
	}
	return strings.Join(p, " · ")
}

// buildPlan: saf plan hesabı (üç motor için ortak).
func buildPlan(branch, profile string, selected []string, cur model.ProtectionSettings, state string, available []string) (model.ProtectionPlan, error) {
	plan := model.ProtectionPlan{Branch: branch, Profile: profile, State: state, Current: cur, AvailableChecks: available}
	if plan.AvailableChecks == nil {
		plan.AvailableChecks = []string{}
	}
	checks, err := cleanChecks(selected)
	if err != nil {
		return plan, err
	}
	want, err := profileTarget(profile, checks)
	if err != nil {
		return plan, err
	}
	plan.Target = strengthen(cur, want)
	plan.Changes = diffProtection(cur, plan.Target)
	if plan.Changes == nil {
		plan.Changes = []model.ProtectionChange{}
	}
	switch state {
	case "none", "active":
		plan.Applicable = true
	case "ruleset":
		plan.Notes = append(plan.Notes, "Bu dal bir GitHub Ruleset ile korunuyor; GitArc Ruleset'leri değiştirmez. Ayarı GitHub'da yapın.")
	case "unsupported":
		plan.Notes = append(plan.Notes, "Bu depo planında dal koruması yok (özel depo için GitHub Pro/Team gerekir).")
	default:
		plan.Notes = append(plan.Notes, "Koruma durumu okunamadı (yetki yok: yalnızca depo yöneticileri okuyup değiştirebilir).")
	}
	if plan.Applicable {
		if (profile == "ci" || profile == "siki") && len(plan.Target.Checks) == 0 {
			plan.Notes = append(plan.Notes, "Zorunlu CI kontrolü seçilmedi: yalnızca “dal güncel olmalı” eklenir, CI geçmeden merge yine engellenmez. Aşağıdan en az bir kontrol seçin.")
		}
		if profile == "siki" && cur.Approvals == 0 && !cur.RequirePR {
			plan.Notes = append(plan.Notes, "1 onay + yöneticiler dahil kuralı, tek geliştiricili depoda kendi PR'ınızı birleştirmenizi engeller.")
		}
		if len(plan.Changes) == 0 {
			plan.Notes = append(plan.Notes, "Koruma bu profili zaten karşılıyor; değişecek bir şey yok.")
		}
		plan.Fingerprint = protectionFingerprint(branch, plan.Target)
	}
	return plan, nil
}

// ---- Canlı (GitHub) ----

type ghProtectionRead struct {
	settings    model.ProtectionSettings
	raw         map[string]json.RawMessage
	fromRuleset bool // settings, Ruleset kurallarından (salt okunur) okundu
}

func ghAPIBranchPath(branch string) string {
	return fmt.Sprintf("repos/:owner/:repo/branches/%s/protection", branch)
}

// readLiveProtection: mevcut klasik korumayı okur. state: none | active | ruleset | unsupported | unknown.
func readLiveProtection(ctx context.Context, repoPath, branch string) (ghProtectionRead, string) {
	var res ghProtectionRead
	if _, err := exec.LookPath("gh"); err != nil {
		return res, "unknown"
	}
	out, _, err := runCmd(ctx, repoPath, "gh", "api", ghAPIBranchPath(branch))
	if err == nil {
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(out), &raw) != nil {
			return res, "unknown"
		}
		res.raw = raw
		res.settings = parseProtectionSettings(raw)
		return res, "active"
	}
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "upgrade to github pro"):
		return res, "unsupported"
	case strings.Contains(low, "branch not protected"):
		if hasActiveRuleset(ctx, repoPath) {
			res.settings, res.fromRuleset = readRulesetSettings(ctx, repoPath, branch)
			return res, "ruleset"
		}
		return res, "none"
	}
	if hasActiveRuleset(ctx, repoPath) {
		res.settings, res.fromRuleset = readRulesetSettings(ctx, repoPath, branch)
		return res, "ruleset"
	}
	return res, "unknown"
}

// readRulesetSettings: dala UYGULANAN GitHub Ruleset kurallarını (`rules/branches/<dal>`; okuma yetkisi yeter) ProtectionSettings'e çevirir.
// SALT OKUNUR: GitArc Ruleset'i değiştirmez. ok=false: kurallar okunamadı (çağıran "yok" değil "okunamadı" der).
// Yöneticiler için geçerlilik (enforceAdmins) bu uçtan bilinmez (bypass listesi ayrı); false bırakılır.
func readRulesetSettings(ctx context.Context, repoPath, branch string) (model.ProtectionSettings, bool) {
	var s model.ProtectionSettings
	out, _, err := runCmd(ctx, repoPath, "gh", "api", "repos/:owner/:repo/rules/branches/"+branch)
	if err != nil {
		return s, false
	}
	var rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			Required []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
			Strict       bool `json:"strict_required_status_checks_policy"`
			Approvals    int  `json:"required_approving_review_count"`
			DismissStale bool `json:"dismiss_stale_reviews_on_push"`
		} `json:"parameters"`
	}
	if json.Unmarshal([]byte(out), &rules) != nil {
		return s, false
	}
	for _, r := range rules {
		switch r.Type {
		case "pull_request":
			s.RequirePR = true
			if r.Parameters.Approvals > s.Approvals {
				s.Approvals = r.Parameters.Approvals
			}
			s.DismissStale = s.DismissStale || r.Parameters.DismissStale
		case "required_status_checks":
			for _, c := range r.Parameters.Required {
				if c.Context != "" {
					s.Checks = append(s.Checks, c.Context)
				}
			}
			s.StrictUpToDate = s.StrictUpToDate || r.Parameters.Strict
		case "non_fast_forward":
			s.BlockForcePush = true
		case "deletion":
			s.BlockDeletion = true
		}
	}
	return s, true
}

func hasActiveRuleset(ctx context.Context, repoPath string) bool {
	out, _, err := runCmd(ctx, repoPath, "gh", "api", "repos/:owner/:repo/rulesets")
	if err != nil || strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return false
	}
	var items []struct {
		Target      string `json:"target"`
		Enforcement string `json:"enforcement"`
	}
	if json.Unmarshal([]byte(out), &items) != nil {
		return false
	}
	for _, r := range items {
		enf := strings.ToLower(r.Enforcement)
		if (enf == "active" || enf == "evaluate") && (r.Target == "" || r.Target == "branch") {
			return true
		}
	}
	return false
}

func rawEnabled(raw map[string]json.RawMessage, key string) bool {
	var v struct {
		Enabled bool `json:"enabled"`
	}
	if b, ok := raw[key]; ok {
		_ = json.Unmarshal(b, &v)
	}
	return v.Enabled
}

func parseProtectionSettings(raw map[string]json.RawMessage) model.ProtectionSettings {
	var s model.ProtectionSettings
	if b, ok := raw["required_status_checks"]; ok {
		var rs struct {
			Strict   bool     `json:"strict"`
			Contexts []string `json:"contexts"`
			Checks   []struct {
				Context string `json:"context"`
			} `json:"checks"`
		}
		if json.Unmarshal(b, &rs) == nil {
			s.StrictUpToDate = rs.Strict
			names := append([]string(nil), rs.Contexts...)
			for _, c := range rs.Checks {
				names = append(names, c.Context)
			}
			s.Checks, _ = cleanChecks(names)
		}
	}
	if b, ok := raw["required_pull_request_reviews"]; ok && string(b) != "null" {
		var pr struct {
			Dismiss   bool `json:"dismiss_stale_reviews"`
			Approving int  `json:"required_approving_review_count"`
		}
		if json.Unmarshal(b, &pr) == nil {
			s.RequirePR = true
			s.Approvals = pr.Approving
			s.DismissStale = pr.Dismiss
		}
	}
	s.EnforceAdmins = rawEnabled(raw, "enforce_admins")
	s.BlockForcePush = !rawEnabled(raw, "allow_force_pushes")
	s.BlockDeletion = !rawEnabled(raw, "allow_deletions")
	return s
}

// namesOf: GET çıktısındaki {users:[{login}],teams:[{slug}],apps:[{slug}]} → PUT'un beklediği string dizileri.
func namesOf(b json.RawMessage) any {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	var v struct {
		Users []struct{ Login string } `json:"users"`
		Teams []struct{ Slug string }  `json:"teams"`
		Apps  []struct{ Slug string }  `json:"apps"`
	}
	if json.Unmarshal(b, &v) != nil {
		return nil
	}
	out := map[string]any{"users": []string{}, "teams": []string{}, "apps": []string{}}
	us, ts, as := []string{}, []string{}, []string{}
	for _, u := range v.Users {
		us = append(us, u.Login)
	}
	for _, t := range v.Teams {
		ts = append(ts, t.Slug)
	}
	for _, a := range v.Apps {
		as = append(as, a.Slug)
	}
	out["users"], out["teams"], out["apps"] = us, ts, as
	return out
}

// buildProtectionPayload: PUT gövdesi. GitArc'ın yönetmediği alanlar (raw) korunur.
func buildProtectionPayload(t model.ProtectionSettings, raw map[string]json.RawMessage) ([]byte, error) {
	p := map[string]any{}
	if t.StrictUpToDate || len(t.Checks) > 0 {
		p["required_status_checks"] = map[string]any{"strict": t.StrictUpToDate, "contexts": append([]string{}, t.Checks...)}
	} else {
		p["required_status_checks"] = nil
	}
	p["enforce_admins"] = t.EnforceAdmins
	if t.RequirePR {
		pr := map[string]any{
			"required_approving_review_count": t.Approvals,
			"dismiss_stale_reviews":           t.DismissStale,
		}
		if b, ok := raw["required_pull_request_reviews"]; ok && string(b) != "null" {
			var old map[string]json.RawMessage
			if json.Unmarshal(b, &old) == nil {
				for _, k := range []string{"require_code_owner_reviews", "require_last_push_approval"} {
					if v, ok := old[k]; ok {
						var x bool
						if json.Unmarshal(v, &x) == nil {
							pr[k] = x
						}
					}
				}
				for _, k := range []string{"dismissal_restrictions", "bypass_pull_request_allowances"} {
					if v, ok := old[k]; ok {
						if n := namesOf(v); n != nil {
							pr[k] = n
						}
					}
				}
			}
		}
		p["required_pull_request_reviews"] = pr
	} else {
		p["required_pull_request_reviews"] = nil
	}
	p["restrictions"] = namesOf(raw["restrictions"])
	p["allow_force_pushes"] = !t.BlockForcePush
	p["allow_deletions"] = !t.BlockDeletion
	for _, k := range []string{"required_linear_history", "required_conversation_resolution", "lock_branch", "block_creations", "allow_fork_syncing"} {
		if _, ok := raw[k]; ok {
			p[k] = rawEnabled(raw, k)
		}
	}
	return json.Marshal(p)
}

// fetchAvailableChecks: varsayılan dalın son commit'inde görülen CI kontrol adları (zorunlu yapılabilecekler).
func fetchAvailableChecks(ctx context.Context, repoPath, branch string) []string {
	out, _, err := runCmd(ctx, repoPath, "gh", "api", fmt.Sprintf("repos/:owner/:repo/commits/%s/check-runs", branch), "--jq", "[.check_runs[].name]")
	if err != nil {
		return []string{}
	}
	var names []string
	if json.Unmarshal([]byte(strings.TrimSpace(out)), &names) != nil {
		return []string{}
	}
	cleaned, _ := cleanChecks(names)
	return cleaned
}

func friendlyProtectionError(branch, out string) error {
	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "upgrade to github pro"):
		return fmt.Errorf("bu depo planında dal koruması kullanılamaz (özel depo için GitHub Pro/Team gerekir)")
	case strings.Contains(low, "not found") || strings.Contains(low, "403") || strings.Contains(low, "must have admin") || strings.Contains(low, "resource not accessible"):
		return fmt.Errorf("%s dalı korunamadı: bu işlem için depo yöneticisi yetkisi gerekir (GitHub: %s)", branch, shortErr(out, nil))
	}
	return fmt.Errorf("Branch Protection API hatası (%s): %s", branch, strings.TrimSpace(out))
}

func (m *Manager) livePlan(ctx context.Context, live *repoEntry, profile string, checks []string) (model.ProtectionPlan, map[string]json.RawMessage, error) {
	branch := detectDefaultBranch(ctx, live.path)
	if _, err := exec.LookPath("gh"); err != nil {
		return model.ProtectionPlan{}, nil, fmt.Errorf("Branch Protection için gh CLI gereklidir")
	}
	read, state := readLiveProtection(ctx, live.path, branch)
	plan, err := buildPlan(branch, profile, checks, read.settings, state, fetchAvailableChecks(ctx, live.path, branch))
	switch {
	case state == "active":
		plan.Source = "classic"
	case state == "ruleset" && read.fromRuleset:
		plan.Source = "ruleset"
	case state == "ruleset":
		plan.Notes = append(plan.Notes, "Ruleset kuralları okunamadı (ayrıntı gösterilemiyor); bu \"kural yok\" demek değildir.")
	}
	return plan, read.raw, err
}

// ProtectionPlan: profilin mevcut duruma göre ne değiştireceğini döndürür; hiçbir şey uygulamaz.
func (m *Manager) ProtectionPlan(profile string, checks []string) (model.ProtectionPlan, error) {
	if live, ok := m.activeLiveRepo(); ok {
		ctx, cancel := context.WithTimeout(context.Background(), protectionTimeout)
		defer cancel()
		plan, _, err := m.livePlan(ctx, live, profile, checks)
		return plan, err
	}
	if m.NoProject() {
		return model.ProtectionPlan{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return simProtectionPlanLocked(&m.simRepo, profile, checks)
}

// ApplyProtection: planı uygular. confirm, ProtectionPlan'ın Fingerprint değeri olmalıdır.
func (m *Manager) ApplyProtection(profile string, checks []string, confirm string) (model.CockpitSnapshot, error) {
	if live, ok := m.activeLiveRepo(); ok {
		ctx, cancel := context.WithTimeout(context.Background(), protectionTimeout)
		defer cancel()
		plan, raw, err := m.livePlan(ctx, live, profile, checks)
		if err != nil {
			m.recordLog("error", "Branch Protection", plan.Branch, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
		if err := checkApplicable(plan, confirm); err != nil {
			m.recordLog("error", "Branch Protection", plan.Branch, err.Error(), "")
			return model.CockpitSnapshot{}, err
		}
		payload, err := buildProtectionPayload(plan.Target, raw)
		if err != nil {
			return model.CockpitSnapshot{}, err
		}
		out, _, runErr := runCmdStdin(ctx, live.path, string(payload), "gh", "api", "-X", "PUT", ghAPIBranchPath(plan.Branch), "--input", "-")
		if runErr != nil {
			errOut := friendlyProtectionError(plan.Branch, out)
			m.recordLog("error", "Branch Protection", plan.Branch, errOut.Error(), fmt.Sprintf("gh api -X PUT %s", ghAPIBranchPath(plan.Branch)))
			return model.CockpitSnapshot{}, errOut
		}
		InvalidateGHCache(live.path)
		m.recordLog("ok", "Branch Protection", plan.Branch, fmt.Sprintf("%s dalı korumaya alındı (%s): %s", plan.Branch, profile, protectionSummaryOf(plan.Target)), "")
		return m.Snapshot()
	}
	if m.NoProject() {
		return model.CockpitSnapshot{}, fmt.Errorf("henüz açık bir proje yok")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	plan, err := simProtectionPlanLocked(&m.simRepo, profile, checks)
	if err == nil {
		err = checkApplicable(plan, confirm)
	}
	if err != nil {
		m.recordLogLocked("error", "Branch Protection", m.simRepo.repo.DefaultBranch, err.Error(), "")
		return model.CockpitSnapshot{}, err
	}
	t := plan.Target
	m.simRepo.protection = &t
	m.simRepo.repo.BranchProtected = true
	m.simRepo.repo.ProtectionState = "active"
	m.simRepo.repo.ProtectionSummary = protectionSummaryOf(t)
	m.recordLogLocked("ok", "Branch Protection", plan.Branch, fmt.Sprintf("%s dalı korumaya alındı (%s): %s", plan.Branch, profile, protectionSummaryOf(t)), "")
	return m.buildSimSnapshotLocked(m.repoOptionsLocked()), nil
}

func checkApplicable(plan model.ProtectionPlan, confirm string) error {
	if !plan.Applicable {
		return fmt.Errorf("koruma uygulanamıyor: %s", strings.Join(plan.Notes, " "))
	}
	if len(plan.Changes) == 0 {
		return fmt.Errorf("koruma bu profili zaten karşılıyor; değişecek bir şey yok")
	}
	if confirm == "" || confirm != plan.Fingerprint {
		return fmt.Errorf("koruma planı değişti veya onaylanmadı; planı yeniden inceleyip onaylayın")
	}
	return nil
}

// simProtectionPlanLocked: demo depo için aynı plan mantığı (kontrol adları demo verisidir).
func simProtectionPlanLocked(ws *simWorkspace, profile string, checks []string) (model.ProtectionPlan, error) {
	state := "none"
	var cur model.ProtectionSettings
	if ws.protection != nil {
		state, cur = "active", *ws.protection
	}
	return buildPlan(ws.repo.DefaultBranch, profile, checks, cur, state, []string{"build", "lint", "test"})
}

// runCmdStdin: runCmd gibi, ama komuta stdin verir (gh api --input -).
func runCmdStdin(ctx context.Context, dir, stdin, bin string, args ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	b, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		code = 1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	return string(b), code, err
}
