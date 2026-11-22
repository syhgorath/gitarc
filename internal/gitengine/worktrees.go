package gitengine

import (
	"context"
	"fmt"
	"strings"

	"gitarc/internal/model"
)

// Git'in yerel durumuna dair iki OKUMA-yalnızca bilgi: başka klasörde (git worktree) açık dallar ve unutulmuş stash kayıtları.
// Üçlü motor notu: ikisi de gerçek bir depoya özgü olgular; simülasyon ve çevrimdışı yedek motor ikisini de ÜRETMEZ (uydurma yok),
// alanlar boş kalır ve arayüz hiçbir şey göstermez.

// worktreeBranches: `git worktree list --porcelain` → dal adı → o dalı tutan klasör. Ana çalışma klasörü (repoPath) hariçtir:
// yalnızca BAŞKA bir klasörde açık olan dallar döner (git, öyle bir dalı silmeyi/geçmeyi reddeder).
func worktreeBranches(ctx context.Context, repoPath string) map[string]string {
	out, _, err := runCmd(ctx, repoPath, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	res := map[string]string{}
	var path string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "worktree "):
			path = strings.TrimPrefix(l, "worktree ")
		case strings.HasPrefix(l, "branch refs/heads/"):
			if path != "" && !sameRepoPath(path, repoPath) {
				res[strings.TrimPrefix(l, "branch refs/heads/")] = path
			}
		case l == "":
			path = ""
		}
	}
	return res
}

func annotateWorktrees(ctx context.Context, repoPath string, branches []model.BranchArc) {
	wt := worktreeBranches(ctx, repoPath)
	if len(wt) == 0 {
		return
	}
	for i := range branches {
		if p, ok := wt[branches[i].Name]; ok && branches[i].HasLocal {
			branches[i].WorktreePath = p
		}
	}
}

// stashEntries: `git stash list` (en fazla 20).
func stashEntries(ctx context.Context, repoPath string) []model.StashInfo {
	out, _, err := runCmd(ctx, repoPath, "git", "stash", "list", "--format=%gd\x1f%gs")
	if err != nil {
		return nil
	}
	var res []model.StashInfo
	for _, l := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(l), "\x1f", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		res = append(res, model.StashInfo{Ref: parts[0], Message: parts[1]})
		if len(res) >= 20 {
			break
		}
	}
	return res
}

// stashDiagnostics: stash'ler gizli iş saklar; unutulmasın diye Radar'da bilgi kartı (yalnızca BİLGİ; silme/uygulama önerilmez).
func stashDiagnostics(stashes []model.StashInfo) []model.DiagnosticItem {
	if len(stashes) == 0 {
		return nil
	}
	var lines []string
	for _, s := range stashes {
		lines = append(lines, fmt.Sprintf("%s — %s", s.Ref, s.Message))
	}
	return []model.DiagnosticItem{{
		ID:         "stash-list",
		Severity:   "info",
		Kind:       "stash",
		Title:      fmt.Sprintf("Unutulmuş olabilecek %d stash kaydı", len(stashes)),
		Detail:     strings.Join(lines, "\n") + "\n\nGitArc stash'i silmez ya da uygulamaz; içeriğe bakmak için: git stash show -p <kayıt>",
		ActionHint: "git stash list",
	}}
}
