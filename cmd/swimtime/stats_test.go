package main

import (
	"slices"
	"testing"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		in   []int
		want float64
	}{
		{nil, 0},
		{[]int{4}, 4},
		{[]int{9, 1, 5}, 5},
		{[]int{8, 2, 4, 6}, 5},
	}
	for _, tt := range tests {
		if got := median(tt.in); got != tt.want {
			t.Errorf("median(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
	in := []int{3, 1, 2}
	median(in)
	if in[0] != 3 {
		t.Error("median must not reorder its input")
	}
}

func TestComputeStats(t *testing.T) {
	pools := testPools(t, twoPools)
	samples := []Sample{
		// Monday 21: two readings in the 10:00 slot and one at 10:30.
		{T: at(2026, 9, 21, 10, 0), Pool: "big", Count: 10},
		{T: at(2026, 9, 21, 10, 15), Pool: "big", Count: 20},
		{T: at(2026, 9, 21, 10, 30), Pool: "big", Count: 4},
		// Monday 28: another reading in the 10:00 slot.
		{T: at(2026, 9, 28, 10, 5), Pool: "big", Count: 60},
		// Saturday 26 at 07:00 is outside the weekend hours: ignored.
		{T: at(2026, 9, 26, 7, 0), Pool: "big", Count: 99},
		// Unknown pool: ignored.
		{T: at(2026, 9, 21, 10, 0), Pool: "ghost", Count: 1},
		// "small" is closed all day on the 22nd (only zeros): dropped.
		{T: at(2026, 9, 22, 10, 0), Pool: "small", Count: 0},
		{T: at(2026, 9, 22, 11, 0), Pool: "small", Count: 0},
	}
	stats := computeStats(pools, samples)
	big, small := stats[0], stats[1]

	if big.Samples != 4 {
		t.Errorf("big.Samples = %d, want 4", big.Samples)
	}
	if got := big.Med[0][20]; got != 20 || big.N[0][20] != 3 { // median of 10, 20, 60
		t.Errorf("Monday 10:00 = %v (n=%d), want 20 (n=3)", got, big.N[0][20])
	}
	if got := big.Med[0][21]; got != 4 {
		t.Errorf("Monday 10:30 = %v, want 4", got)
	}
	if big.N[5][14] != 0 {
		t.Error("readings outside opening hours must be ignored")
	}
	if big.Max != 20 {
		t.Errorf("Max = %v, want 20", big.Max)
	}
	if big.DayMed[20] != 20 || big.DayN[20] != 3 {
		t.Errorf("DayMed[10:00] = %v (n=%d)", big.DayMed[20], big.DayN[20])
	}
	if got := big.Overall; got != 15 { // median of 4, 10, 20, 60
		t.Errorf("Overall = %v, want 15", got)
	}
	if small.HasData() {
		t.Error("a day with only zeros must be dropped as closed")
	}

	// The only full hour with data is Monday 10:00–11:00.
	if want := []Window{{Slot: 20, Value: 12}}; !slices.Equal(big.Quiet[0], want) {
		t.Errorf("Quiet[Mon] = %+v, want %+v", big.Quiet[0], want)
	}
	if len(big.Quiet[1]) != 0 {
		t.Errorf("Quiet[Tue] = %+v, want none", big.Quiet[1])
	}
	if want := []Window{{Slot: 20, Value: 12}}; !slices.Equal(big.DayQuiet, want) {
		t.Errorf("DayQuiet = %+v, want %+v", big.DayQuiet, want)
	}
	if big.DayMax != 20 {
		t.Errorf("DayMax = %v, want 20", big.DayMax)
	}
}

func TestQuietWindows(t *testing.T) {
	var med [slotsPerDay]float64
	var n [slotsPerDay]int
	for s := 16; s < 42; s++ { // 08:00–21:00, busier every slot
		med[s], n[s] = float64(s), 1
	}
	med[30], med[31] = 1, 1 // 15:00–16:00 is the quietest

	got := quietWindows(&med, &n, 3, 4)
	want := []Window{{Slot: 30, Value: 1}, {Slot: 16, Value: 16.5}, {Slot: 20, Value: 20.5}}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("window %d = %+v, want %+v (quietest first, starts 2 h apart)", i, got[i], want[i])
		}
	}

	// 14:30–15:30 averages 15, quieter than 08:00, but it starts less than
	// two hours from the quietest window, so it is skipped.
	for _, w := range got[1:] {
		if abs(w.Slot-30) < 4 {
			t.Errorf("window at slot %d is too close to the quietest one", w.Slot)
		}
	}
	if got := quietWindows(&med, &n, 1, 0); len(got) != 1 || got[0].Slot != 30 {
		t.Errorf("k=1: got %+v", got)
	}
}

func TestQuietWindowsEdgeCases(t *testing.T) {
	var med [slotsPerDay]float64
	var n [slotsPerDay]int
	set := func(slot int, v float64) { med[slot], n[slot] = v, 1 }

	if got := quietWindows(&med, &n, 3, 4); len(got) != 0 {
		t.Errorf("no data should give no windows, got %+v", got)
	}

	set(16, 5) // 08:00, alone: not a full hour
	set(20, 9)
	set(21, 1) // 10:00–11:00 averages 5
	set(30, 4)
	set(31, 6) // 15:00–16:00 also averages 5, later
	if got := quietWindows(&med, &n, 1, 0); len(got) != 1 || got[0] != (Window{Slot: 20, Value: 5}) {
		t.Errorf("got %+v; want slot 20 (the earliest tie) with 5", got)
	}

	set(35, 0)
	set(37, 0) // 17:30 and 18:30 are not consecutive
	if got := quietWindows(&med, &n, 1, 0); got[0].Slot != 20 {
		t.Errorf("windows must not bridge a gap, got slot %d", got[0].Slot)
	}
}
