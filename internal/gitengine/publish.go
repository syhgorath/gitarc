package gitengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"gitarc/internal/model"
)

// "Toplu yayınla": yerel dalları TEK onayla yayınlar: her dal için pushla (+ PR yoksa PR aç), sonra PR'ları sırayla kuyruk koşusuna sokar.
// Amaç, kullanıcının her dalda elle Push & PR / kuyruk planı / koşu başlat tıklamaları yapmak zorunda kalmamasıdır.
//
// Güvenlik (mevcut kurallar aynen sürer): plan SALT OKUNURDUR; yürütme yalnızca plan parmak iziyle çalışır (dal ucu değiştiyse reddedilir);
// bir adım patlarsa ilk sorunda DURUR; kuyruk koşusu kendi planını/parmak izini ve "temiz önek" kuralını uygular (çakışma/CI sorununda başlamaz);
// hiçbir yerde zorla push (--force) yok; kaydedilmemiş dosya olan aktif dal yayınlanmaz (önce commit).

// isPublishCompanion: "(yerel)" eşlik kartı = açık PR'ı olan dalda pushlanmamış commit'ler.
func isPublishCompanion(b model.BranchArc) bool { return b.ParentBranch != "" }

// publishItemFor: bir dalın yayın planı satırı; yayınlanamıyorsa Action "blocked".
func publishItemFor(b model.BranchArc, defaultBranch string) model.PublishItem {
	it := model.PublishItem{BranchID: b.ID, Branch: realBranchName(b)}
	block := func(reason string) model.PublishItem {
		it.Action, it.Reason = "blocked", reason
		return it
	}
	switch {
	case b.ID == "local-wip-working-tree" || (strings.HasPrefix(b.ID, "local-wip-") && !isPublishCompanion(b)):
		return block("kaydedilmemiş dosyalar: önce commit edin (Çalışma Alanı)")
	case it.Branch == defaultBranch || b.LongLived:
		return block("ana/uzun ömürlü dal yayınlanmaz")
	case b.State != "open" || b.OriginGone || b.HistoryOnly:
		return block("dal birleşmiş ya da uzakta silinmiş; yeni bir dal açın")
	case !b.HasLocal:
		return block("yerel kopyası yok (zaten yalnızca uzakta)")
	case b.IsHEAD && b.UncommittedCount > 0:
		return block(fmt.Sprintf("aktif dalda %d kaydedilmemiş dosya var: önce commit edin", b.UncommittedCount))
	case b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved:
		return block("ana dalla çakışıyor; önce Çakışma Stüdyosu'ndan çözün")
	}
	if isPublishCompanion(b) {
		if b.RemoteAhead <= 0 {
			return block("pushlanacak yerel commit yok")
		}
		it.Action, it.Commits = "push", b.RemoteAhead
		if b.PR != nil {
			it.PR = b.PR.Number
		}
		return it
	}
	if b.PR != nil && b.PR.Number > 0 {
		if b.RemoteAhead <= 0 {
			return block(fmt.Sprintf("PR #%d zaten güncel (pushlanacak commit yok)", b.PR.Number))
		}
		it.Action, it.PR, it.Commits = "push", b.PR.Number, b.RemoteAhead
		return it
	}
	if b.AheadMain <= 0 {
		return block("ana dala göre yeni commit yok; yayınlanacak bir şey yok")
	}
	title := strings.TrimSpace(b.DraftPRTitle)
	if title == "" {
		title = strings.TrimSpace(b.LastCommitMsg)
	}
	if title == "" {
		title = "feat: " + it.Branch
	}
	it.Action, it.Title, it.Body, it.Commits = "push_pr", title, strings.TrimSpace(b.DraftPRBody), b.AheadMain
	return it
}

