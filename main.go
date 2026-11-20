package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"gitarc/internal/gitengine"
	"gitarc/internal/mcp"
)

//go:embed web
var webFS embed.FS

const (
	cookieName            = "gitarc_session"
	contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; style-src-attr 'none'; img-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"
)

type App struct {
	host     string
	token    string
	mgr      *gitengine.Manager
	sessMu   sync.RWMutex
	sessions map[string]time.Time
}

func newApp(host string, mgr *gitengine.Manager) *App {
	return &App{
		host:     host,
		token:    randomHex(24),
		mgr:      mgr,
		sessions: make(map[string]time.Time),
	}
}

func randomHex(nBytes int) string {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (a *App) newSession() string {
	tok := randomHex(24)
	a.sessMu.Lock()
	a.sessions[tok] = time.Now().Add(12 * time.Hour)
	a.sessMu.Unlock()
	return tok
}

func (a *App) sessionValid(tok string) bool {
	if tok == "" {
		return false
	}
	a.sessMu.RLock()
	exp, ok := a.sessions[tok]
	a.sessMu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		a.sessMu.Lock()
		delete(a.sessions, tok)
		a.sessMu.Unlock()
		return false
	}
	return true
}

// noProjectAllowed: "proje yok" durumunda da çalışan uçlar.
var noProjectAllowed = map[string]bool{
	"/api/snapshot":               true,
	"/api/repo/add":               true,
	"/api/repo/switch":            true,
	"/api/repo/remove":            true,
	"/api/system/choose-folder":   true,
	"/api/system/update-check":    true,
	"/api/system/git-status-ping": true,
	"/api/logs/clear":             true,
}

// repoBindingHeader: istemcinin (sekmenin) gösterdiği projenin kimliği. Sunucuda "etkin proje" tek ve geneldir; başka bir
// sekme projeyi değiştirdiyse eski sekmedeki bir işlem (dal sil, merge, commit...) sessizce ÖTEKİ projeye giderdi.
const repoBindingHeader = "X-GitArc-Repo"

// repoBindingExempt: projeyi değiştiren/seçen ya da projeden bağımsız uçlar bağlamadan muaftır (anlık görüntü ve yoklama
// da: sekme yeni etkin projeyi bunlarla öğrenir).
func repoBindingExempt(path string) bool {
	switch path {
	case "/api/snapshot", "/api/repo/switch", "/api/repo/add", "/api/repo/remove":
		return true
	}
	return strings.HasPrefix(path, "/api/system/")
}

func (a *App) guard(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// DNS rebinding guard
		if r.Host != a.host {
			http.Error(w, "yasak host", http.StatusForbidden)
			return
		}
		c, err := r.Cookie(cookieName)
		if err != nil || !a.sessionValid(c.Value) {
			writeJSONError(w, http.StatusUnauthorized, "oturum yok ya da süresi doldu")
			return
		}
		// CSRF guard for mutating requests
		if r.Method == http.MethodPost {
			if r.Header.Get("X-Requested-With") != "gitarc" {
				writeJSONError(w, http.StatusForbidden, "eksik X-Requested-With başlığı")
				return
			}
			if o := r.Header.Get("Origin"); o != "" && o != "http://"+a.host {
				writeJSONError(w, http.StatusForbidden, "yasak Origin")
				return
			}
		}
		// Proje bağlama: başlık varsa ve sunucudaki etkin projeyle uyuşmuyorsa işlem YAPILMAZ. Başlık yoksa (CLI, testler,
		// eski istemci) bağlama uygulanmaz.
		if want := r.Header.Get(repoBindingHeader); want != "" && !repoBindingExempt(r.URL.Path) {
			if active := a.mgr.ActiveRepoID(); want != active {
				writeJSON(w, http.StatusConflict, map[string]string{
					"error":        "Proje değişti: bu sekme başka bir projeyi gösteriyor, sunucuda farklı bir proje açık. İşlem yapılmadı; sayfa yenileniyor.",
					"code":         "repo_mismatch",
					"activeRepoId": active,
				})
				return
			}
		}
		// Açık proje yokken yalnızca proje ekleme/seçme ve salt okunur uçlar çalışır; diğerleri sahte bir depoya
		// işlem yapmasın diye reddedilir.
		if !noProjectAllowed[r.URL.Path] && a.mgr.NoProject() {
			writeJSONError(w, http.StatusConflict, "Önce bir proje ekleyin: açık bir Git deposu yok")
			return
		}
		next(w, r)
	}
}

