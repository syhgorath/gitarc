package gitengine

import (
	"sort"

	"gitarc/internal/model"
)

type interval struct {
	idx   int
	start int
	end   int
	span  int
}

// AssignLanes performs deterministic interval packing independently for
// the upper (Remote/PR) and lower (Local/Unpushed) hemispheres.
// Shorter spans are placed on inner lanes closest to the central spine (Lane = 1)
// so that parabolic arches nest cleanly without crossing.
func AssignLanes(branches []model.BranchArc) {
	assignHemisphereLanes(branches, "upper")
	assignHemisphereLanes(branches, "lower")
}

func assignHemisphereLanes(branches []model.BranchArc, hemisphere string) {
	var items []interval
	for i := range branches {
		b := &branches[i]
		h := b.Hemisphere
		if h == "" {
			h = "upper"
			b.Hemisphere = h
		}
		if h != hemisphere {
			continue
		}
		// Gerçek bir konumu olmayan geçmiş dalları çizilmez, şerit de tüketmez.
		if b.HistoryOnly {
			continue
		}
		start := b.StartSpineIndex
		end := b.EndSpineIndex
		if end < start {
			end = start
		}
		// Open/gone branches render a Branch Pair Capsule at their tip,
		// so reserve extra horizontal clearance to avoid overlapping capsules.
		effectiveEnd := end
		if b.State != "merged" {
			effectiveEnd = end + 1
		}
		span := effectiveEnd - start
		if span < 1 {
			span = 1
		}
		items = append(items, interval{
			idx:   i,
			start: start,
			end:   effectiveEnd,
			span:  span,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].span != items[j].span {
			return items[i].span < items[j].span
		}
		if items[i].start != items[j].start {
			return items[i].start < items[j].start
		}
		return branches[items[i].idx].Name < branches[items[j].idx].Name
	})

	// laneIntervals[lane-1] stores non-overlapping [start, end] ranges already placed on that lane.
	type spanRange struct{ s, e int }
	var laneRanges [][]spanRange

	for _, it := range items {
		placed := false
		for laneIdx := range laneRanges {
			overlaps := false
			for _, existing := range laneRanges[laneIdx] {
				// Overlap occurs unless one interval finishes strictly at or before the other starts
				// (using > rather than >= allows back-to-back merged arches on the same lane,
				// while open capsules already include +1 tip buffer).
				if it.start < existing.e && it.end > existing.s {
					overlaps = true
					break
				}
			}
			if !overlaps {
				laneRanges[laneIdx] = append(laneRanges[laneIdx], spanRange{s: it.start, e: it.end})
				branches[it.idx].Lane = laneIdx + 1
				placed = true
				break
			}
		}
		if !placed {
			laneRanges = append(laneRanges, []spanRange{{s: it.start, e: it.end}})
			branches[it.idx].Lane = len(laneRanges)
		}
	}
}
