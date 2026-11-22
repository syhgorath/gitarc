package gitengine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"gitarc/internal/model"
)

// Iraksayan dalı kendiliğinden birleştir ve pushla ("kullanıcıyı çıkmaz sokma" ilkesi).
//
// Durum: yerel dalda uzakta olmayan commit'ler VAR, uzak dalda da yerelde olmayanlar VAR (örn. kuyruk, PR dalına main'i birleştirip pushladı).
// Git düz push'u `non-fast-forward` diye reddeder; ham git çıktısıyla kullanıcıyı bırakmak yerine: uzak ucu çekip yerel uçla İZOLE bir worktree'de
// (kullanıcının çalışma ağacına dokunmadan) birleştiririz. Temizse pushlarız ve yerel dalı ilerletiriz; çakışıyorsa HİÇBİR ŞEY DEĞİŞTİRMEDEN
// hangi dosyaların çakıştığını söyleriz. Zorla push (--force) yoktur.

// isPushRejectedNonFastForward: push'un uzak ucun ilerlemiş olması yüzünden reddedildiği (zorla push gerektirmeyen) durum.
func isPushRejectedNonFastForward(out string) bool {
	o := strings.ToLower(out)
	return strings.Contains(o, "non-fast-forward") || strings.Contains(o, "fetch first") || strings.Contains(o, "remote contains work")
}

// integrateRemoteAndPush: uzak ucu yerel uçla birleştirip pushlar. Başarıda yeni uç SHA'sını ve kısa bir özet döndürür.
func (m *Manager) integrateRemoteAndPush(live *repoEntry, snap model.CockpitSnapshot, b model.BranchArc, remote string) (string, error) {
	if remote == "" {
		remote = "origin"
	}
	name := realBranchName(b)
	if err := validateBranchName(name); err != nil {
		return "", err
	}
	if b.WorktreePath != "" {
		return "", fmt.Errorf("%s dalı başka bir klasörde açık (%s); orada `git pull` ile uzaktaki değişiklikleri alıp tekrar deneyin", name, b.WorktreePath)
	}
	if b.IsHEAD && m.IsTeamMode() {
		return "", fmt.Errorf("Team Server modunda paylaşılan HEAD değiştirilemez: %s uzakta ilerlemiş; birleştirmeyi kendi makinenizde yapın", name)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if out, err := gitFetchRetry(ctx, live.path, remote); err != nil {
		return "", fmt.Errorf("uzak depo okunamadı (git fetch): %s", shortErr(out, err))
	}
	remoteRef := "refs/remotes/" + remote + "/" + name
	remoteSHA, err := revParse(ctx, live.path, remoteRef)
	if err != nil {
		return "", fmt.Errorf("uzak dal (%s/%s) okunamadı: %v", remote, name, err)
	}
	localSHA, err := revParse(ctx, live.path, "refs/heads/"+name)
	if err != nil {
		return "", fmt.Errorf("yerel dal %s okunamadı: %v", name, err)
	}

	wtDir, err := os.MkdirTemp("", "gitarc-integrate-*")
	if err != nil {
		return "", err
	}
	defer func() {
		cc, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		_, _, _, _ = runCmdStdout(cc, live.path, "git", "worktree", "remove", "--force", wtDir)
		_ = os.RemoveAll(wtDir)
	}()
	if _, errOut, _, err := runCmdStdout(ctx, live.path, "git", "worktree", "add", "--detach", wtDir, localSHA); err != nil {
		return "", fmt.Errorf("izole worktree oluşturulamadı: %s", strings.TrimSpace(errOut))
	}

	identity := buildGitIdentityArgs(ctx, live.path)
	mergeArgs := append(append([]string{}, identity...), "merge", "--no-edit", "-m", fmt.Sprintf("Merge remote-tracking branch '%s/%s' into %s (GitArc)", remote, name, name), remoteSHA)
	_, mergeErr, code, merr := runCmdStdout(ctx, wtDir, "git", mergeArgs...)
	if merr != nil && code != 0 {
		if _, _, _, mh := runCmdStdout(ctx, wtDir, "git", "rev-parse", "-q", "--verify", "MERGE_HEAD"); mh == nil {
			files, _, _, _ := runCmdStdout(ctx, wtDir, "git", "diff", "--name-only", "--diff-filter=U")
			list := strings.Join(strings.Fields(files), ", ")
			if list == "" {
				list = "çakışan dosyalar okunamadı"
			}
			return "", fmt.Errorf("%s dalında uzakta yerelde olmayan commit'ler var ve yerel commit'lerinle ÇAKIŞIYOR (%s). Hiçbir şey değiştirilmedi; önce çakışmayı çözün (Çakışma Stüdyosu ya da `git merge %s/%s`)", name, list, remote, name)
		}
		return "", fmt.Errorf("uzak değişiklikler birleştirilemedi: %s", shortErr(mergeErr, merr))
	}
	newSHA, _, _, err := runCmdStdout(ctx, wtDir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("birleşim SHA'sı okunamadı: %w", err)
	}
	newSHA = strings.TrimSpace(newSHA)

	if newSHA != remoteSHA {
		if _, pErr, _, err := runCmdStdout(ctx, wtDir, "git", "push", remote, newSHA+":refs/heads/"+name); err != nil {
			return "", fmt.Errorf("birleşim pushlanamadı (uzak yine ilerlemiş olabilir; tekrar deneyin): %s", strings.TrimSpace(pErr))
		}
	}

	// Yerel dalı yeni uca taşı (push başarılı: artık yerel de uzakla aynı olmalı). Yerel uç yeni ucun atasıdır: yalnızca ileri-sarma.
	if b.IsHEAD {
		if _, mErr, _, err := runCmdStdout(ctx, live.path, "git", "merge", "--ff-only", newSHA); err != nil {
			return newSHA, fmt.Errorf("uzak dal güncellendi ama aktif yerel dal ilerletilemedi (kaydedilmemiş değişiklikleriniz çakışıyor olabilir): %s; `git merge --ff-only %s/%s` ile tamamlayın", strings.TrimSpace(mErr), remote, name)
		}
	} else if _, bErr, _, err := runCmdStdout(ctx, live.path, "git", "branch", "-f", name, newSHA); err != nil {
		return newSHA, fmt.Errorf("uzak dal güncellendi ama yerel dal ilerletilemedi: %s", strings.TrimSpace(bErr))
	}
	return newSHA, nil
}