func (a *App) root(static http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != a.host {
			http.Error(w, "yasak host", http.StatusForbidden)
			return
		}
		if t := r.URL.Query().Get("t"); t != "" {
			if subtle.ConstantTimeCompare([]byte(t), []byte(a.token)) == 1 {
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    a.newSession(),
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
				})
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		// Issue a session cookie on initial root page navigation for local loopback UX
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			c, err := r.Cookie(cookieName)
			if err != nil || !a.sessionValid(c.Value) {
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    a.newSession(),
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
				})
			}
		}
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		static.ServeHTTP(w, r)
	})
}

func (a *App) routes() http.Handler {
	sub, err := fs.Sub(webFS, "web")
	if err != nil {
		log.Fatalf("embed web fs error: %v", err)
	}
	mux := http.NewServeMux()
	mux.Handle("/", a.root(http.FileServer(http.FS(sub))))
	mux.HandleFunc("/api/snapshot", a.guard(a.handleSnapshot))
	mux.HandleFunc("/api/repo/switch", a.guard(a.handleSwitchRepo))
	mux.HandleFunc("/api/repo/add", a.guard(a.handleAddProject))
	mux.HandleFunc("/api/repo/remove", a.guard(a.handleRemoveProject))
	mux.HandleFunc("/api/repo/base-branch", a.guard(a.handleSetBaseBranch))
	mux.HandleFunc("/api/branch/create", a.guard(a.handleCreateBranch))
	mux.HandleFunc("/api/branch/checkout", a.guard(a.handleCheckoutBranch))
	mux.HandleFunc("/api/branch/merge", a.guard(a.handleMergeBranch))
	mux.HandleFunc("/api/branch/promote", a.guard(a.handlePromoteBranch))
	mux.HandleFunc("/api/working-tree/commit", a.guard(a.handleCommitWorkingTree))
	mux.HandleFunc("/api/working-tree/diff-stat", a.guard(a.handleDiffStat))
	mux.HandleFunc("/api/diff", a.guard(a.handleGetDiff))
	mux.HandleFunc("/api/working-tree/discard", a.guard(a.handleDiscardWorkingTree))
	mux.HandleFunc("/api/proposal/dismiss", a.guard(a.handleDismissProposal))
	mux.HandleFunc("/api/proposal/approve", a.guard(a.handleApproveProposal))
	mux.HandleFunc("/api/conflict/inspect", a.guard(a.handleInspectConflict))
	mux.HandleFunc("/api/branch/resolve", a.guard(a.handleResolveConflict))
	mux.HandleFunc("/api/branch/fix-ci", a.guard(a.handleFixCI))
	mux.HandleFunc("/api/branch/delete", a.guard(a.handleDeleteBranch))
	mux.HandleFunc("/api/branch/prune", a.guard(a.handlePruneGone))
	mux.HandleFunc("/api/branch/prune-gone", a.guard(a.handlePruneGone))
	mux.HandleFunc("/api/pr/edit", a.guard(a.handleEditPR))
	mux.HandleFunc("/api/commit/amend", a.guard(a.handleAmendCommit))
	mux.HandleFunc("/api/spine/sync", a.guard(a.handleSyncSpine))
	mux.HandleFunc("/api/sync-spine", a.guard(a.handleSyncSpine))
	mux.HandleFunc("/api/logs/clear", a.guard(a.handleClearLogs))
	mux.HandleFunc("/api/system/update-check", a.guard(a.handleUpdateCheck))
	mux.HandleFunc("/api/repo/protect", a.guard(a.handleProtectBranch))
	mux.HandleFunc("/api/repo/protection/plan", a.guard(a.handleProtectionPlan))
	mux.HandleFunc("/api/queue/plan", a.guard(a.handleQueuePlan))
	mux.HandleFunc("/api/branch/update/plan", a.guard(a.handleBranchUpdatePlan))
	mux.HandleFunc("/api/branch/update", a.guard(a.handleBranchUpdate))
	mux.HandleFunc("/api/system/choose-folder", a.guard(a.handleChooseFolder))
	mux.HandleFunc("/api/system/git-status-ping", a.guard(a.handleGitStatusPing))
	mux.HandleFunc("/api/simulation/reset", a.guard(a.handleResetSimulation))
	mux.HandleFunc("/api/reset", a.guard(a.handleResetSimulation))
	mux.HandleFunc("/api/test/run", a.guard(a.handleRunLocalTest))
	mux.HandleFunc("/api/run-local", a.guard(a.handleRunLocalTest))
	return mux
}

