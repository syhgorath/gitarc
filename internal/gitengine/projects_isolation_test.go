package gitengine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gitarc/internal/model"
)

// Proje algoritması denetimi: birden çok proje açıkken HİÇBİR durum projeler arasında sızmamalı.
// Her test ayrı bir HOME kullanır (kayıtlı projeler/günlükler/baz dal sabitleri birbirine karışmasın).

func isoHome(t *testing.T) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("USERPROFILE", h)
}

func isoSame(a, b string) bool {
	n := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	return n(a) == n(b)
}

func isoGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.x", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (%s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// isoRepo: kendine özgü bir depo. unique=dalı yalnızca bu depoda olan, shared=her depoda var ama içeriği farklı.
func isoRepo(t *testing.T, label string) string {
	t.Helper()
	dir, run := newTempRepo(t)
	write := func(name, s string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("checkout", "-q", "-b", "feat/only-"+label)
	write("only-"+label+".txt", label+"\n")
	run("add", "-A")
	run("commit", "-qm", "only "+label)
	run("checkout", "-q", "main")
	run("checkout", "-q", "-b", "feat/shared")
	write("shared.txt", "shared body of "+label+"\n")
	run("add", "-A")
	run("commit", "-qm", "shared in "+label)
	run("checkout", "-q", "main")
	return dir
}

type isoWorld struct {
	mgr  *Manager
	dirs map[string]string // etiket -> yol
	ids  map[string]string // etiket -> proje kimliği
}

func newIsoWorld(t *testing.T, labels ...string) *isoWorld {
	t.Helper()
	isoHome(t)
	w := &isoWorld{dirs: map[string]string{}, ids: map[string]string{}}
	for _, l := range labels {
		w.dirs[l] = isoRepo(t, l)
	}
	w.mgr = NewManager(w.dirs[labels[0]])
	var snap model.CockpitSnapshot
	var err error
	for _, l := range labels[1:] {
		if snap, err = w.mgr.AddProject(w.dirs[l], l); err != nil {
			t.Fatalf("AddProject %s: %v", l, err)
		}
	}
	if len(labels) == 1 {
		snap, _ = w.mgr.Snapshot()
	}
	for _, ro := range snap.Repos {
		for l, d := range w.dirs {
			if isoSame(ro.Path, d) {
				w.ids[l] = ro.ID
			}
		}
	}
	for _, l := range labels {
		if w.ids[l] == "" {
			t.Fatalf("%s seçicide yok: %+v", l, snap.Repos)
		}
	}
	return w
}

func (w *isoWorld) switchTo(t *testing.T, label string) model.CockpitSnapshot {
	t.Helper()
	snap, err := w.mgr.SwitchRepo(w.ids[label])
	if err != nil {
		t.Fatalf("SwitchRepo(%s): %v", label, err)
	}
	return snap
}

func branchNames(s model.CockpitSnapshot) map[string]bool {
	m := map[string]bool{}
	for _, b := range s.Branches {
		m[b.Name] = true
	}
	return m
}

// 1) Anlık görüntüler karışmaz: her projede yalnızca kendi dalları, doğru kimlik/yol; aynı adlı dal başka projedeki
// içeriği göstermez; art arda geçişlerde de.
func TestIso_SnapshotsNeverMixProjects(t *testing.T) {
	w := newIsoWorld(t, "a", "b", "c")
	order := []string{"a", "b", "c", "a", "c", "b", "a", "b"}
	for i, l := range order {
		snap := w.switchTo(t, l)
		if snap.Repo.ID != w.ids[l] || !isoSame(snap.Repo.Path, w.dirs[l]) {
			t.Fatalf("[%d] %s: yanlış proje görüntüsü: %s %s", i, l, snap.Repo.ID, snap.Repo.Path)
		}
		got := branchNames(snap)
		if !got["feat/only-"+l] {
			t.Fatalf("[%d] %s: kendi dalı (feat/only-%s) yok: %v", i, l, l, got)
		}
		for _, other := range []string{"a", "b", "c"} {
			if other != l && got["feat/only-"+other] {
				t.Fatalf("[%d] %s projesinde %s'nin dalı göründü: %v", i, l, other, got)
			}
		}
		for _, b := range snap.Branches {
			if b.Name != "feat/shared" {
				continue
			}
			joined := strings.Join(b.ChangedFiles, ",")
			if !strings.Contains(joined, "shared.txt") {
				t.Fatalf("[%d] %s: shared dalının değişen dosyası yok: %v", i, l, b.ChangedFiles)
			}
			if b.LastCommitMsg != "" && !strings.Contains(b.LastCommitMsg, "shared in "+l) {
				t.Fatalf("[%d] %s: shared dalı BAŞKA projenin commit'ini gösteriyor: %q", i, l, b.LastCommitMsg)
			}
		}
	}
}

// 2) Kirli dosyalar yalnızca kendi projesinde.
func TestIso_DirtyFilesStayInTheirProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	if err := os.WriteFile(filepath.Join(w.dirs["a"], "kirli-a.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if s := w.switchTo(t, "b"); s.Repo.DirtyCount != 0 {
		t.Fatalf("b'nin kirli dosyası olmamalı, a'nınki göründü: %d", s.Repo.DirtyCount)
	}
	if s := w.switchTo(t, "a"); s.Repo.DirtyCount != 1 {
		t.Fatalf("a'nın 1 kirli dosyası olmalı: %d", s.Repo.DirtyCount)
	}
}

// 3) Değiştirici işlemler yalnızca etkin projeye dokunur.
func TestIso_MutationsOnlyTouchTheActiveProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b", "c")
	w.switchTo(t, "b")
	if _, err := w.mgr.CreateBranch("feat/yalnizca-b", "lower", ""); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	for _, l := range []string{"a", "c"} {
		if out := isoGit(t, w.dirs[l], "branch", "--list", "feat/yalnizca-b"); out != "" {
			t.Fatalf("%s projesinde b'nin dalı oluştu: %q", l, out)
		}
	}
	if out := isoGit(t, w.dirs["b"], "branch", "--list", "feat/yalnizca-b"); out == "" {
		t.Fatal("dal etkin projede (b) oluşmalıydı")
	}
}

