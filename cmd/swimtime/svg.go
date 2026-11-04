package main

import (
	"embed"
	"encoding/base64"
	"fmt"
	"html"
	"math"
	"strings"
	"text/template"
)

// Every chart uses the full width GitHub gives a README image. Each row is a
// typical-day curve with a shaded attendance strip under it and the start
// time and occupancy of its quiet hours below that.
const (
	chartW    = 838.0
	axisW     = 48.0 // right margin for the y-axis ticks and labels, e.g. "- 400"
	headerH   = 64.0 // legend and hour labels
	bandPad   = 6.0  // quiet-hour bands reach this far above the plot
	stripGap  = 3.0  // between the baseline and the attendance strip
	stripH    = 6.0
	rowBelow  = 48.0 // strip and quiet-hour labels under the baseline
	minHeat   = 0.06 // strip opacity of an empty slot, so it stays visible
	swatches  = 5    // legend steps of the attendance strip
	swatchW   = 14.0
	hatchStep = 6.0 // px between quiet-hour hatch lines, along x
	emptyW    = 480.0
)

// JetBrains Mono, subset to the characters the charts use. SVGs shown as
// <img> cannot load external fonts, so it is inlined in every chart.
//
//go:embed fonts/*.woff2
var fontsFS embed.FS

var fonts = map[string]string{
	"FontSemiBold": mustFontDataURI("fonts/JetBrainsMono-SemiBold.subset.woff2"),
}

func mustFontDataURI(name string) string {
	b, err := fontsFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return "data:font/woff2;base64," + base64.StdEncoding.EncodeToString(b)
}

// chart is the geometry handed to templates/chart.svg.tmpl.
type chart struct {
	Title  string
	W, H   float64
	Rules  []line   // hour grid, drawn behind everything
	Ticks  []line   // y-axis ticks
	Heat   []rect   // attendance strips and the legend swatches
	Curves []string // typical-day curves, as SVG paths
	Quiet  []band
	Labels []label
}

type line struct{ X1, Y1, X2, Y2 float64 }

// rect is a heat cell; Opacity scales the ink colour with attendance.
type rect struct{ X, Y, W, H, Opacity float64 }

// band is a quiet hour: a hatched column with the curve drawn over it in a
// stronger stroke. Hatch and Curve overflow the column; the template clips
// both to it.
type band struct {
	X, Y, W, H float64
	Hatch      string
	Curve      string
	Best       bool // quietest hour of the row, drawn in the accent colour
}

type label struct {
	X, Y   float64
	Text   string
	Anchor string // start, middle or end
	Muted  bool
	Off    bool // name of a pool without data
	Accent bool // quietest hour of the row
}

var svgTmpl = template.Must(template.New("chart.svg.tmpl").Funcs(template.FuncMap{
	"f":    num,
	"o":    func(v float64) string { return fmt.Sprintf("%.2f", v) },
	"x":    html.EscapeString,
	"font": func(name string) string { return fonts[name] },
}).ParseFS(templatesFS, "templates/chart.svg.tmpl"))

// num formats a coordinate with at most one decimal.
func num(v float64) string {
	return strings.TrimSuffix(strings.TrimRight(fmt.Sprintf("%.1f", v), "0"), ".")
}

// plot maps the slots [lo, hi) onto x positions from left to right. Each
// slot spans pitch, from the start of slot lo at left to the end of slot
// hi-1 at right, so a window ending at closing time reaches the edge.
type plot struct {
	left, right float64
	lo, hi      int
}

func (p plot) pitch() float64 { return (p.right - p.left) / float64(max(p.hi-p.lo, 1)) }

// x is where slot starts; it is also where slot-1 ends.
func (p plot) x(slot int) float64 { return p.left + float64(slot-p.lo)*p.pitch() }

// mid is the centre of slot, where its sample is drawn.
func (p plot) mid(slot int) float64 { return p.x(slot) + p.pitch()/2 }

// row describes one curve whose plot area starts at top.
type row struct {
	top, height float64
	med         *[slotsPerDay]float64
	n           *[slotsPerDay]int
	max         float64  // occupancy at the top of the plot
	quiet       []Window // quietest first
}

func (r row) baseline() float64 { return r.top + r.height }

func (r row) y(v float64) float64 { return r.baseline() - ratio(v, r.max)*r.height }

// rowHeight is the vertical space a row with a plot of the given height
// takes, including the bands' overhang and everything under the baseline.
func rowHeight(plotH float64) float64 { return 2*bandPad + plotH + rowBelow }

// header draws the legend of the attendance strip and the hour labels.
// top is the occupancy the darkest swatch stands for.
func (p plot) header(c *chart, top string) {
	x := chartW - axisW - swatches*swatchW
	c.Labels = append(c.Labels,
		label{X: x - 8, Y: 12, Text: "0", Anchor: "end", Muted: true},
		label{X: chartW - axisW + 16, Y: 12, Text: top, Anchor: "start", Muted: true},
	)
	for i := range swatches {
		c.Heat = append(c.Heat, rect{X: x + float64(i)*swatchW, Y: 4, W: swatchW, H: 8, Opacity: heat(float64(i) / (swatches - 1))})
	}
	c.Labels = append(c.Labels, p.hourLabels(headerH-20)...)
}

