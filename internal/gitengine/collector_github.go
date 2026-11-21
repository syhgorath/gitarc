package gitengine

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitarc/internal/model"
)

const ghCacheTTL = 10 * time.Second

// Bu süreler testlerde kısaltılabilir. Yavaş GitHub sorguları (örn. büyük bir depoda
// `gh pr list --json statusCheckRollup` ~5 sn sürer) arka planda, bol süreyle çalışır;
// anlık snapshot yalnızca ghWait / fetchWait kadar bekler ve eldeki (bayat) veriyi gösterir.
var (
	ghWait    = 2 * time.Second // snapshot'ın gh sonucunu en fazla ne kadar bekleyeceği (ilk veri / bir mutasyondan sonra)
	fetchWait = 1 * time.Second // snapshot'ın git fetch sonucunu en fazla ne kadar bekleyeceği (ilk fetch / mutasyondan sonra)
	// Eldeki veri yalnızca süresi dolduğu için bayatsa (kimse bir şeyi değiştirmedi) çok kısa beklenir: bayat veri
	// hemen gösterilir, yenisi arka planda gelince imza (bgVersion) değişir ve arayüz kendiliğinden yeniler. Eskiden
	// her 10 sn'lik süre dolumunda bir sonraki anlık görüntü boşuna ~3 sn (2 + 1) bekliyordu.
	ghWaitPassive    = 300 * time.Millisecond
	fetchWaitPassive = 200 * time.Millisecond
	fetchTTL         = 10 * time.Second // aynı depoda iki git fetch arası en az süre
	ghRefreshLimit   = 90 * time.Second // arka plan yenilemesinin toplam üst sınırı
	ghPRTimeout      = 60 * time.Second
	ghMergedLimit    = 30 * time.Second
	ghProtTimeout    = 15 * time.Second
	ghLogTimeout     = 15 * time.Second
	gitFetchLimit    = 90 * time.Second
)

// ghData, GitHub'dan alınan son başarılı sonuçlardır (bayat gösterim için saklanır).
type ghData struct {
	openPRs           map[string]model.PRStatus
	mergedPRs         map[string]model.PRStatus
	mergedArches      []model.BranchArc
	spineKey          string // arch indeksleri hangi omurgaya göre hesaplandı
	protectionState   string
	protectionSummary string
	mainCI            *model.MainCIStatus
}

// repoBG, bir deponun arka plan veri toplama durumudur (tek uçuş / single-flight).
type repoBG struct {
	mu sync.Mutex

	data     ghData
	haveData bool
	ghAt     time.Time // son gh yenilemesinin bittiği an (sıfır = süresi dolmuş)
	ghBusy   bool
	ghDone   chan struct{}

	// forceFresh: bir mutasyon (merge, push, PR...) önbelleği geçersiz kıldı; sonraki anlık görüntü yeni sonucu tam
	// süreyle bekler, böylece işlemin sonucu ekranda eski PR/CI durumuyla görünmez. Yenileme bitince temizlenir.
	forceFresh bool
	ghTTL      time.Duration // son yenilemenin süresine göre uyarlanan önbellek ömrü (en az ghCacheTTL)

	fetchAt   time.Time
	fetchBusy bool
	fetchDone chan struct{}

	// remoteDefault: uzak deponun BİLDİRDİĞİ varsayılan dal (`git ls-remote --symref <uzak> HEAD`; son başarılı okuma, "" = bilinmiyor).
	// Yerel `origin/HEAD` klonlama anında yazılır ve `git fetch` onu güncellemez: GitHub'da varsayılan dal değişince eski kalır.
	remoteDefault string

	steps   map[string]model.SyncStep
	version uint64 // arka plan sonuçları değiştikçe artar; QuickRepoSignature'a girer
}

var (
	bgMu     sync.Mutex
	bgStates = map[string]*repoBG{}
)

func bgFor(repoPath string) *repoBG {
	key := filepath.Clean(repoPath)
	bgMu.Lock()
	defer bgMu.Unlock()
	st, ok := bgStates[key]
	if !ok {
		st = &repoBG{steps: map[string]model.SyncStep{}}
		bgStates[key] = st
	}
	return st
}

