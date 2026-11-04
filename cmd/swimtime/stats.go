package main

import (
	"slices"
	"sort"
	"time"
)

// Stats is the aggregated occupancy of one pool over the analysis window.
type Stats struct {
	Pool *Pool
	// Med and N are the median occupancy and sample count per weekday
	// (Monday first) and 30-minute slot. Slots with N == 0 have no data.
	Med [7][slotsPerDay]float64
	N   [7][slotsPerDay]int
	// DayMed and DayN aggregate all weekdays per slot, for the summary.
	DayMed  [slotsPerDay]float64
	DayN    [slotsPerDay]int
	Overall float64 // median of every kept sample
	Max     float64 // highest value of Med
	DayMax  float64 // highest value of DayMed
	// Quiet and DayQuiet are the quiet hours of each weekday and of the
	// combined day: up to quietCount windows, quietest first.
	Quiet    [7][]Window
	DayQuiet []Window
	Samples  int
}

// Quiet hours are one-hour windows; the charts mark three per day, starting
// at least two hours apart.
const (
	quietCount = 3
	quietGap   = 4 // slots
)

// Window is a one-hour block (two consecutive slots) starting at Slot.
type Window struct {
	Slot  int
	Value float64 // mean of the two slots' medians
}

func (s *Stats) HasData() bool { return s.Samples > 0 }

// computeStats aggregates samples per pool. Days on which a pool reported
// zero people all day are treated as closed and dropped.
func computeStats(pools []*Pool, samples []Sample) []*Stats {
	type dayKey struct {
		pool string
		date string
	}
	byDay := map[dayKey][]Sample{}
	for _, s := range samples {
		k := dayKey{s.Pool, s.T.Format(time.DateOnly)}
		byDay[k] = append(byDay[k], s)
	}

	type bucket struct {
		cells [7][slotsPerDay][]int
		day   [slotsPerDay][]int
		all   []int
	}
	buckets := map[string]*bucket{}
	for _, p := range pools {
		buckets[p.Slug] = &bucket{}
	}
	poolBySlug := map[string]*Pool{}
	for _, p := range pools {
		poolBySlug[p.Slug] = p
	}
	for k, day := range byDay {
		b, p := buckets[k.pool], poolBySlug[k.pool]
		if b == nil || allZero(day) {
			continue
		}
		for _, s := range day {
			d, slot := weekday(s.T), minuteOfDay(s.T)/slotMinutes
			if !p.SlotOpen(d, slot) {
				continue
			}
			b.cells[d][slot] = append(b.cells[d][slot], s.Count)
			b.day[slot] = append(b.day[slot], s.Count)
			b.all = append(b.all, s.Count)
		}
	}

	out := make([]*Stats, len(pools))
	for i, p := range pools {
		b := buckets[p.Slug]
		st := &Stats{Pool: p, Samples: len(b.all), Overall: median(b.all)}
		for d := range 7 {
			for s := range slotsPerDay {
				if v := b.cells[d][s]; len(v) > 0 {
					st.Med[d][s], st.N[d][s] = median(v), len(v)
				}
			}
		}
		for s := range slotsPerDay {
			if v := b.day[s]; len(v) > 0 {
				st.DayMed[s], st.DayN[s] = median(v), len(v)
			}
		}
		st.derive()
		out[i] = st
	}
	return out
}

// derive fills the fields computed from Med, N, DayMed and DayN.
func (st *Stats) derive() {
	st.Max, st.DayMax = 0, 0
	for d := range 7 {
		for s := range slotsPerDay {
			st.Max = max(st.Max, st.Med[d][s])
		}
		st.Quiet[d] = quietWindows(&st.Med[d], &st.N[d], quietCount, quietGap)
	}
	for s := range slotsPerDay {
		st.DayMax = max(st.DayMax, st.DayMed[s])
	}
	st.DayQuiet = quietWindows(&st.DayMed, &st.DayN, quietCount, quietGap)
}

// quietWindows returns up to k one-hour windows with the lowest mean of
// med, quietest first. Their starts are at least gap slots apart, so the
// picks are different moments of the day rather than one long quiet stretch.
// Ties go to the earliest window.
func quietWindows(med *[slotsPerDay]float64, n *[slotsPerDay]int, k, gap int) []Window {
	var cand []Window
	for s := 0; s+1 < slotsPerDay; s++ {
		if n[s] == 0 || n[s+1] == 0 {
			continue
		}
		cand = append(cand, Window{Slot: s, Value: (med[s] + med[s+1]) / 2})
	}
	sort.SliceStable(cand, func(i, j int) bool { return cand[i].Value < cand[j].Value })
	var out []Window
	for _, w := range cand {
		if len(out) == k {
			break
		}
		if !slices.ContainsFunc(out, func(o Window) bool { return abs(w.Slot-o.Slot) < gap }) {
			out = append(out, w)
		}
	}
	return out
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func allZero(samples []Sample) bool {
	for _, s := range samples {
		if s.Count != 0 {
			return false
		}
	}
	return true
}

func median(v []int) float64 {
	if len(v) == 0 {
		return 0
	}
	c := append([]int(nil), v...)
	sort.Ints(c)
	m := len(c) / 2
	if len(c)%2 == 1 {
		return float64(c[m])
	}
	return float64(c[m-1]+c[m]) / 2
}