// hourLabels puts a label on every full hour of the plot, including its
// right edge when that falls on one.
func (p plot) hourLabels(y float64) []label {
	var out []label
	perHour := 60 / slotMinutes
	for s := p.lo; s <= p.hi; s++ {
		if s%perHour == 0 {
			out = append(out, label{X: p.x(s), Y: y, Text: fmt.Sprintf("%d:00", s/perHour), Anchor: "middle", Muted: true})
		}
	}
	return out
}

// draw adds the grid, axis, strip, curve and quiet hours of r.
func (p plot) draw(c *chart, r row) {
	perHour := 60 / slotMinutes
	for s := p.lo; s <= p.hi; s++ {
		if s%perHour == 0 {
			c.Rules = append(c.Rules, line{p.x(s), r.top - bandPad, p.x(s), r.baseline()})
		}
	}
	for _, v := range axisTicks(r.max) {
		y := r.y(v)
		c.Ticks = append(c.Ticks, line{chartW - axisW + 6, y, chartW - axisW + 11, y})
		c.Labels = append(c.Labels, label{X: chartW - axisW + 16, Y: y + 4, Text: fmt.Sprintf("%.0f", v), Anchor: "start", Muted: true})
	}

	for s := p.lo; s < p.hi; s++ {
		if r.n[s] == 0 {
			continue
		}
		c.Heat = append(c.Heat, rect{X: p.x(s), Y: r.baseline() + stripGap, W: p.pitch(), H: stripH, Opacity: heat(ratio(r.med[s], r.max))})
	}

	for _, seg := range p.segments(r.n) {
		// Samples sit at slot centres; the ends run flat to the edges of
		// the first and last slot so the curve covers the same span as the
		// strip and the bands.
		first, last := seg[0], seg[len(seg)-1]
		pts := []point{{p.x(first), r.y(r.med[first])}}
		for _, s := range seg {
			pts = append(pts, point{p.mid(s), r.y(r.med[s])})
		}
		pts = append(pts, point{p.x(last + 1), r.y(r.med[last])})
		path := smoothPath(pts)
		c.Curves = append(c.Curves, path)
		for i, w := range r.quiet {
			if w.Slot < seg[0] || w.Slot > seg[len(seg)-1] {
				continue
			}
			b := band{X: p.x(w.Slot), Y: r.top - bandPad, W: 2 * p.pitch(), H: r.height + bandPad, Curve: path, Best: i == 0}
			b.Hatch = hatch(b.X, b.Y, b.W, b.H)
			c.Quiet = append(c.Quiet, b)
		}
	}
	for i, w := range r.quiet {
		x := p.x(w.Slot + 1)
		y := r.baseline() + stripGap + stripH
		c.Labels = append(c.Labels,
			label{X: x, Y: y + 15, Text: clock(w.Slot), Anchor: "middle", Muted: i != 0, Accent: i == 0},
			label{X: x, Y: y + 28, Text: fmt.Sprintf("~%.0f", w.Value), Anchor: "middle", Muted: i != 0, Accent: i == 0},
		)
	}
}

// hatch draws 45° lines across a rectangle, as one path to be clipped to
// it. Real lines replace an SVG pattern: pattern tiles cut the stroke at
// their edges, which shows as broken stripes once GitHub scales the chart.
// Lines sit on a grid shared by the whole chart, so every band matches.
func hatch(x, y, w, h float64) string {
	var b strings.Builder
	bottom := y + h
	// Each line is x + y = c, running up and to the right.
	for k := math.Ceil((x + bottom - h) / hatchStep); k*hatchStep <= x+w+bottom; k++ {
		c := k * hatchStep
		fmt.Fprintf(&b, "M%s,%sL%s,%s", num(c-bottom), num(bottom), num(c-y), num(y))
	}
	return b.String()
}

// segments splits [lo, hi) into runs of consecutive slots with data.
func (p plot) segments(n *[slotsPerDay]int) [][]int {
	var out [][]int
	var run []int
	for s := p.lo; s < p.hi; s++ {
		if n[s] > 0 {
			run = append(run, s)
			continue
		}
		if len(run) > 0 {
			out, run = append(out, run), nil
		}
	}
	if len(run) > 0 {
		out = append(out, run)
	}
	return out
}

// heat is the strip opacity for an attendance ratio.
func heat(r float64) float64 { return math.Max(r, minHeat) }

// axisTicks returns 0 and the largest round number not above max.
func axisTicks(max float64) []float64 {
	step := 500.0
	for _, s := range []float64{5, 10, 20, 25, 50, 100, 200, 250} {
		if max/s <= 3 {
			step = s
			break
		}
	}
	if top := math.Floor(max/step) * step; top > 0 {
		return []float64{0, top}
	}
	return []float64{0}
}

type point struct{ X, Y float64 }

