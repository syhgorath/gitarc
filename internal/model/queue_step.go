package model

// QueueStepPlan: merge kuyruğunun BİR dalı için bir sonraki adımı (3a: adım adım mod). Plan hiçbir şey yazmaz.
// Uygulama (güncelle ya da birleştir) yalnızca planın Fingerprint'i ile çalışır.
type QueueStepPlan struct {
	Mode     string `json:"mode"` // "live" | "simulation"
	Branch   string `json:"branch"`
	BranchID string `json:"branchId"`
	PR       int    `json:"pr,omitempty"`
	BaseRef  string `json:"baseRef"`
	HeadSHA  string `json:"headSha,omitempty"`
	BaseSHA  string `json:"baseSha,omitempty"`
	Behind   int    `json:"behind"`
	// Action: "update" (ana dalı dalın içine merge et + push) | "merge" (PR'ı birleştir) | "wait" (CI sürüyor ya da bilinmiyor;
	// şimdilik yapılacak bir şey yok) | "blocked" (insan müdahalesi gerekir; Reason'a bakın).
	Action string `json:"action"`
	Reason string `json:"reason"`
	// CI: SUCCESS | FAILURE | RUNNING | UNKNOWN. Bilinmiyorsa asla SUCCESS değildir.
	CI            string   `json:"ci"`
	Review        string   `json:"review,omitempty"`
	ConflictFiles []string `json:"conflictFiles,omitempty"`
	// Fingerprint: yalnızca Action "update" ya da "merge" iken dolu (dal ucu + ana dal ucu + eylem + CI durumu).
	Fingerprint string   `json:"fingerprint,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}