// InvalidateGHCache, repoPath için GitHub ve fetch önbelleğini "süresi dolmuş" işaretler.
// Eldeki veri silinmez: yeni sonuç gelene kadar bayat olarak gösterilmeye devam eder
// (aksi halde her mutasyondan sonra PR/CI bilgisi bir an için boşalırdı).
func InvalidateGHCache(repoPath string) {
	st := bgFor(repoPath)
	st.mu.Lock()
	st.ghAt = time.Time{}
	st.fetchAt = time.Time{}
	st.forceFresh = true
	st.mu.Unlock()
	// Bir mutasyondan (merge, push, PR) sonra "birleşmiş PR yok" yanıtları bayatlamış olabilir: olumsuzları sil.
	prefix := filepath.Clean(repoPath) + "|"
	ghMergedMu.Lock()
	for k, e := range ghMergedCache {
		if !e.merged && strings.HasPrefix(k, prefix) {
			delete(ghMergedCache, k)
		}
	}
	ghMergedMu.Unlock()
}

// bumpBGVersion: arka planda (ör. dal başına birleşmiş-PR sorgusunda) yeni bilgi bulununca imzayı değiştirir.
func bumpBGVersion(repoPath string) {
	st := bgFor(repoPath)
	st.mu.Lock()
	st.version++
	st.mu.Unlock()
}

// bgVersion, arka plan sonuçlarının sürümüdür; değişince arayüz yeni snapshot ister.
func bgVersion(repoPath string) uint64 {
	st := bgFor(repoPath)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.version
}

var syncStepOrder = []struct{ id, label string }{
	{"fetch", "git fetch"},
	{"prs", "PR listesi"},
	{"merged", "Birleşmiş PR"},
	{"protection", "Koruma"},
	{"mainci", "main CI"},
}

// syncStepsFor, bilinen adımları sabit sırada döndürür.
func syncStepsFor(repoPath string) []model.SyncStep {
	st := bgFor(repoPath)
	st.mu.Lock()
	defer st.mu.Unlock()
	var out []model.SyncStep
	for _, o := range syncStepOrder {
		if step, ok := st.steps[o.id]; ok {
			out = append(out, step)
		}
	}
	return out
}

func (st *repoBG) setStep(id string, mutate func(*model.SyncStep)) {
	step := st.steps[id]
	mutate(&step)
	// mutate bazen yapıyı baştan kurar; kimlik ve etiket her zaman burada yeniden yazılır.
	step.ID = id
	for _, o := range syncStepOrder {
		if o.id == id {
			step.Label = o.label
		}
	}
	st.steps[id] = step
}

func shortErr(out string, err error) string {
	msg := strings.TrimSpace(out)
	if msg == "" && err != nil {
		msg = err.Error()
	}
	if i := strings.Index(msg, "\n"); i > 0 {
		msg = msg[:i]
	}
	if len(msg) > 140 {
		msg = msg[:140] + "…"
	}
	if strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "signal: killed") {
		return "zaman aşımı"
	}
	return msg
}

var runIDRegex = regexp.MustCompile(`/actions/runs/(\d+)`)

// extractRunIDFromURL extracts the GitHub Actions run ID from a check detailsUrl.
func extractRunIDFromURL(detailsURL string) string {
	m := runIDRegex.FindStringSubmatch(detailsURL)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

type ghPRPayload struct {
	Number         int    `json:"number"`
	HeadRefName    string `json:"headRefName"`
	Title          string `json:"title"`
	Body           string `json:"body"`
	ReviewDecision string `json:"reviewDecision"`
	URL            string `json:"url"`
	Author         struct {
		Login string `json:"login"`
	} `json:"author"`
	StatusCheckRollup []struct {
		Name         string `json:"name"`
		Context      string `json:"context"`
		Status       string `json:"status"`
		State        string `json:"state"`
		Conclusion   string `json:"conclusion"`
		DetailsURL   string `json:"detailsUrl"`
		TargetURL    string `json:"targetUrl"`
		WorkflowName string `json:"workflowName"`
	} `json:"statusCheckRollup"`
}

type ghMergedPRPayload struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	MergeCommit *struct {
		OID string `json:"oid"`
	} `json:"mergeCommit"`
}

func spineKeyOf(nodes []model.CommitNode) string {
	if len(nodes) == 0 {
		return ""
	}
	return fmt.Sprintf("%s:%d", nodes[len(nodes)-1].SHA, len(nodes))
}

