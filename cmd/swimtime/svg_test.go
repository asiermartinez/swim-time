package main

import (
	"bytes"
	"flag"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNum(t *testing.T) {
	for in, want := range map[float64]string{0: "0", 1: "1", 1.25: "1.2", 1.26: "1.3", 12.5: "12.5", -3: "-3"} {
		if got := num(in); got != want {
			t.Errorf("num(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestPlotX(t *testing.T) {
	p := plot{left: 60, right: 780, lo: 12, hi: 43} // 06:00–21:30
	if p.x(12) != 60 || p.x(43) != 780 {
		t.Errorf("06:00 and 21:30 at %v and %v, want the plot edges", p.x(12), p.x(43))
	}
	if p.mid(12) != 60+p.pitch()/2 {
		t.Errorf("mid(12) = %v, want the centre of the first slot", p.mid(12))
	}
}

// near compares chart coordinates, which go through float arithmetic.
func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// A quiet hour that ends at closing time keeps its full width.
func TestBandAtClosing(t *testing.T) {
	p := plot{left: 60, right: 780, lo: 12, hi: 43}
	var med [slotsPerDay]float64
	var n [slotsPerDay]int
	for s := 12; s < 43; s++ {
		med[s], n[s] = 10, 3
	}
	var c chart
	p.draw(&c, row{top: 0, height: 50, med: &med, n: &n, max: 10, quiet: []Window{{Slot: 41}}})
	if len(c.Quiet) != 1 || !near(c.Quiet[0].W, 2*p.pitch()) || !near(c.Quiet[0].X+c.Quiet[0].W, 780) {
		t.Errorf("got bands %+v, want one hour ending at the right edge", c.Quiet)
	}
}

func TestHourLabels(t *testing.T) {
	p := plot{left: 0, right: 300, lo: 13, hi: 43} // 06:30–21:30
	ls := p.hourLabels(44)
	if len(ls) != 15 || ls[0].Text != "7:00" || ls[14].Text != "21:00" {
		t.Errorf("got %+v", ls)
	}
	if ls[0].X != p.x(14) {
		t.Errorf("labels should sit on hour boundaries: %v", ls[0].X)
	}
}

func TestAxisTicks(t *testing.T) {
	for max, want := range map[float64][2]float64{58: {0, 40}, 587: {0, 400}, 12: {0, 10}, 30: {0, 30}} {
		got := axisTicks(max)
		if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
			t.Errorf("axisTicks(%v) = %v, want %v", max, got, want)
		}
	}
	if got := axisTicks(3); len(got) != 1 {
		t.Errorf("axisTicks(3) = %v, want only 0", got)
	}
}

func TestSmoothPath(t *testing.T) {
	pts := []point{{0, 10}, {10, 0}, {20, 0}, {30, 10}}
	path := smoothPath(pts)
	if !strings.HasPrefix(path, "M0,10 C") || strings.Count(path, "C") != 3 {
		t.Fatalf("got %q", path)
	}
	// Flat between the two minima: the curve must not dip below them.
	if !strings.Contains(path, " C13.3,0 16.7,0 20,0") {
		t.Errorf("flat stretch should stay flat, got %q", path)
	}
	if got := smoothPath(pts[:1]); got != "M0,10 h0" {
		t.Errorf("single point = %q", got)
	}
}

func TestHatch(t *testing.T) {
	// A 12×6 box at the origin: lines x+y=c for c = 0, 6, 12, 18.
	got := hatch(0, 0, 12, 6)
	want := "M-6,6L0,0M0,6L6,0M6,6L12,0M12,6L18,0"
	if got != want {
		t.Errorf("hatch = %q, want %q", got, want)
	}
	// Same grid wherever the box is, so neighbouring bands line up.
	if got := hatch(6, 0, 12, 6); !strings.HasPrefix(got, "M0,6L6,0") {
		t.Errorf("shifted hatch = %q", got)
	}
}

func TestSlotRange(t *testing.T) {
	stats := []*Stats{{Pool: testPools(t, twoPools)[1]}, {Pool: testPools(t, twoPools)[0]}}
	if lo, hi := slotRange(stats); lo != 12 || hi != 43 {
		t.Errorf("got %d, %d; want the union 12, 43", lo, hi)
	}
}

func TestPoolChart(t *testing.T) {
	pools := testPools(t, twoPools)
	st := &Stats{Pool: pools[0], Samples: 1}
	for d := range 7 {
		for s := 16; s < 40; s++ {
			st.Med[d][s], st.N[d][s] = float64(s), 1
		}
	}
	st.derive()

	c := poolChart(es, st, 12, 43)
	if c.W != chartW || len(c.Curves) != 7 {
		t.Errorf("got width %v and %d curves, want %v and 7", c.W, len(c.Curves), chartW)
	}
	if len(c.Quiet) != 7*quietCount {
		t.Fatalf("got %d quiet bands, want %d", len(c.Quiet), 7*quietCount)
	}
	best := 0
	for _, b := range c.Quiet {
		if b.Best {
			best++
		}
	}
	if best != 7 {
		t.Errorf("got %d best bands, want one per day", best)
	}
	// Occupancy grows through the day: the quietest hour is 08:00–09:00.
	p := plot{left: 44, right: chartW - axisW, lo: 12, hi: 43}
	if b := c.Quiet[0]; !b.Best || !near(b.X, p.x(16)) || !near(b.W, p.x(18)-p.x(16)) {
		t.Errorf("first band = %+v, want the 08:00 hour", b)
	}
	var accent []string
	for _, l := range c.Labels {
		if l.Accent {
			accent = append(accent, l.Text)
		}
	}
	if len(accent) != 14 || accent[0] != "08:00" || accent[1] != "~16" {
		t.Errorf("accented labels = %v, want time and count of each day's best hour", accent)
	}

	if c := poolChart(es, &Stats{Pool: pools[0]}, 12, 43); len(c.Curves)+len(c.Quiet) != 0 {
		t.Error("a pool without data should have no curves")
	}
}

func TestSummary(t *testing.T) {
	pools := testPools(t, twoPools)
	withData := &Stats{Pool: pools[0], Samples: 1}
	for s := 20; s < 24; s++ {
		withData.DayMed[s], withData.DayN[s] = float64(100-s), 1
	}
	withData.derive()

	c := summaryRow(es, withData, 12, 43)
	if c.W != chartW || len(c.Curves) != 1 || len(c.Quiet) != 1 || !c.Quiet[0].Best {
		t.Errorf("got width %v, %d curves and %d quiet bands, want %v and one of each", c.W, len(c.Curves), len(c.Quiet), chartW)
	}
	if c.Labels[0].Text != "Grande" || c.Labels[0].Off {
		t.Errorf("first label = %+v, want the pool name", c.Labels[0])
	}

	c = summaryRow(es, &Stats{Pool: pools[1]}, 12, 43)
	if len(c.Curves) != 0 || len(c.Labels) != 2 || !c.Labels[0].Off || c.Labels[1].Text != "sin datos" {
		t.Errorf("pool without data = %+v, want a dimmed name and sin datos", c.Labels)
	}

	if h := summaryHeader(es, 12, 43); h.H != headerH || len(h.Heat) != swatches {
		t.Errorf("header = %v tall with %d swatches, want %v and %d", h.H, len(h.Heat), headerH, swatches)
	}
	if h := summaryHeader(es, 0, 0); len(h.Labels) != 1 || h.Labels[0].Text != "Sin datos" {
		t.Errorf("empty header = %+v", h.Labels)
	}
}

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// TestPoolChartGolden renders a fixed pool and compares it with
// testdata/pool.golden.svg, so any change to the chart output shows up as a
// diff. After an intended change, refresh it with:
//
//	go test ./cmd/swimtime -run Golden -update
func TestPoolChartGolden(t *testing.T) {
	pool := testPools(t, twoPools)[0] // weekdays from 06:00, weekends from 08:00
	st := &Stats{Pool: pool, Samples: 1}
	for d := range 7 {
		for s := range slotsPerDay {
			if pool.SlotOpen(d, s) {
				st.Med[d][s], st.N[d][s] = float64(5+(s*7+d*11)%37), 1
			}
		}
	}
	st.derive()
	lo, hi := pool.SlotRange()

	var got bytes.Buffer
	if err := svgTmpl.Execute(&got, poolChart(es, st, lo, hi)); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "pool.golden.svg")
	if *update {
		if err := os.WriteFile(path, got.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("chart differs from %s; if the change is intended, rerun with -update and review the diff", path)
	}
}
