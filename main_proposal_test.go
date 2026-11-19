package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitarc/internal/gitengine"
	"gitarc/internal/model"
)

func TestProposalApproveEndpoint(t *testing.T) {
	mgr := gitengine.NewManager("")
	prop, err := mgr.ProposeCommit("feat/ai-http", "feat: x (1 dosya)", []string{"internal/x.go"}, "AI PR", "AI body")
	if err != nil {
		t.Fatal(err)
	}
	c := newAPIClient(t, mgr)

	if rec := c.do(http.MethodGet, "/api/proposal/approve", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET must be 405, got %d", rec.Code)
	}
	if rec := c.do(http.MethodPost, "/api/proposal/approve", "{"); rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed JSON must be 400, got %d", rec.Code)
	}

	// Wrong id: refused with a clear message, proposal stays pending.
	rec := c.do(http.MethodPost, "/api/proposal/approve", `{"proposalId":"prop-nope","branchName":"feat/ai-http","message":"m"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "öneri değişmiş") {
		t.Fatalf("stale id: %d %s", rec.Code, rec.Body)
	}
	// A client cannot smuggle extra files: unknown JSON fields are ignored, files always come from the proposal.
	rec = c.do(http.MethodPost, "/api/proposal/approve", `{"proposalId":"`+prop.ID+`","branchName":"feat/ai-http","message":"feat: x (1 dosya)","prTitle":"T","prBody":"B","files":[".env","other.go"]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid approval: %d %s", rec.Code, rec.Body)
	}
	var snap model.CockpitSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if snap.PendingProposal != nil {
		t.Fatal("approved proposal must be gone from the snapshot")
	}
	var found *model.BranchArc
	for i := range snap.Branches {
		if snap.Branches[i].Name == "feat/ai-http" {
			found = &snap.Branches[i]
		}
	}
	if found == nil || found.PR == nil || found.PR.Title != "T" {
		t.Fatalf("branch with PR must exist: %+v", found)
	}
	if strings.Join(found.ChangedFiles, ",") != "internal/x.go" {
		t.Fatalf("client-supplied files must be ignored, got %v", found.ChangedFiles)
	}

	// Second approval of the same proposal fails.
	rec = c.do(http.MethodPost, "/api/proposal/approve", `{"proposalId":"`+prop.ID+`","branchName":"feat/ai-http2","message":"m"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "bekleyen AI önerisi yok") {
		t.Fatalf("second approval must be 400: %d %s", rec.Code, rec.Body)
	}
}
