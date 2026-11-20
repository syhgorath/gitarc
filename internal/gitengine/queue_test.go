package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitarc/internal/model"
)

// queueRepo: main + dört dal.
//
//	feat/a : file1'i değiştirir
//	feat/b : file1'i BAŞKA türlü değiştirir (main ile tek başına temiz, ama a birleşince çakışır)
//	feat/c : file2'yi değiştirir (bağımsız)
//	feat/d : file3'ü değiştirir, ama main de file3'ü sonradan değiştirdi (main ile tek başına çakışır)
func queueRepo(t *testing.T) string {
	t.Helper()
	isoHome(t)
	dir, run := newTempRepo(t)
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range []string{"file1.txt", "file2.txt", "file3.txt"} {
		write(n, "base\n")
	}
	run("add", "-A")
	run("commit", "-qm", "base files")
	branch := func(name, file, body string) {
		run("checkout", "-q", "-b", name, "main")
		write(file, body)
		run("add", "-A")
		run("commit", "-qm", name)
		run("checkout", "-q", "main")
	}
	branch("feat/a", "file1.txt", "A\n")
	branch("feat/b", "file1.txt", "B\n")
	branch("feat/c", "file2.txt", "C\n")
	branch("feat/d", "file3.txt", "D\n")
	write("file3.txt", "main changed\n")
	run("add", "-A")
	run("commit", "-qm", "main changes file3")
	return dir
}

func queueItemsByBranch(p model.QueuePlan) map[string]model.QueueItem {
	m := map[string]model.QueueItem{}
	for _, it := range p.Items {
		m[it.Branch] = it
	}
	return m
}

// Çekirdek: SIRA önemlidir. b tek başına temiz, ama a'dan SONRA çakışır; kuyruk orada durur, c bekler.
func TestQueuePlanLiveIsSequentialNotPerBranch(t *testing.T) {
	dir := queueRepo(t)
	mgr := switchLive(t, dir)

	plan, err := mgr.QueuePlan([]string{"feat/a", "feat/b", "feat/c"})
	if err != nil {
		t.Fatal(err)
	}
	it := queueItemsByBranch(plan)
	if it["feat/a"].Status != "clean" {
		t.Fatalf("a temiz birleşmeli: %+v", it["feat/a"])
	}
	b := it["feat/b"]
	if b.Status != "conflict" || b.ConflictsOnMain {
		t.Fatalf("b yalnızca a'dan SONRA çakışmalı (main ile tek başına temiz): %+v", b)
	}
	if len(b.ConflictFiles) != 1 || b.ConflictFiles[0] != "file1.txt" {
		t.Fatalf("çakışan dosya file1.txt olmalı: %v", b.ConflictFiles)
	}
	if len(b.ConflictsWithBranches) != 1 || b.ConflictsWithBranches[0] != "feat/a" {
		t.Fatalf("çakışmanın sebebi önceki feat/a olmalı: %v", b.ConflictsWithBranches)
	}
	c := it["feat/c"]
	if c.Status != "clean" || !c.AfterBlocker {
		t.Fatalf("c temiz ama engelin ARKASINDA beklemeli: %+v", c)
	}
	if plan.StopsAt != 2 || plan.Mergeable != 1 || plan.Mode != "live" {
		t.Fatalf("kuyruk 2. sırada durmalı, 1 dal birleşebilir: stopsAt=%d mergeable=%d mode=%s", plan.StopsAt, plan.Mergeable, plan.Mode)
	}
	if !strings.Contains(strings.Join(plan.Notes, "\n"), "2. sırada") {
		t.Fatalf("not, kuyruğun nerede durduğunu söylemeli: %v", plan.Notes)
	}
}

// Sıra değişince sonuç değişir: b önce gelirse sorun a'dadır.
func TestQueuePlanOrderChangesTheOutcome(t *testing.T) {
	mgr := switchLive(t, queueRepo(t))
	plan, err := mgr.QueuePlan([]string{"feat/b", "feat/a", "feat/c"})
	if err != nil {
		t.Fatal(err)
	}
	it := queueItemsByBranch(plan)
	if it["feat/b"].Status != "clean" || it["feat/a"].Status != "conflict" {
		t.Fatalf("b önce gelince a çakışmalı: b=%+v a=%+v", it["feat/b"], it["feat/a"])
	}
	if got := it["feat/a"].ConflictsWithBranches; len(got) != 1 || got[0] != "feat/b" {
		t.Fatalf("a'nın çakışma sebebi feat/b olmalı: %v", got)
	}
}