// 4) Günlükler (hata/işlem kayıtları) proje başına olmalı: a'daki bir hata b'nin günlüğünde görünmemeli.
func TestIso_LogsArePerProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	// a'da bilerek başarısız bir işlem (var olmayan dalı sil)
	_, _ = w.mgr.DeleteBranch("yok-boyle-bir-dal")
	snapA, _ := w.mgr.Snapshot()
	if len(snapA.Logs) == 0 {
		t.Skip("test düzeneği: a'da günlük oluşmadı")
	}
	snapB := w.switchTo(t, "b")
	for _, l := range snapB.Logs {
		if strings.Contains(l.Branch+l.Message, "yok-boyle-bir-dal") {
			t.Fatalf("b projesinin günlüğünde a'nın işlemi görünüyor: %+v", l)
		}
	}
}

// 5) Öneri kartı proje başına.
func TestIso_ProposalIsPerProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	if err := os.WriteFile(filepath.Join(w.dirs["a"], "yeni.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	prop, err := w.mgr.ProposeCommit("feat/a-karti", "feat: a", []string{"yeni.txt"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if s := w.switchTo(t, "b"); s.PendingProposal != nil {
		t.Fatalf("b projesinde a'nın kartı görünüyor: %+v", s.PendingProposal.BranchName)
	}
	if _, err := w.mgr.ApproveProposal(prop.ID, "", "", "", ""); err == nil {
		t.Fatal("b açıkken a'nın kartı onaylanabildi")
	}
	if out := isoGit(t, w.dirs["b"], "branch", "--list", "feat/a-karti"); out != "" {
		t.Fatalf("b projesinde a'nın kartının dalı açıldı: %q", out)
	}
	if s := w.switchTo(t, "a"); s.PendingProposal == nil {
		t.Fatal("a'ya dönünce kart yeniden görünmeli")
	}
}

