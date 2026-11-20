package gitengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// MCP (öneriyi yazan) ve arayüz (okuyan) AYRI süreçlerdir; ikisi `.git/gitarc_proposal.json` ile haberleşir.
// İki ayrı Manager bu iki süreci temsil eder.

func TestProposalCrossProcessVisibilityAndSignature(t *testing.T) {
	dir, _ := newTempRepo(t)
	ui := switchLive(t, dir)  // arayüz süreci
	mcp := switchLive(t, dir) // MCP süreci

	sig0 := ui.QuickRepoSignature()
	if ui.GetPendingProposal() != nil {
		t.Fatal("başlangıçta bekleyen öneri olmamalı")
	}

	p1, err := mcp.ProposeCommit("feat/bir", "feat: bir", []string{"README.md"}, "", "")
	if err != nil {
		t.Fatalf("ProposeCommit: %v", err)
	}
	// Arayüz süreci kartı görür ve YOKLAMA İMZASI değişir (eskiden değişmiyor, arayüz kartı hiç çekmiyordu).
	if got := ui.GetPendingProposal(); got == nil || got.ID != p1.ID {
		t.Fatalf("arayüz MCP'nin kartını görmeli: %+v", got)
	}
	sig1 := ui.QuickRepoSignature()
	if sig1 == sig0 {
		t.Fatal("kart dosyası yazılınca yoklama imzası değişmeli")
	}

	// Yeni kart eskisinin önüne geçer (eskiden arayüzün bellekteki eski kartı kazanıyordu).
	p2, err := mcp.ProposeCommit("feat/iki", "feat: iki", []string{"README.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := ui.GetPendingProposal(); got == nil || got.ID != p2.ID {
		t.Fatalf("arayüz EN YENİ kartı göstermeli (bellekteki eski kart değil): %+v", got)
	}

	// Kart başka süreçte onaylanıp/reddedilip dosya silinince arayüzde de kalkar.
	if err := os.Remove(filepath.Join(dir, ".git", "gitarc_proposal.json")); err != nil {
		t.Fatal(err)
	}
	if got := ui.GetPendingProposal(); got != nil {
		t.Fatalf("dosya silinince bekleyen öneri kalkmalı: %+v", got)
	}
	if ui.QuickRepoSignature() == sig1 {
		t.Fatal("kart silinince de yoklama imzası değişmeli")
	}
}

func TestProposalSaveFailureIsReportedNotSilent(t *testing.T) {
	dir, _ := newTempRepo(t)
	mcp := switchLive(t, dir)
	// Dosya yolu bir klasör: yazma başarısız olur.
	if err := os.Mkdir(filepath.Join(dir, ".git", "gitarc_proposal.json"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := mcp.ProposeCommit("feat/x", "feat: x", []string{"README.md"}, "", ""); err == nil {
		t.Fatal("kart arayüze iletilemiyorsa ProposeCommit hata vermeli (eskiden sessizce 'iletildi' diyordu)")
	}
	if mcp.GetPendingProposal() != nil {
		t.Fatal("kaydedilemeyen kart bekleyen öneri olarak kalmamalı")
	}
}

func TestProposalInSimulationStaysInMemory(t *testing.T) {
	mgr := NewManager("sim")
	p, err := mgr.ProposeCommit("feat/demo-x", "feat: demo", []string{"CHANGELOG.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetPendingProposal(); got == nil || got.ID != p.ID {
		t.Fatalf("simülasyonda kart bellekte durmalı: %+v", got)
	}
}

// Gerçek olay: öneri bir projede oluşturuldu, onay başka bir projedeyken yapıldı ve o projede (sshpocket) yeni bir
// dal açıldı. Öneri yalnızca oluşturulduğu depoda onaylanabilir; başka depoda reddedilir ve hiçbir dal açılmaz.
func TestApproveRefusesProposalFromAnotherProject(t *testing.T) {
	dirA, _ := newTempRepo(t) // önerinin oluşturulduğu proje
	dirB, _ := newTempRepo(t) // onayın yanlışlıkla yapıldığı proje
	a := switchLive(t, dirA)
	prop, err := a.ProposeCommit("feat/yanlis-yer", "feat: x", []string{"README.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if prop.RepoPath == "" {
		t.Fatal("öneri hangi depo için oluşturulduğunu taşımalı")
	}

	// Kart dosyası B'nin .git'ine bulaşmış (kopya/bayat dosya) gibi: arayüz B'de bunu bekleyen öneri sanar.
	data, err := os.ReadFile(filepath.Join(dirA, ".git", "gitarc_proposal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirB, ".git", "gitarc_proposal.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	b := switchLive(t, dirB)
	if b.GetPendingProposal() == nil {
		t.Fatal("test düzeneği: B kartı görmeli")
	}

	_, err = b.ApproveProposal(prop.ID, "", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "başka bir proje") {
		t.Fatalf("başka projede onay reddedilmeli ve nedenini söylemeli: %v", err)
	}
	out, _ := exec.Command("git", "-C", dirB, "branch", "--list", "feat/yanlis-yer").Output()
	if strings.TrimSpace(string(out)) != "" {
		t.Fatalf("yanlış projede dal AÇILMAMALI: %s", out)
	}
	if head, _ := exec.Command("git", "-C", dirB, "rev-parse", "--abbrev-ref", "HEAD").Output(); strings.TrimSpace(string(head)) != "main" {
		t.Fatalf("yanlış projenin HEAD'i değişmemeli: %s", head)
	}
}

func TestSameRepoPathResolvesSymlinks(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skip("symlink desteklenmiyor")
	}
	if !sameRepoPath(dir, link) {
		t.Fatal("sembolik bağ aynı klasör sayılmalı")
	}
	if sameRepoPath(dir, t.TempDir()) {
		t.Fatal("farklı klasörler aynı sayılmamalı")
	}
}

// Gerçek senaryo: kart GitArc projesi açıkken oluşturuldu; arayüzde sshpocket'e geçilince kart GÖRÜNMEMELİ, geri dönünce
// görünmeli (eskiden bellekteki kart her projede görünüyordu ve yanlış projede onaylanabiliyordu).
func TestProposalFollowsTheActiveProject(t *testing.T) {
	dirA, _ := newTempRepo(t)
	dirB, _ := newTempRepo(t)
	mgr := switchLive(t, dirA) // etkin proje: A
	snapB, err := mgr.AddProject(dirB, "B")
	if err != nil {
		t.Fatal(err)
	}
	idA := snapB.Repo.ID // AddProject B'yi etkin yapar; A'nın kimliğini aşağıda seçiciden buluruz
	_ = idA
	var idAOpt, idBOpt string
	for _, r := range snapB.Repos {
		if sameRepoPath(r.Path, dirA) {
			idAOpt = r.ID
		}
		if sameRepoPath(r.Path, dirB) {
			idBOpt = r.ID
		}
	}
	if idAOpt == "" || idBOpt == "" {
		t.Fatalf("iki proje de seçicide olmalı: %+v", snapB.Repos)
	}

	if _, err := mgr.SwitchRepo(idAOpt); err != nil {
		t.Fatal(err)
	}
	prop, err := mgr.ProposeCommit("feat/sadece-a", "feat: a", []string{"README.md"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetPendingProposal(); got == nil || got.ID != prop.ID {
		t.Fatalf("A projesinde kart görünmeli: %+v", got)
	}

	if _, err := mgr.SwitchRepo(idBOpt); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetPendingProposal(); got != nil {
		t.Fatalf("B projesine geçince A'nın kartı GÖRÜNMEMELİ: %+v", got)
	}
	if snap, err := mgr.Snapshot(); err != nil || snap.PendingProposal != nil {
		t.Fatalf("B'nin anlık görüntüsünde kart olmamalı: %v %+v", err, snap.PendingProposal)
	}
	if _, err := mgr.ApproveProposal(prop.ID, "", "", "", ""); err == nil {
		t.Fatal("B projesindeyken A'nın kartı onaylanamamalı")
	}

	if _, err := mgr.SwitchRepo(idAOpt); err != nil {
		t.Fatal(err)
	}
	if got := mgr.GetPendingProposal(); got == nil || got.ID != prop.ID {
		t.Fatalf("A'ya dönünce kart tekrar görünmeli: %+v", got)
	}
}
