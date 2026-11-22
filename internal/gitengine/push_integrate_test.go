package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// divergedWorld: feat/x hem yerelde (pushlanmamış commit) hem uzakta (başka biri/kuyruk pushladı) ilerlemiş.
// otherFile/otherBody: uzaktaki commit'in dokunduğu dosya; localFile/localBody: yerel commit'in dokunduğu dosya.
func divergedWorld(t *testing.T, localFile, localBody, otherFile, otherBody string) (dir string, run func(args ...string), mgr *Manager, id string) {
	t.Helper()
	isoHome(t)
	gh := newGHStub(t)
	gh.set("pr_list_open", "[]", 0)
	dir, run = newGitHubLikeRepo(t)
	write := func(root, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(dir, "ortak.txt", "temel\n")
	run("add", "-A")
	run("commit", "-qm", "temel")
	run("push", "-q", "origin", "main")
	run("checkout", "-q", "-b", "feat/x")
	write(dir, "x.txt", "x\n")
	run("add", "-A")
	run("commit", "-qm", "x")
	run("push", "-q", "-u", "origin", "feat/x")

	// uzak ilerler (kuyruğun/başka birinin pushu): ikinci klon
	other := t.TempDir()
	url := strings.TrimSpace(mustGit(t, dir, "remote", "get-url", "origin"))
	for _, a := range [][]string{{"clone", "-q", url, "."}, {"config", "user.email", "o@o"}, {"config", "user.name", "o"}, {"checkout", "-q", "feat/x"}} {
		if out, err := runGitOut(other, a...); err != nil {
			t.Fatalf("%v: %v %s", a, err, out)
		}
	}
	write(other, otherFile, otherBody)
	for _, a := range [][]string{{"add", "-A"}, {"commit", "-qm", "uzak ilerledi"}, {"push", "-q", "origin", "feat/x"}} {
		if out, err := runGitOut(other, a...); err != nil {
			t.Fatalf("%v: %v %s", a, err, out)
		}
	}

	// yerel de ilerler (pushlanmamış)
	write(dir, localFile, localBody)
	run("add", "-A")
	run("commit", "-qm", "yerel ilerledi")

	mgr = switchLive(t, dir)
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range snap.Branches {
		if b.Name == "feat/x" && b.State == "open" && b.ParentBranch == "" {
			id = b.ID
		}
	}
	if id == "" {
		t.Fatal("feat/x listelenmeli")
	}
	return dir, run, mgr, id
}

func TestPromoteIntegratesADivergedRemoteBranchAndPushesWithoutForce(t *testing.T) {
	dir, run, mgr, id := divergedWorld(t, "yerel.txt", "y\n", "uzak.txt", "u\n")
	run("checkout", "-q", "main")
	// kullanıcının çalışma ağacındaki kirli dosya KORUNMALI (zero-touch)
	if err := os.WriteFile(filepath.Join(dir, "benim-isim.txt"), []byte("dokunma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainBefore := strings.TrimSpace(mustGit(t, dir, "rev-parse", "main"))

	if _, err := mgr.PromoteBranch(id); err != nil {
		t.Fatalf("ıraksayan dal kendiliğinden birleştirilip pushlanmalı (eskiden ham 'rejected' hatası): %v", err)
	}
	run("fetch", "-q", "origin")
	remoteTip := strings.TrimSpace(mustGit(t, dir, "rev-parse", "origin/feat/x"))
	localTip := strings.TrimSpace(mustGit(t, dir, "rev-parse", "feat/x"))
	if remoteTip != localTip {
		t.Fatalf("yerel ve uzak dal aynı uçta olmalı: yerel %s uzak %s", localTip, remoteTip)
	}
	for _, f := range []string{"x.txt", "yerel.txt", "uzak.txt"} {
		if out := mustGit(t, dir, "show", "origin/feat/x:"+f); strings.TrimSpace(out) == "" {
			t.Fatalf("%s birleşimde olmalı", f)
		}
	}
	if n := strings.TrimSpace(mustGit(t, dir, "rev-list", "--merges", "--count", "origin/feat/x")); n != "1" {
		t.Fatalf("tek bir birleşim commit'i beklenir: %s", n)
	}
	// çalışma ağacına dokunulmadı
	if got, _ := os.ReadFile(filepath.Join(dir, "benim-isim.txt")); string(got) != "dokunma\n" {
		t.Fatal("kullanıcının kirli dosyası korunmalı")
	}
	if strings.TrimSpace(mustGit(t, dir, "rev-parse", "main")) != mainBefore || currentBranch(t, dir) != "main" {
		t.Fatal("main/HEAD değişmemeli")
	}
}

func TestPromoteWithAConflictingDivergenceChangesNothingAndNamesTheFiles(t *testing.T) {
	dir, run, mgr, id := divergedWorld(t, "ortak.txt", "yerel surum\n", "ortak.txt", "uzak surum\n")
	run("checkout", "-q", "main")
	localBefore := strings.TrimSpace(mustGit(t, dir, "rev-parse", "feat/x"))
	remoteBefore := strings.TrimSpace(mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/x"))

	_, err := mgr.PromoteBranch(id)
	if err == nil || !strings.Contains(err.Error(), "ÇAKIŞIYOR") || !strings.Contains(err.Error(), "ortak.txt") || !strings.Contains(err.Error(), "Hiçbir şey değiştirilmedi") {
		t.Fatalf("çakışma açıklayıcı bildirilmeli (dosya adıyla): %v", err)
	}
	if strings.Contains(err.Error(), "[rejected]") {
		t.Fatalf("ham git çıktısı sızmamalı: %v", err)
	}
	if strings.TrimSpace(mustGit(t, dir, "rev-parse", "feat/x")) != localBefore {
		t.Fatal("yerel dal değişmemeli")
	}
	if strings.TrimSpace(mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/x")) != remoteBefore {
		t.Fatal("uzak dal değişmemeli (zorla push yok)")
	}
	if out := mustGit(t, dir, "worktree", "list"); strings.Contains(out, "gitarc-integrate") {
		t.Fatalf("geçici worktree kalmamalı: %s", out)
	}
}

// Aktif (HEAD) dalda: kaydedilmemiş, ilgisiz dosya korunur; HEAD uzağa yetişir.
func TestPromoteIntegratesWhenTheDivergedBranchIsCheckedOutAndKeepsUnrelatedDirtyFiles(t *testing.T) {
	dir, _, mgr, id := divergedWorld(t, "yerel.txt", "y\n", "uzak.txt", "u\n")
	if err := os.WriteFile(filepath.Join(dir, "taslak.txt"), []byte("kaydedilmemis\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = id
	// kirli HEAD dalı Toplu Yayınla'da engellenir; burada doğrudan promote davranışı sınanır (yalnızca ilgisiz kirli dosya)
	snap, _ := mgr.Snapshot()
	var hid string
	for _, b := range snap.Branches {
		if b.Name == "feat/x" && b.ParentBranch == "" && b.State == "open" {
			hid = b.ID
		}
	}
	if _, err := mgr.PromoteBranch(hid); err != nil {
		t.Fatalf("HEAD dalı için de çalışmalı: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "taslak.txt")); string(got) != "kaydedilmemis\n" {
		t.Fatal("ilgisiz kirli dosya korunmalı")
	}
	if _, err := os.Stat(filepath.Join(dir, "uzak.txt")); err != nil {
		t.Fatalf("HEAD uzağa yetişmeli (uzak.txt çalışma ağacına gelmeli): %v", err)
	}
	if currentBranch(t, dir) != "feat/x" {
		t.Fatal("HEAD aynı dalda kalmalı")
	}
}

func TestIsPushRejectedNonFastForward(t *testing.T) {
	yes := []string{" ! [rejected]        feat/x -> feat/x (fetch first)", "! [rejected] a -> a (non-fast-forward)", "Updates were rejected because the remote contains work that you do not"}
	no := []string{"fatal: unable to access 'https://x': Could not resolve host", "remote: Permission denied", "error: src refspec feat/y does not match any"}
	for _, s := range yes {
		if !isPushRejectedNonFastForward(s) {
			t.Errorf("reddedilmeli (birleştirilebilir): %q", s)
		}
	}
	for _, s := range no {
		if isPushRejectedNonFastForward(s) {
			t.Errorf("başka hata, birleştirmeye çalışılmamalı: %q", s)
		}
	}
}
