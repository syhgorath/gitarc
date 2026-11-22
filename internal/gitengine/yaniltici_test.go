package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

func mustBranch(t *testing.T, snap model.CockpitSnapshot, name string) model.BranchArc {
	t.Helper()
	for _, b := range snap.Branches {
		if b.Name == name {
			return b
		}
	}
	t.Fatalf("%s listelenmeli (dallar: %d)", name, len(snap.Branches))
	return model.BranchArc{}
}

// Olayın birebir hâli: `git checkout -b x origin/main` dalı origin/main'i İZLEYEN yapar. Bu dalın kendi uzak kopyası yoktur;
// "uzakta var → silinmiş kalıntı" sayılıp üzerinde çalışılan dal silinecek gibi gösteriliyordu.
func TestBranchTrackingMainIsNotItsOwnRemoteNorALeftover(t *testing.T) {
	isoHome(t)
	dir, run := newGitHubLikeRepo(t)
	run("checkout", "-q", "-b", "feat/yeni-is", "origin/main") // upstream = origin/main
	if err := os.WriteFile(filepath.Join(dir, "calisiyorum.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := NewManager(dir).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	b := mustBranch(t, snap, "feat/yeni-is")
	if b.HasRemote || b.RemoteRef != "" {
		t.Fatalf("main'i izleyen dalın kendi uzak kopyası yok: hasRemote=%v remoteRef=%q", b.HasRemote, b.RemoteRef)
	}
	if b.State == "merged" || b.OriginGone || b.Prunable || b.HistoryOnly {
		t.Fatalf("üzerinde çalışılan yeni dal kalıntı/geçmiş sayılmamalı: state=%s gone=%v prunable=%v history=%v", b.State, b.OriginGone, b.Prunable, b.HistoryOnly)
	}
	if !b.IsHEAD || b.UncommittedCount == 0 {
		t.Fatalf("HEAD ve kirli dosya bilgisi korunmalı: %+v", b)
	}
	for _, d := range snap.Diagnostics {
		if d.BranchName == "feat/yeni-is" && d.Kind == "prune_candidate" {
			t.Fatalf("Radar bu dal için silinmiş-kalıntı kartı açmamalı: %+v", d)
		}
	}
	// gerçek bir kalıntı hâlâ yakalanır (gerileme koruması): pushlanıp main'e birleşmiş + uzakta silinmiş dal
	run("checkout", "-q", "main")
	run("checkout", "-q", "-b", "eski")
	if err := os.WriteFile(filepath.Join(dir, "e.txt"), []byte("e\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "e.txt")
	run("commit", "-qm", "eski")
	run("push", "-q", "-u", "origin", "eski")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge eski", "eski")
	run("push", "-q", "origin", "main")
	run("push", "-q", "origin", "--delete", "eski")
	run("fetch", "-q", "--prune", "origin")
	snap2, _ := NewManager(dir).Snapshot()
	if e := mustBranch(t, snap2, "eski"); !e.Prunable {
		t.Fatalf("gerçekten birleşip uzakta silinen dal temizlenebilir kalmalı: %+v", e)
	}
}

func TestTracksDefaultBranch(t *testing.T) {
	cases := map[string]bool{"origin/main": true, "upstream/main": true, "origin/feat/x": false, "origin/main-x": false, "": false, "main": false}
	for up, want := range cases {
		if got := tracksDefaultBranch(up, "main"); got != want {
			t.Errorf("tracksDefaultBranch(%q)=%v want %v", up, got, want)
		}
	}
}

// Başka klasörde (git worktree) açık dalı git silmeyi reddeder: "temizle" adayı sayılmaz ve nedeni söylenir.
func TestBranchOpenInAnotherWorktreeIsNotPrunableAndSaysWhere(t *testing.T) {
	isoHome(t)
	dir, run := newTempRepo(t)
	run("checkout", "-q", "-b", "bitti")
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "bitti")
	run("checkout", "-q", "main")
	run("merge", "-q", "--no-ff", "-m", "merge bitti", "bitti")
	wt := filepath.Join(t.TempDir(), "baska-klasor")
	run("worktree", "add", "-q", wt, "bitti")
	snap, err := NewManager(dir).Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	b := mustBranch(t, snap, "bitti")
	if b.Prunable {
		t.Fatalf("başka klasörde açık dal temizlenebilir sayılmamalı: %+v", b)
	}
	if b.WorktreePath == "" || !sameRepoPath(b.WorktreePath, wt) {
		t.Fatalf("worktree yolu bildirilmeli: %q (beklenen %q)", b.WorktreePath, wt)
	}
	// ana klasörde açık dal (main) kendi klasörü için işaretlenmez
	if m := worktreeBranches(t.Context(), dir); m["main"] != "" {
		t.Fatalf("ana klasörün dalı 'başka klasör' sayılmamalı: %v", m)
	}
	// worktree kaldırılınca dal yine temizlenebilir (kalıntı olmaktan çıkmaz)
	run("worktree", "remove", "--force", wt)
	snap2, _ := NewManager(dir).Snapshot()
	if b2 := mustBranch(t, snap2, "bitti"); !b2.Prunable || b2.WorktreePath != "" {
		t.Fatalf("worktree kalkınca temizlenebilir olmalı: %+v", b2)
	}
}

// Stash gizli iş saklar: Radar'da bilgi kartı çıkar (silme/uygulama önerilmez); yoksa kart yok.
func TestStashesAreListedInRadarAndSnapshot(t *testing.T) {
	isoHome(t)
	dir, run := newTempRepo(t)
	snap, _ := NewManager(dir).Snapshot()
	if len(snap.Repo.Stashes) != 0 {
		t.Fatal("stash yokken liste boş olmalı")
	}
	for _, d := range snap.Diagnostics {
		if d.Kind == "stash" {
			t.Fatal("stash yokken kart olmamalı")
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# degisti\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("stash", "push", "-m", "unutulan is")
	snap, _ = NewManager(dir).Snapshot()
	if len(snap.Repo.Stashes) != 1 || !strings.Contains(snap.Repo.Stashes[0].Message, "unutulan is") || snap.Repo.Stashes[0].Ref != "stash@{0}" {
		t.Fatalf("stash listelenmeli: %+v", snap.Repo.Stashes)
	}
	var card *model.DiagnosticItem
	for i := range snap.Diagnostics {
		if snap.Diagnostics[i].Kind == "stash" {
			card = &snap.Diagnostics[i]
		}
	}
	if card == nil || !strings.Contains(card.Detail, "unutulan is") || strings.Contains(card.ActionHint, "drop") || strings.Contains(card.ActionHint, "pop") {
		t.Fatalf("stash kartı bilgi vermeli, silme/uygulama önermemeli: %+v", card)
	}
}

// Kaydedilmemiş iş varken önerilen komut dalı SİLMEZ ve stash'e göndermez.
func TestSuggestedCommandNeverDeletesOrStashesUnsavedWork(t *testing.T) {
	b := model.BranchArc{Name: "feat/x", State: "merged", HasLocal: true, IsHEAD: true, UncommittedCount: 15}
	cmd := BuildSuggestedCommand(b, "main")
	for _, bad := range []string{"branch -D", "stash", "git switch"} {
		if strings.Contains(cmd, bad) && bad != "stash" {
			t.Fatalf("kirli HEAD için %q önerilmemeli: %s", bad, cmd)
		}
	}
	if strings.Contains(cmd, "git stash") || !strings.Contains(cmd, "commit") {
		t.Fatalf("stash değil commit önerilmeli: %s", cmd)
	}
	// temiz HEAD'de eski davranış (gerileme koruması)
	b.UncommittedCount = 0
	if got := BuildSuggestedCommand(b, "main"); !strings.Contains(got, "git branch -D feat/x") {
		t.Fatalf("temiz kalıntıda silme komutu sürmeli: %s", got)
	}
}
