package model

// BranchUpdatePlan: bir PR dalının ana dala göre güncellenmesinin (ana dalı dalın İÇİNE merge etmek) planı. Plan hiçbir şey
// yazmaz. Uygulama uzak dala PUSH eder (açık bir PR'ı değiştirir), bu yüzden yalnızca planın Fingerprint'i ile çalışır.
type BranchUpdatePlan struct {
	Mode     string `json:"mode"` // "live" | "simulation"
	Branch   string `json:"branch"`
	BranchID string `json:"branchId"`
	PR       int    `json:"pr,omitempty"`
	BaseRef  string `json:"baseRef"`
	HeadSHA  string `json:"headSha,omitempty"` // dalın (uzaktaki) ucu
	BaseSHA  string `json:"baseSha,omitempty"` // ana dalın ucu
	Behind   int    `json:"behind"`            // ana dalda olup dalda olmayan commit sayısı
	Ahead    int    `json:"ahead"`
	// Status: "ready" (temiz merge edilebilir) | "up_to_date" | "conflict" | "blocked" (başka bir engel; Reason'a bakın)
	Status        string   `json:"status"`
	Reason        string   `json:"reason"`
	ConflictFiles []string `json:"conflictFiles,omitempty"`
	// Fingerprint: yalnızca Status == "ready" iken dolu. Dalın ucu ve ana dalın ucu değişirse değişir.
	Fingerprint string   `json:"fingerprint,omitempty"`
	Notes       []string `json:"notes,omitempty"`
}