func copyPRMap(in map[string]model.PRStatus) map[string]model.PRStatus {
	out := make(map[string]model.PRStatus, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// fetchGHMetadataCached, GitHub metaverisini "bayat veri + arka planda yenile" modeliyle döndürür:
//   - Önbellek tazeyse (ghCacheTTL) doğrudan döner.
//   - Değilse tek bir arka plan yenilemesi başlatır ve en fazla ghWait kadar bekler. Bu sürede
//     bitmezse eldeki son başarılı sonucu (yoksa boşu) döndürür; adımlar SyncSteps'te "pending/stale" görünür.
//   - Başarısız sorgu eldeki veriyi silmez ve boş sonuç önbelleğe yazılmaz.
func fetchGHMetadataCached(ctx context.Context, repoPath, remoteName, defaultBranch string, nodes []model.CommitNode, shaToIdx map[string]int, existing []model.BranchArc) (map[string]model.PRStatus, map[string]model.PRStatus, []model.BranchArc, string, string) {
	st := bgFor(repoPath)

	if urlOut, _, err := runCmd(ctx, repoPath, "git", "remote", "get-url", remoteName); err != nil || !strings.Contains(strings.ToLower(urlOut), "github.com") {
		summary := "GitHub dışı uzak depo (Saf Git modu)"
		st.mu.Lock()
		for _, id := range []string{"prs", "merged", "protection"} {
			st.setStep(id, func(s *model.SyncStep) { *s = model.SyncStep{State: "skipped", Detail: "GitHub dışı uzak depo"} })
		}
		st.mu.Unlock()
		return map[string]model.PRStatus{}, map[string]model.PRStatus{}, nil, "unknown", summary
	}

	spineKey := spineKeyOf(nodes)

	st.mu.Lock()
	ttl := ghCacheTTL
	if st.ghTTL > ttl {
		ttl = st.ghTTL
	}
	fresh := st.haveData && time.Since(st.ghAt) < ttl
	if !fresh && !st.ghBusy {
		st.ghBusy = true
		st.ghDone = make(chan struct{})
		// Arka plan, çağıranın haritalarını okurken yarış olmasın diye kopya alır.
		nodesCopy := append([]model.CommitNode(nil), nodes...)
		idxCopy := make(map[string]int, len(shaToIdx))
		for k, v := range shaToIdx {
			idxCopy[k] = v
		}
		existingCopy := append([]model.BranchArc(nil), existing...)
		st.markGHPending()
		go st.refreshGH(repoPath, defaultBranch, nodesCopy, idxCopy, existingCopy, spineKey)
	}
	busy, done := st.ghBusy, st.ghDone
	wait := ghWait
	if st.haveData && !st.forceFresh && ghWaitPassive < wait {
		wait = ghWaitPassive // yalnızca süre dolumu: bayat veriyi hemen göster
	}
	st.mu.Unlock()

	if !fresh && busy {
		timer := time.NewTimer(wait)
		select {
		case <-done:
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	d := st.data
	arches := d.mergedArches
	if d.spineKey != spineKey {
		arches = nil // başka bir omurgaya göre hesaplanmış indeksler yanlış konumlara düşer
	}
	prot, protSum := d.protectionState, d.protectionSummary
	if prot == "" {
		prot, protSum = "unknown", "Koruma durumu henüz okunmadı"
	}
	return copyPRMap(d.openPRs), copyPRMap(d.mergedPRs), append([]model.BranchArc(nil), arches...), prot, protSum
}

// markGHPending: yenileme başlarken adımları "bekliyor" (veri varsa "bayat") yapar. st.mu kilitliyken çağrılır.
func (st *repoBG) markGHPending() {
	for _, id := range []string{"prs", "merged", "protection"} {
		id := id
		st.setStep(id, func(s *model.SyncStep) {
			if st.haveData && s.At != "" {
				s.State = "stale"
			} else {
				s.State = "pending"
			}
		})
	}
}

func (st *repoBG) refreshGH(repoPath, defaultBranch string, nodes []model.CommitNode, shaToIdx map[string]int, existing []model.BranchArc, spineKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), ghRefreshLimit)
	defer cancel()
	refreshStart := time.Now()

	var (
		wg                sync.WaitGroup
		prs, merged       map[string]model.PRStatus
		arches            []model.BranchArc
		prErr, mErr       error
		protState, protSm string
		dPR, dMerged, dPr time.Duration
		mainCI            *model.MainCIStatus
		mainErr           error
		dMain             time.Duration
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		t0 := time.Now()
		prs, prErr = fetchGHPRs(ctx, repoPath)
		dPR = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		t0 := time.Now()
		merged, arches, mErr = fetchGHMergedPRs(ctx, repoPath, nodes, shaToIdx, existing)
		dMerged = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		t0 := time.Now()
		protState, protSm = fetchGHBranchProtection(ctx, repoPath, defaultBranch)
		dPr = time.Since(t0)
	}()
	go func() {
		defer wg.Done()
		t0 := time.Now()
		mainCI, mainErr = fetchMainCI(ctx, repoPath, defaultBranch)
		dMain = time.Since(t0)
	}()
	wg.Wait()

	now := time.Now()
	nowStr := now.Format(time.RFC3339)

	st.mu.Lock()
	defer st.mu.Unlock()

	apply := func(id string, err error, dur time.Duration, count int) bool {
		if err == nil {
			st.setStep(id, func(s *model.SyncStep) {
				*s = model.SyncStep{State: "ok", At: nowStr, DurationMs: dur.Milliseconds(), Count: count}
			})
			return true
		}
		state, detail := "failed", err.Error()
		if err == errGHMissing {
			state, detail = "skipped", "gh CLI kurulu değil"
		} else if st.haveData {
			detail += " · önceki sonuç gösteriliyor"
		}
		st.setStep(id, func(s *model.SyncStep) {
			s.State, s.DurationMs, s.Detail, s.Count = state, dur.Milliseconds(), detail, 0
		})
		return false
	}

	if apply("prs", prErr, dPR, len(prs)) {
		st.data.openPRs = prs
		st.haveData = true
	}
	if apply("merged", mErr, dMerged, len(merged)) {
		st.data.mergedPRs = merged
		st.data.mergedArches = arches
		st.data.spineKey = spineKey
		st.haveData = true
	}
	if apply("mainci", mainErr, dMain, 0) {
		st.data.mainCI = mainCI // nil = çalıştırma yok; yeşil varsayılmaz
		st.haveData = true
	}
	st.data.protectionState, st.data.protectionSummary = protState, protSm
	st.haveData = true
	protStep := "ok"
	if protState == "unknown" {
		protStep = "unknown"
	}
	st.setStep("protection", func(s *model.SyncStep) {
		*s = model.SyncStep{State: protStep, At: nowStr, DurationMs: dPr.Milliseconds(), Detail: protSm}
	})

	st.ghAt = now
	// Yenileme yavaşsa (büyük depoda ~20 sn) sürekli yeniden başlamasın: ömür, süresinin 3 katına çıkar (üst sınır 2 dk).
	if took := time.Since(refreshStart); took*3 > ghCacheTTL {
		st.ghTTL = took * 3
		if st.ghTTL > 2*time.Minute {
			st.ghTTL = 2 * time.Minute
		}
	} else {
		st.ghTTL = 0
	}
	st.ghBusy = false
	st.forceFresh = false
	st.version++
	close(st.ghDone)
}

// fetchRemoteCached, `git fetch --prune` komutunu arka planda (tek uçuş, fetchTTL) çalıştırır ve
// en fazla fetchWait bekler. Önceden her snapshot içinde 3 sn sınırıyla çalışıyor, büyük depolarda
// sürekli yarıda kesiliyordu.
func fetchRemoteCached(ctx context.Context, repoPath, remoteName string) {
	st := bgFor(repoPath)
	st.mu.Lock()
	fresh := !st.fetchAt.IsZero() && time.Since(st.fetchAt) < fetchTTL
	if !fresh && !st.fetchBusy {
		st.fetchBusy = true
		st.fetchDone = make(chan struct{})
		st.setStep("fetch", func(s *model.SyncStep) {
			if s.At != "" {
				s.State = "stale"
			} else {
				s.State = "pending"
			}
		})
		go st.doFetch(repoPath, remoteName)
	}
	busy, done := st.fetchBusy, st.fetchDone
	wait := fetchWait
	if !st.fetchAt.IsZero() && fetchWaitPassive < wait {
		wait = fetchWaitPassive // daha önce başarıyla çekildi, yalnızca süre doldu
	}
	st.mu.Unlock()

	if !fresh && busy {
		timer := time.NewTimer(wait)
		select {
		case <-done:
		case <-timer.C:
		case <-ctx.Done():
		}
		timer.Stop()
	}
}

func (st *repoBG) doFetch(repoPath, remoteName string) {
	ctx, cancel := context.WithTimeout(context.Background(), gitFetchLimit)
	defer cancel()
	t0 := time.Now()
	out, _, err := runCmd(ctx, repoPath, "git", "fetch", "--prune", remoteName)
	dur := time.Since(t0)
	// Uzağın gerçek varsayılan dalı (salt okunur; yerel ref'lere yazmaz). Okunamazsa eski değer korunur: boş sonuç yazılmaz.
	headOut, _, headErr := runCmd(ctx, repoPath, "git", "ls-remote", "--symref", remoteName, "HEAD")
	remoteDef := ""
	if headErr == nil {
		remoteDef = parseSymrefHead(headOut)
	}
	now := time.Now()

	st.mu.Lock()
	defer st.mu.Unlock()
	if remoteDef != "" {
		st.remoteDefault = remoteDef
	}
	if err == nil {
		st.setStep("fetch", func(s *model.SyncStep) {
			*s = model.SyncStep{State: "ok", At: now.Format(time.RFC3339), DurationMs: dur.Milliseconds()}
		})
	} else {
		st.setStep("fetch", func(s *model.SyncStep) {
			s.State, s.DurationMs, s.Detail = "failed", dur.Milliseconds(), shortErr(out, err)
		})
	}
	st.fetchAt = now
	st.fetchBusy = false
	st.version++
	close(st.fetchDone)
}

// parseSymrefHead: `git ls-remote --symref <uzak> HEAD` çıktısından varsayılan dal adı ("ref: refs/heads/main\tHEAD" → "main").
func parseSymrefHead(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		rest, ok := strings.CutPrefix(line, "ref: refs/heads/")
		if !ok {
			continue
		}
		if name, _, found := strings.Cut(rest, "\t"); found && name != "" {
			return name
		}
	}
	return ""
}

// remoteDefaultBranch: son bilinen uzak varsayılan dal ("" = henüz okunmadı ya da okunamadı).
func remoteDefaultBranch(repoPath string) string {
	st := bgFor(repoPath)
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.remoteDefault
}

var errGHMissing = fmt.Errorf("gh CLI kurulu değil")

// fetchGHBranchProtection, varsayılan dalın koruma durumunu üç değerli döndürür:
//
//	"active"  – klasik koruma veya aktif Ruleset var
//	"none"    – GitHub açıkça "korunmuyor" dedi (yönetici yetkisiyle okundu) veya Free plan
//	"unknown" – okunamadı: yetki yok (GitHub yönetici olmayanlara 404 verir), zaman aşımı, gh yok
//
// "unknown" bilerek "Korumasız" ile karıştırılmaz: Prometheus gibi korumalı bir depoda yönetici
// olmayan kullanıcıya "Korumasız" demek yanlış güvence/alarm olurdu.
func fetchGHBranchProtection(ctx context.Context, repoPath, defaultBranch string) (string, string) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "unknown", "gh CLI yok; koruma okunamadı"
	}
	ghCtx, cancel := context.WithTimeout(ctx, ghProtTimeout)
	defer cancel()

	// 1. Klasik branch protection denetimi
	out, _, err := runCmd(ghCtx, repoPath, "gh", "api", fmt.Sprintf("repos/:owner/:repo/branches/%s/protection", defaultBranch), "--jq", ".required_status_checks.strict")
	if err == nil {
		return "active", fmt.Sprintf("Aktif (strict checks: %s)", strings.TrimSpace(out))
	}

	// 2. Modern GitHub Ruleset denetimi (okuma yetkisi olan herkes görebilir)
	rulesetCtx, rCancel := context.WithTimeout(ctx, ghProtTimeout)
	defer rCancel()
	rOut, _, rErr := runCmd(rulesetCtx, repoPath, "gh", "api", "repos/:owner/:repo/rulesets")
	if rErr == nil && strings.TrimSpace(rOut) != "" && strings.TrimSpace(rOut) != "[]" {
		type ghRulesetItem struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Target      string `json:"target"`
			Enforcement string `json:"enforcement"`
		}
		var rItems []ghRulesetItem
		if json.Unmarshal([]byte(rOut), &rItems) == nil {
			for _, r := range rItems {
				enf := strings.ToLower(r.Enforcement)
				if (enf == "active" || enf == "evaluate") && (r.Target == "" || r.Target == "branch") {
					return "active", fmt.Sprintf("Aktif (GitHub Ruleset: %s)", r.Name)
				}
			}
		}
	}

	low := strings.ToLower(out)
	switch {
	case strings.Contains(low, "upgrade to github pro"):
		return "none", "Korumasız (GitHub Free özel repo / Pro gerekli)"
	case strings.Contains(low, "branch not protected"):
		return "none", "Korumasız (Doğrudan push/force-push açık)"
	case strings.Contains(low, "context deadline exceeded") || strings.Contains(low, "signal: killed") || ghCtx.Err() != nil:
		return "unknown", "Koruma durumu okunamadı (zaman aşımı)"
	default:
		// "Not Found" / 403: GitHub, koruma ayarlarını yalnızca depo yöneticilerine gösterir.
		return "unknown", "Koruma durumu okunamadı (yetki yok: yalnızca depo yöneticileri görebilir)"
	}
}