// Bağımsız dallar tümüyle temiz: kuyruk durmaz.
func TestQueuePlanFullyCleanQueueNeverStops(t *testing.T) {
	mgr := switchLive(t, queueRepo(t))
	plan, err := mgr.QueuePlan([]string{"feat/a", "feat/c"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.StopsAt != 0 || plan.Mergeable != 2 {
		t.Fatalf("temiz kuyruk durmamalı: stopsAt=%d mergeable=%d", plan.StopsAt, plan.Mergeable)
	}
	for _, it := range plan.Items {
		if it.AfterBlocker || it.Status != "clean" {
			t.Fatalf("hiçbir dal engelde olmamalı: %+v", it)
		}
	}
}

// Kuyruğa girmeden de main ile çakışan dal ayrı söylenir (sebep kuyruk değil).
func TestQueuePlanDistinguishesConflictWithMainFromConflictWithQueue(t *testing.T) {
	mgr := switchLive(t, queueRepo(t))
	plan, err := mgr.QueuePlan([]string{"feat/d", "feat/c"})
	if err != nil {
		t.Fatal(err)
	}
	d := queueItemsByBranch(plan)["feat/d"]
	if d.Status != "conflict" || !d.ConflictsOnMain || len(d.ConflictsWithBranches) != 0 {
		t.Fatalf("d tek başına main ile çakışmalı, kuyruktaki biri sebep olmamalı: %+v", d)
	}
	if plan.StopsAt != 1 || plan.Mergeable != 0 {
		t.Fatalf("kuyruk ilk sırada durmalı: %d/%d", plan.StopsAt, plan.Mergeable)
	}
}

// SIFIR DOKUNUŞ: plan çalışma ağacına, indekse, ref'lere ve HEAD'e dokunmaz; kirli dosya olduğu gibi kalır.
func TestQueuePlanNeverTouchesWorkingTreeIndexOrRefs(t *testing.T) {
	dir := queueRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "kirli.txt"), []byte("benim işim\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "file2.txt"), []byte("yerel değişiklik\n"), 0644); err != nil {
		t.Fatal(err)
	}
	snapshotState := func() string {
		return strings.Join([]string{
			isoGit(t, dir, "status", "--porcelain"),
			isoGit(t, dir, "for-each-ref"),
			isoGit(t, dir, "rev-parse", "HEAD"),
			isoGit(t, dir, "branch", "--show-current"),
		}, "\n--\n")
	}
	before := snapshotState()
	mgr := switchLive(t, dir)
	if _, err := mgr.QueuePlan([]string{"feat/a", "feat/b", "feat/c", "feat/d"}); err != nil {
		t.Fatal(err)
	}
	if after := snapshotState(); after != before {
		t.Fatalf("plan depoyu değiştirdi:\nÖNCE:\n%s\nSONRA:\n%s", before, after)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "kirli.txt")); string(b) != "benim işim\n" {
		t.Fatal("kirli dosya korunmalı")
	}
}

func TestQueuePlanRejectsUnknownAndMergedBranches(t *testing.T) {
	dir := queueRepo(t)
	isoGit(t, dir, "merge", "-q", "--no-ff", "-m", "merge c", "feat/c") // c birleşti
	mgr := switchLive(t, dir)
	if _, err := mgr.QueuePlan([]string{"feat/yok"}); err == nil || !strings.Contains(err.Error(), "bulunamadı") {
		t.Fatalf("bilinmeyen dal reddedilmeli: %v", err)
	}
	if _, err := mgr.QueuePlan([]string{"feat/a", "feat/c"}); err == nil || !strings.Contains(err.Error(), "açık değil") {
		t.Fatalf("birleşmiş dal reddedilmeli: %v", err)
	}
}

func TestQueuePlanWithNoOpenPRsIsEmptyNotAnError(t *testing.T) {
	mgr := switchLive(t, queueRepo(t)) // yerel depo: PR yok, boş seçim = tüm açık PR'lar = hiçbiri
	plan, err := mgr.QueuePlan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 0 || plan.StopsAt != 0 || plan.Mergeable != 0 || !strings.Contains(strings.Join(plan.Notes, " "), "yok") {
		t.Fatalf("boş plan beklenir: %+v", plan)
	}
}

// ---- git'siz (simülasyon / tarayıcı yedeği) mantığı ----

func qBranch(id string, pr int, ci string, files []string) model.BranchArc {
	b := model.BranchArc{ID: id, Name: id, State: "open", ChangedFiles: files, PR: &model.PRStatus{Number: pr, Title: id}}
	if ci != "" {
		b.PR.CI = &model.CIStatus{State: ci}
	}
	return b
}

