package main

import (
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDaysBetween(t *testing.T) {
	tests := []struct {
		name string
		a, b time.Time
		want int
	}{
		{"same day", at(2026, 9, 21, 8, 0), at(2026, 9, 21, 21, 0), 0},
		{"late to early", at(2026, 9, 21, 21, 0), at(2026, 9, 22, 6, 0), 1},
		{"spring forward (23 h day)", at(2026, 3, 28, 10, 0), at(2026, 3, 30, 12, 0), 2},
		{"fall back (25 h day)", at(2026, 10, 24, 10, 0), at(2026, 10, 26, 12, 0), 2},
		{"eight weeks", at(2026, 8, 1, 0, 0), at(2026, 9, 26, 0, 0), 56},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daysBetween(tt.a, tt.b); got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWindowStart(t *testing.T) {
	now := at(2026, 9, 26, 22, 0)
	start := windowStart(now)
	if !start.Equal(at(2026, 8, 2, 0, 0)) {
		t.Errorf("got %v", start)
	}
	if daysBetween(start, now)+1 != 7*windowWeeks {
		t.Error("the window should span exactly the configured weeks")
	}
}

func TestBuildReport(t *testing.T) {
	pools := testPools(t, `[
	  {"code":"1","slug":"busy","name":"Busy","hours":{"mon-sun":"08:00-21:30"}},
	  {"code":"2","slug":"closed","name":"Closed","hours":{"mon-sun":"08:00-21:30"}},
	  {"code":"3","slug":"quiet","name":"Quiet","hours":{"mon-sun":"08:00-21:30"}}
	]`)
	samples := []Sample{
		{T: at(2026, 9, 24, 10, 0), Pool: "busy", Count: 50},
		{T: at(2026, 9, 22, 10, 0), Pool: "quiet", Count: 5},
	}
	r := buildReport(pools, samples, at(2026, 9, 26, 22, 0))

	var order []string
	for _, st := range r.Stats {
		order = append(order, st.Pool.Slug)
	}
	if strings.Join(order, ",") != "quiet,busy,closed" {
		t.Errorf("order = %v, want quietest first and no data last", order)
	}
	if r.Days != 5 || !r.Partial {
		t.Errorf("Days = %d, Partial = %v; want 5, true", r.Days, r.Partial)
	}
	if r.Updated != "26/09/2026 22:00" {
		t.Errorf("Updated = %q", r.Updated)
	}
	if empty := buildReport(pools, nil, at(2026, 9, 26, 22, 0)); empty.Days != 0 || !empty.Partial {
		t.Errorf("no samples: Days = %d, Partial = %v", empty.Days, empty.Partial)
	}
}

func TestHeadingAnchor(t *testing.T) {
	for in, want := range map[string]string{
		"C.C. Zabalgana":     "cc-zabalgana",
		"C.D. Mendizorrotza": "cd-mendizorrotza",
		"Pol. San Andrés":    "pol-san-andrés",
		"A & B <piscina>":    "a--b-piscina",
		"Sala_2 - Norte":     "sala_2---norte",
	} {
		if got := headingAnchor(in); got != want {
			t.Errorf("headingAnchor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClock(t *testing.T) {
	for slot, want := range map[int]string{0: "00:00", 13: "06:30", 43: "21:30", 48: "24:00"} {
		if got := clock(slot); got != want {
			t.Errorf("clock(%d) = %q, want %q", slot, got, want)
		}
	}
}

// TestWriteReport renders a full report into a temporary directory and
// checks the README links and that every chart is well-formed XML.
func TestWriteReport(t *testing.T) {
	pools := testPools(t, `[
	  {"code":"1","slug":"a-b","name":"A & B <piscina>","hours":{"mon-sun":"08:00-21:30"},"capacity":100,"note":"Nota de prueba."},
	  {"code":"2","slug":"empty","name":"Vacía","hours":{"mon-sun":"08:00-21:30"}}
	]`)
	var samples []Sample
	for d := 21; d <= 27; d++ {
		for h := 8; h < 21; h++ {
			for _, m := range []int{0, 30} {
				samples = append(samples, Sample{T: at(2026, 9, d, h, m), Pool: "a-b", Count: h})
			}
		}
	}
	r := buildReport(pools, samples, at(2026, 9, 27, 22, 0))

	dir := t.TempDir()
	charts, readme := filepath.Join(dir, "charts"), filepath.Join(dir, "README.md")
	if err := writeReport(r, es, charts, readme); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	for _, want := range []string{
		`<img src="../charts/es/summary/hours.svg"`,
		`<a href="#a--b-piscina"><img src="../charts/es/summary/a-b.svg" alt="Ocupación típica de A &amp; B &lt;piscina&gt;"></a>`,
		`<a href="#vacía"><img src="../charts/es/summary/empty.svg"`,
		"# Piscinas Vitoria-Gasteiz\n\n[EN](../README.md) | **ES**\n",
		"[calendarios](../calendar/es/README.md)",
		"](../charts/es/a-b.svg)",
		"Nota de prueba.",
		"Sin datos desde que empezó el muestreo",
		"ir a su gráfica.\n\n> [!NOTE]",
		"(7 de 56 días)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("README is missing %q", want)
		}
	}
	if !strings.HasSuffix(md, "puede que esté cerrada.\n\n<sub>Actualizado el 27/09/2026 22:00 · Muestreo cada 15 min en horario de apertura</sub>\n") {
		t.Errorf("README should end with the update line after the last pool, got %q", md[max(0, len(md)-200):])
	}
	if svg, err := os.ReadFile(filepath.Join(charts, "a-b.svg")); err != nil {
		t.Fatal(err)
	} else {
		// Occupancy grows through the day, so the quiet hours are the first
		// three that start two hours apart.
		for _, want := range []string{">08:00<", ">~8<", ">10:00<", ">~10<", ">12:00<", ">~12<"} {
			if !strings.Contains(string(svg), want) {
				t.Errorf("chart is missing the quiet-hour label %q", want)
			}
		}
	}

	for _, name := range []string{"summary/hours.svg", "summary/a-b.svg", "summary/empty.svg", "a-b.svg", "empty.svg"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(charts, name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			d := xml.NewDecoder(f)
			for {
				if _, err := d.Token(); errors.Is(err, io.EOF) {
					break
				} else if err != nil {
					t.Fatalf("not well-formed XML: %v", err)
				}
			}
		})
	}
}

// TestWriteReportEnglish checks the English version: its own text in the
// README and the charts, the link to Spanish, and the note fallback.
func TestWriteReportEnglish(t *testing.T) {
	pools := testPools(t, `[
	  {"code":"1","slug":"both","name":"Both","hours":{"mon-sun":"08:00-21:30"},"note":"Nota.","note_en":"Note."},
	  {"code":"2","slug":"only-es","name":"Only ES","hours":{"mon-sun":"08:00-21:30"},"note":"Solo en español."}
	]`)
	samples := []Sample{{T: at(2026, 9, 21, 10, 0), Pool: "both", Count: 5}}
	r := buildReport(pools, samples, at(2026, 9, 27, 22, 0))

	dir := t.TempDir()
	charts, readme := filepath.Join(dir, "charts"), filepath.Join(dir, "README.md")
	if err := writeReport(r, en, charts, readme); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(readme)
	if err != nil {
		t.Fatal(err)
	}
	md := string(b)
	for _, want := range []string{
		"**EN** | [ES](es/README.md)",
		"[calendars](calendar/en/README.md)",
		"(7 of 56 days)",
		"![Typical occupancy of Both](charts/en/both.svg)",
		"No data since sampling began",
		"\n\nNote.\n",
		"\n\nSolo en español.\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("README is missing %q", want)
		}
	}
	if strings.Contains(md, "Nota.") {
		t.Error("README should use note_en when there is one")
	}

	svg, err := os.ReadFile(filepath.Join(charts, "both.svg"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{">MON<", ">SUN<", "<title>Typical occupancy of Both</title>"} {
		if !strings.Contains(string(svg), want) {
			t.Errorf("chart is missing %q", want)
		}
	}
}

func TestLocalesHaveReadmeTemplates(t *testing.T) {
	for _, l := range locales {
		for _, name := range []string{l.Readme, l.CalendarReadme} {
			if readmeTmpls.Lookup(name) == nil {
				t.Errorf("no template %q", name)
			}
		}
	}
}

func TestWriteCalendarIndex(t *testing.T) {
	pools := testPools(t, `[
	  {"code":"1","slug":"open","name":"Open","hours":{"mon-sun":"08:00-21:30"}},
	  {"code":"2","slug":"closed","name":"Closed","hours":{"mon-sun":"08:00-21:30"}}
	]`)
	samples := []Sample{{T: at(2026, 9, 21, 10, 0), Pool: "open", Count: 5}}
	r := buildReport(pools, samples, at(2026, 9, 27, 22, 0))

	for _, l := range locales {
		t.Run(l.Code, func(t *testing.T) {
			dir := t.TempDir()
			if err := writeCalendarIndex(r, l, dir); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(filepath.Join(dir, "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			md := string(b)
			want := "| Open | https://raw.githubusercontent.com/asiermartinez/swim-time/main/calendar/" + l.Code + "/open.ics |\n"
			if !strings.Contains(md, want) {
				t.Errorf("page is missing %q", want)
			}
			if strings.Contains(md, "closed.ics") {
				t.Error("pools without data should not be listed")
			}
		})
	}
}