// publishCandidateIDs: varsayılan aday listesi — yayınlanabilecek (blocked olmayan) yerel dallar, anlık görüntü sırasıyla. Eşlik kartı varsa
// gerçek dalın kendisi (PR'lı üst kart) aday olmaz, tekrar saymamak için yalnızca eşlik kartı alınır.
func publishCandidateIDs(snap model.CockpitSnapshot) []string {
	hasCompanion := map[string]bool{}
	for _, b := range snap.Branches {
		if isPublishCompanion(b) {
			hasCompanion[b.ParentBranch] = true
		}
	}
	var ids []string
	for _, b := range snap.Branches {
		if !isPublishCompanion(b) && hasCompanion[b.Name] {
			continue
		}
		if publishItemFor(b, snap.Repo.DefaultBranch).Action != "blocked" {
			ids = append(ids, b.ID)
		}
	}
	return ids
}

func publishFingerprint(items []model.PublishItem, snap model.CockpitSnapshot) string {
	shaOf := map[string]string{}
	for _, b := range snap.Branches {
		shaOf[b.ID] = b.LocalSHA
	}
	parts := make([]string, 0, len(items)+1)
	for _, it := range items {
		parts = append(parts, it.BranchID+"|"+it.Action+"|"+shaOf[it.BranchID]+"|"+fmt.Sprint(it.Commits))
	}
	parts = append(parts, snap.Repo.DefaultBranch)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:8])
}

// PublishPlan: SALT OKUNUR. branchIDs boşsa tüm yayınlanabilir yerel dallar; doluysa o sırayla.
func (m *Manager) PublishPlan(branchIDs []string) (model.PublishPlan, error) {
	if m.NoProject() {
		return model.PublishPlan{}, fmt.Errorf("henüz açık bir proje yok")
	}
	snap, err := m.Snapshot()
	if err != nil {
		return model.PublishPlan{}, err
	}
	plan := model.PublishPlan{Mode: snap.Repo.Mode, Items: []model.PublishItem{}}
	ids := branchIDs
	if len(ids) == 0 {
		ids = publishCandidateIDs(snap)
	}
	byID := map[string]model.BranchArc{}
	for _, b := range snap.Branches {
		byID[b.ID] = b
	}
	seen := map[string]bool{}
	for _, id := range ids {
		b, ok := byID[id]
		if !ok {
			return model.PublishPlan{}, fmt.Errorf("dal bulunamadı: %s", id)
		}
		it := publishItemFor(b, snap.Repo.DefaultBranch)
		if seen[it.Branch] {
			continue // aynı gerçek dal iki kez seçilmesin (üst kart + eşlik kartı)
		}
		seen[it.Branch] = true
		it.Position = len(plan.Items) + 1
		plan.Items = append(plan.Items, it)
	}
	blocked := 0
	for _, it := range plan.Items {
		if it.Action == "blocked" {
			blocked++
		} else {
			plan.Candidates++
		}
	}
	switch {
	case len(plan.Items) == 0:
		plan.Reason = "yayınlanacak yerel dal yok"
	case blocked > 0:
		plan.Reason = fmt.Sprintf("%d dal yayınlanamıyor (aşağıdaki nedenlere bakın); onları listeden çıkarın", blocked)
	default:
		plan.Publishable = true
		plan.Fingerprint = publishFingerprint(plan.Items, snap)
	}
	plan.Notes = append(plan.Notes, "Sıra, kuyruğun birleştirme sırasıdır: çakışma ihtimali olan dalları sonraya koyun.")
	return plan, nil
}