func TestQueuePlanFromSnapshotDataUsesQueueOrderAndSharedFiles(t *testing.T) {
	a := qBranch("a", 1, "SUCCESS", []string{"x.go", "y.go"})
	b := qBranch("b", 2, "SUCCESS", []string{"y.go"}) // a ile ortak: a'dan SONRA çakışır
	c := qBranch("c", 3, "RUNNING", []string{"z.go"})
	strict := true
	plan, err := QueuePlanFromSnapshotData("simulation", "main", []model.BranchArc{a, b, c}, []string{"a", "b", "c"}, &strict)
	if err != nil {
		t.Fatal(err)
	}
	it := queueItemsByBranch(plan)
	if it["a"].Status != "clean" || it["b"].Status != "conflict" || it["c"].Status != "clean" {
		t.Fatalf("a temiz, b çakışır, c temiz: %+v", plan.Items)
	}
	if got := it["b"].ConflictsWith; len(got) != 1 || got[0] != 1 || it["b"].ConflictFiles[0] != "y.go" {
		t.Fatalf("b, #1 ile y.go'da çakışmalı: %+v", it["b"])
	}
	if plan.StopsAt != 2 || plan.Mergeable != 1 || !it["c"].AfterBlocker {
		t.Fatalf("durma/engel: %d %d %v", plan.StopsAt, plan.Mergeable, it["c"].AfterBlocker)
	}
	// Ters sıra: b önce gelirse a çakışır.
	plan2, _ := QueuePlanFromSnapshotData("simulation", "main", []model.BranchArc{a, b, c}, []string{"b", "a"}, &strict)
	if it2 := queueItemsByBranch(plan2); it2["b"].Status != "clean" || it2["a"].Status != "conflict" {
		t.Fatalf("ters sırada sonuç değişmeli: %+v", plan2.Items)
	}
	if plan.CIRounds != 3 {
		t.Fatalf("'dal güncel olmalı' açıkken her dal için bir CI turu: %d", plan.CIRounds)
	}
	joined := strings.Join(plan.Notes, "\n")
	if !strings.Contains(joined, "CI sürüyor") || !strings.Contains(joined, "ardışık CI turu") {
		t.Fatalf("notlar CI durumunu ve tur sayısını söylemeli:\n%s", joined)
	}
}

func TestQueuePlanFromSnapshotDataHonestAboutUnknownCIAndMainConflicts(t *testing.T) {
	a := qBranch("a", 1, "", []string{"x.go"}) // CI bilinmiyor
	a.Conflict = &model.ConflictDetail{HasConflict: true, Files: []string{"x.go"}}
	plan, err := QueuePlanFromSnapshotData("simulation", "main", []model.BranchArc{a}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	it := plan.Items[0]
	if it.Status != "conflict" || !it.ConflictsOnMain || it.CI != "" {
		t.Fatalf("main ile çakışma ve bilinmeyen CI: %+v", it)
	}
	joined := strings.Join(plan.Notes, "\n")
	if !strings.Contains(joined, "bilinmiyor (yeşil varsayılmaz)") || !strings.Contains(joined, "okunamadı") {
		t.Fatalf("bilinmeyen CI ve koruma dürüstçe söylenmeli:\n%s", joined)
	}
}

func TestQueuePlanDefaultsToOpenPRsByNumber(t *testing.T) {
	bs := []model.BranchArc{qBranch("late", 9, "SUCCESS", nil), qBranch("early", 2, "SUCCESS", nil), {ID: "nopr", Name: "nopr", State: "open"}, {ID: "old", Name: "old", State: "merged", PR: &model.PRStatus{Number: 1}}}
	plan, err := QueuePlanFromSnapshotData("simulation", "main", bs, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 2 || plan.Items[0].Branch != "early" || plan.Items[1].Branch != "late" {
		t.Fatalf("yalnızca açık PR'lar, PR numarasına göre: %+v", plan.Items)
	}
}

func TestAssembleQueuePlanInvariants(t *testing.T) {
	items := []model.QueueItem{{Position: 1, Status: "clean"}, {Position: 2, Status: "unknown", Reason: "?"}, {Position: 3, Status: "clean"}}
	p := assembleQueuePlan("live", "main", items, nil)
	if p.StopsAt != 2 || p.Mergeable != 1 || p.Items[0].AfterBlocker || p.Items[1].AfterBlocker || !p.Items[2].AfterBlocker {
		t.Fatalf("bilinmeyen durum da kuyruğu durdurur (yeşil varsayılmaz): %+v", p)
	}
	if p := assembleQueuePlan("live", "main", nil, nil); p.Items == nil || len(p.Items) != 0 {
		t.Fatal("boş plan JSON'da null değil boş dizi olmalı")
	}
}

func TestQueuePlanInSimulation(t *testing.T) {
	plan, err := NewManager("").QueuePlan(nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "simulation" || plan.BaseBranch == "" {
		t.Fatalf("simülasyon planı: %+v", plan)
	}
	if plan.StopsAt < 0 || plan.StopsAt > len(plan.Items) || plan.Mergeable > len(plan.Items) {
		t.Fatalf("tutarsız sayılar: %+v", plan)
	}
}