// 6) Baz (omurga) dal sabitleme proje başına.
func TestIso_BaseBranchPinIsPerProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	isoGit(t, w.dirs["a"], "branch", "develop")
	if _, err := w.mgr.SetBaseBranch("develop"); err != nil {
		t.Fatalf("SetBaseBranch: %v", err)
	}
	if s := w.switchTo(t, "b"); s.Repo.BaseBranchPinned || s.Repo.DefaultBranch == "develop" {
		t.Fatalf("a'nın baz dalı b'ye sızdı: pinned=%v default=%q", s.Repo.BaseBranchPinned, s.Repo.DefaultBranch)
	}
	if s := w.switchTo(t, "a"); !s.Repo.BaseBranchPinned || s.Repo.DefaultBranch != "develop" {
		t.Fatalf("a'nın sabiti korunmalı: pinned=%v default=%q", s.Repo.BaseBranchPinned, s.Repo.DefaultBranch)
	}
}

// 7) Kaldır / yeniden ekle: etkin proje geçerli kalır, kayıtlı projeler bellektekiyle aynı kimliklere sahip,
// yeniden başlatınca aynı kimlikler gelir.
func TestIso_RemoveAndReAddKeepsStateConsistent(t *testing.T) {
	w := newIsoWorld(t, "a", "b", "c")
	w.switchTo(t, "b")
	snap, err := w.mgr.RemoveProject(w.ids["b"])
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.ID == w.ids["b"] || snap.Repo.ID == "" {
		t.Fatalf("etkin proje kaldırılınca başka geçerli bir projeye düşmeli: %+v", snap.Repo)
	}
	for _, ro := range snap.Repos {
		if ro.ID == w.ids["b"] {
			t.Fatal("kaldırılan proje seçicide kalmamalı")
		}
	}
	// kayıtlı dosya bellekle aynı
	saved := loadSavedProjects()
	for _, sp := range saved {
		if isoSame(sp.Path, w.dirs["b"]) {
			t.Fatalf("kaldırılan proje projects.json'da kalmış: %+v", sp)
		}
	}

	snap2, err := w.mgr.AddProject(w.dirs["b"], "b")
	if err != nil {
		t.Fatal(err)
	}
	newID := snap2.Repo.ID
	if _, err := w.mgr.SwitchRepo(newID); err != nil {
		t.Fatalf("yeniden eklenen proje seçilebilmeli: %v", err)
	}
	// kayıttaki kimlik bellekteki kimlikle AYNI olmalı (aksi hâlde yeniden başlatınca arayüzün hatırladığı kimlik tutmaz)
	for _, sp := range loadSavedProjects() {
		if isoSame(sp.Path, w.dirs["b"]) && sp.ID != newID {
			t.Fatalf("kayıtlı kimlik (%s) bellekteki kimlikle (%s) uyuşmuyor", sp.ID, newID)
		}
	}
	// yeniden başlat: aynı kimlikler
	m2 := NewManager(w.dirs["a"])
	s2, err := m2.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ro := range s2.Repos {
		got[ro.Path] = ro.ID
	}
	for l, d := range w.dirs {
		var id string
		for p, i := range got {
			if isoSame(p, d) {
				id = i
			}
		}
		if id == "" {
			t.Fatalf("yeniden başlatınca %s projesi yok: %v", l, got)
		}
	}
}

