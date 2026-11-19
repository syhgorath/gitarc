package gitengine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitarc/internal/model"
)

func simPropose(t *testing.T, mgr *Manager, branch string, files ...string) model.CommitProposal {
	t.Helper()
	if len(files) == 0 {
		files = []string{"internal/x.go", "docs/a.md"}
	}
	prop, err := mgr.ProposeCommit(branch, "feat: add x and docs (2 dosya)", files, "AI başlık", "AI gövde")
	if err != nil {
		t.Fatalf("ProposeCommit: %v", err)
	}
	return prop
}

func pendingOf(t *testing.T, mgr *Manager) *model.CommitProposal {
	t.Helper()
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snap.PendingProposal
}

func branchByName(snap model.CockpitSnapshot, name string) *model.BranchArc {
	for i := range snap.Branches {
		if snap.Branches[i].Name == name {
			return &snap.Branches[i]
		}
	}
	return nil
}

func TestApproveProposalSimulationCreatesBranchWithOnlyProposedFiles(t *testing.T) {
	mgr := NewManager("")
	prop := simPropose(t, mgr, "feat/ai-x")

	snap, err := mgr.ApproveProposal(prop.ID, "feat/ai-edited", "feat: edited message", "Düzenlenmiş başlık", "Düzenlenmiş gövde")
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if snap.PendingProposal != nil {
		t.Fatal("proposal must be cleared after approval")
	}
	b := branchByName(snap, "feat/ai-edited")
	if b == nil {
		t.Fatalf("edited branch name must be used, branches: %+v", snap.Branches)
	}
	if b.Hemisphere != "upper" || b.PR == nil || b.PR.Title != "Düzenlenmiş başlık" || b.PR.Body != "Düzenlenmiş gövde" {
		t.Fatalf("approved branch must carry the edited PR title/body: %+v", b.PR)
	}
	if strings.Join(b.ChangedFiles, ",") != "internal/x.go,docs/a.md" {
		t.Fatalf("changed files must be exactly the proposal's files, got %v", b.ChangedFiles)
	}
	if branchByName(snap, "feat/ai-x") != nil {
		t.Fatal("the original proposed name must not be created when the user renamed it")
	}
	found := false
	for _, l := range snap.Logs {
		if l.Action == "AI Önerisi Onaylandı" && l.Level == "ok" {
			found = true
		}
	}
	if !found {
		t.Fatalf("approval must be logged: %+v", snap.Logs)
	}
}