func (a *App) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "GET bekleniyor")
		return
	}
	snap, err := a.mgr.Snapshot()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleSwitchRepo(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoID string `json:"repoId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.SwitchRepo(req.RepoID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleAddProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.AddProject(req.Path, req.Name)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleRemoveProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoID string `json:"repoId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.RemoveProject(req.RepoID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleSetBaseBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Branch string `json:"branch"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.SetBaseBranch(req.Branch)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleChooseFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}

	var selectedPath string
	var err error

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.Command("osascript", "-e", `POSIX path of (choose folder with prompt "GitArc için Git Deposu / Proje Klasörünü Seçin:")`)
		out, e := cmd.Output()
		if e == nil {
			selectedPath = strings.TrimSpace(string(out))
		} else {
			err = e
		}
	case "linux":
		cmd := exec.Command("zenity", "--file-selection", "--directory", "--title=GitArc için Git Proje Klasörünü Seçin")
		out, e := cmd.Output()
		if e == nil {
			selectedPath = strings.TrimSpace(string(out))
		} else {
			err = e
		}
	case "windows":
		psScript := `Add-Type -AssemblyName System.Windows.Forms; $f = New-Object System.Windows.Forms.FolderBrowserDialog; if($f.ShowDialog() -eq 'OK'){ $f.SelectedPath }`
		cmd := exec.Command("powershell", "-NoProfile", "-Command", psScript)
		out, e := cmd.Output()
		if e == nil {
			selectedPath = strings.TrimSpace(string(out))
		} else {
			err = e
		}
	default:
		err = fmt.Errorf("otomatik klasör seçici bu işletim sisteminde desteklenmiyor")
	}

	if err != nil || selectedPath == "" {
		writeJSONError(w, http.StatusBadRequest, "Klasör seçimi iptal edildi")
		return
	}

	name := filepath.Base(selectedPath)
	writeJSON(w, http.StatusOK, map[string]string{
		"path": selectedPath,
		"name": name,
	})
}

func (a *App) handleGitStatusPing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "GET bekleniyor")
		return
	}
	sig := a.mgr.QuickRepoSignature()
	writeJSON(w, http.StatusOK, map[string]any{
		"sig": sig,
		"bg":  a.mgr.BackgroundVersion(),
	})
}

