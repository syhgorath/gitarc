package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Gerçek olay: "öneri onayı" yeni bir dal açtı, sonra silinmiş dosyalar stage edilemediği için patladı; geride boş bir dal
// kaldı, HEAD ona taşınmış durdu ve yeniden denemeler "dal zaten birleşmiş" diye engellendi.

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return isoGit(t, dir, args...)
}

func branchNamed(snapBranches []string, name string) bool {
	for _, b := range snapBranches {
		if b == name {
			return true
		}
	}
	return false
}

// Silinmiş dosyalar (git rm ile stage edilmiş, yalnızca çalışma ağacından silinmiş) ve değişen dosya birlikte commit'lenir.
func TestCommitWorkingTreeStagesDeletedFiles(t *testing.T) {
	isoHome(t)
	dir, run := newTempRepo(t)
	for _, n := range []string{"git-rm.txt", "sadece-sil.txt", "degisen.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("ilk\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("add", "-A")
	run("commit", "-qm", "dosyalar")
	run("rm", "-q", "git-rm.txt")                                                  // silme zaten stage edilmiş
	_ = os.Remove(filepath.Join(dir, "sadece-sil.txt"))                            // yalnızca çalışma ağacından silinmiş
	_ = os.WriteFile(filepath.Join(dir, "degisen.txt"), []byte("degisti\n"), 0644) // değişmiş

	mgr := switchLive(t, dir)
	if _, err := mgr.CommitWorkingTree("feat/silmeler", "chore: silmeler", "", "", false, []string{"git-rm.txt", "sadece-sil.txt", "degisen.txt"}); err != nil {
		t.Fatalf("silinmiş dosyalar commit'lenebilmeli: %v", err)
	}
	changed := gitOut(t, dir, "show", "--name-status", "--format=", "HEAD")
	for _, want := range []string{"D\tgit-rm.txt", "D\tsadece-sil.txt", "M\tdegisen.txt"} {
		if !strings.Contains(changed, want) {
			t.Fatalf("commit %q içermeli:\n%s", want, changed)
		}
	}
	if gitOut(t, dir, "branch", "--show-current") != "feat/silmeler" {
		t.Fatal("commit yeni dalda olmalı")
	}
}

// Stage patlarsa açılan boş dal geri alınır: HEAD eski dala döner, dal silinir, indeks/çalışma ağacı korunur ve yeniden
// deneme çalışır.
func TestFailedCommitRollsBackTheNewBranchAndRetryWorks(t *testing.T) {
	isoHome(t)
	dir, _ := newTempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "gercek.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := switchLive(t, dir)

	// Var olmayan, izlenmeyen bir yol: stage patlar.
	_, err := mgr.CommitWorkingTree("feat/geri-al", "feat: x", "", "", false, []string{"gercek.txt", "yok-boyle-dosya.txt"})
	if err == nil {
		t.Fatal("var olmayan dosya hata vermeli")
	}
	if !strings.Contains(err.Error(), "geri alındı") {
		t.Fatalf("hata, açılan dalın geri alındığını söylemeli: %v", err)
	}
	if got := gitOut(t, dir, "branch", "--show-current"); got != "main" {
		t.Fatalf("HEAD eski dala (main) dönmeli, şu an %q", got)
	}
	if out := gitOut(t, dir, "branch", "--list", "feat/geri-al"); out != "" {
		t.Fatalf("açılan boş dal silinmeli: %q", out)
	}
	if _, serr := os.Stat(filepath.Join(dir, "gercek.txt")); serr != nil {
		t.Fatal("çalışma ağacındaki dosya korunmalı")
	}

	// Aynı ad ile yeniden deneme: "dal zaten var/birleşmiş" engeli çıkmamalı, commit atılmalı.
	if _, err := mgr.CommitWorkingTree("feat/geri-al", "feat: x", "", "", false, []string{"gercek.txt"}); err != nil {
		t.Fatalf("yeniden deneme çalışmalı: %v", err)
	}
	if gitOut(t, dir, "log", "-1", "--format=%s") != "feat: x" {
		t.Fatal("yeniden denemede commit atılmış olmalı")
	}
}

// Commit atıldıktan sonra (örn. push patlarsa) dal GERİ ALINMAZ: içinde gerçek iş var.
func TestCommitSucceededBranchIsNeverRolledBack(t *testing.T) {
	isoHome(t)
	dir, _ := newTempRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "is.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := switchLive(t, dir)
	// Uzak depo yok: push patlar, ama commit atılmıştı.
	_, err := mgr.CommitWorkingTree("feat/is-var", "feat: is", "", "", true, []string{"is.txt"})
	if err == nil {
		t.Skip("bu ortamda push başarılı oldu; senaryo uygulanamaz")
	}
	if gitOut(t, dir, "branch", "--show-current") != "feat/is-var" || gitOut(t, dir, "log", "-1", "--format=%s") != "feat: is" {
		t.Fatalf("commit atılmış dal korunmalı: %v", err)
	}
}

// Taze yerel dal (main'in ucunda, hiç commit'i yok, upstream yok) kalıntı değildir: açık/yerel dal, temizlenemez.
func TestFreshLocalBranchIsNotAMergedLeftover(t *testing.T) {
	isoHome(t)
	dir, run := newTempRepo(t)
	run("branch", "taze-dal")
	// gerçekten yaşanmış, main'e birleşmiş dal: kalıntı olarak bulunmaya DEVAM eder (gerileme koruması)
	run("checkout", "-q", "-b", "eski-ozellik")
	if err := os.WriteFile(filepath.Join(dir, "o.txt"), []byte("o\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "ozellik")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge eski-ozellik", "eski-ozellik")

	mgr := NewManager(dir)
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]struct {
		state              string
		gone, prune, histo bool
	}{}
	for _, b := range snap.Branches {
		got[b.Name] = struct {
			state              string
			gone, prune, histo bool
		}{b.State, b.OriginGone, b.Prunable, b.HistoryOnly}
	}
	taze, ok := got["taze-dal"]
	if !ok {
		t.Fatalf("taze dal listelenmeli: %v", got)
	}
	if taze.state == "merged" || taze.gone || taze.prune || taze.histo {
		t.Fatalf("taze dal kalıntı sayılmamalı (state=%q gone=%v prunable=%v historyOnly=%v)", taze.state, taze.gone, taze.prune, taze.histo)
	}
	eski, ok := got["eski-ozellik"]
	if !ok || eski.state != "merged" || !eski.prune {
		t.Fatalf("gerçekten birleşmiş dal kalıntı olarak bulunmaya devam etmeli: %+v ok=%v", eski, ok)
	}
}

// HEAD'deki taze dal + kaydedilmemiş dosyalar (olayın birebir hâli): "Kalıntıyı Sil" çıkmamalı.
func TestFreshHeadBranchWithDirtyFilesIsNotALeftover(t *testing.T) {
	isoHome(t)
	dir, run := newTempRepo(t)
	run("checkout", "-q", "-b", "chore/yeni-is")
	if err := os.WriteFile(filepath.Join(dir, "calisiyorum.txt"), []byte("x\n"), 0644); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(dir)
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range snap.Branches {
		if b.Name == "chore/yeni-is" {
			if b.State == "merged" || b.OriginGone || b.Prunable || b.HistoryOnly {
				t.Fatalf("HEAD'deki taze dal kalıntı sayılmamalı: %+v", b)
			}
			return
		}
	}
	t.Fatal("HEAD dalı (taze) listelenmeli")
}