// fetchGHPRs açık PR'ları listeler. Hata durumunda boş harita ile birlikte hatayı döndürür;
// çağıran, hatayı "boş sonuç" sanıp önceki veriyi ezmemelidir.
func fetchGHPRs(ctx context.Context, repoPath string) (map[string]model.PRStatus, error) {
	res := map[string]model.PRStatus{}
	if _, err := exec.LookPath("gh"); err != nil {
		return res, errGHMissing
	}
	ghCtx, cancel := context.WithTimeout(ctx, ghPRTimeout)
	defer cancel()
	out, _, err := runCmd(ghCtx, repoPath, "gh", "pr", "list", "--state", "open", "--limit", "20",
		"--json", "number,headRefName,title,body,author,reviewDecision,statusCheckRollup,url")
	if err != nil {
		return res, fmt.Errorf("gh pr list: %s", shortErr(out, err))
	}
	if strings.TrimSpace(out) == "" {
		return res, nil
	}
	var items []ghPRPayload
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return res, fmt.Errorf("gh pr list çıktısı çözümlenemedi: %v", err)
	}
	for _, it := range items {
		// GitHub boş `reviewDecision` döner: inceleme zorunlu değil ya da henüz karar yok. Boş bırakılır; eskiden her PR'a
		// "REVIEW_REQUIRED" (inceleme gerekli) uydurulup inceleme zorunluluğu olmayan depoda da gösteriliyordu.
		rev := it.ReviewDecision
		ciState := "SUCCESS"
		knownChecks := 0 // tanınan (durumu/sonucu olan) kontrol sayısı: yoksa CI durumu BİLİNMİYOR demektir
		failedStep := ""
		runID := ""
		workflow := "" // bilinmiyorsa uydurma: gerçek iş akışı adı kontrol listesinden gelir
		var checks []model.CICheckStep
		for _, chk := range it.StatusCheckRollup {
			chkName := chk.Name
			if chkName == "" {
				chkName = chk.Context
			}
			if chk.WorkflowName != "" {
				workflow = chk.WorkflowName
			}
			conc := strings.ToUpper(chk.Conclusion)
			st := strings.ToUpper(chk.Status)
			ctxSt := strings.ToUpper(chk.State)
			if conc == "" && ctxSt != "" {
				conc = ctxSt
			}
			if conc == "" && st == "COMPLETED" {
				conc = "SUCCESS"
			}
			if conc != "" || st != "" {
				knownChecks++
			}
			detURL := chk.DetailsURL
			if detURL == "" {
				detURL = chk.TargetURL
			}
			checks = append(checks, model.CICheckStep{
				Name:       chkName,
				Workflow:   chk.WorkflowName,
				Status:     st,
				Conclusion: conc,
				DetailsURL: detURL,
			})

			if conc == "FAILURE" || conc == "TIMED_OUT" || conc == "CANCELLED" || ctxSt == "FAILURE" || ctxSt == "ERROR" {
				ciState = "FAILURE"
				if failedStep == "" {
					failedStep = chkName
				}
				if runID == "" {
					runID = extractRunIDFromURL(chk.DetailsURL)
					if runID == "" {
						runID = extractRunIDFromURL(chk.TargetURL)
					}
				}
			} else if (st != "" && st != "COMPLETED") || ctxSt == "PENDING" {
				if ciState != "FAILURE" {
					ciState = "RUNNING"
				}
			}
		}

		var rawLog string
		var extracts []model.CIFailExtract
		if ciState == "FAILURE" && runID != "" {
			logCtx, logCancel := context.WithTimeout(ctx, ghLogTimeout)
			if logOut, _, err := runCmd(logCtx, repoPath, "gh", "run", "view", runID, "--log-failed"); err == nil && strings.TrimSpace(logOut) != "" {
				if len(logOut) > 16000 {
					logOut = logOut[len(logOut)-16000:]
				}
				rawLog = logOut
				extracts = ExtractCILog(rawLog)
			}
			logCancel()
		}

		// Hiç kontrol yok ya da hiçbirinin durumu tanınmıyorsa CI nil kalır (bilinmiyor). Eskiden boş liste "RUNNING",
		// tanınmayan kayıtlar "SUCCESS" olurdu: kontrolü olmayan depoda PR elle birleştirilemiyor ("CI bitince merge et" bekliyordu).
		var ciStatus *model.CIStatus
		if knownChecks > 0 {
			ciStatus = &model.CIStatus{
				State:      ciState,
				Workflow:   workflow,
				RunID:      runID,
				Duration:   "—",
				FailedStep: failedStep,
				Extracts:   extracts,
				RawLog:     rawLog,
				Checks:     checks,
			}
		}
		res[it.HeadRefName] = model.PRStatus{
			Number:      it.Number,
			Title:       it.Title,
			Body:        it.Body,
			Author:      it.Author.Login,
			URL:         it.URL,
			ReviewState: rev,
			CI:          ciStatus,
		}
	}
	return res, nil
}

