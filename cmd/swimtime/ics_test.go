package main

import (
	"regexp"
	"strings"
	"testing"
)

// calendarStats returns stats for the "small" pool with two quiet windows on
// Mondays and one on Sundays.
func calendarStats(t *testing.T) *Stats {
	t.Helper()
	p := testPools(t, twoPools)[1]
	p.Address = "Calle Falsa, 1, 01001 Vitoria-Gasteiz"
	p.Geo = []float64{42.85, -2.67}
	st := &Stats{Pool: p}
	st.Quiet[0] = []Window{{Slot: 20, Value: 3.4}, {Slot: 30, Value: 7}}
	st.Quiet[6] = []Window{{Slot: 20, Value: 5}}
	return st
}

var uidRe = regexp.MustCompile(`(?m)^UID:(.*)\r$`)

func uids(feed []byte) []string {
	var out []string
	for _, m := range uidRe.FindAllStringSubmatch(string(feed), -1) {
		out = append(out, m[1])
	}
	return out
}

func TestBuildCalendarEvents(t *testing.T) {
	st := calendarStats(t)
	feed := buildCalendar(es, st, nil, at(2026, 9, 21, 22, 0)) // Monday
	got := uids(feed)
	want := []string{
		"small-20260921-1@swim-time", "small-20260921-2@swim-time",
		"small-20260927-1@swim-time",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("UIDs = %v, want %v", got, want)
	}
	s := strings.ReplaceAll(string(feed), "\r\n ", "")
	for _, frag := range []string{
		"DTSTART;TZID=Europe/Madrid:20260921T100000\r\n",
		"DTEND;TZID=Europe/Madrid:20260921T110000\r\n",
		"SUMMARY:Piscina (Hora tranquila #1)\r\n",
		"SUMMARY:Piscina (Hora tranquila #2)\r\n",
		"Pequeña: un lunes típico hay unas 3 personas en esta hora (aforo 100)",
		`LOCATION:Pequeña\nCalle Falsa\, 1\, 01001 Vitoria-Gasteiz` + "\r\n",
		"X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:ENABLED\r\n",
		"STATUS:TENTATIVE\r\n",
		"TRANSP:TRANSPARENT\r\n",
	} {
		if !strings.Contains(s, frag) {
			t.Errorf("feed lacks %q", frag)
		}
	}
	if !strings.Contains(strings.ReplaceAll(s, "\r\n ", ""), `X-TITLE="Pequeña":geo:42.85,-2.67`) {
		t.Error("feed lacks the structured location")
	}
	if strings.Contains(s, "VALARM") {
		t.Error("feed has alerts")
	}
	if !strings.HasPrefix(s, "BEGIN:VCALENDAR\r\n") || !strings.HasSuffix(s, "END:VCALENDAR\r\n") {
		t.Error("feed is not a VCALENDAR")
	}
}

func TestBuildCalendarEnglish(t *testing.T) {
	feed := buildCalendar(en, calendarStats(t), nil, at(2026, 9, 21, 22, 0))
	s := strings.ReplaceAll(string(feed), "\r\n ", "")
	for _, frag := range []string{
		"PRODID:-//swim-time//swimtime//EN\r\n",
		"X-WR-CALNAME:Pequeña pool\r\n",
		"SUMMARY:Pool (Quiet hour #2)\r\n",
		"on a typical Monday there are about 3 people at this time (capacity 100). It is the #1 quietest hour of the day.",
	} {
		if !strings.Contains(s, frag) {
			t.Errorf("feed lacks %q", frag)
		}
	}
}

func TestBuildCalendarDST(t *testing.T) {
	// 2026-10-25 is a Sunday with a 25-hour day.
	feed := string(buildCalendar(es, calendarStats(t), nil, at(2026, 10, 19, 22, 0)))
	if !strings.Contains(feed, "DTSTART;TZID=Europe/Madrid:20261025T100000\r\n") {
		t.Error("quiet hour moved on the DST change")
	}
}

func TestBuildCalendarKeepsPast(t *testing.T) {
	st := calendarStats(t)
	first := buildCalendar(es, st, nil, at(2026, 9, 21, 22, 0))

	// Later data moves Monday's windows. Past Mondays keep the old ones.
	st.Quiet[0] = []Window{{Slot: 24, Value: 2}}
	later := buildCalendar(es, st, first, at(2026, 9, 22, 22, 0)) // Tuesday
	s := string(later)
	if !strings.Contains(s, "DTSTART;TZID=Europe/Madrid:20260921T150000\r\n") {
		t.Error("past event from the previous feed was dropped")
	}
	if !strings.Contains(s, "DTSTART;TZID=Europe/Madrid:20260928T120000\r\n") {
		t.Error("upcoming event doesn't follow the new data")
	}
	seen := map[string]bool{}
	for _, u := range uids(later) {
		if seen[u] {
			t.Errorf("duplicate UID %s", u)
		}
		seen[u] = true
	}
	if !seen["small-20260921-1@swim-time"] || !seen["small-20260921-2@swim-time"] || seen["small-20260928-2@swim-time"] {
		t.Errorf("unexpected events: %v", uids(later))
	}

	// Past events older than calendarKeep days are dropped.
	much := buildCalendar(es, st, later, at(2026, 9, 21+calendarKeep+1, 22, 0))
	for _, u := range uids(much) {
		if strings.HasPrefix(u, "small-20260921") {
			t.Errorf("kept %s for more than %d days", u, calendarKeep)
		}
	}
}

func TestICSLineFolding(t *testing.T) {
	var w icsWriter
	long := "DESCRIPTION:" + strings.Repeat("ñandú ", 40)
	w.line(long)
	out := w.buf.String()
	for _, l := range strings.Split(strings.TrimSuffix(out, "\r\n"), "\r\n") {
		if len(l) > 75 {
			t.Errorf("line of %d octets: %q", len(l), l)
		}
	}
	if got := strings.ReplaceAll(strings.TrimSuffix(out, "\r\n"), "\r\n ", ""); got != long {
		t.Errorf("unfolded line differs:\n got %q\nwant %q", got, long)
	}
}

func TestEscapeText(t *testing.T) {
	if got, want := escapeText("a,b;c\\d\ne"), `a\,b\;c\\d\ne`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPastEventsIgnoresGarbage(t *testing.T) {
	feed := []byte("BEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nDTSTART;TZID=Europe/Madrid:20260921T100000\r\nEND:VEVENT\r\n")
	evs := pastEvents(feed)
	if len(evs) != 1 || !evs[0].start.Equal(at(2026, 9, 21, 10, 0)) {
		t.Errorf("got %+v", evs)
	}
}
