package gitengine

import (
	"context"
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