// PublishRun: planı YÜRÜTÜR. confirm, PublishPlan'ın Fingerprint'i olmalıdır. startQueue ise PR'lar açıldıktan sonra kuyruk koşusu başlatılır.
func (m *Manager) PublishRun(branchIDs []string, confirm string, startQueue bool) (model.PublishResult, error) {
	plan, err := m.PublishPlan(branchIDs)
	if err != nil {
		return model.PublishResult{}, err
	}
	if !plan.Publishable {
		err := fmt.Errorf("yayınlanamaz: %s", plan.Reason)
		m.recordLog("error", "Toplu Yayınla", "", err.Error(), "")
		return model.PublishResult{}, err
	}
	if confirm == "" || confirm != plan.Fingerprint {
		err := fmt.Errorf("yayın planı değişti veya onaylanmadı (dal ucu ilerlemiş olabilir); planı yeniden inceleyip onaylayın. Hiçbir şey yapılmadı")
		m.recordLog("error", "Toplu Yayınla", "", err.Error(), "")
		return model.PublishResult{}, err
	}

	res := model.PublishResult{Steps: []model.PublishStep{}}
	for _, it := range plan.Items {
		step := model.PublishStep{Branch: it.Branch, Action: it.Action, PR: it.PR}
		if _, err := m.PromoteBranchWithMeta(it.BranchID, it.Title, it.Body); err != nil {
			step.Message = err.Error()
			res.Steps = append(res.Steps, step)
			res.QueueNote = "Kuyruk başlatılmadı: ilk sorunda durdu."
			m.recordLog("error", "Toplu Yayınla", it.Branch, err.Error(), "")
			return res, nil // kısmi sonuç hata değil rapordur (önceki dallar yayınlandı)
		}
		step.OK = true
		if it.Action == "push_pr" {
			step.Message = "pushlandı, PR açıldı"
		} else {
			step.Message = "pushlandı; mevcut PR güncellendi"
		}
		res.Steps = append(res.Steps, step)
	}
	res.Completed = true
	m.recordLog("ok", "Toplu Yayınla", "", fmt.Sprintf("%d dal yayınlandı", len(plan.Items)), "")

	if !startQueue {
		res.QueueNote = "Kuyruk başlatılmadı (istenmedi)."
	} else {
		res.QueueStarted, res.QueueNote = m.publishStartQueue(plan.Items, &res)
	}
	if snap, err := m.Snapshot(); err == nil {
		res.Snapshot = &snap
	}
	return res, nil
}

// publishWait: PR bilgisi anlık görüntüye gelene kadar (arka plan gh sorgusu) bekler.
var publishWait = 25 * time.Second

// publishStartQueue: PR'lar anlık görüntüde görününce kuyruk koşusunu planlayıp başlatır. Başlatamazsa nedenini döndürür (PR'lar yine de açıktır).
func (m *Manager) publishStartQueue(items []model.PublishItem, res *model.PublishResult) (bool, string) {
	deadline := time.Now().Add(publishWait)
	var ids []string
	for {
		snap, err := m.Snapshot()
		if err != nil {
			return false, "kuyruk başlatılamadı: " + err.Error()
		}
		ids = ids[:0]
		pr := map[string]int{}
		for _, it := range items {
			for _, b := range snap.Branches {
				if b.Name == it.Branch && !isPublishCompanion(b) && b.State == "open" && b.PR != nil && b.PR.Number > 0 {
					ids = append(ids, b.ID)
					pr[it.Branch] = b.PR.Number
				}
			}
		}
		if len(ids) == len(items) {
			for i := range res.Steps {
				if n := pr[res.Steps[i].Branch]; n > 0 {
					res.Steps[i].PR = n
				}
			}
			break
		}
		if time.Now().After(deadline) {
			return false, "kuyruk başlatılamadı: PR bilgisi GitHub'dan henüz gelmedi; birkaç saniye sonra 🚦 Kuyruk penceresinden başlatın"
		}
		time.Sleep(500 * time.Millisecond)
	}
	if live, ok := m.activeLiveRepo(); ok {
		if snap, err := m.Snapshot(); err == nil {
			remote := snap.Repo.RemoteName
			if remote == "" {
				remote = "origin"
			}
			ctxFetch, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			_, _ = gitFetchRetry(ctxFetch, live.path, remote)
			cancel()
		}
	}
	plan, err := m.QueueRunPlan(ids)
	if err != nil {
		return false, "kuyruk planı hesaplanamadı: " + err.Error()
	}
	if !plan.Startable {
		return false, "kuyruk başlatılmadı: " + plan.Reason
	}
	if _, err := m.StartQueueRun(ids, plan.Fingerprint); err != nil {
		return false, "kuyruk başlatılamadı: " + err.Error()
	}
	if len(plan.Excluded) > 0 {
		return true, fmt.Sprintf("kuyruk koşusu başladı (%d PR); %d PR sorunlu olduğu için dışarıda", len(plan.Items), len(plan.Excluded))
	}
	return true, fmt.Sprintf("kuyruk koşusu başladı (%d PR)", len(plan.Items))
}