// smoothPath draws a monotone cubic through pts (Fritsch–Carlson), so the
// curve never overshoots the samples: it can't show a dip or a peak that
// isn't in the data.
func smoothPath(pts []point) string {
	if len(pts) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "M%s,%s", num(pts[0].X), num(pts[0].Y))
	if len(pts) == 1 {
		fmt.Fprintf(&b, " h0")
		return b.String()
	}
	n := len(pts)
	d := make([]float64, n-1) // secant slopes
	for i := range d {
		d[i] = (pts[i+1].Y - pts[i].Y) / (pts[i+1].X - pts[i].X)
	}
	m := make([]float64, n) // tangents
	m[0], m[n-1] = d[0], d[n-2]
	for i := 1; i < n-1; i++ {
		if d[i-1]*d[i] > 0 {
			m[i] = (d[i-1] + d[i]) / 2
		}
	}
	for i, di := range d {
		if di == 0 {
			m[i], m[i+1] = 0, 0
			continue
		}
		a, c := m[i]/di, m[i+1]/di
		if h := a*a + c*c; h > 9 {
			t := 3 / math.Sqrt(h)
			m[i], m[i+1] = t*a*di, t*c*di
		}
	}
	for i := range d {
		h := (pts[i+1].X - pts[i].X) / 3
		fmt.Fprintf(&b, " C%s,%s %s,%s %s,%s",
			num(pts[i].X+h), num(pts[i].Y+m[i]*h),
			num(pts[i+1].X-h), num(pts[i+1].Y-m[i+1]*h),
			num(pts[i+1].X), num(pts[i+1].Y))
	}
	return b.String()
}

// slotRange returns the union of the opening slots of every pool. The
// summary uses it so every row shares the same hours and width.
func slotRange(stats []*Stats) (lo, hi int) {
	lo, hi = slotsPerDay, 0
	for _, st := range stats {
		l, h := st.Pool.SlotRange()
		if l < h {
			lo, hi = min(lo, l), max(hi, h)
		}
	}
	return lo, hi
}

// poolChart draws one row per weekday, all on the pool's own scale.
func poolChart(l locale, st *Stats, lo, hi int) chart {
	const (
		left  = 44.0
		plotH = 44.0
	)
	title := fmt.Sprintf(l.Title, st.Pool.Name)
	if !st.HasData() {
		return emptyChart(title, l.NoDataWeeks)
	}
	p := plot{left: left, right: chartW - axisW, lo: lo, hi: hi}
	rowH := rowHeight(plotH)
	c := chart{Title: title, W: chartW, H: headerH + 7*rowH}
	p.header(&c, fmt.Sprintf("%.0f", st.Max))
	for d := range 7 {
		r := row{top: headerH + float64(d)*rowH + 2*bandPad, height: plotH, med: &st.Med[d], n: &st.N[d], max: st.Max, quiet: st.Quiet[d]}
		c.Labels = append(c.Labels, label{X: 0, Y: r.baseline(), Text: l.Days[d], Anchor: "start"})
		p.draw(&c, r)
	}
	return c
}

// The summary is one image per pool, stacked under a header image, so each
// row can link to the pool's section: links inside an SVG shown as <img>
// don't work. Every row has its own scale, so small pools keep their shape
// next to the big ones; the axis labels give the numbers. Pool names sit
// above their row, so the plot can use almost the full width.
const (
	summaryLeft  = 20.0 // room for half of the first hour label
	summaryPlotH = 44.0 // same as the pool charts
	summaryNameH = 18.0 // pool name above each row
)

// summaryHeader draws the legend and hour labels shared by every row.
func summaryHeader(l locale, lo, hi int) chart {
	title := l.SummaryTitle
	if lo >= hi {
		return emptyChart(title, l.NoData)
	}
	p := plot{left: summaryLeft, right: chartW - axisW, lo: lo, hi: hi}
	c := chart{Title: title, W: chartW, H: headerH}
	p.header(&c, l.Max)
	return c
}

// summaryRow draws one pool with every weekday combined, on the hours
// [lo, hi) shared by all rows.
func summaryRow(l locale, st *Stats, lo, hi int) chart {
	p := plot{left: summaryLeft, right: chartW - axisW, lo: lo, hi: hi}
	c := chart{Title: fmt.Sprintf(l.Title, st.Pool.Name), W: chartW, H: summaryNameH + rowHeight(summaryPlotH)}
	r := row{top: summaryNameH + 2*bandPad, height: summaryPlotH, med: &st.DayMed, n: &st.DayN, max: st.DayMax, quiet: st.DayQuiet}
	c.Labels = append(c.Labels, label{X: summaryLeft, Y: r.top - bandPad - 8, Text: st.Pool.Name, Anchor: "start", Off: !st.HasData()})
	if !st.HasData() {
		c.Labels = append(c.Labels, label{X: summaryLeft, Y: r.baseline(), Text: l.NoDataRow, Anchor: "start", Muted: true})
		return c
	}
	p.draw(&c, r)
	return c
}

func ratio(v, max float64) float64 {
	if max <= 0 {
		return 0
	}
	return v / max
}

func emptyChart(title, msg string) chart {
	return chart{
		Title: title, W: emptyW, H: 32,
		Labels: []label{{X: 0, Y: 20, Text: msg, Anchor: "start", Muted: true}},
	}
}
