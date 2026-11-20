package model

// QueueRunItem: otomatik koşuya girecek bir PR (planın temiz önekinden).
type QueueRunItem struct {
	Position int    `json:"position"`
	BranchID string `json:"branchId"`
	Branch   string `json:"branch"`
	PR       int    `json:"pr,omitempty"`
	Title    string `json:"title,omitempty"`
	HeadSHA  string `json:"headSha,omitempty"` // planın görüp onaylandığı uzak dal ucu
}

// QueueRunPlan: kuyruğu OTOMATİK çalıştırmanın planı. Salt okunur; başlatmak yalnızca Fingerprint ile mümkündür.
// Koşu yalnızca sorunsuz birleşen ÖNEKİ içerir (ilk sorunda durur); geri kalanı Excluded'dadır.
type QueueRunPlan struct {
	Mode           string         `json:"mode"` // "live" | "simulation"
	BaseBranch     string         `json:"baseBranch"`
	BaseSHA        string         `json:"baseSha,omitempty"`
	Items          []QueueRunItem `json:"items"`
	Excluded       []QueueRunItem `json:"excluded,omitempty"` // sorunlu dal ve ondan sonrakiler (koşuya girmez)
	Startable      bool           `json:"startable"`
	Reason         string         `json:"reason,omitempty"`      // başlatılamıyorsa neden
	Fingerprint    string         `json:"fingerprint,omitempty"` // yalnızca Startable iken: sıralı (dal, uç) listesi + ana dal ucu
	PollSeconds    int            `json:"pollSeconds"`
	TimeoutMinutes int            `json:"timeoutMinutes"` // her CI beklemesi için üst sınır
	Notes          []string       `json:"notes,omitempty"`
}

// QueueRunEvent: koşunun günlüğündeki bir satır.
type QueueRunEvent struct {
	Time    string `json:"time"`
	Branch  string `json:"branch,omitempty"`
	PR      int    `json:"pr,omitempty"`
	Kind    string `json:"kind"` // "info" | "update" | "merge" | "wait" | "stop"
	Message string `json:"message"`
}

// QueueRun: bir otomatik koşunun durumu (proje başına tek koşu; `.git/gitarc_queue_run.json` kalıcıdır).
type QueueRun struct {
	ID string `json:"id"`
	// Status: "running" | "completed" | "stopped" (kullanıcı durdurdu) | "blocked" (ilk sorunda durdu: StopReason) |
	// "timeout" | "interrupted" (uygulama kapandı; kendiliğinden devam ETMEZ).
	Status         string          `json:"status"`
	Mode           string          `json:"mode"`
	Items          []QueueRunItem  `json:"items"`
	Current        int             `json:"current"` // 0 tabanlı; şu an işlenen PR'ın sırası
	Merged         int             `json:"merged"`
	StopReason     string          `json:"stopReason,omitempty"`
	Message        string          `json:"message,omitempty"`
	StartedAt      string          `json:"startedAt"`
	UpdatedAt      string          `json:"updatedAt"`
	FinishedAt     string          `json:"finishedAt,omitempty"`
	TimeoutMinutes int             `json:"timeoutMinutes"`
	Events         []QueueRunEvent `json:"events"`
}
