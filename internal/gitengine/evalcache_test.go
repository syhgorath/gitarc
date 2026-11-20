package gitengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// conflictFixture: main ile "feat" aynı satırı farklı değiştirir (gerçek merge çakışması).
func conflictFixture(t *testing.T) (dir, baseSHA, branchSHA string, nodes []model.CommitNode, idx map[string]int) {
	t.Helper()
	dir, run := newTempRepo(t)
	write := func(s string) {
		if err := os.WriteFile(filepath.Join(dir, "app.txt"), []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("a\n")
	run("add", "app.txt")
	run("commit", "-m", "base")
	run("checkout", "-q", "-b", "feat")
	write("feat-side\n")
	run("commit", "-qam", "feat change")
	run("checkout", "-q", "main")
	write("main-side\n")
	run("commit", "-qam", "main change")

	rev := func(ref string) string {
		out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(out))
	}
	baseSHA, branchSHA = rev("main"), rev("feat")
	out, _ := exec.Command("git", "-C", dir, "rev-list", "--first-parent", "--reverse", "main").Output()
	idx = map[string]int{}
	for i, s := range strings.Fields(string(out)) {
		nodes = append(nodes, model.CommitNode{SHA: s, ShortSHA: s[:7]})
		idx[s] = i
	}
	return dir, baseSHA, branchSHA, nodes, idx
}

// Geçici bir hata (zaman aşımı/iptal) "çakışma yok" sonucunu kalıcılaştırmamalı: SHA anahtarlı önbellek
// yalnızca BAŞARIYLA tamamlanmış değerlendirmeyi saklar.
func TestFailedEvaluationIsNeverCached(t *testing.T) {
	dir, baseSHA, branchSHA, nodes, idx := conflictFixture(t)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, _, _, c, _, _ := evaluateBranchVsBase(cancelled, dir, "main", baseSHA, "feat", branchSHA, nodes, idx, len(nodes))
	if c != nil {
		t.Fatalf("iptal edilmiş bağlamda çakışma bulunamaz: %+v", c)
	}

	_, _, ahead, behind, _, conflict, _, hit := evaluateBranchVsBase(context.Background(), dir, "main", baseSHA, "feat", branchSHA, nodes, idx, len(nodes))
	if hit {
		t.Fatal("başarısız değerlendirme önbelleğe yazılmamalıydı (önbellek isabeti)")
	}
	if ahead != 1 || behind != 1 || conflict == nil || !conflict.HasConflict {
		t.Fatalf("sonraki sağlam değerlendirme gerçek çakışmayı görmeli: ahead=%d behind=%d conflict=%+v", ahead, behind, conflict)
	}

	// Başarılı sonuç önbelleğe yazılır.
	_, _, _, _, _, _, _, hit = evaluateBranchVsBase(context.Background(), dir, "main", baseSHA, "feat", branchSHA, nodes, idx, len(nodes))
	if !hit {
		t.Fatal("başarılı değerlendirme önbelleğe yazılmalı")
	}
}

// Stüdyo, anlık görüntüdeki bayrağa körü körüne güvenmez: dal gerçekten çakışıyorsa (güncel merge-tree) açılır.
func TestFreshConflictCheckFindsRealConflict(t *testing.T) {
	dir, _, _, _, _ := conflictFixture(t)
	c := freshConflict(context.Background(), dir, "main", "feat")
	if c == nil || !c.HasConflict || len(c.Files) != 1 || c.Files[0] != "app.txt" {
		t.Fatalf("gerçek çakışma bulunmalı: %+v", c)
	}
	clean, _ := newTempRepo(t)
	if got := freshConflict(context.Background(), clean, "main", "main"); got != nil {
		t.Fatalf("çakışma yokken nil beklenir: %+v", got)
	}
}

// Kokpitte görünen çakışma, Stüdyo'da "bulunamadı" olmamalı: önbellekte bayat bir "çakışma yok" kaydı olsa bile
// Stüdyo dalı güncel merge-tree ile yeniden denetler.
func TestStudioOpensEvenWhenCachedSnapshotMissedTheConflict(t *testing.T) {
	dir, _, _, _, _ := conflictFixture(t)
	mgr := switchLive(t, dir)

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, b := range snap.Branches {
		if b.Name == "feat" {
			id = b.ID
		}
	}
	if id == "" {
		t.Fatalf("feat dalı anlık görüntüde yok: %+v", snap.Branches)
	}

	// Önbelleği bozulmuş duruma getir: tüm kayıtlarda çakışma yok.
	evalCacheMu.Lock()
	for k, v := range evalCache {
		v.conflict = nil
		evalCache[k] = v
	}
	evalCacheMu.Unlock()

	payload, err := mgr.InspectConflictStudio(id)
	if err != nil {
		t.Fatalf("Stüdyo gerçek çakışmayı bulmalı: %v", err)
	}
	if len(payload.Files) != 1 || payload.Files[0].Path != "app.txt" {
		t.Fatalf("çakışan dosya bekleniyordu: %+v", payload.Files)
	}
}

// Çözüm de bayat "çakışma yok" önbelleğine güvenmemeli: eskiden bayrak yoksa sessizce "başarılı" dönüp hiçbir şey
// yapmıyordu (MCP aracı buna "resolved: true" diyordu).
func TestResolveConflictWorksEvenWhenCachedSnapshotMissedTheConflict(t *testing.T) {
	dir, _, _, _, _ := conflictFixture(t)
	mgr := switchLive(t, dir)

	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, b := range snap.Branches {
		if b.Name == "feat" {
			id = b.ID
		}
	}
	if id == "" {
		t.Fatal("feat dalı anlık görüntüde yok")
	}
	evalCacheMu.Lock()
	for k, v := range evalCache {
		v.conflict = nil
		evalCache[k] = v
	}
	evalCacheMu.Unlock()

	if _, err := mgr.ResolveBranchConflict(id, "ours", nil, false); err != nil {
		t.Fatalf("çözüm: %v", err)
	}
	// Dal artık main'i içermeli ve main ile birleştirilebilir olmalı (çakışma kalkmış).
	if err := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", "main", "feat").Run(); err != nil {
		t.Fatal("çözümden sonra feat dalı main'i içermeli (çözüm gerçekten uygulanmalı)")
	}
	if c := freshConflict(context.Background(), dir, "main", "feat"); c != nil {
		t.Fatalf("çözümden sonra çakışma kalmamalı: %+v", c)
	}
}
