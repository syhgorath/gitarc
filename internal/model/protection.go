package model

// ProtectionSettings: varsayılan dalın GitArc'ın yönettiği koruma ayarları (GitHub klasik dal koruması).
// Alanlar yalnızca "güçlendirme" yönünde değişir; plan hiçbir kuralı gevşetmez.
type ProtectionSettings struct {
	RequirePR      bool     `json:"requirePR"`      // PR olmadan push yok
	Approvals      int      `json:"approvals"`      // gereken onay sayısı
	DismissStale   bool     `json:"dismissStale"`   // yeni commit eski onayı düşürür
	StrictUpToDate bool     `json:"strictUpToDate"` // dal, taban dalla güncel olmalı
	Checks         []string `json:"checks"`         // zorunlu CI kontrol adları
	EnforceAdmins  bool     `json:"enforceAdmins"`  // kurallar yöneticiler için de geçerli
	BlockForcePush bool     `json:"blockForcePush"` // force-push kapalı
	BlockDeletion  bool     `json:"blockDeletion"`  // dal silme kapalı
}

// ProtectionChange: planın bir satırı (ne, neyden neye).
type ProtectionChange struct {
	Field string `json:"field"`
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// ProtectionPlan: bir koruma profilinin mevcut duruma göre ne değiştireceği. Plan hiçbir şey uygulamaz.
// Fingerprint, uygulamanın yalnızca gösterilen planı yapmasını sağlar.
type ProtectionPlan struct {
	Branch  string `json:"branch"`
	Profile string `json:"profile"` // "temel" | "ci" | "siki"
	// State: "none" (korumasız) | "active" (klasik koruma var) | "ruleset" (Ruleset ile korunuyor, GitArc dokunmaz)
	//        | "unknown" (okunamadı / yetki yok) | "unsupported" (planda dal koruması yok)
	State string `json:"state"`
	// Source: Current'ın nereden okunduğu: "classic" (klasik dal koruması) | "ruleset" (dala uygulanan GitHub Ruleset kuralları, SALT OKUNUR) | "".
	Source          string             `json:"source,omitempty"`
	Applicable      bool               `json:"applicable"`
	Current         ProtectionSettings `json:"current"`
	Target          ProtectionSettings `json:"target"`
	Changes         []ProtectionChange `json:"changes"`
	AvailableChecks []string           `json:"availableChecks"`
	Notes           []string           `json:"notes,omitempty"`
	Fingerprint     string             `json:"fingerprint,omitempty"`
}
