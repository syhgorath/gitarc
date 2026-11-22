package gitengine

import (
	"strings"

	"gitarc/internal/model"
)

// IsPrunable: "🧹 Temizle" bu dal için gerçekten bir şey yapar mı? Üstteki sayaç ve temizlik aynı kuralı
// kullanır; eskiden sayaç ayrı bir kuralla sayıyordu, bu yüzden temizledikten sonra sayı değişmiyor,
// bazı dallar silinirken bazıları silinmiyormuş gibi görünüyordu. Kural, pruneGoneLive ile birebir aynıdır:
//   - yerel kopya: uzakta silinmiş/birleşmiş dalın yerel kopyası silinir;
//   - uzak kopya: birleşmiş ve ana dalda olmayan commit'i kalmamış dalın uzak kopyası silinir (yerel kopyayla
//     BİRLİKTE, tek tıkta);
//   - uzun ömürlü dallar, sanal çalışma dalı ve kaydedilmemiş değişikliği olan aktif dal hariç.
func IsPrunable(b model.BranchArc) bool {
	if b.LongLived || b.ID == "local-wip-working-tree" || strings.HasPrefix(b.ID, "local-wip-") {
		return false
	}
	if b.IsHEAD && b.UncommittedCount > 0 {
		return false
	}
	if b.WorktreePath != "" {
		return false // başka klasörde açık: git silmeyi reddeder; "temizle" düğmesi hata verirdi
	}
	mergedPR := b.PR != nil && b.PR.ReviewState == "MERGED" && b.RemoteAhead == 0
	gone := b.OriginGone || b.State == "gone"
	merged := b.State == "merged" || mergedPR
	if b.State == "gone" {
		return true // simülasyonda yalnızca kalıntı kaydı olabilir
	}
	if b.HasLocal && (gone || merged) {
		return true
	}
	return b.HasRemote && merged && b.AheadMain == 0
}

func markPrunable(branches []model.BranchArc) {
	for i := range branches {
		branches[i].Prunable = IsPrunable(branches[i])
	}
}
