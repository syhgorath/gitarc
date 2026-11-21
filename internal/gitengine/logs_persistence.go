package gitengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gitarc/internal/model"
)

var logsFileMu sync.Mutex

const (
	maxStoredLogs        = 1000 // tüm projelerin toplam kayıt sınırı
	maxStoredLogsPerRepo = 200  // BİR projenin saklanan kayıt sınırı: çok işlem yapan proje ötekilerin geçmişini dışarı itmesin
	maxVisibleLogs       = 50   // bir projenin görünümünde en fazla
)

// trimLogs: yeniden eskiye sıralı günlüğü proje başına ve toplam sınıra göre keser (sırayı korur).
func trimLogs(logs []model.ActionLogEntry) []model.ActionLogEntry {
	perRepo := map[string]int{}
	out := make([]model.ActionLogEntry, 0, len(logs))
	for _, e := range logs {
		if perRepo[e.RepoID] >= maxStoredLogsPerRepo {
			continue
		}
		perRepo[e.RepoID]++
		out = append(out, e)
		if len(out) >= maxStoredLogs {
			break
		}
	}
	return out
}

// legacySyntheticRepoIDs: eski sürümlerde -repo/ortam/klasör ile açılan projeye verilen GEÇİCİ kimlikler. Aynı kimlik farklı zamanlarda
// farklı projelere denk geldiği için kayıtların hangi projeye ait olduğu belirsizdir: bu kayıtlar "projesi bilinmiyor" (boş) sayılır.
var legacySyntheticRepoIDs = map[string]bool{"live-custom": true, "live-env": true, "live-cwd": true}

// maxLogSeq: kayıtlı günlük kimliklerinden ("log-N") en büyük N; yeni kimlikler bundan sonra başlar.
func maxLogSeq(logs []model.ActionLogEntry) int {
	max := 0
	for _, e := range logs {
		var n int
		if _, err := fmt.Sscanf(e.ID, "log-%d", &n); err == nil && n > max {
			max = n
		}
	}
	return max
}

func getLogsFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".gitarc")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs.json"), nil
}

func loadSavedLogs() []model.ActionLogEntry {
	filePath, err := getLogsFilePath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	var logs []model.ActionLogEntry
	if err := json.Unmarshal(data, &logs); err != nil {
		return nil
	}
	for i := range logs {
		if legacySyntheticRepoIDs[logs[i].RepoID] {
			logs[i].RepoID = ""
		}
		// Eski sürümler "koruma kuralı engelliyor" (CI bitmedi) ve "dal artık yok" (çift tıklama) durumlarını `error` olarak kaydetti; bunlar
		// hiçbir şeyi değiştirmeyen, çıkış yolu olan beklenen durumlardır (yeni sürümde `info`). Eski kayıtlar Hata Radarı'nda sayfa her açıldığında
		// kart olarak geri geliyordu: yüklerken aynı seviyeye çevrilir.
		if logs[i].Level == "error" && isExpectedPreconditionMessage(logs[i].Message) {
			logs[i].Level = "info"
		}
	}
	return trimLogs(logs)
}

func persistSavedLogs(logs []model.ActionLogEntry) error {
	logsFileMu.Lock()
	defer logsFileMu.Unlock()

	filePath, err := getLogsFilePath()
	if err != nil {
		return err
	}
	toSave := trimLogs(logs)
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// isExpectedPreconditionMessage: hata olarak kaydedilmiş ama aslında beklenen/zararsız ön koşul durumları (bkz. errMergePolicyBlocked, errBranchGone).
func isExpectedPreconditionMessage(msg string) bool {
	return strings.Contains(msg, "koruma kuralı engelliyor") ||
		strings.Contains(msg, "base branch policy prohibits the merge") ||
		strings.Contains(msg, "canlı depoda dal bulunamadı") ||
		strings.Contains(msg, "dal artık depoda yok")
}
