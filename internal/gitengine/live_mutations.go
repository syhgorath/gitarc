package gitengine

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"gitarc/internal/model"
)

func (m *Manager) findLiveBranch(live *repoEntry, branchID string) (model.BranchArc, model.CockpitSnapshot, error) {
	m.mu.RLock()
	opts := m.repoOptionsLocked()
	m.mu.RUnlock()

	snap, err := CollectLiveRepo(live.id, live.name, live.path, opts)
	if err != nil {
		return model.BranchArc{}, snap, err
	}
	for _, b := range snap.Branches {
		if b.ID == branchID {
			if err := validateBranchName(b.Name); err != nil {
				return model.BranchArc{}, snap, err
			}
			return b, snap, nil
		}
	}
	var fallback *model.BranchArc
	for i := range snap.Branches {
		b := snap.Branches[i]
		if b.Name == branchID {
			if b.State == "open" {
				if err := validateBranchName(b.Name); err != nil {
					return model.BranchArc{}, snap, err
				}
				return b, snap, nil
			}
			if fallback == nil {
				cp := b
				fallback = &cp
			}
		}
	}
	if fallback != nil {
		if err := validateBranchName(fallback.Name); err != nil {
			return model.BranchArc{}, snap, err
		}
		return *fallback, snap, nil
	}
	return model.BranchArc{}, snap, fmt.Errorf("canlı depoda dal bulunamadı: %s", branchID)
}

