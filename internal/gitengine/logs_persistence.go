package gitengine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gitarc/internal/model"
)

var logsFileMu sync.Mutex

const (
	maxStoredLogs  = 200 // tüm projelerin toplam kayıt sınırı
	maxVisibleLogs = 50  // bir projenin görünümünde en fazla
)

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
	if len(logs) > maxStoredLogs {
		logs = logs[:maxStoredLogs]
	}
	return logs
}

func persistSavedLogs(logs []model.ActionLogEntry) error {
	logsFileMu.Lock()
	defer logsFileMu.Unlock()

	filePath, err := getLogsFilePath()
	if err != nil {
		return err
	}
	toSave := logs
	if len(toSave) > maxStoredLogs {
		toSave = toSave[:maxStoredLogs]
	}
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}