func (a *App) handleDiscardWorkingTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	snap, err := a.mgr.DiscardWorkingTree()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string `json:"name"`
		Hemisphere string `json:"hemisphere"`
		FromSHA    string `json:"fromSha"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.CreateBranch(req.Name, req.Hemisphere, req.FromSHA)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleCheckoutBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.CheckoutBranch(req.BranchID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleMergeBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
		Strategy string `json:"strategy"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.MergeBranch(req.BranchID, req.Strategy)
	if err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleEditPR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.UpdatePRMeta(req.BranchID, req.Title, req.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleAmendCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
		Message  string `json:"message"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.AmendLastCommit(req.BranchID, req.Message)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handlePromoteBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
		Title    string `json:"title"`
		Body     string `json:"body"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.PromoteBranchWithMeta(req.BranchID, req.Title, req.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleCommitWorkingTree(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchName string   `json:"branchName"`
		CommitMsg  string   `json:"commitMsg"`
		PRTitle    string   `json:"prTitle"`
		PRBody     string   `json:"prBody"`
		PushAndPR  bool     `json:"pushAndPr"`
		Files      []string `json:"files,omitempty"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.CommitWorkingTree(req.BranchName, req.CommitMsg, req.PRTitle, req.PRBody, req.PushAndPR, req.Files)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleDiffStat(w http.ResponseWriter, r *http.Request) {
	stat, err := a.mgr.GetDiffStat()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, stat)
}

func (a *App) handleGetDiff(w http.ResponseWriter, r *http.Request) {
	var target, diffType string
	if r.Method == http.MethodPost {
		var req struct {
			Target string `json:"target"`
			Type   string `json:"type"`
		}
		if !decodePOST(w, r, &req) {
			return
		}
		target = req.Target
		diffType = req.Type
	} else {
		target = r.URL.Query().Get("target")
		diffType = r.URL.Query().Get("type")
	}
	diff, err := a.mgr.GetDiff(target, diffType)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

func (a *App) handleDismissProposal(w http.ResponseWriter, r *http.Request) {
	snap, err := a.mgr.DismissProposal()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleApproveProposal(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProposalID string `json:"proposalId"`
		BranchName string `json:"branchName"`
		Message    string `json:"message"`
		PRTitle    string `json:"prTitle"`
		PRBody     string `json:"prBody"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.ApproveProposal(req.ProposalID, req.BranchName, req.Message, req.PRTitle, req.PRBody)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleInspectConflict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	payload, err := a.mgr.InspectConflictStudio(req.BranchID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (a *App) handleResolveConflict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID    string            `json:"branchId"`
		Strategy    string            `json:"strategy"`
		CustomFiles map[string]string `json:"customFiles"`
		RunVet      *bool             `json:"runVet"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	runVet := true
	if req.RunVet != nil {
		runVet = *req.RunVet
	}
	snap, err := a.mgr.ResolveBranchConflict(req.BranchID, req.Strategy, req.CustomFiles, runVet)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleFixCI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.FixCI(req.BranchID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleDeleteBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.DeleteBranch(req.BranchID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handlePruneGone(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	snap, removed, err := a.mgr.PruneGone()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"snapshot": snap,
		"removed":  removed,
	})
}

func (a *App) handleSyncSpine(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	snap, err := a.mgr.SyncSpine()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleClearLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	snap, err := a.mgr.ClearLogs()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (a *App) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	res, err := a.mgr.CheckForUpdates()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (a *App) handleProtectBranch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string   `json:"profile"`
		Checks  []string `json:"checks"`
		Confirm string   `json:"confirm"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.ApplyProtection(req.Profile, req.Checks, req.Confirm)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleBranchUpdatePlan: PR dalının ana dala göre güncellenmesinin SALT OKUNUR planı.
func (a *App) handleBranchUpdatePlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	plan, err := a.mgr.BranchUpdatePlan(req.BranchID)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// handleBranchUpdate: planı uygular (uzak PR dalına PUSH eder); yalnızca planın parmak iziyle (confirm) çalışır.
func (a *App) handleBranchUpdate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchID string `json:"branchId"`
		Confirm  string `json:"confirm"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.UpdateBranchFromBase(req.BranchID, req.Confirm)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// handleQueuePlan: seçilen dalların sırayla main'e birleşmesinin SALT OKUNUR planı (boş liste = tüm açık PR'lar).
func (a *App) handleQueuePlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BranchIDs []string `json:"branchIds"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	plan, err := a.mgr.QueuePlan(req.BranchIDs)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

// handleProtectionPlan: koruma profilinin ne değiştireceğini döndürür; hiçbir şey uygulamaz.
func (a *App) handleProtectionPlan(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Profile string   `json:"profile"`
		Checks  []string `json:"checks"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	plan, err := a.mgr.ProtectionPlan(req.Profile, req.Checks)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func (a *App) handleResetSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return
	}
	writeJSON(w, http.StatusOK, a.mgr.ResetSimulation())
}

func (a *App) handleRunLocalTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Cmd string `json:"cmd"`
	}
	if !decodePOST(w, r, &req) {
		return
	}
	snap, err := a.mgr.Snapshot()
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if snap.Repo.Mode == "live" && strings.HasPrefix(strings.TrimSpace(req.Cmd), "go test ") {
		fields := strings.Fields(req.Cmd)
		if len(fields) >= 2 && len(fields) <= 8 {
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, fields[0], fields[1:]...)
			cmd.Dir = snap.Repo.Path
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			runErr := cmd.Run()
			out := fmt.Sprintf("$ %s\n%s", req.Cmd, buf.String())
			if runErr != nil {
				out += fmt.Sprintf("\nexit: %v", runErr)
			}
			writeJSON(w, http.StatusOK, map[string]string{"output": out})
			return
		}
	}

	simOut := fmt.Sprintf("$ %s\n=== RUN   TestShellGoldCompare\n    compare_test.go:118: golden output mismatch on key 'hardening_pve_apply': got \"true\", want \"false\"\n--- FAIL: TestShellGoldCompare (0.41s)\nFAIL\tkeylab/internal/shellgold\t0.431s", req.Cmd)
	writeJSON(w, http.StatusOK, map[string]string{"output": simOut})
}

func decodePOST(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST bekleniyor")
		return false
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		writeJSONError(w, http.StatusBadRequest, "geçersiz JSON gövdesi")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	}
	if cmd != nil {
		_ = cmd.Start()
	}
}

func main() {
	addr := flag.String("addr", "127.0.0.1:7432", "dinlenecek yerel adres (örn. 127.0.0.1:7432 veya 127.0.0.1:0)")
	repoPath := flag.String("repo", ".", "doğrudan açılacak yerel git deposu yolu")
	demoMode := flag.Bool("demo", false, "örnek GitArc playground deposuyla başlat")
	openUI := flag.Bool("open", true, "başlatınca varsayılan tarayıcıda aç")
	mcpMode := flag.Bool("mcp", false, "stdio üzerinden MCP JSON-RPC 2.0 sunucusu olarak çalıştır")
	teamMode := flag.Bool("team", false, "paylaşılan ekip sunucusu modu (sunucudaki HEAD dalı korur)")
	flag.Parse()

	targetRepo := *repoPath
	if *demoMode || *repoPath == "demo" || *repoPath == "playground" {
		playgroundPath, err := gitengine.EnsurePlaygroundRepo("", false)
		if err != nil {
			log.Fatalf("playground hazırlanamadı: %v", err)
		}
		targetRepo = playgroundPath
	}

	mgr := gitengine.NewManager(targetRepo)
	if *teamMode {
		mgr.SetTeamMode(true)
	}

	if *mcpMode {
		if err := mcp.ServeStdio(os.Stdin, os.Stdout, mgr); err != nil {
			log.Fatalf("mcp sunucu hatası: %v", err)
		}
		return
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		// Port meşgulse rastgele boş port seç
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			log.Fatalf("sunucu dinlenemedi: %v", err)
		}
	}

	host := ln.Addr().String()
	app := newApp(host, mgr)
	srv := &http.Server{
		Handler:           app.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	url := fmt.Sprintf("http://%s/?t=%s", host, app.token)
	fmt.Printf("GitArc Kokpit hazır: http://%s/\n", host)
	if *openUI {
		openBrowser(url)
	}

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Fatalf("sunucu hatası: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
