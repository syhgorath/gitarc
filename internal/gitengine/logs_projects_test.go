package gitengine

import (
	"errors"
	"fmt"
	"testing"

	"gitarc/internal/model"
)

func TestTrimLogsKeepsOtherProjectsHistoryWhenOneIsBusy(t *testing.T) {
	var logs []model.ActionLogEntry // yeniden eskiye
	for i := 0; i < 500; i++ {
		logs = append(logs, model.ActionLogEntry{ID: fmt.Sprintf("busy-%d", i), RepoID: "yogun"})
	}
	for i := 0; i < 5; i++ {
		logs = append(logs, model.ActionLogEntry{ID: fmt.Sprintf("sakin-%d", i), RepoID: "sakin"})
	}
	got := trimLogs(logs)
	counts := map[string]int{}
	for _, e := range got {
		counts[e.RepoID]++
	}
	if counts["yogun"] != maxStoredLogsPerRepo {
		t.Fatalf("yoğun proje %d kayıtla sınırlanmalı, %d", maxStoredLogsPerRepo, counts["yogun"])
	}
	if counts["sakin"] != 5 {
		t.Fatalf("sakin projenin eski kayıtları yoğun proje yüzünden silinmemeli: %d", counts["sakin"])
	}
	if got[0].ID != "busy-0" {
		t.Fatalf("sıra (yeniden eskiye) korunmalı: %s", got[0].ID)
	}
}

// Eskiden -repo/ortam/klasörle açılan proje "live-custom" gibi GEÇİCİ kimlik alırdı: aynı kimlik farklı zamanlarda farklı projelere denk
// geldiği için bu kayıtların sahibi belirsizdir ve "projesi bilinmiyor" (boş kimlik) sayılır.
func TestLegacySyntheticLogRepoIDsBecomeUnknownOnLoad(t *testing.T) {
	err := persistSavedLogs([]model.ActionLogEntry{
		{ID: "log-1", RepoID: "live-custom", Message: "eski"},
		{ID: "log-2", RepoID: "live-proj-abc", Message: "kayıtlı proje"},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := loadSavedLogs()
	if len(got) != 2 {
		t.Fatalf("kayıt sayısı: %d", len(got))
	}
	if got[0].RepoID != "" {
		t.Fatalf("geçici kimlik boşaltılmalı: %q", got[0].RepoID)
	}
	if got[1].RepoID != "live-proj-abc" {
		t.Fatalf("kayıtlı proje kimliği korunmalı: %q", got[1].RepoID)
	}
}

// Kayıtlı bir proje -repo ile açılınca KAYITLI kimliğini kullanır: aksi halde açılış yoluna göre iki ayrı günlük/öneri kartı oluşurdu.
func TestLaunchedRegisteredProjectKeepsItsSavedID(t *testing.T) {
	dir, _ := newTempRepo(t)
	seed := NewManager(dir)
	if _, err := seed.AddProject(dir, "kayitli"); err != nil {
		t.Fatal(err)
	}
	want := projectIDForPath(dir)

	m := NewManager(dir)
	snap, err := m.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.ID != want {
		t.Fatalf("kayıtlı proje açılışta kayıtlı kimliğini (%s) kullanmalı, geldi %s", want, snap.Repo.ID)
	}
}

// Çift tıklama/eski ekran: ilk istek dalı zaten birleştirip sildi, ikincisi "dal yok" der. Zararsızdır; hata kartı çıkarmaz.
func TestVanishedBranchIsInfoNotErrorAndExplainsWhy(t *testing.T) {
	gh := newGHStub(t)
	mgr, _ := prSetup(t, gh, "feat/var")
	_, err := mgr.MergeBranch("local-feat-yok-artik", "merge")
	if err == nil || !errors.Is(err, errBranchGone) {
		t.Fatalf("tipli 'dal artık yok' hatası beklenirdi: %v", err)
	}
	for _, l := range mgr.recentLogs() {
		if l.Branch == "local-feat-yok-artik" && l.Level != "info" {
			t.Fatalf("kaybolan dal hata kartı üretmemeli: level=%q", l.Level)
		}
	}
}

// Eski sürümlerde "error" olarak saklanmış beklenen durumlar (CI bitmedi, dal artık yok) yüklenirken info olur; gerçek hatalar error kalır.
func TestOldExpectedPreconditionErrorsAreDowngradedOnLoad(t *testing.T) {
	err := persistSavedLogs([]model.ActionLogEntry{
		{ID: "log-1", Level: "error", RepoID: "p", Message: "PR #134 şu an birleştirilemiyor: main dalının koruma kuralı engelliyor (zorunlu CI ... onay eksik olabilir)"},
		{ID: "log-2", Level: "error", RepoID: "p", Message: "X Pull request o/r#5 is not mergeable: the base branch policy prohibits the merge."},
		{ID: "log-3", Level: "error", RepoID: "p", Message: "canlı depoda dal bulunamadı: local-feat-x"},
		{ID: "log-4", Level: "error", RepoID: "p", Message: "PR #9 merge edilemedi: gerçek bir hata"},
	})
	if err != nil {
		t.Fatal(err)
	}
	lv := map[string]string{}
	for _, l := range loadSavedLogs() {
		lv[l.ID] = l.Level
	}
	for _, id := range []string{"log-1", "log-2", "log-3"} {
		if lv[id] != "info" {
			t.Fatalf("%s beklenen durum: info olmalı, %q", id, lv[id])
		}
	}
	if lv["log-4"] != "error" {
		t.Fatalf("gerçek hata error kalmalı: %q", lv["log-4"])
	}
}
