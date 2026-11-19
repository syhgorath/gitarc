package gitengine

import (
	"regexp"
	"strings"

	"gitarc/internal/model"
)

// Uzun ömürlü dallar (develop, release/*, 1.x ...): özellik dalı gibi "birleşince silinir" mantığına
// girmezler. Kalıntı temizliği (`PruneGone`) ve dal silme bunları KORUR; aksi halde develop ana dala
// birleşmiş göründüğünde uzak `develop` bile silinebilirdi. Kural web/fallback.js ile birebir aynıdır.
var longLivedExact = map[string]bool{
	"develop": true, "development": true, "dev": true, "staging": true, "stage": true,
	"production": true, "prod": true, "stable": true, "trunk": true, "next": true,
	"release": true, "releases": true, "main": true, "master": true,
}

var longLivedPrefixes = []string{
	"release/", "release-", "releases/", "support/", "maint/", "maintenance/", "stable/", "stable-",
}

var longLivedVersion = regexp.MustCompile(`^v?\d+(\.(\d+|x))*$`)

// IsLongLivedBranch: ad kalıbına göre uzun ömürlü dal mı? Varsayılan dalın kendisi omurgadır, sayılmaz.
func IsLongLivedBranch(name, defaultBranch string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "" || name == defaultBranch {
		return false
	}
	if longLivedExact[n] || longLivedVersion.MatchString(n) {
		return true
	}
	for _, p := range longLivedPrefixes {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}

// markLongLived işaretleri yerinde yazar (canlı ve simülasyon anlık görüntüleri aynı kuralı kullanır).
func markLongLived(branches []model.BranchArc, defaultBranch string) {
	for i := range branches {
		branches[i].LongLived = IsLongLivedBranch(branches[i].Name, defaultBranch)
	}
}