func (m *Manager) createBranchLive(live *repoEntry, name, hemisphere, fromSHA string) (model.CockpitSnapshot, error) {
	snap, _ := m.Snapshot()
	for _, b := range snap.Branches {
		if strings.EqualFold(b.Name, name) {
			statusTxt := "açık"
			if b.State == "merged" || b.OriginGone {
				statusTxt = "birleşmiş/kalıntı"
			}
			return model.CockpitSnapshot{}, fmt.Errorf("'%s' dalı depoda zaten mevcut (%s); lütfen benzersiz bir dal adı belirleyin", name, statusTxt)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	args := []string{"branch", name}
	if fromSHA != "" {
		if !reSafeSHA.MatchString(fromSHA) {
			return model.CockpitSnapshot{}, fmt.Errorf("geçersiz kaynak SHA: %q", fromSHA)
		}
		args = append(args, fromSHA)
	}
	if out, _, err := runCmd(ctx, live.path, "git", args...); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("dal oluşturulamadı: %s (%v)", strings.TrimSpace(out), err)
	}
	if hemisphere == "upper" || m.IsTeamMode() {
		if remoteOut, _, err := runCmd(ctx, live.path, "git", "remote"); err == nil && strings.TrimSpace(remoteOut) != "" {
			if out, _, err := runCmd(ctx, live.path, "git", "push", "-u", "origin", name); err != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("dal yerelde açıldı ancak origin'e pushlanamadı: %s", strings.TrimSpace(out))
			}
		}
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) checkoutBranchLive(live *repoEntry, branchID string) (model.CockpitSnapshot, error) {
	b, _, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if b.ID == "local-wip-working-tree" || strings.HasPrefix(b.ID, "local-wip-") {
		return model.CockpitSnapshot{}, fmt.Errorf("sanal çalışma dalına doğrudan geçiş yapılamaz")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	args := []string{"switch", b.Name}
	if !b.HasLocal && b.HasRemote {
		remoteRef := b.RemoteRef
		if remoteRef == "" {
			remoteRef = "origin/" + b.Name
		}
		args = []string{"switch", "--track", remoteRef}
	}
	if out, _, err := runCmd(ctx, live.path, "git", args...); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("%s dalına geçilemedi (uncommitted çakışma olabilir): %s", b.Name, strings.TrimSpace(out))
	}
	return m.Snapshot()
}

func (m *Manager) commitWorkingTreeLive(live *repoEntry, branchName, commitMsg, prTitle, prBody string, pushAndPR bool, explicitFiles []string) (model.CockpitSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	defBranch := detectDefaultBranch(ctx, live.path)
	curOut, _, _ := runCmd(ctx, live.path, "git", "branch", "--show-current")
	curBranch := strings.TrimSpace(curOut)

	if branchName != "" && branchName != curBranch {
		if pushAndPR && branchName == defBranch {
			return model.CockpitSnapshot{}, fmt.Errorf("PR açmak için %s dışında yeni bir özellik dalı adı girin", defBranch)
		}
		if _, _, err := runCmd(ctx, live.path, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName); err == nil {
			if out, _, swErr := runCmd(ctx, live.path, "git", "switch", branchName); swErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("%s dalına geçilemedi: %s", branchName, strings.TrimSpace(out))
			}
		} else {
			out, _, cbErr := runCmd(ctx, live.path, "git", "checkout", "-b", branchName)
			if cbErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("%s dalı açılamadı: %s", branchName, strings.TrimSpace(out))
			}
		}
		curBranch = branchName
	}

	if pushAndPR {
		if curBranch == "" || curBranch == defBranch {
			return model.CockpitSnapshot{}, fmt.Errorf("PR açmak için %s dışında bir özellik dalı adı girin", defBranch)
		}
		// Block pushing to an already merged branch
		snap, _ := m.Snapshot()
		for _, b := range snap.Branches {
			if b.Name == curBranch {
				if b.State == "merged" || b.OriginGone || (b.PR != nil && b.PR.ReviewState == "MERGED") {
					return model.CockpitSnapshot{}, fmt.Errorf("'%s' dalı zaten birleşmiş (merged/gone). Birleşmiş bir dalın üzerine push yapılamaz, lütfen yeni bir dal açın", curBranch)
				}
				break
			}
		}
	}

	cfg := LoadGuardConfig(live.path)
	stagedFiles, err := StageSafeFiles(ctx, live.path, explicitFiles, cfg)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	_ = stagedFiles

	if out, _, err := runCmd(ctx, live.path, "git", "commit", "-m", commitMsg); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("commit atılamadı: %s", strings.TrimSpace(out))
	}

	if pushAndPR {
		remoteName := "origin"
		if rOut, _, err := runCmd(ctx, live.path, "git", "remote"); err == nil && strings.TrimSpace(rOut) != "" {
			remoteName = strings.Fields(rOut)[0]
		}
		if out, _, err := runCmd(ctx, live.path, "git", "push", "-u", remoteName, curBranch); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("commit atıldı ancak %s dalı pushlanamadı: %s", curBranch, strings.TrimSpace(out))
		}
		if _, err := exec.LookPath("gh"); err == nil {
			title := strings.TrimSpace(prTitle)
			if title == "" {
				title = commitMsg
			}
			body := strings.TrimSpace(prBody)
			if body == "" {
				body = "GitArc kokpitinden oluşturuldu."
			}
			if out, _, prErr := runCmd(ctx, live.path, "gh", "pr", "create", "--head", curBranch, "--title", title, "--body", body); prErr != nil {
				if !strings.Contains(strings.ToLower(out), "already exists") {
					return model.CockpitSnapshot{}, fmt.Errorf("dal pushlandı ancak PR açılamadı: %s", strings.TrimSpace(out))
				}
			}
		}
	}

	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) promoteBranchLive(live *repoEntry, branchID, prTitle, prBody string) (model.CockpitSnapshot, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	title := strings.TrimSpace(prTitle)
	if title == "" {
		title = b.LastCommitMsg
	}
	if title == "" {
		title = "feat: " + b.Name
	}

	if b.AheadMain == 0 {
		if b.IsHEAD && snap.Repo.DirtyCount > 0 && !m.IsTeamMode() {
			cfg := LoadGuardConfig(live.path)
			if _, addErr := StageSafeFiles(ctx, live.path, nil, cfg); addErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("güvenli stage hatası: %w", addErr)
			}
			if out, _, comErr := runCmd(ctx, live.path, "git", "commit", "-m", title); comErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("otomatik commit atılamadı: %s", strings.TrimSpace(out))
			}
		} else {
			return model.CockpitSnapshot{}, fmt.Errorf("%s dalında %s dalına göre henüz yeni bir commit veya değişiklik yok; önce değişiklik yapın ya da boş dalı silin", b.Name, snap.Repo.DefaultBranch)
		}
	}

	if out, _, err := runCmd(ctx, live.path, "git", "push", "-u", "origin", b.Name); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("push başarısız: %s (%v)", strings.TrimSpace(out), err)
	}
	if b.PR == nil {
		if _, err := exec.LookPath("gh"); err == nil {
			body := strings.TrimSpace(prBody)
			if body == "" {
				body = "GitArc canlı kokpitinden açıldı."
			}
			out, _, prErr := runCmd(ctx, live.path, "gh", "pr", "create", "--head", b.Name, "--title", title, "--body", body)
			if prErr != nil {
				if !strings.Contains(strings.ToLower(out), "already exists") {
					return model.CockpitSnapshot{}, fmt.Errorf("dal pushlandı ancak PR açılamadı: %s", strings.TrimSpace(out))
				}
			}
		}
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) mergeBranchLive(live *repoEntry, branchID, strategy string) (model.CockpitSnapshot, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if b.Conflict != nil && b.Conflict.HasConflict && !b.Conflict.Resolved {
		return model.CockpitSnapshot{}, fmt.Errorf("%s dalında git merge-tree çakışması tespit edildi; önce çakışmayı çözün", b.Name)
	}
	if b.PR != nil && b.PR.CI != nil && b.PR.CI.State == "FAILURE" {
		return model.CockpitSnapshot{}, fmt.Errorf("%s dalında GitHub Actions CI başarısız; merge öncesi testleri düzeltin", b.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	defBranch := snap.Repo.DefaultBranch
	if defBranch == "" {
		defBranch = "main"
	}

	if b.PR != nil && b.PR.Number > 0 {
		if _, err := exec.LookPath("gh"); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("PR merge için gh CLI gerekli")
		}
		curOut, _, _ := runCmd(ctx, live.path, "git", "branch", "--show-current")
		curBranch := strings.TrimSpace(curOut)
		if curBranch == b.Name && snap.Repo.DirtyCount == 0 {
			_, _, _ = runCmd(ctx, live.path, "git", "checkout", defBranch)
			curBranch = defBranch
		}
		strat := normalizeMergeStrategy(strategy)
		mergeArgs := []string{"pr", "merge", strconv.Itoa(b.PR.Number)}
		if strat == "auto" {
			mergeArgs = append(mergeArgs, "--merge", "--auto")
		} else {
			mergeArgs = append(mergeArgs, "--"+strat)
		}
		if snap.Repo.DirtyCount == 0 || curBranch != b.Name {
			mergeArgs = append(mergeArgs, "--delete-branch")
		}
		out, _, err := runCmd(ctx, live.path, "gh", mergeArgs...)
		if err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("PR #%d merge edilemedi: %s", b.PR.Number, strings.TrimSpace(out))
		}
		syncDefaultBranchNonDestructive(ctx, live.path, defBranch)
		InvalidateGHCache(live.path)
		return m.Snapshot()
	}

	if snap.Repo.DirtyCount > 0 {
		return model.CockpitSnapshot{}, fmt.Errorf("çalışma dizininde %d uncommitted değişiklik var; yerel merge öncesi commit veya stash yapın", snap.Repo.DirtyCount)
	}
	if cur, _, err := runCmd(ctx, live.path, "git", "branch", "--show-current"); err == nil && strings.TrimSpace(cur) != defBranch {
		if out, _, err := runCmd(ctx, live.path, "git", "checkout", defBranch); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("%s dalına geçilemedi: %s", defBranch, strings.TrimSpace(out))
		}
	}
	args := []string{"merge", "--no-ff", b.Name}
	if strategy == "squash" {
		args = []string{"merge", "--squash", b.Name}
	}
	if out, _, err := runCmd(ctx, live.path, "git", args...); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("yerel merge başarısız: %s", strings.TrimSpace(out))
	}
	if strategy == "squash" {
		_, _, _ = runCmd(ctx, live.path, "git", "commit", "-m", fmt.Sprintf("squash merge %s", b.Name))
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) updatePRMetaLive(live *repoEntry, branchID, title, body string) (model.CockpitSnapshot, error) {
	b, _, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if b.PR == nil || b.PR.Number <= 0 {
		return model.CockpitSnapshot{}, fmt.Errorf("%s dalında düzenlenecek açık PR yok", b.Name)
	}
	if _, err := exec.LookPath("gh"); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("PR düzenlemek için gh CLI gerekli")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	out, _, err := runCmd(ctx, live.path, "gh", "pr", "edit", strconv.Itoa(b.PR.Number), "--title", title, "--body", body)
	if err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("PR #%d güncellenemedi: %s", b.PR.Number, strings.TrimSpace(out))
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) amendLastCommitLive(live *repoEntry, branchID, message string) (model.CockpitSnapshot, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if !b.IsHEAD || b.State != "open" || b.Name == snap.Repo.DefaultBranch || b.AheadMain <= 0 {
		return model.CockpitSnapshot{}, fmt.Errorf("yalnızca aktif açık özellik dalının (HEAD) son commit mesajı değiştirilebilir")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if out, _, err := runCmd(ctx, live.path, "git", "commit", "--amend", "--only", "-m", message); err != nil {
		return model.CockpitSnapshot{}, fmt.Errorf("commit mesajı güncellenemedi: %s", strings.TrimSpace(out))
	}
	if b.HasRemote {
		if out, _, err := runCmd(ctx, live.path, "git", "push", "--force-with-lease", "origin", b.Name); err != nil {
			return model.CockpitSnapshot{}, fmt.Errorf("yerelde mesaj değişti ancak force-with-lease push başarısız: %s", strings.TrimSpace(out))
		}
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) pruneGoneLive(live *repoEntry) (model.CockpitSnapshot, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_, _, _ = runCmd(ctx, live.path, "git", "fetch", "--prune", "origin")
	snapBefore, snapErr := m.Snapshot()
	if snapErr != nil {
		return model.CockpitSnapshot{}, 0, snapErr
	}

	curOut, _, _ := runCmd(ctx, live.path, "git", "branch", "--show-current")
	currentBranch := strings.TrimSpace(curOut)
	defBranch := detectDefaultBranch(ctx, live.path)

	if currentBranch != "" && currentBranch != defBranch && !m.IsTeamMode() && snapBefore.Repo.DirtyCount == 0 {
		for _, b := range snapBefore.Branches {
			if b.Name != currentBranch {
				continue
			}
			isMergedPR := b.PR != nil && b.PR.ReviewState == "MERGED" && b.RemoteAhead == 0
			if (b.OriginGone || b.State == "gone" || b.State == "merged" || isMergedPR) && b.UncommittedCount == 0 {
				if _, _, swErr := runCmd(ctx, live.path, "git", "switch", defBranch); swErr == nil {
					currentBranch = defBranch
				} else if _, _, coErr := runCmd(ctx, live.path, "git", "checkout", defBranch); coErr == nil {
					currentBranch = defBranch
				}
			}
			break
		}
	}

	removed := 0
	for _, b := range snapBefore.Branches {
		if !b.HasLocal || b.Name == currentBranch || b.Name == defBranch {
			continue
		}
		if b.ID == "local-wip-working-tree" || strings.HasPrefix(b.ID, "local-wip-") {
			continue
		}
		isMergedPR := b.PR != nil && b.PR.ReviewState == "MERGED" && b.RemoteAhead == 0 && b.UncommittedCount == 0
		if !b.OriginGone && b.State != "gone" && !isMergedPR && b.State != "merged" {
			continue
		}
		if err := validateBranchName(b.Name); err != nil {
			continue
		}
		if _, _, err := runCmd(ctx, live.path, "git", "branch", "-d", b.Name); err == nil {
			removed++
		} else if isMergedPR || b.OriginGone || b.State == "merged" {
			if _, _, err := runCmd(ctx, live.path, "git", "branch", "-D", b.Name); err == nil {
				removed++
			}
		}
	}

	remoteName := snapBefore.Repo.RemoteName
	if remoteName == "" {
		remoteName = "origin"
	}
	for _, b := range snapBefore.Branches {
		if b.HasLocal || !b.HasRemote || b.Name == currentBranch || b.Name == defBranch {
			continue
		}
		if (b.State != "merged" && !b.OriginGone) || b.AheadMain != 0 {
			continue
		}
		if err := validateBranchName(b.Name); err != nil {
			continue
		}
		if _, _, err := runCmd(ctx, live.path, "git", "show-ref", "--verify", "--quiet", "refs/remotes/"+remoteName+"/"+b.Name); err == nil {
			if _, _, err := runCmd(ctx, live.path, "git", "push", remoteName, "--delete", b.Name); err == nil {
				removed++
			}
		}
	}

	InvalidateGHCache(live.path)
	snap, err := m.Snapshot()
	return snap, removed, err
}

func syncDefaultBranchNonDestructive(ctx context.Context, repoPath, defBranch string) error {
	if out, _, err := runCmd(ctx, repoPath, "git", "fetch", "--prune", "origin"); err != nil {
		return fmt.Errorf("git fetch başarısız: %s", strings.TrimSpace(out))
	}
	curOut, _, _ := runCmd(ctx, repoPath, "git", "branch", "--show-current")
	statusOut, _, _ := runCmd(ctx, repoPath, "git", "status", "--porcelain")
	if strings.TrimSpace(curOut) == defBranch && strings.TrimSpace(statusOut) == "" {
		_, _, _ = runCmd(ctx, repoPath, "git", "merge", "--ff-only", "origin/"+defBranch)
	} else {
		_, _, _ = runCmd(ctx, repoPath, "git", "fetch", "--prune", "origin", defBranch+":"+defBranch)
	}
	return nil
}

func (m *Manager) syncSpineLive(live *repoEntry) (model.CockpitSnapshot, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	defBranch := detectDefaultBranch(ctx, live.path)
	if err := syncDefaultBranchNonDestructive(ctx, live.path, defBranch); err != nil {
		return model.CockpitSnapshot{}, err
	}
	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

func (m *Manager) deleteBranchLive(live *repoEntry, branchID string) (model.CockpitSnapshot, error) {
	b, snap, err := m.findLiveBranch(live, branchID)
	if err != nil {
		return model.CockpitSnapshot{}, err
	}
	if b.ID == "local-wip-working-tree" || strings.HasPrefix(b.ID, "local-wip-") {
		return model.CockpitSnapshot{}, fmt.Errorf("sanal çalışma dalı doğrudan silinemez")
	}
	if !b.HasLocal && !b.HasRemote {
		return model.CockpitSnapshot{}, fmt.Errorf("silinebilecek yerel veya uzak dal bulunamadı: %s", b.Name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	if b.Name == snap.Repo.DefaultBranch {
		return model.CockpitSnapshot{}, fmt.Errorf("aktif çalışma dalı (%s) silinemez; önce başka dala geçin", b.Name)
	}
	if b.IsHEAD {
		if m.IsTeamMode() {
			return model.CockpitSnapshot{}, fmt.Errorf("Team Server modunda aktif çalışma dalı (%s) silinemez", b.Name)
		}
		defBranch := snap.Repo.DefaultBranch
		if defBranch == "" {
			defBranch = "main"
		}
		if out, _, swErr := runCmd(ctx, live.path, "git", "switch", defBranch); swErr != nil {
			if outCo, _, coErr := runCmd(ctx, live.path, "git", "checkout", defBranch); coErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("%s omurgasına otomatik geçilemedi (kaydedilmemiş değişiklikleriniz çakışıyor olabilir): %s", defBranch, strings.TrimSpace(out)+" "+strings.TrimSpace(outCo))
			}
		}
	}

	if b.State == "open" && b.PR != nil && b.PR.Number > 0 {
		if _, err := exec.LookPath("gh"); err == nil {
			if prOut, _, prErr := runCmd(ctx, live.path, "gh", "pr", "close", strconv.Itoa(b.PR.Number), "--delete-branch"); prErr != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("PR #%d kapatılamadı: %s", b.PR.Number, strings.TrimSpace(prOut))
			}
		}
	}

	if b.HasLocal {
		if _, _, err := runCmd(ctx, live.path, "git", "branch", "-d", b.Name); err != nil {
			if outD, _, errD := runCmd(ctx, live.path, "git", "branch", "-D", b.Name); errD != nil {
				return model.CockpitSnapshot{}, fmt.Errorf("yerel dal silinemedi: %s", strings.TrimSpace(outD))
			}
		}
	}

	if b.HasRemote {
		remoteName := snap.Repo.RemoteName
		if remoteName == "" {
			remoteName = "origin"
		}
		refPath := "refs/remotes/" + remoteName + "/" + b.Name
		if _, _, err := runCmd(ctx, live.path, "git", "show-ref", "--verify", "--quiet", refPath); err == nil {
			if _, _, errR := runCmd(ctx, live.path, "git", "push", remoteName, "--delete", b.Name); errR != nil {
				_, _, _ = runCmd(ctx, live.path, "git", "update-ref", "-d", refPath)
			}
		}
	}

	InvalidateGHCache(live.path)
	removeProposalFile(live.path)
	m.mu.Lock()
	m.pendingProposal = nil
	m.mu.Unlock()
	return m.Snapshot()
}

// GetDiffStat returns summary statistics of uncommitted or staged changes.
func (m *Manager) GetDiffStat() (model.DiffStat, error) {
	if live, ok := m.activeLiveRepo(); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		out, _, err := runCmd(ctx, live.path, "git", "diff", "--cached", "--stat")
		if err != nil || strings.TrimSpace(out) == "" {
			out, _, _ = runCmd(ctx, live.path, "git", "diff", "--stat", "HEAD")
			if strings.TrimSpace(out) == "" {
				out, _, _ = runCmd(ctx, live.path, "git", "diff", "--stat")
			}
		}
		return ParseDiffStat(out), nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getDiffStatSimLocked(), nil
}