// 10) Eşzamanlılık: bir yandan proje değiştirilirken bir yandan anlık görüntü alınınca, her görüntü kendi projesine ait
// olmalı (yarım/karışık görüntü yok).
func TestIso_ConcurrentSwitchAndSnapshotNeverTear(t *testing.T) {
	w := newIsoWorld(t, "a", "b", "c")
	labels := []string{"a", "b", "c"}
	var wg sync.WaitGroup
	errs := make(chan string, 64)
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // değiştirici
		defer wg.Done()
		for i := 0; i < 24; i++ {
			if _, err := w.mgr.SwitchRepo(w.ids[labels[i%3]]); err != nil {
				errs <- "switch: " + err.Error()
				return
			}
		}
		close(stop)
	}()
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() { // okuyucular
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				snap, err := w.mgr.Snapshot()
				if err != nil {
					errs <- "snapshot: " + err.Error()
					return
				}
				var owner string
				for l, id := range w.ids {
					if snap.Repo.ID == id {
						owner = l
					}
				}
				if owner == "" {
					errs <- "bilinmeyen proje kimliği: " + snap.Repo.ID
					return
				}
				got := branchNames(snap)
				for _, other := range labels {
					if other != owner && got["feat/only-"+other] {
						errs <- fmt.Sprintf("%s görüntüsünde %s'nin dalı var", owner, other)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// ---- günlük (proje başına) ayrıntıları ----

func logTexts(s model.CockpitSnapshot) string {
	var b strings.Builder
	for _, l := range s.Logs {
		b.WriteString(l.Action + "|" + l.Branch + "|" + l.Message + "\n")
	}
	return b.String()
}

// Her proje kendi hatasını görür, ötekinin hatasını görmez; geri dönünce kendi kaydı durur.
func TestIso_EachProjectSeesOnlyItsOwnLogs(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	_, _ = w.mgr.DeleteBranch("yalnizca-a-hatasi")
	w.switchTo(t, "b")
	_, _ = w.mgr.DeleteBranch("yalnizca-b-hatasi")

	snapB, _ := w.mgr.Snapshot()
	if txt := logTexts(snapB); !strings.Contains(txt, "yalnizca-b-hatasi") || strings.Contains(txt, "yalnizca-a-hatasi") {
		t.Fatalf("b yalnızca kendi günlüğünü görmeli:\n%s", txt)
	}
	snapA := w.switchTo(t, "a")
	if txt := logTexts(snapA); !strings.Contains(txt, "yalnizca-a-hatasi") || strings.Contains(txt, "yalnizca-b-hatasi") {
		t.Fatalf("a yalnızca kendi günlüğünü görmeli:\n%s", txt)
	}
}

// "Temizle" yalnızca görünen (etkin projenin) kayıtlarını siler; ötekinin günlüğü korunur.
func TestIso_ClearLogsOnlyClearsTheActiveProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	_, _ = w.mgr.DeleteBranch("hata-a")
	w.switchTo(t, "b")
	_, _ = w.mgr.DeleteBranch("hata-b")

	snap, err := w.mgr.ClearLogs() // b etkin
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Logs) != 0 {
		t.Fatalf("b'nin günlüğü temizlenmeli: %s", logTexts(snap))
	}
	if txt := logTexts(w.switchTo(t, "a")); !strings.Contains(txt, "hata-a") {
		t.Fatalf("a'nın günlüğü korunmalı:\n%s", txt)
	}
}

// Eski (projesiz) kayıtlar hiçbir projede kaybolmaz; yeni kayıtlar proje kimliği taşır.
func TestIso_LegacyLogsStayVisibleAndNewOnesCarryTheProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.mgr.mu.Lock()
	w.mgr.logs = append(w.mgr.logs, model.ActionLogEntry{ID: "log-0", Level: "ok", Action: "Eski Kayıt", Message: "projesiz"})
	w.mgr.mu.Unlock()
	for _, l := range []string{"a", "b"} {
		if txt := logTexts(w.switchTo(t, l)); !strings.Contains(txt, "Eski Kayıt") {
			t.Fatalf("%s: eski projesiz kayıt görünmeli:\n%s", l, txt)
		}
	}
	w.switchTo(t, "a")
	_, _ = w.mgr.DeleteBranch("yeni-kayit")
	w.mgr.mu.RLock()
	defer w.mgr.mu.RUnlock()
	for _, e := range w.mgr.logs {
		if strings.Contains(e.Branch, "yeni-kayit") && e.RepoID != w.ids["a"] {
			t.Fatalf("yeni kayıt etkin projenin kimliğini taşımalı: %+v", e)
		}
	}
}

