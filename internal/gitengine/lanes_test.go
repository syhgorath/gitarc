package gitengine

import (
	"fmt"
	"testing"

	"gitarc/internal/model"
)

func laneOf(t *testing.T, branches []model.BranchArc, id string) int {
	t.Helper()
	for _, b := range branches {
		if b.ID == id {
			return b.Lane
		}
	}
	t.Fatalf("branch %q not found", id)
	return 0
}

func TestAssignLanesEmptyAndNil(t *testing.T) {
	AssignLanes(nil)
	AssignLanes([]model.BranchArc{})
}

func TestAssignLanesDefaultsEmptyHemisphereToUpper(t *testing.T) {
	branches := []model.BranchArc{{ID: "a", Name: "a", State: "merged", StartSpineIndex: 0, EndSpineIndex: 1}}
	AssignLanes(branches)
	if branches[0].Hemisphere != "upper" {
		t.Fatalf("expected empty hemisphere defaulted to upper, got %q", branches[0].Hemisphere)
	}
	if branches[0].Lane != 1 {
		t.Fatalf("expected lane 1, got %d", branches[0].Lane)
	}
}

func TestAssignLanesHemispheresPackIndependently(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "u", Name: "u", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 3},
		{ID: "l", Name: "l", Hemisphere: "lower", State: "merged", StartSpineIndex: 0, EndSpineIndex: 3},
	}
	AssignLanes(branches)
	if laneOf(t, branches, "u") != 1 || laneOf(t, branches, "l") != 1 {
		t.Fatalf("overlapping branches in different hemispheres must both be lane 1: %+v", branches)
	}
}

func TestAssignLanesOverlappingMergedGetDistinctLanes(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "a", Name: "a", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 4},
		{ID: "b", Name: "b", Hemisphere: "upper", State: "merged", StartSpineIndex: 1, EndSpineIndex: 5},
		{ID: "c", Name: "c", Hemisphere: "upper", State: "merged", StartSpineIndex: 2, EndSpineIndex: 6},
	}
	AssignLanes(branches)
	seen := map[int]bool{}
	for _, b := range branches {
		if b.Lane < 1 {
			t.Fatalf("lane must be >= 1, got %d for %s", b.Lane, b.ID)
		}
		if seen[b.Lane] {
			t.Fatalf("overlapping branches share lane %d: %+v", b.Lane, branches)
		}
		seen[b.Lane] = true
	}
}

func TestAssignLanesBackToBackMergedShareLane(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "a", Name: "a", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 2},
		{ID: "b", Name: "b", Hemisphere: "upper", State: "merged", StartSpineIndex: 2, EndSpineIndex: 4},
	}
	AssignLanes(branches)
	if laneOf(t, branches, "a") != 1 || laneOf(t, branches, "b") != 1 {
		t.Fatalf("back-to-back merged arches must share lane 1: %+v", branches)
	}
}

func TestAssignLanesOpenBranchReservesTipBuffer(t *testing.T) {
	// An open branch ending at index 2 reserves up to index 3 for its capsule,
	// so a branch starting at 2 must not share its lane.
	branches := []model.BranchArc{
		{ID: "open", Name: "open", Hemisphere: "upper", State: "open", StartSpineIndex: 1, EndSpineIndex: 2},
		{ID: "next", Name: "next", Hemisphere: "upper", State: "merged", StartSpineIndex: 2, EndSpineIndex: 3},
	}
	AssignLanes(branches)
	if laneOf(t, branches, "open") == laneOf(t, branches, "next") {
		t.Fatalf("open capsule buffer should push overlapping branch to another lane: %+v", branches)
	}
}

func TestAssignLanesInvertedRangeIsClamped(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "x", Name: "x", Hemisphere: "upper", State: "merged", StartSpineIndex: 5, EndSpineIndex: 2},
	}
	AssignLanes(branches)
	if branches[0].Lane != 1 {
		t.Fatalf("inverted range should still get lane 1, got %d", branches[0].Lane)
	}
}

func TestAssignLanesNoLaneCap(t *testing.T) {
	// Regression: the old MaxHemisphereLanes=4 cap is gone; N fully overlapping
	// branches must occupy N distinct lanes.
	const n = 12
	var branches []model.BranchArc
	for i := 0; i < n; i++ {
		branches = append(branches, model.BranchArc{
			ID: fmt.Sprintf("b%02d", i), Name: fmt.Sprintf("b%02d", i),
			Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 5,
		})
	}
	AssignLanes(branches)
	max := 0
	seen := map[int]bool{}
	for _, b := range branches {
		seen[b.Lane] = true
		if b.Lane > max {
			max = b.Lane
		}
	}
	if max != n || len(seen) != n {
		t.Fatalf("expected %d distinct lanes up to %d, got max=%d distinct=%d", n, n, max, len(seen))
	}
}

func TestAssignLanesDeterministic(t *testing.T) {
	build := func() []model.BranchArc {
		return []model.BranchArc{
			{ID: "c", Name: "c", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 3},
			{ID: "a", Name: "a", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 3},
			{ID: "b", Name: "b", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 3},
		}
	}
	first, second := build(), build()
	AssignLanes(first)
	AssignLanes(second)
	for i := range first {
		if first[i].Lane != second[i].Lane {
			t.Fatalf("non-deterministic lanes: %+v vs %+v", first, second)
		}
	}
	// Ties broken by name: a < b < c.
	if laneOf(t, first, "a") != 1 || laneOf(t, first, "b") != 2 || laneOf(t, first, "c") != 3 {
		t.Fatalf("expected name tie-break a=1 b=2 c=3, got %+v", first)
	}
}

func TestAssignLanesShorterSpanGetsInnerLane(t *testing.T) {
	branches := []model.BranchArc{
		{ID: "long", Name: "a-long", Hemisphere: "upper", State: "merged", StartSpineIndex: 0, EndSpineIndex: 8},
		{ID: "short", Name: "z-short", Hemisphere: "upper", State: "merged", StartSpineIndex: 3, EndSpineIndex: 4},
	}
	AssignLanes(branches)
	if laneOf(t, branches, "short") != 1 || laneOf(t, branches, "long") != 2 {
		t.Fatalf("short span must nest inside long span: %+v", branches)
	}
}
