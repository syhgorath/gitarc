package gitengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"gitarc/internal/model"
)

var logsFileMu sync.Mutex

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
	if len(logs) > 50 {
		logs = logs[:50]
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
	if len(toSave) > 50 {
		toSave = toSave[:50]
	}
	data, err := json.MarshalIndent(toSave, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

func clearSavedLogsFile() {
	logsFileMu.Lock()
	defer logsFileMu.Unlock()

	filePath, err := getLogsFilePath()
	if err != nil {
		return
	}
	_ = os.WriteFile(filePath, []byte("[]"), 0644)
}