// Bir projenin günlüğü diğerlerini dışarı itmez ve her projenin görünümü sınırlıdır.
func TestIso_LogCapIsPerProjectView(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "b")
	_, _ = w.mgr.DeleteBranch("b-hatasi")
	w.switchTo(t, "a")
	for i := 0; i < maxVisibleLogs+30; i++ {
		_, _ = w.mgr.DeleteBranch(fmt.Sprintf("a-hata-%d", i))
	}
	if n := len(w.switchTo(t, "a").Logs); n != maxVisibleLogs {
		t.Fatalf("a'nın görünümü %d kayıtla sınırlı olmalı, %d geldi", maxVisibleLogs, n)
	}
	if txt := logTexts(w.switchTo(t, "b")); !strings.Contains(txt, "b-hatasi") {
		t.Fatalf("a'nın çok sayıdaki kaydı b'nin kaydını silmemeli:\n%s", txt)
	}
}

// Yeniden başlatınca günlük kimlikleri çakışmamalı (arayüz günlük değişimini kimlikten anlar; çakışma rozeti güncellemez).
func TestIso_LogIDsAreUniqueAcrossRestarts(t *testing.T) {
	w := newIsoWorld(t, "a")
	for i := 0; i < 3; i++ {
		_, _ = w.mgr.DeleteBranch(fmt.Sprintf("x%d", i))
	}
	m2 := NewManager(w.dirs["a"])
	_, _ = m2.DeleteBranch("restart-sonrasi")
	m2.mu.RLock()
	defer m2.mu.RUnlock()
	seen := map[string]bool{}
	for _, e := range m2.logs {
		if seen[e.ID] {
			t.Fatalf("günlük kimliği çakıştı: %s", e.ID)
		}
		seen[e.ID] = true
	}
}

// Öneri kartı hangi projeye ait olduğunu söyler (arayüzde "Proje: X" etiketi için).
func TestIso_ProposalCarriesItsProjectName(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "b")
	if err := os.WriteFile(filepath.Join(w.dirs["b"], "yeni.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	prop, err := w.mgr.ProposeCommit("feat/b-karti", "feat: b", []string{"yeni.txt"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if prop.RepoName == "" || !strings.HasPrefix(prop.RepoName, "b") || !isoSame(prop.RepoPath, w.dirs["b"]) {
		t.Fatalf("kart projesini taşımalı: name=%q path=%q", prop.RepoName, prop.RepoPath)
	}
}