func fetchGHMergedPRs(ctx context.Context, repoPath string, nodes []model.CommitNode, shaToIdx map[string]int, existing []model.BranchArc) (map[string]model.PRStatus, []model.BranchArc, error) {
	mergedMap := map[string]model.PRStatus{}
	if _, err := exec.LookPath("gh"); err != nil {
		return mergedMap, nil, errGHMissing
	}
	seenMergeIdx := map[int]bool{}
	for _, a := range existing {
		seenMergeIdx[a.EndSpineIndex] = true
	}

	ghCtx, cancel := context.WithTimeout(ctx, ghMergedLimit)
	defer cancel()
	out, _, err := runCmd(ghCtx, repoPath, "gh", "pr", "list", "--state", "merged", "--limit", "60",
		"--json", "number,headRefName,title,body,author,mergeCommit,url")
	if err != nil {
		return mergedMap, nil, fmt.Errorf("gh pr list (merged): %s", shortErr(out, err))
	}
	if strings.TrimSpace(out) == "" {
		return mergedMap, nil, nil
	}
	var items []ghMergedPRPayload
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		return mergedMap, nil, fmt.Errorf("gh pr list (merged) çıktısı çözümlenemedi: %v", err)
	}

	var arches []model.BranchArc
	for _, it := range items {
		prObj := model.PRStatus{
			Number:      it.Number,
			Title:       it.Title,
			Body:        it.Body,
			Author:      it.Author.Login,
			URL:         it.URL,
			ReviewState: "MERGED",
			// CI bilerek boş: eskiden sabit "SUCCESS" yazılıyordu, yani birleşen her PR gerçek sonuçtan bağımsız yeşil
			// görünüyordu. Merge sonrası gerçek sonuç RepoMeta.MainCI'dadır.
		}
		if it.HeadRefName != "" {
			if _, exists := mergedMap[it.HeadRefName]; !exists {
				mergedMap[it.HeadRefName] = prObj
			}
		}
		if it.MergeCommit == nil || it.MergeCommit.OID == "" {
			continue
		}
		endIdx, ok := shaToIdx[it.MergeCommit.OID]
		if !ok || seenMergeIdx[endIdx] || endIdx == 0 {
			continue
		}
		seenMergeIdx[endIdx] = true
		startIdx := endIdx - 1
		if startIdx < 0 {
			startIdx = 0
		}
		arches = append(arches, model.BranchArc{
			ID:              fmt.Sprintf("gh-merged-%d", it.Number),
			Name:            it.HeadRefName,
			Hemisphere:      "upper",
			State:           "merged",
			StartSpineIndex: startIdx,
			EndSpineIndex:   endIdx,
			BaseSHA:         nodes[startIdx].ShortSHA,
			MergeSHA:        nodes[endIdx].ShortSHA,
			HasRemote:       false,
			RemoteRef:       "",
			PR:              &prObj,
			LastCommitMsg:   it.Title,
		})
	}
	return mergedMap, arches, nil
}

