package model

// PublishItem: toplu yayınlanacak bir yerel dal ve YAPILACAK iş (salt okunur plan satırı).
type PublishItem struct {
	Position int    `json:"position"`
	BranchID string `json:"branchId"`
	Branch   string `json:"branch"` // GERÇEK git dal adı ("(yerel)" eşlik kartında bile)
	// Action: "push_pr" (pushla + yeni PR aç) | "push" (mevcut PR'ın dalına pushla; yeni PR yok) | "blocked" (yayınlanamaz, Reason'da neden)
	Action  string `json:"action"`
	Title   string `json:"title,omitempty"` // push_pr için PR başlığı (dalın kendi CHANGELOG/commit bilgisinden)
	Body    string `json:"body,omitempty"`
	Commits int    `json:"commits,omitempty"` // pushlanacak commit sayısı (biliniyorsa)
	PR      int    `json:"pr,omitempty"`      // push için mevcut PR numarası
	Reason  string `json:"reason,omitempty"`
}

// PublishPlan: "Toplu yayınla" planı. SALT OKUNUR; yürütmek yalnızca Fingerprint ile mümkündür.
type PublishPlan struct {
	Mode        string        `json:"mode"` // "live" | "simulation"
	Items       []PublishItem `json:"items"`
	Candidates  int           `json:"candidates"`            // yayınlanabilecek (blocked olmayan) dal sayısı
	Publishable bool          `json:"publishable"`           // seçilenlerin HEPSİ yayınlanabilir ve en az biri var
	Reason      string        `json:"reason,omitempty"`      // yayınlanamıyorsa neden
	Fingerprint string        `json:"fingerprint,omitempty"` // yalnızca Publishable iken
	Notes       []string      `json:"notes,omitempty"`
}

// PublishStep: yürütülen bir adımın sonucu.
type PublishStep struct {
	Branch  string `json:"branch"`
	Action  string `json:"action"`
	OK      bool   `json:"ok"`
	PR      int    `json:"pr,omitempty"`
	Message string `json:"message"`
}

// PublishResult: toplu yayının sonucu. Bir adım patlarsa durur; önceki adımlar geri alınmaz (pushlanan/PR'ı açılan dal kalır).
type PublishResult struct {
	Steps        []PublishStep    `json:"steps"`
	Completed    bool             `json:"completed"`           // tüm dallar yayınlandı
	QueueStarted bool             `json:"queueStarted"`        // otomatik kuyruk koşusu başladı
	QueueNote    string           `json:"queueNote,omitempty"` // koşu başlamadıysa neden / başladıysa kısa özet
	Snapshot     *CockpitSnapshot `json:"snapshot,omitempty"`
}