// Proje seçicideki 📥 rozeti: onay bekleyen öneri kartı olan proje işaretlenir (etkin olmasa da) ve kart başka projede
// yazılıp silinince yoklama imzası değişir (arayüz rozeti yeniler).
func TestIso_RepoSelectorFlagsProjectsWithAPendingProposal(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	w.switchTo(t, "a")
	if err := os.WriteFile(filepath.Join(w.dirs["a"], "yeni.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.mgr.ProposeCommit("feat/rozet", "feat: rozet", []string{"yeni.txt"}, "", ""); err != nil {
		t.Fatal(err)
	}
	flags := func(s model.CockpitSnapshot) map[string]bool {
		m := map[string]bool{}
		for _, r := range s.Repos {
			m[r.ID] = r.PendingProposal
		}
		return m
	}

	snapB := w.switchTo(t, "b") // kart a'da; etkin proje b
	f := flags(snapB)
	if !f[w.ids["a"]] || f[w.ids["b"]] {
		t.Fatalf("yalnızca kartı olan proje (a) işaretlenmeli: %v", f)
	}
	if snapB.PendingProposal != nil {
		t.Fatal("b'de kart görünmemeli (rozet yalnızca hangi projede kart beklediğini söyler)")
	}

	// Kart a'da silinince (başka süreçte onaylandı/reddedildi) imza değişir ve rozet kalkar; b hâlâ etkin.
	sig1 := w.mgr.QuickRepoSignature()
	if err := os.Remove(filepath.Join(w.dirs["a"], ".git", "gitarc_proposal.json")); err != nil {
		t.Fatal(err)
	}
	if sig2 := w.mgr.QuickRepoSignature(); sig2 == sig1 {
		t.Fatal("etkin olmayan projedeki kart silinince de yoklama imzası değişmeli")
	}
	if snap, _ := w.mgr.Snapshot(); flags(snap)[w.ids["a"]] {
		t.Fatal("kart silinince rozet kalkmalı")
	}
}

// Hiç dalı olmayan taze bir depo: dizi alanları JSON'da null değil boş dizi olmalı (arayüz dizi bekler; null olunca
// renderInspector çöküp arayüzü sessizce Offline Simülasyon kipine düşürüyordu).
func TestFreshRepoSnapshotHasEmptyArraysNotNull(t *testing.T) {
	isoHome(t)
	dir, _ := newTempRepo(t) // yalnızca main, tek commit
	mgr := NewManager(dir)
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Branches == nil {
		t.Fatal("Branches nil: JSON'da null olur")
	}
	b, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"branches":null`) {
		t.Fatal(`anlık görüntüde "branches":null var`)
	}
}

// 8) Eski kimlik: yeniden eklenen projenin eski kimliğiyle seçim hatasız bir şekilde yönetilmeli (arayüz localStorage'da
// eski kimliği tutar). Kimlik, yeniden eklemede AYNI kalmalı (kararlı kimlik).
func TestIso_ProjectIDIsStableAcrossRemoveAndReAdd(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	oldID := w.ids["b"]
	if _, err := w.mgr.RemoveProject(oldID); err != nil {
		t.Fatal(err)
	}
	snap, err := w.mgr.AddProject(w.dirs["b"], "b")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.ID != oldID {
		t.Fatalf("aynı yol yeniden eklenince kimlik değişti (%s -> %s); arayüzün hatırladığı kimlik geçersiz kalır", oldID, snap.Repo.ID)
	}
}

// 9) Aynı depo farklı yol yazımlarıyla (alt klasör, sembolik bağ) ikinci bir proje olarak eklenmemeli.
func TestIso_SamePathSpellingsAreOneProject(t *testing.T) {
	w := newIsoWorld(t, "a", "b")
	before, _ := w.mgr.Snapshot()
	n := len(before.Repos)

	sub := filepath.Join(w.dirs["b"], "alt")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := w.mgr.AddProject(sub, "alt klasör"); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "b-link")
	if err := os.Symlink(w.dirs["b"], link); err != nil {
		t.Skip("symlink yok")
	}
	if _, err := w.mgr.AddProject(link, "b sembolik"); err != nil {
		t.Fatal(err)
	}
	after, _ := w.mgr.Snapshot()
	if len(after.Repos) != n {
		var names []string
		for _, r := range after.Repos {
			names = append(names, r.Name+"="+r.Path)
		}
		t.Fatalf("aynı depo ikinci kez eklendi (%d -> %d): %v", n, len(after.Repos), names)
	}
}

// Proje kimliği yoldan türetilir: biçim sabit, farklı depolar farklı kimlik alır, aynı depo hangi yazımla verilirse aynı.
func TestProjectIDForPathIsStableAndPathBased(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	idA := projectIDForPath(a)
	if !strings.HasPrefix(idA, "live-proj-") || len(idA) != len("live-proj-")+8 {
		t.Fatalf("beklenen biçim live-proj-<8 hex>: %q", idA)
	}
	if idA != projectIDForPath(a) {
		t.Fatal("aynı yol aynı kimliği vermeli")
	}
	if idA == projectIDForPath(b) {
		t.Fatal("farklı depolar farklı kimlik almalı")
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(a, link); err != nil {
		t.Skip("symlink yok")
	}
	if projectIDForPath(link) != idA {
		t.Fatal("sembolik bağ yazımı aynı kimliği vermeli")
	}
	if projectIDForPath(a+"/") != idA {
		t.Fatal("sondaki eğik çizgi kimliği değiştirmemeli")
	}
}