func TestApproveProposalEmptyFieldsFallBackToTheProposal(t *testing.T) {
	mgr := NewManager("")
	prop := simPropose(t, mgr, "feat/ai-defaults")
	snap, err := mgr.ApproveProposal(prop.ID, "", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	b := branchByName(snap, "feat/ai-defaults")
	if b == nil || b.PR == nil || b.PR.Title != "AI başlık" || b.LastCommitMsg != prop.Message {
		t.Fatalf("empty edits must keep the proposal's branch/message/title: %+v", b)
	}
}

func TestApproveProposalRejectsMissingStaleAndDuplicateApprovals(t *testing.T) {
	mgr := NewManager("")
	if _, err := mgr.ApproveProposal("prop-1", "feat/x", "m", "", ""); err == nil || !strings.Contains(err.Error(), "bekleyen AI önerisi yok") {
		t.Fatalf("no pending proposal must be refused clearly, got %v", err)
	}

	prop := simPropose(t, mgr, "feat/ai-stale")
	if _, err := mgr.ApproveProposal("prop-OTHER", "feat/ai-stale", "m", "", ""); err == nil || !strings.Contains(err.Error(), "öneri değişmiş") {
		t.Fatalf("a different proposal id must be refused, got %v", err)
	}
	if _, err := mgr.ApproveProposal("", "feat/ai-stale", "m", "", ""); err == nil {
		t.Fatal("an empty proposal id must be refused")
	}
	if pendingOf(t, mgr) == nil {
		t.Fatal("a refused approval must leave the proposal pending")
	}

	if _, err := mgr.ApproveProposal(prop.ID, "feat/ai-stale", "m", "", ""); err != nil {
		t.Fatalf("valid approval failed: %v", err)
	}
	if _, err := mgr.ApproveProposal(prop.ID, "feat/ai-stale", "m", "", ""); err == nil {
		t.Fatal("approving the same proposal twice must fail (no double branch / double PR)")
	}
}

func TestApproveProposalRejectsUnsafeBranchNamesAndKeepsProposal(t *testing.T) {
	mgr := NewManager("")
	prop := simPropose(t, mgr, "feat/ai-names")
	for _, bad := range []string{"bad name", "../escape", "main", "feat/-x;rm"} {
		if _, err := mgr.ApproveProposal(prop.ID, bad, "m", "", ""); err == nil {
			t.Errorf("branch name %q must be refused", bad)
		}
	}
	if pendingOf(t, mgr) == nil {
		t.Fatal("proposal must stay pending after refused approvals")
	}
}

func TestApproveProposalRefusesMergedBranchNameAndTeamMode(t *testing.T) {
	mgr := NewManager("")
	prop := simPropose(t, mgr, "feat/ai-merged")
	snap, _ := mgr.Snapshot()
	var mergedName string
	for _, b := range snap.Branches {
		if b.State == "merged" && b.Name != "" {
			mergedName = b.Name
			break
		}
	}
	if mergedName == "" {
		t.Skip("demo has no merged branch")
	}
	if _, err := mgr.ApproveProposal(prop.ID, mergedName, "m", "", ""); err == nil || !strings.Contains(err.Error(), "birleşmiş") {
		t.Fatalf("reusing a merged branch name must be refused, got %v", err)
	}

	mgr.SetTeamMode(true)
	if _, err := mgr.ApproveProposal(prop.ID, "feat/ai-team", "m", "", ""); err == nil || !strings.Contains(err.Error(), "Team Server") {
		t.Fatalf("team mode must block approval (shared HEAD), got %v", err)
	}
	if pendingOf(t, mgr) == nil {
		t.Fatal("proposal must stay pending")
	}
}

func TestApproveProposalReChecksSecretsAtApprovalTime(t *testing.T) {
	mgr := NewManager("")
	// A proposal that somehow carries a secret file (tampered IPC file / race) must still be refused at approval.
	mgr.mu.Lock()
	mgr.pendingProposal = &model.CommitProposal{ID: "prop-x", BranchName: "feat/leak", Message: "m", Files: []string{"ok.go", ".env"}}
	mgr.mu.Unlock()
	if _, err := mgr.ApproveProposal("prop-x", "feat/leak", "m", "", ""); err == nil || !strings.Contains(err.Error(), "GÜVENLİK ENGELİ") {
		t.Fatalf("secret files must be blocked at approval time, got %v", err)
	}
	if pendingOf(t, mgr) == nil {
		t.Fatal("proposal stays pending after a blocked approval")
	}
}

func TestApproveProposalRejectsConcurrentApproval(t *testing.T) {
	mgr := NewManager("")
	prop := simPropose(t, mgr, "feat/ai-busy")
	mgr.mu.Lock()
	mgr.proposalBusy = true
	mgr.mu.Unlock()
	if _, err := mgr.ApproveProposal(prop.ID, "feat/ai-busy", "m", "", ""); err == nil || !strings.Contains(err.Error(), "zaten işleniyor") {
		t.Fatalf("a second approval while one is running must be refused, got %v", err)
	}
}

// End to end on a real git repo with a fake gh: only the proposal's files are committed, the rest of the dirty tree is untouched.
func TestApproveProposalLiveCommitsOnlyProposedFilesAndOpensPR(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	for name, content := range map[string]string{"a.txt": "a\n", "b.txt": "b\n", "c.txt": "not proposed\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	setWaits(t, 5*time.Second, 5*time.Second)
	mgr := switchLive(t, dir)

	prop, err := mgr.ProposeCommit("feat/ai-live", "feat: add a and b (2 dosya)", []string{"a.txt", "b.txt"}, "AI PR", "AI body")
	if err != nil {
		t.Fatalf("ProposeCommit: %v", err)
	}
	if _, err := os.Stat(proposalFilePath(dir)); err != nil {
		t.Fatalf("proposal IPC file must exist before approval: %v", err)
	}

	snap, err := mgr.ApproveProposal(prop.ID, "feat/ai-live", "feat: add a and b (2 dosya)", "Edited PR", "Edited body")
	if err != nil {
		t.Fatalf("ApproveProposal: %v", err)
	}
	if snap.PendingProposal != nil {
		t.Fatal("proposal must be cleared")
	}
	if _, err := os.Stat(proposalFilePath(dir)); !os.IsNotExist(err) {
		t.Fatal("proposal IPC file must be removed after approval")
	}

	if cur := currentBranch(t, dir); cur != "feat/ai-live" {
		t.Fatalf("HEAD should be on the new branch, got %q", cur)
	}
	files, _ := runGitOut(dir, "show", "--name-only", "--format=", "HEAD")
	if got := strings.Fields(files); len(got) != 2 || !strings.Contains(files, "a.txt") || !strings.Contains(files, "b.txt") {
		t.Fatalf("commit must contain exactly a.txt and b.txt, got %q", files)
	}
	status, _ := runGitOut(dir, "status", "--porcelain")
	if !strings.Contains(status, "?? c.txt") {
		t.Fatalf("the un-proposed file must stay uncommitted, status=%q", status)
	}
	if out, _ := runGitOut(dir, "ls-remote", "--heads", "origin", "feat/ai-live"); !strings.Contains(out, "feat/ai-live") {
		t.Fatalf("branch must be pushed: %q", out)
	}
	if !gh.called("pr create", "--head feat/ai-live", "--title Edited PR", "--body Edited body") {
		t.Fatalf("PR must be opened with the edited title/body: %v", gh.calls())
	}
}

func TestApproveProposalLiveFailureKeepsProposalForRetry(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	setWaits(t, 5*time.Second, 5*time.Second)
	mgr := switchLive(t, dir)
	prop, err := mgr.ProposeCommit("feat/ai-retry", "feat: a (1 dosya)", []string{"a.txt"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	gh.set("pr_create", "GraphQL: boom", 1)
	if _, err := mgr.ApproveProposal(prop.ID, "feat/ai-retry", "feat: a (1 dosya)", "", ""); err == nil || !strings.Contains(err.Error(), "PR açılamadı") {
		t.Fatalf("PR failure must surface, got %v", err)
	}
	if pendingOf(t, mgr) == nil {
		t.Fatal("after a failed approval the proposal must still be pending so the user can retry")
	}
	if _, err := os.Stat(proposalFilePath(dir)); err != nil {
		t.Fatalf("IPC file must survive a failed approval: %v", err)
	}
}
