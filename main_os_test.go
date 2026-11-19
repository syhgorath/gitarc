package main

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

// ---------- helpers ----------

// stubBin writes an executable shell script named `name` into a temp dir that is
// prepended to PATH, so tests can fake OS tools (osascript, zenity, open, ...).
func stubBin(t *testing.T, name, script string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell stubs need a POSIX shell")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

type child struct {
	cmd  *exec.Cmd
	base string
	out  *bufio.Reader
	in   io.WriteCloser
}

// startGitArc re-executes the test binary as the real program (see TestMain).
func startGitArc(t *testing.T, args string, extraEnv ...string) *child {
	t.Helper()
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), "GITARC_TEST_RUN_MAIN=1", "GITARC_TEST_ARGS="+args)
	cmd.Env = append(cmd.Env, extraEnv...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	c := &child{cmd: cmd, out: bufio.NewReader(stdout), in: in}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	return c
}

var readyRe = regexp.MustCompile(`http://(\S+?)/`)

// waitReady reads stdout until the "hazır" banner and records the base URL.
func (c *child) waitReady(t *testing.T) {
	t.Helper()
	done := make(chan string, 1)
	go func() {
		for {
			line, err := c.out.ReadString('\n')
			if err != nil {
				done <- ""
				return
			}
			if strings.Contains(line, "Kokpit hazır") {
				done <- line
				return
			}
		}
	}()
	select {
	case line := <-done:
		m := readyRe.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("no URL in banner %q", line)
		}
		c.base = "http://" + m[1]
	case <-time.After(20 * time.Second):
		t.Fatal("GitArc did not print its ready banner in time")
	}
}

func (c *child) stopGracefully(t *testing.T) {
	t.Helper()
	if err := c.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- c.cmd.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("expected clean exit after SIGTERM, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("process did not shut down after SIGTERM")
	}
}

func (c *child) snapshot(t *testing.T) model.CockpitSnapshot {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 15 * time.Second}
	resp, err := client.Get(c.base + "/") // root issues the session cookie
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	resp, err = client.Get(c.base + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot status %d", resp.StatusCode)
	}
	var snap model.CockpitSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	return snap
}

// ---------- main() ----------