// mainCIFor, arka planda toplanan son omurga CI sonucunu döndürür (yoksa nil).
func mainCIFor(repoPath string) *model.MainCIStatus {
	st := bgFor(repoPath)
	st.mu.Lock()
	defer st.mu.Unlock()
	if st.data.mainCI == nil {
		return nil
	}
	cp := *st.data.mainCI
	return &cp
}

type ghRunItem struct {
	Status       string `json:"status"`
	Conclusion   string `json:"conclusion"`
	HeadSHA      string `json:"headSha"`
	WorkflowName string `json:"workflowName"`
	URL          string `json:"url"`
	DisplayTitle string `json:"displayTitle"`
	UpdatedAt    string `json:"updatedAt"`
}

// fetchMainCI: omurga dalındaki en son commit'in CI çalıştırmalarını okur (merge sonrası testler). Aynı commit için
// birden çok iş akışı olabilir (CI, CodeQL...): biri başarısızsa FAILURE, biri sürüyorsa RUNNING, yoksa SUCCESS.
// Çalıştırma yoksa (nil, nil): arayüz yeşil göstermez.
func fetchMainCI(ctx context.Context, repoPath, defaultBranch string) (*model.MainCIStatus, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errGHMissing
	}
	if defaultBranch == "" {
		return nil, nil
	}
	rCtx, cancel := context.WithTimeout(ctx, ghProtTimeout)
	defer cancel()
	out, _, err := runCmd(rCtx, repoPath, "gh", "run", "list", "--branch", defaultBranch, "--event", "push", "--limit", "8",
		"--json", "status,conclusion,headSha,workflowName,url,displayTitle,updatedAt")
	if err != nil {
		return nil, fmt.Errorf("gh run list: %s", shortErr(out, err))
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	var runs []ghRunItem
	if err := json.Unmarshal([]byte(out), &runs); err != nil {
		return nil, fmt.Errorf("gh run list çıktısı çözümlenemedi: %v", err)
	}
	if len(runs) == 0 {
		return nil, nil
	}
	head := runs[0].HeadSHA // gh en yeniyi başa koyar
	// Omurganın GERÇEK ucu (origin/<omurga>): GitHub, push'tan sonra yeni commit için iş akışı kaydını birkaç saniye (bazen
	// dakika) sonra oluşturur; o aralıkta "en son koşu" bir ÖNCEKİ commit'indir ve yeşil görünüp yeni merge'ün testlerini gizlerdi.
	if tip := mainTipSHA(rCtx, repoPath, defaultBranch); tip != "" {
		has := false
		for _, r := range runs {
			if r.HeadSHA == tip {
				has = true
				break
			}
		}
		if has {
			head = tip
		} else {
			return pendingMainCI(rCtx, repoPath, defaultBranch, tip), nil
		}
	}
	first := runs[0]
	for _, r := range runs {
		if r.HeadSHA == head {
			first = r
			break
		}
	}
	res := &model.MainCIStatus{State: "SUCCESS", Branch: defaultBranch, Title: first.DisplayTitle, UpdatedAt: first.UpdatedAt, Workflow: first.WorkflowName, URL: first.URL}
	if len(head) >= 7 {
		res.SHA = head[:7]
	}
	counted := 0
	for _, r := range runs {
		if r.HeadSHA != head {
			continue
		}
		st := strings.ToLower(r.Status)
		conc := strings.ToLower(r.Conclusion)
		switch {
		case st != "" && st != "completed":
			if res.State != "FAILURE" {
				res.State = "RUNNING"
				res.Workflow, res.URL = r.WorkflowName, r.URL
			}
		case conc == "failure" || conc == "timed_out" || conc == "cancelled" || conc == "startup_failure":
			res.State = "FAILURE"
			res.Workflow, res.URL = r.WorkflowName, r.URL
		}
		if conc != "skipped" && conc != "neutral" {
			counted++
		}
	}
	if counted == 0 {
		return nil, nil // yalnızca atlanmış çalıştırmalar: sonuç yok
	}
	return res, nil
}

