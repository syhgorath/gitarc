package model

// QueueItem: merge kuyruğu planındaki bir dal/PR. Durum, kuyruktaki ÖNCEKİ dallar birleşmiş gibi hesaplanır.
type QueueItem struct {
	Position int    `json:"position"` // 1'den başlar
	BranchID string `json:"branchId"`
	Branch   string `json:"branch"`
	PR       int    `json:"pr,omitempty"`
	Title    string `json:"title,omitempty"`
	URL      string `json:"url,omitempty"`
	// Status: "clean" (önceki kuyruk birleşmiş gibi temiz birleşir) | "conflict" | "unknown" (simülasyon yapılamadı)
	Status string `json:"status"`
	Reason string `json:"reason"`
	// ConflictFiles: çakışan dosyalar. ConflictsOnMain: dal daha kuyruğa girmeden `main` ile de çakışıyor.
	ConflictFiles   []string `json:"conflictFiles,omitempty"`
	ConflictsOnMain bool     `json:"conflictsOnMain,omitempty"`
	// ConflictsWith: çakışmaya sebep olan, kuyrukta ÖNCEKİ PR numaraları (numarası yoksa boş).
	ConflictsWith []int `json:"conflictsWith,omitempty"`
	// ConflictsWithBranches: aynı şey dal adıyla (PR numarası olmayan dallar için de dolu).
	ConflictsWithBranches []string `json:"conflictsWithBranches,omitempty"`
	Behind                int      `json:"behind"`       // main'in kaç commit gerisinde
	CI                    string   `json:"ci,omitempty"` // "SUCCESS" | "FAILURE" | "RUNNING" | "" (bilinmiyor; yeşil varsayılmaz)
	Review                string   `json:"review,omitempty"`
	// UnpushedCommits: dalın YEREL ucu GitHub'daki PR ucundan bu kadar commit önde. Plan, GitHub'ın gerçekte birleştireceği UZAK ucu
	// simüle eder; yereldeki (pushlanmamış) çözüm/commit'ler planda YOKTUR. 0 = fark yok (ya da PR/uzak dal yok).
	UnpushedCommits int `json:"unpushedCommits,omitempty"`
	// AfterBlocker: kuyruk daha önceki bir engelde durduğu için bu dal fiilen o engel aşılana kadar bekler.
	AfterBlocker bool `json:"afterBlocker,omitempty"`
}

// QueuePlan: seçilen dalların sırayla `main`'e birleşmesinin SALT OKUNUR planı. Hiçbir şey değiştirmez.
type QueuePlan struct {
	Mode       string      `json:"mode"` // "live" | "simulation"
	BaseBranch string      `json:"baseBranch"`
	Items      []QueueItem `json:"items"`
	// StopsAt: kuyruğun durduğu ilk dalın sırası (1'den); 0 = kuyruğun tamamı temiz.
	StopsAt int `json:"stopsAt"`
	// Mergeable: engele kadar sorunsuz birleşebilecek dal sayısı.
	Mergeable int `json:"mergeable"`
	// StrictUpToDate: korumada "dal güncel olmalı" açık mı (nil = okunamadı/bilinmiyor).
	StrictUpToDate *bool `json:"strictUpToDate,omitempty"`
	// CIRounds: tahmini ardışık CI turu (dal güncel olmalı açıkken her birleşmeden sonra sıradaki yeniden test edilir).
	CIRounds int      `json:"ciRounds"`
	Notes    []string `json:"notes,omitempty"`
}