func TestMainServesSimulationAndShutsDownGracefully(t *testing.T) {
	c := startGitArc(t, "-repo sim -open=false -addr 127.0.0.1:0")
	c.waitReady(t)

	resp, err := http.Get(c.base + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(resp.Header.Get("Content-Security-Policy"), "style-src-attr 'none'") {
		t.Fatalf("root must be served with strict CSP, got %d %q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}

	resp, err = http.Get(c.base + "/api/snapshot")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("API without session must be 401, got %d", resp.StatusCode)
	}

	snap := c.snapshot(t)
	if snap.Repo.Mode == "live" || len(snap.Branches) == 0 {
		t.Fatalf("expected populated simulation snapshot, got %+v", snap.Repo)
	}
	if snap.Repo.TeamMode {
		t.Fatal("team mode must be off by default")
	}
	c.stopGracefully(t)
}

func TestMainFallsBackToFreePortWhenAddressIsBusy(t *testing.T) {
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()

	c := startGitArc(t, "-repo sim -open=false -addr "+busy.Addr().String())
	c.waitReady(t)
	if strings.TrimPrefix(c.base, "http://") == busy.Addr().String() {
		t.Fatal("server must pick another port when the requested one is busy")
	}
	if snap := c.snapshot(t); len(snap.Branches) == 0 {
		t.Fatal("fallback-port server must serve a working snapshot")
	}
	c.stopGracefully(t)
}

func TestMainTeamFlagEnablesTeamMode(t *testing.T) {
	c := startGitArc(t, "-repo sim -open=false -team -addr 127.0.0.1:0")
	c.waitReady(t)
	if !c.snapshot(t).Repo.TeamMode {
		t.Fatal("-team must enable team server mode")
	}
	c.stopGracefully(t)
}

func TestMainDemoFlagCreatesPlaygroundRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := t.TempDir()
	c := startGitArc(t, "-demo -open=false -addr 127.0.0.1:0", "HOME="+home, "USERPROFILE="+home)
	c.waitReady(t)
	if _, err := os.Stat(filepath.Join(home, ".gitarc", "playground", ".git")); err != nil {
		t.Fatalf("playground repo must be created under the isolated HOME: %v", err)
	}
	if snap := c.snapshot(t); snap.Repo.Mode != "live" {
		t.Fatalf("demo playground must open in live mode, got %q", snap.Repo.Mode)
	}
	c.stopGracefully(t)
}

func TestMainMCPModeSpeaksJSONRPCOverStdio(t *testing.T) {
	c := startGitArc(t, "-mcp -repo sim")
	send := func(s string) {
		if _, err := io.WriteString(c.in, s+"\n"); err != nil {
			t.Fatal(err)
		}
	}
	read := func() map[string]any {
		type res struct {
			line string
			err  error
		}
		ch := make(chan res, 1)
		go func() { l, e := c.out.ReadString('\n'); ch <- res{l, e} }()
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatalf("read: %v", r.err)
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(r.line), &m); err != nil {
				t.Fatalf("invalid JSON-RPC line %q: %v", r.line, err)
			}
			return m
		case <-time.After(15 * time.Second):
			t.Fatal("no MCP response in time")
			return nil
		}
	}

	send(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if m := read(); m["jsonrpc"] != "2.0" || m["error"] != nil {
		t.Fatalf("initialize failed: %v", m)
	}
	send(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	raw, _ := json.Marshal(read())
	for _, tool := range []string{"gitarc_status", "gitarc_check_conflicts", "gitarc_ci_failure_digest", "gitarc_sync_main", "gitarc_resolve_conflict"} {
		if !strings.Contains(string(raw), tool) {
			t.Errorf("tools/list missing %s", tool)
		}
	}

	// EOF on stdin ends the server cleanly (MCP clients close the pipe on exit).
	_ = c.in.Close()
	waited := make(chan error, 1)
	go func() { waited <- c.cmd.Wait() }()
	select {
	case err := <-waited:
		if err != nil {
			t.Fatalf("MCP mode must exit cleanly on EOF, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("MCP mode did not exit on stdin EOF")
	}
}

// ---------- openBrowser ----------

func TestOpenBrowserInvokesPlatformOpener(t *testing.T) {
	var opener string
	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	case "linux":
		opener = "xdg-open"
	default:
		t.Skip("no shell-stubbable opener on " + runtime.GOOS)
	}
	logFile := filepath.Join(t.TempDir(), "args.txt")
	stubBin(t, opener, `echo "$@" > "`+logFile+`"`+"\n")

	openBrowser("http://127.0.0.1:7432/?t=abc")

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(logFile); err == nil {
			if got := strings.TrimSpace(string(b)); got != "http://127.0.0.1:7432/?t=abc" {
				t.Fatalf("opener got %q", got)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%s was never invoked", opener)
}

func TestOpenBrowserNeverPanicsWhenOpenerIsMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // nothing on PATH
	openBrowser("http://127.0.0.1:1/")
}

// ---------- handleChooseFolder ----------

func chooserTool() (string, string) {
	if runtime.GOOS == "darwin" {
		return "osascript", "choose folder"
	}
	return "zenity", "--directory"
}

func TestChooseFolderReturnsSelectedPath(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("folder chooser stub covers darwin/linux")
	}
	tool, wantArg := chooserTool()
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	stubBin(t, tool, `echo "$@" > "`+argsFile+`"`+"\necho '/tmp/work/my-project/'\n")

	c := newAPIClient(t, gitengine.NewManager(""))
	rec := c.do(http.MethodPost, "/api/system/choose-folder", `{}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var res map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res["path"] != "/tmp/work/my-project/" || res["name"] != "my-project" {
		t.Fatalf("unexpected result %+v", res)
	}
	if b, _ := os.ReadFile(argsFile); !strings.Contains(string(b), wantArg) {
		t.Fatalf("chooser must be asked for a directory, args=%q", b)
	}
}

func TestChooseFolderCancelledOrEmptyIsBadRequest(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		t.Skip("folder chooser stub covers darwin/linux")
	}
	tool, _ := chooserTool()

	stubBin(t, tool, "exit 1\n") // user pressed Cancel
	c := newAPIClient(t, gitengine.NewManager(""))
	rec := c.do(http.MethodPost, "/api/system/choose-folder", `{}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "iptal") {
		t.Fatalf("cancel must be 400 with a clear message, got %d %s", rec.Code, rec.Body)
	}

	stubBin(t, tool, "echo\n") // empty selection
	rec = c.do(http.MethodPost, "/api/system/choose-folder", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty selection must be 400, got %d", rec.Code)
	}

	t.Setenv("PATH", t.TempDir()) // chooser not installed
	rec = c.do(http.MethodPost, "/api/system/choose-folder", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing chooser must be 400, got %d", rec.Code)
	}
}