// mainTipSHA: yerel `origin/<omurga>` ucu; okunamazsa "" (eski davranış: son koşuya güven).
func mainTipSHA(ctx context.Context, repoPath, branch string) string {
	sha, err := revParse(ctx, repoPath, "origin/"+branch)
	if err != nil {
		return ""
	}
	return sha
}

// mainCIPendingWindow: omurga ucu bu kadar yeniyse ve GitHub henüz o commit için koşu kaydı açmadıysa "testler başlıyor" sayılır.
var mainCIPendingWindow = 15 * time.Minute

// pendingMainCI: omurga ucunun CI koşusu GitHub'da henüz görünmüyor. Uç yeniyse RUNNING ("başlıyor"; asla yeşil değil), eskiyse
// (iş akışı bu push'ta hiç koşmuyor olabilir) sonuç yok (nil): rozet gösterilmez, sahte yeşil/sahte bekleme olmaz.
func pendingMainCI(ctx context.Context, repoPath, branch, tip string) *model.MainCIStatus {
	out, _, err := runCmd(ctx, repoPath, "git", "show", "-s", "--format=%ct", tip)
	if err != nil {
		return nil
	}
	ts, perr := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	if perr != nil || time.Since(time.Unix(ts, 0)) > mainCIPendingWindow {
		return nil
	}
	return &model.MainCIStatus{State: "RUNNING", Branch: branch, SHA: tip[:7], Title: "bu commit için GitHub henüz test kaydı açmadı (başlıyor)"}
}

// protectionIsStrict: klasik koruma özetinden ("Aktif (strict checks: true)") "dal güncel olmalı" bilgisi. Ruleset ya da okunamayan koruma
// için false: bilinmeyen, "açık" sayılmaz.
func protectionIsStrict(summary string) bool {
	return strings.Contains(summary, "strict checks: true")
}
