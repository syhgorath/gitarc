package gitengine

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"gitarc/internal/model"
)

func setWaits(t *testing.T, gw, fw time.Duration) {
	t.Helper()
	oldG, oldF := ghWait, fetchWait
	ghWait, fetchWait = gw, fw
	t.Cleanup(func() { ghWait, fetchWait = oldG, oldF })
}

func waitFor(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func stepByID(steps []model.SyncStep, id string) *model.SyncStep {
	for i := range steps {
		if steps[i].ID == id {
			return &steps[i]
		}
	}
	return nil
}

func prsOn(snap model.CockpitSnapshot, branch string) *model.PRStatus {
	for _, b := range snap.Branches {
		if b.Name == branch && b.PR != nil {
			return b.PR
		}
	}
	return nil
}

// Prometheus regression: `gh pr list --json statusCheckRollup` takes ~5s on huge repos. The snapshot must not
// block on it (nor silently drop the result): it returns quickly, reports the step as pending, and the data
// arrives on a later snapshot — and the cheap signature changes so the UI knows to refresh.
func TestSlowGHDoesNotBlockSnapshotAndArrivesLater(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green")
	run("push", "-u", "origin", "feat/green")
	gh.set("pr_list_open", openPRsJSON, 0)
	gh.slow("pr_list_open", "1.5")
	setWaits(t, 200*time.Millisecond, 200*time.Millisecond)

	mgr := switchLive(t, dir)
	sigBefore := mgr.QuickRepoSignature()

	t0 := time.Now()
	snap, err := CollectLiveRepo("live-slow", "slow", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(t0); elapsed > 1200*time.Millisecond {
		t.Fatalf("snapshot must not wait for the slow gh query, took %v", elapsed)
	}
	if prsOn(snap, "feat/green") != nil {
		t.Fatalf("PR data cannot be there yet (gh is still running)")
	}
	if step := stepByID(snap.Repo.SyncSteps, "prs"); step == nil || step.State != "pending" {
		t.Fatalf("slow PR step must be reported as pending, got %+v", snap.Repo.SyncSteps)
	}

	waitFor(t, 10*time.Second, "background gh refresh to finish", func() bool {
		s := stepByID(syncStepsFor(dir), "prs")
		return s != nil && s.State == "ok"
	})
	if mgr.QuickRepoSignature() == sigBefore {
		t.Fatal("the cheap signature must change when background results arrive, otherwise the UI never refreshes")
	}

	snap, err = CollectLiveRepo("live-slow", "slow", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	pr := prsOn(snap, "feat/green")
	if pr == nil || pr.Number != 7 {
		t.Fatalf("PR must appear once the background refresh completes: %+v", pr)
	}
	step := stepByID(snap.Repo.SyncSteps, "prs")
	if step == nil || step.State != "ok" || step.Count != 5 || step.DurationMs < 1000 {
		t.Fatalf("finished PR step must be ok with count and duration, got %+v", step)
	}
}

// A failing/timed-out gh query must never wipe what we already know (the old code cached an EMPTY result).
func TestFailedGHRefreshKeepsPreviousDataAndSaysSo(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green")
	run("push", "-u", "origin", "feat/green")
	gh.set("pr_list_open", openPRsJSON, 0)
	setWaits(t, 5*time.Second, 5*time.Second)

	snap, err := CollectLiveRepo("live-keep", "keep", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prsOn(snap, "feat/green") == nil {
		t.Fatal("precondition: PR must be known after the first (fast) refresh")
	}

	gh.set("pr_list_open", "HTTP 502: bad gateway", 1)
	InvalidateGHCache(dir)
	snap, err = CollectLiveRepo("live-keep", "keep", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prsOn(snap, "feat/green") == nil {
		t.Fatal("previous PR data must survive a failed refresh")
	}
	step := stepByID(snap.Repo.SyncSteps, "prs")
	if step == nil || step.State != "failed" || !strings.Contains(step.Detail, "önceki sonuç") {
		t.Fatalf("failure must be visible and mention the stale data, got %+v", step)
	}
}

func TestInvalidateKeepsStaleDataWhileRefreshing(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green")
	run("push", "-u", "origin", "feat/green")
	gh.set("pr_list_open", openPRsJSON, 0)
	setWaits(t, 5*time.Second, 5*time.Second)
	if _, err := CollectLiveRepo("live-stale", "stale", dir, nil); err != nil {
		t.Fatal(err)
	}

	gh.slow("pr_list_open", "1.5")
	setWaits(t, 100*time.Millisecond, 100*time.Millisecond)
	InvalidateGHCache(dir) // what every mutation does
	snap, err := CollectLiveRepo("live-stale", "stale", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if prsOn(snap, "feat/green") == nil {
		t.Fatal("after invalidation the last known PRs must still be shown while refreshing")
	}
	if step := stepByID(snap.Repo.SyncSteps, "prs"); step == nil || step.State != "stale" {
		t.Fatalf("data shown during refresh must be flagged stale, got %+v", step)
	}
	waitFor(t, 10*time.Second, "refresh to finish", func() bool {
		s := stepByID(syncStepsFor(dir), "prs")
		return s != nil && s.State == "ok"
	})
}

func TestSyncStepsReportEveryStep(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	gh.set("pr_list_open", openPRsJSON, 0)
	gh.set("api_protection_get", "true", 0)
	setWaits(t, 5*time.Second, 5*time.Second)

	snap, err := CollectLiveRepo("live-steps", "steps", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"fetch", "prs", "merged", "protection"} {
		step := stepByID(snap.Repo.SyncSteps, id)
		if step == nil {
			t.Fatalf("missing sync step %q in %+v", id, snap.Repo.SyncSteps)
		}
		if step.State != "ok" || step.At == "" {
			t.Fatalf("step %q must be ok with a timestamp, got %+v", id, step)
		}
		if _, err := time.Parse(time.RFC3339, step.At); err != nil {
			t.Fatalf("step %q timestamp not RFC3339: %v", id, err)
		}
	}
	if snap.Repo.ProtectionState != "active" || !snap.Repo.BranchProtected {
		t.Fatalf("protection from gh must be active: %+v", snap.Repo)
	}
}

// Prometheus regression: 404 (no admin rights) must surface as "unknown", never as "Korumasız".
func TestProtectionUnknownWhenGitHubReturnsNotFound(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	gh.set("api_protection_get", `{"message":"Not Found","status":"404"}`, 1)
	setWaits(t, 5*time.Second, 5*time.Second)

	snap, err := CollectLiveRepo("live-404", "p404", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Repo.ProtectionState != "unknown" || snap.Repo.BranchProtected {
		t.Fatalf("expected unknown protection, got state=%q protected=%v", snap.Repo.ProtectionState, snap.Repo.BranchProtected)
	}
	if step := stepByID(snap.Repo.SyncSteps, "protection"); step == nil || step.State != "unknown" {
		t.Fatalf("protection step must be 'unknown', got %+v", step)
	}
}

func TestFetchFailureIsRecordedWithoutBreakingSnapshot(t *testing.T) {
	dir, run := newTempRepo(t)
	run("remote", "add", "origin", dir+"/does-not-exist.git")
	setWaits(t, time.Second, 5*time.Second)

	snap, err := CollectLiveRepo("live-badremote", "bad", dir, nil)
	if err != nil {
		t.Fatalf("a broken remote must not break the local snapshot: %v", err)
	}
	step := stepByID(snap.Repo.SyncSteps, "fetch")
	if step == nil || step.State != "failed" || step.Detail == "" {
		t.Fatalf("failed fetch must be reported with a reason, got %+v", step)
	}
	if len(snap.Nodes) == 0 {
		t.Fatal("local data must still be collected")
	}
}

func TestNonGitHubRemoteMarksStepsSkippedAndProtectionUnknown(t *testing.T) {
	newGHStub(t)
	dir, run := newTempRepo(t)
	bare := t.TempDir()
	if out, err := runGitOut(bare, "init", "--bare", "-b", "main", bare); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	run("remote", "add", "origin", bare)
	run("push", "-u", "origin", "main")
	setWaits(t, 5*time.Second, 5*time.Second)

	snap, err := CollectLiveRepo("live-plain2", "plain", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"prs", "merged", "protection"} {
		if step := stepByID(snap.Repo.SyncSteps, id); step == nil || step.State != "skipped" {
			t.Fatalf("step %q must be skipped for a non-GitHub remote, got %+v", id, step)
		}
	}
	if snap.Repo.ProtectionState != "unknown" {
		t.Fatalf("non-GitHub remote must not claim 'Korumasız', got %q", snap.Repo.ProtectionState)
	}
	if step := stepByID(snap.Repo.SyncSteps, "fetch"); step == nil || step.State != "ok" {
		t.Fatalf("plain git fetch still runs and must be ok, got %+v", step)
	}
}

func TestSingleFlightDoesNotStartParallelGHRefreshes(t *testing.T) {
	gh := newGHStub(t)
	dir, _ := newGitHubLikeRepo(t)
	gh.set("pr_list_open", openPRsJSON, 0)
	gh.slow("pr_list_open", "0.8")
	setWaits(t, 50*time.Millisecond, 50*time.Millisecond)

	for i := 0; i < 4; i++ {
		if _, err := CollectLiveRepo("live-sf", "sf", dir, nil); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, 10*time.Second, "refresh to finish", func() bool {
		s := stepByID(syncStepsFor(dir), "prs")
		return s != nil && s.State == "ok"
	})
	n := 0
	data, _ := os.ReadFile(gh.dir + "/calls.log")
	for _, c := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.HasPrefix(c, "pr list --state open") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("4 rapid snapshots must share ONE gh pr list call, got %d", n)
	}
}

// Tri-mode parity: the simulation engine reports the same SyncSteps/ProtectionState shape as the live engine,
// so the two-line badge and the protection button behave identically in demo mode.
func TestSimulationSnapshotHasSyncStepsAndProtectionState(t *testing.T) {
	mgr := NewManager("")
	snap, err := mgr.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"fetch", "prs", "merged", "protection"} {
		step := stepByID(snap.Repo.SyncSteps, id)
		if step == nil || step.State != "ok" || step.Label == "" {
			t.Fatalf("simulation must report step %q as ok, got %+v", id, step)
		}
	}
	if prs := stepByID(snap.Repo.SyncSteps, "prs"); prs.Count == 0 {
		t.Fatalf("simulation PR step should count the demo's open PRs, got %+v", prs)
	}
	if snap.Repo.ProtectionState != "none" {
		t.Fatalf("sandbox demo repo is unprotected: %q", snap.Repo.ProtectionState)
	}
	prot, err := mgr.ProtectDefaultBranch()
	if err != nil {
		t.Fatal(err)
	}
	if prot.Repo.ProtectionState != "active" || !prot.Repo.BranchProtected {
		t.Fatalf("after protecting the demo repo the state must be active: %+v", prot.Repo)
	}
	ent, _ := mgr.SwitchRepo("sim-enterprise")
	if ent.Repo.ProtectionState != "active" {
		t.Fatalf("enterprise demo is protected, got %q", ent.Repo.ProtectionState)
	}
}

func TestSyncStepJSONContract(t *testing.T) {
	raw, err := json.Marshal(model.SyncStep{ID: "prs", Label: "PR listesi", State: "ok", Count: 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"id":"prs"`, `"label":"PR listesi"`, `"state":"ok"`, `"count":3`} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("missing %s in %s", key, raw)
		}
	}
	if strings.Contains(string(raw), "durationMs") || strings.Contains(string(raw), `"at"`) || strings.Contains(string(raw), "detail") {
		t.Fatalf("empty optional fields must be omitted: %s", raw)
	}
}

func setPassiveWaits(t *testing.T, gp, fp time.Duration) {
	t.Helper()
	oldG, oldF := ghWaitPassive, fetchWaitPassive
	ghWaitPassive, fetchWaitPassive = gp, fp
	t.Cleanup(func() { ghWaitPassive, fetchWaitPassive = oldG, oldF })
}

// Süre dolumu yüzünden bayatlayan veri hemen gösterilir (eskiden her dolumda ~3 sn bekleniyordu); bir mutasyon
// önbelleği geçersiz kıldıysa yeni sonuç tam süreyle beklenir. Yeni veri gelince BackgroundVersion artar.
func TestExpiredCacheShowsStaleDataFastButMutationsWaitForFresh(t *testing.T) {
	gh := newGHStub(t)
	dir, run := newGitHubLikeRepo(t)
	addFeatureBranch(t, dir, run, "feat/green")
	run("push", "-u", "origin", "feat/green")
	gh.set("pr_list_open", openPRsJSON, 0)
	setWaits(t, 1200*time.Millisecond, 300*time.Millisecond)
	setPassiveWaits(t, 100*time.Millisecond, 50*time.Millisecond)

	mgr := switchLive(t, dir)
	// 1) Veriyi topla (hızlı gh).
	if _, err := CollectLiveRepo("live-x", "x", dir, nil); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "first data", func() bool { return prsOn(mustSnap(t, dir), "feat/green") != nil })
	v0 := mgr.BackgroundVersion()

	// 2) Yalnızca süre doldu, gh artık yavaş: bayat veri hemen gelir.
	gh.slow("pr_list_open", "1.5")
	st := bgFor(dir)
	st.mu.Lock()
	st.ghAt = time.Time{}
	st.mu.Unlock()
	t0 := time.Now()
	snap := mustSnap(t, dir)
	if d := time.Since(t0); d > 700*time.Millisecond {
		t.Fatalf("expiry-only refresh must not wait for the slow gh query, took %v", d)
	}
	if prsOn(snap, "feat/green") == nil {
		t.Fatal("stale PR data must still be shown")
	}
	waitFor(t, 10*time.Second, "background refresh to bump the version", func() bool { return mgr.BackgroundVersion() > v0 })

	// 3) Bir mutasyon geçersiz kıldı: yeni sonuç tam süre (burada 1,2 sn) beklenir.
	InvalidateGHCache(dir)
	t1 := time.Now()
	mustSnap(t, dir)
	if d := time.Since(t1); d < 900*time.Millisecond {
		t.Fatalf("after a mutation the snapshot must wait for fresh data, returned in %v", d)
	}
}

func mustSnap(t *testing.T, dir string) model.CockpitSnapshot {
	t.Helper()
	snap, err := CollectLiveRepo("live-x", "x", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return snap
}
