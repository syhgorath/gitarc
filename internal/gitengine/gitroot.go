package gitengine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// gitRoot: verilen klasörün ait olduğu git deposunun kök dizinini bulur. Alt klasörlerde, `git worktree`
// klasörlerinde ve submodule'lerde (`.git` bir DOSYA) da çalışır; eskiden yalnızca "bu klasörde .git klasörü var mı"
// diye bakılıyordu ve bu durumlarda GitArc sessizce demo depoya düşüyordu.
func gitRoot(path string) (string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", abs, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", false
	}
	return filepath.Clean(root), true
}

// repoRootFor: yol zaten bir deponun kökü ise AYNEN korunur (kullanıcının yazdığı yol, sembolik bağlar dahil; uygulama
// içindeki önbellek/seçim anahtarları bu yola göre tutulur). Değilse (alt klasör, worktree, submodule) git'ten kök
// bulunur. Depo değilse "".
func repoRootFor(path string) string {
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return ""
	}
	if isGitDir(abs) {
		return abs
	}
	if root, ok := gitRoot(abs); ok {
		return root
	}
	return ""
}

// ---- git dizini yardımcıları (simulation.go'dan taşındı; davranış aynı) ----

func isGitDir(p string) bool {
	if p == "" {
		return false
	}
	return gitDirOf(p) != ""
}

// executablePath: çalışan ikilinin sembolik bağları çözülmüş mutlak yolu; bulunamazsa "".
func executablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		return r
	}
	return exe
}

// gitDirOrDefault: gitDirOf, depo değilse (henüz oluşmamış yol) eski `<p>/.git` varsayımı.
func gitDirOrDefault(p string) string {
	if d := gitDirOf(p); d != "" {
		return d
	}
	return filepath.Join(p, ".git")
}

// gitDirOf: klasörün gerçek git dizini. Normal depoda `<p>/.git`; `git worktree` ve submodule'de `.git` bir DOSYADIR
// ("gitdir: <yol>") ve gerçek dizin (HEAD/index burada, worktree başına ayrı) oradan okunur. Depo değilse "".
func gitDirOf(p string) string {
	dot := filepath.Join(p, ".git")
	st, err := os.Stat(dot)
	if err != nil {
		return ""
	}
	if st.IsDir() {
		return dot
	}
	data, err := os.ReadFile(dot)
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(data), "\n", 2)[0])
	rest, ok := strings.CutPrefix(line, "gitdir:")
	if !ok {
		return ""
	}
	target := strings.TrimSpace(rest)
	if target == "" {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(p, target)
	}
	if st, err := os.Stat(target); err != nil || !st.IsDir() {
		return ""
	}
	return filepath.Clean(target)
}
