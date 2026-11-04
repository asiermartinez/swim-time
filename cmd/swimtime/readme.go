package main

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"
)

//go:embed templates/*.tmpl
var templatesFS embed.FS

var readmeTmpls = template.Must(template.New("").Funcs(template.FuncMap{
	"anchor": headingAnchor,
}).ParseFS(templatesFS, "templates/README.*.md.tmpl", "templates/calendar.*.md.tmpl"))

// headingAnchor returns the id GitHub gives a Markdown heading with this
// text: lower case, spaces as hyphens, and punctuation other than hyphens
// and underscores dropped. "C.C. Zabalgana" becomes "cc-zabalgana".
func headingAnchor(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		switch {
		case r == ' ':
			b.WriteRune('-')
		case r == '-' || r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

func cmdRender(args []string) error {
	pools, err := loadPools(poolsFile)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	samples, err := loadSamples(dataDir, windowStart(now))
	if err != nil {
		return err
	}
	r := buildReport(pools, samples, now)
	for _, l := range locales {
		if err := writeReport(r, l, filepath.Join(chartsDir, l.Code), l.ReadmePath); err != nil {
			return err
		}
		dir := filepath.Join(calendarDir, l.Code)
		if err := writeCalendars(l, r.Stats, dir, now); err != nil {
			return err
		}
		if err := writeCalendarIndex(r, l, dir); err != nil {
			return err
		}
	}
	fmt.Printf("%d muestras, %d días → %s, %s/ y %s/\n", len(samples), r.Days, en.ReadmePath, chartsDir, calendarDir)
	return nil
}

// report is everything the README and the charts are built from.
type report struct {
	Stats      []*Stats // quietest pools first; pools without data last
	Weeks      int
	WindowDays int
	Days       int // days from the first sample to now, inclusive
	Partial    bool
	Updated    string
}

// writeCalendarIndex writes the page listing the links to dir's feeds.
func writeCalendarIndex(r report, l locale, dir string) error {
	return writeTemplate(filepath.Join(dir, "README.md"), readmeTmpls.Lookup(l.CalendarReadme), r)
}

// windowStart is the first instant of the analysis window ending at now.
func windowStart(now time.Time) time.Time {
	return startOfDay(now).AddDate(0, 0, -7*windowWeeks+1)
}

func buildReport(pools []*Pool, samples []Sample, now time.Time) report {
	stats := computeStats(pools, samples)
	sort.SliceStable(stats, func(i, j int) bool {
		a, b := stats[i], stats[j]
		if a.HasData() != b.HasData() {
			return a.HasData()
		}
		return a.Overall < b.Overall
	})
	r := report{
		Stats:      stats,
		Weeks:      windowWeeks,
		WindowDays: 7 * windowWeeks,
		Updated:    now.Format("02/01/2006 15:04"),
	}
	if len(samples) > 0 {
		first := samples[0].T
		for _, s := range samples[1:] {
			if s.T.Before(first) {
				first = s.T
			}
		}
		r.Days = daysBetween(first, now) + 1
	}
	r.Partial = r.Days < r.WindowDays
	return r
}

// writeReport writes every chart into chartsDir and the README to readme,
// both in the language of l.
// The README templates link to the charts by their path in the repository.
func writeReport(r report, l locale, chartsDir, readme string) error {
	lo, hi := slotRange(r.Stats)
	summaryDir := filepath.Join(chartsDir, "summary")
	if err := writeTemplate(filepath.Join(summaryDir, "hours.svg"), svgTmpl, summaryHeader(l, lo, hi)); err != nil {
		return err
	}
	for _, st := range r.Stats {
		if err := writeTemplate(filepath.Join(summaryDir, st.Pool.Slug+".svg"), svgTmpl, summaryRow(l, st, lo, hi)); err != nil {
			return err
		}
	}
	for _, st := range r.Stats {
		// Individual charts can start and end with the pool's own hours;
		// only the summary needs the common range.
		plo, phi := st.Pool.SlotRange()
		if err := writeTemplate(filepath.Join(chartsDir, st.Pool.Slug+".svg"), svgTmpl, poolChart(l, st, plo, phi)); err != nil {
			return err
		}
	}
	return writeTemplate(readme, readmeTmpls.Lookup(l.Readme), r)
}

func clock(slot int) string {
	m := slot * slotMinutes
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// daysBetween counts calendar days from a to b. Days are compared as dates,
// so 23- and 25-hour days around DST changes still count as one.
func daysBetween(a, b time.Time) int {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	da := time.Date(ay, am, ad, 0, 0, 0, 0, time.UTC)
	db := time.Date(by, bm, bd, 0, 0, 0, 0, time.UTC)
	return int(db.Sub(da).Hours() / 24)
}

// writeTemplate executes tmpl into path, creating its directory.
func writeTemplate(path string, tmpl *template.Template, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := tmpl.Execute(f, data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
