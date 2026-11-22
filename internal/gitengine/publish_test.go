package gitengine

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Yerel (pushlanmamış) iki dal: plan ikisini "push_pr" gösterir; yürütme ikisini pushlar ve PR açar; parmak izi/seçim kuralları geçerlidir.
func publishWorld(t *testing.T) (dir string, run func(args ...string), gh *ghStub) {
	t.Helper()
	isoHome(t)
	gh = newGHStub(t)
	dir, run = newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/a")
	addFeatureBranch(t, dir, run, "feat/b")
	return dir, run, gh
}

func TestPublishPlanListsLocalBranchesAndIsReadOnly(t *testing.T) {
	dir, _, gh := publishWorld(t)
	mgr := switchLive(t, dir)
	plan, err := mgr.PublishPlan(nil)
	if err != nil || len(plan.Items) != 2 || !plan.Publishable || plan.Fingerprint == "" {
		t.Fatalf("iki yerel dal yayınlanabilir olmalı: %+v %v", plan, err)
	}
	for _, it := range plan.Items {
		if it.Action != "push_pr" || it.Title == "" || it.Commits != 1 {
			t.Fatalf("pushla + PR aç, başlık dolu: %+v", it)
		}
	}
	// SALT OKUNUR: hiçbir şey pushlanmadı, gh'ye PR açma çağrısı gitmedi
	if out := mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/*"); strings.TrimSpace(out) != "" {
		t.Fatalf("plan hiçbir şey pushlamamalı: %q", out)
	}
	if gh.called("pr", "create") {
		t.Fatal("plan PR açmamalı")
	}
	// ana dal / kirli çalışma alanı / olmayan dal yayınlanamaz
	one, err := mgr.PublishPlan([]string{plan.Items[0].BranchID})
	if err != nil || len(one.Items) != 1 || !one.Publishable {
		t.Fatalf("tek dal seçimi: %+v %v", one, err)
	}
	if _, err := mgr.PublishPlan([]string{"yok-boyle-dal"}); err == nil {
		t.Fatal("bilinmeyen dal reddedilmeli")
	}
}

func TestPublishRunPushesEveryBranchOpensPRsAndRefusesStaleConfirm(t *testing.T) {
	dir, _, gh := publishWorld(t)
	mgr := switchLive(t, dir)
	plan, _ := mgr.PublishPlan(nil)

	if _, err := mgr.PublishRun(nil, "yanlis", false); err == nil || !strings.Contains(err.Error(), "onaylanmadı") {
		t.Fatalf("yanlış parmak izi reddedilmeli: %v", err)
	}
	if out := mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/*"); strings.TrimSpace(out) != "" || gh.called("pr", "create") {
		t.Fatal("reddedilen yayın hiçbir şey yapmamalı")
	}

	res, err := mgr.PublishRun(nil, plan.Fingerprint, false)
	if err != nil || !res.Completed || len(res.Steps) != 2 {
		t.Fatalf("yayın tamamlanmalı: %+v %v", res, err)
	}
	remote := mustGit(t, dir, "ls-remote", "origin", "refs/heads/feat/a", "refs/heads/feat/b")
	if !strings.Contains(remote, "refs/heads/feat/a") || !strings.Contains(remote, "refs/heads/feat/b") {
		t.Fatalf("iki dal da origin'e pushlanmalı: %q", remote)
	}
	creates := 0
	for _, c := range gh.calls() {
		if strings.HasPrefix(c, "pr create") {
			creates++
			if !strings.Contains(c, "--head feat/") || strings.Contains(c, "--force") {
				t.Fatalf("PR gerçek dal adıyla açılmalı: %s", c)
			}
		}
	}
	if creates != 2 {
		t.Fatalf("iki PR açılmalı, açılan: %d", creates)
	}
	if res.QueueStarted || !strings.Contains(res.QueueNote, "istenmedi") {
		t.Fatalf("kuyruk istenmedi: %+v", res)
	}
	for _, s := range res.Steps {
		if !s.OK {
			t.Fatalf("adım başarılı olmalı: %+v", s)
		}
	}
}

// İlk sorunda durur: ikinci dalın push'u patlarsa ilk dal yayınlanmış kalır, sonuç dürüstçe "yarım" der, kuyruk başlamaz.
func TestPublishRunStopsAtTheFirstFailureAndSaysSo(t *testing.T) {
	dir, run, _ := publishWorld(t)
	mgr := switchLive(t, dir)
	plan, _ := mgr.PublishPlan(nil)
	// ikinci dalı origin'de yerelle ÇAKIŞACAK biçimde ilerlet (force yok: push reddedilir ya da birleştirme çakışmayı bildirir)
	other := t.TempDir()
	if out, err := runGitOut(other, "clone", "-q", strings.TrimSpace(mustGit(t, dir, "remote", "get-url", "origin")), "."); err != nil {
		t.Fatalf("clone: %v %s", err, out)
	}
	for _, a := range [][]string{{"config", "user.email", "o@o"}, {"config", "user.name", "o"}, {"checkout", "-q", "-b", "feat/b"}} {
		if out, err := runGitOut(other, a...); err != nil {
			t.Fatalf("%v: %v %s", a, err, out)
		}
	}
	// feat/b'nin KENDİ dosyasını (addFeatureBranch: feat_b.txt) farklı içerikle değiştir: yerel commit'le ÇAKIŞAN bir ıraksama. (Çakışmayan ıraksama
	// artık kendiliğinden birleştirilip pushlanır; yalnızca gerçek çakışma adımı durdurur.)
	if err := os.WriteFile(filepath.Join(other, "feat_b.txt"), []byte("uzaktaki baska surum\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][]string{{"add", "-A"}, {"commit", "-qm", "baska"}, {"push", "-q", "origin", "feat/b"}} {
		if out, err := runGitOut(other, a...); err != nil {
			t.Fatalf("%v: %v %s", a, err, out)
		}
	}
	_ = run
	res, err := mgr.PublishRun(nil, plan.Fingerprint, true)
	if err != nil {
		t.Fatalf("kısmi sonuç hata değil rapordur: %v", err)
	}
	if res.Completed || res.QueueStarted || len(res.Steps) != 2 {
		t.Fatalf("yarım kalmalı, kuyruk başlamamalı: %+v", res)
	}
	if !res.Steps[0].OK || res.Steps[1].OK || !(strings.Contains(res.Steps[1].Message, "push") || strings.Contains(res.Steps[1].Message, "ÇAKIŞIYOR")) {
		t.Fatalf("ilk dal yayınlanmış, ikincisi push hatasıyla durmalı: %+v", res.Steps)
	}
	if !strings.Contains(res.QueueNote, "durdu") {
		t.Fatalf("kuyruk notu durmayı söylemeli: %q", res.QueueNote)
	}
}

// Kuyruk: PR'lar anlık görüntüye gelince koşu başlar (gerçek akış: gh PR listesi arka planda gelir).
func TestPublishRunStartsTheQueueOnceThePRsAreVisible(t *testing.T) {
	dir, _, gh := publishWorld(t)
	fastRun(t)
	old := publishWait
	publishWait = 20 * time.Second
	t.Cleanup(func() { publishWait = old })
	mgr := switchLive(t, dir)
	plan, _ := mgr.PublishPlan(nil)

	// `gh pr create` çağrıldıktan sonra GitHub PR'ları listeler (gerçekte de PR'lar bir an sonra görünür)
	go func() {
		for i := 0; i < 400; i++ {
			n := 0
			for _, c := range gh.calls() {
				if strings.HasPrefix(c, "pr create") {
					n++
				}
			}
			if n >= 2 {
				gh.set("pr_list_open", fmt.Sprintf(`[{"number":1,"headRefName":"feat/a","title":"a","body":"","url":"https://github.com/acme/demo/pull/1","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]},
 {"number":2,"headRefName":"feat/b","title":"b","body":"","url":"https://github.com/acme/demo/pull/2","author":{"login":"a"},"reviewDecision":"","statusCheckRollup":[]}]`), 0)
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
	}()
	res, err := mgr.PublishRun(nil, plan.Fingerprint, true)
	if err != nil || !res.Completed {
		t.Fatalf("yayın: %+v %v", res, err)
	}
	t.Cleanup(func() { _, _ = mgr.StopQueueRun() })
	if !res.QueueStarted || !strings.Contains(res.QueueNote, "kuyruk koşusu başladı") {
		t.Fatalf("PR'lar görününce koşu başlamalı: %+v", res)
	}
	if res.Steps[0].PR != 1 || res.Steps[1].PR != 2 {
		t.Fatalf("PR numaraları sonuca işlenmeli: %+v", res.Steps)
	}
	if r, _ := mgr.QueueRunStatus(); r == nil || len(r.Items) != 2 {
		t.Fatalf("iki PR'lık koşu kaydı olmalı: %+v", r)
	}
}

// Varsayılan sıra KARARLI olmalı (dal adına göre): anlık görüntü sırası değişse bile plan aynı kalır.
func TestPublishDefaultOrderIsStable(t *testing.T) {
	dir, _, _ := publishWorld(t)
	mgr := switchLive(t, dir)
	for i := 0; i < 8; i++ {
		plan, err := mgr.PublishPlan(nil)
		if err != nil || len(plan.Items) != 2 || plan.Items[0].Branch != "feat/a" || plan.Items[1].Branch != "feat/b" {
			t.Fatalf("sıra dal adına göre kararlı olmalı: %+v %v", plan.Items, err)
		}
	}
}
