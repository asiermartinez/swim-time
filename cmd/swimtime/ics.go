package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Each pool gets an iCalendar feed with its quiet hours for the coming days,
// meant for subscribing from Apple Calendar.
const (
	calendarDir   = "calendar"
	calendarAhead = 7  // days of upcoming events, today included
	calendarKeep  = 28 // days of past events kept from the previous feed
	// calendarRefresh is how often clients should poll the feed. It is
	// rebuilt once a day.
	calendarRefresh = "PT6H"
)

// vtimezone is Europe/Madrid as Apple Calendar exports it.
const vtimezone = `BEGIN:VTIMEZONE
TZID:Europe/Madrid
BEGIN:DAYLIGHT
TZOFFSETFROM:+0100
DTSTART:19810329T020000
RRULE:FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU
TZNAME:CEST
TZOFFSETTO:+0200
END:DAYLIGHT
BEGIN:STANDARD
TZOFFSETFROM:+0200
DTSTART:19961027T030000
RRULE:FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU
TZNAME:CET
TZOFFSETTO:+0100
END:STANDARD
END:VTIMEZONE`

// writeCalendars writes one feed per pool into dir, in the language of l. Events before today are
// carried over from the feed already there, so past quiet hours stay where
// they were when the day came instead of following later data.
func writeCalendars(l locale, stats []*Stats, dir string, now time.Time) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, st := range stats {
		path := filepath.Join(dir, st.Pool.Slug+".ics")
		prev, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err := os.WriteFile(path, buildCalendar(l, st, prev, now), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// buildCalendar renders the feed of one pool. prev is the previous feed, or
// nil.
func buildCalendar(l locale, st *Stats, prev []byte, now time.Time) []byte {
	var w icsWriter
	p := st.Pool
	w.line("BEGIN:VCALENDAR")
	w.line("VERSION:2.0")
	w.line("PRODID:-//swim-time//swimtime//" + strings.ToUpper(l.Code))
	w.line("CALSCALE:GREGORIAN")
	w.line("METHOD:PUBLISH")
	w.prop("X-WR-CALNAME", fmt.Sprintf(l.CalName, p.Name))
	w.prop("X-WR-CALDESC", fmt.Sprintf(l.CalDesc, p.Name))
	w.line("X-WR-TIMEZONE:Europe/Madrid")
	w.line("REFRESH-INTERVAL;VALUE=DURATION:" + calendarRefresh)
	w.line("X-PUBLISHED-TTL:" + calendarRefresh)
	w.timezone()

	today := startOfDay(now)
	oldest := today.AddDate(0, 0, -calendarKeep)
	for _, ev := range pastEvents(prev) {
		if !ev.start.Before(oldest) && ev.start.Before(today) {
			w.buf.WriteString(ev.raw)
		}
	}

	stamp := now.UTC().Format("20060102T150405Z")
	for i := range calendarAhead {
		day := today.AddDate(0, 0, i)
		for rank, win := range st.Quiet[weekday(day)] {
			w.event(l, p, day, rank, win, quietUID(p, day, rank), stamp)
		}
	}
	w.line("END:VCALENDAR")
	return w.buf.Bytes()
}

func (w *icsWriter) timezone() {
	for _, l := range strings.Split(vtimezone, "\n") {
		w.line(l)
	}
}

// quietUID identifies the rank-th quiet hour of day.
func quietUID(p *Pool, day time.Time, rank int) string {
	return fmt.Sprintf("%s-%s-%d@swim-time", p.Slug, day.Format("20060102"), rank+1)
}

// event writes the rank-th quiet hour win of day, quietest first.
func (w *icsWriter) event(l locale, p *Pool, day time.Time, rank int, win Window, uid, stamp string) {
	// Built from the clock time, not by adding to midnight, so DST changes
	// don't shift it.
	m := win.Slot * slotMinutes
	start := time.Date(day.Year(), day.Month(), day.Day(), m/60, m%60, 0, 0, loc)
	end := start.Add(2 * slotMinutes * time.Minute)
	d := weekday(day)
	summary := fmt.Sprintf(l.EventTitle, rank+1)
	desc := fmt.Sprintf(l.EventDesc, p.Name, l.WeekDays[d], win.Value)
	if p.Capacity > 0 {
		desc += fmt.Sprintf(l.EventAforo, p.Capacity)
	}
	desc += fmt.Sprintf(l.EventRank, rank+1)

	w.line("BEGIN:VEVENT")
	w.line("UID:" + uid)
	w.line("DTSTAMP:" + stamp)
	w.line("DTSTART;TZID=Europe/Madrid:" + start.Format("20060102T150405"))
	w.line("DTEND;TZID=Europe/Madrid:" + end.Format("20060102T150405"))
	w.prop("SUMMARY", summary)
	w.prop("DESCRIPTION", desc)
	if p.URL != "" {
		w.line("URL:" + p.URL)
	}
	w.location(p)
	// Options to pick from, not plans: they don't block time or alert. Copy
	// the one you'll swim into your own calendar to get travel time and
	// alerts there.
	w.line("STATUS:TENTATIVE")
	w.line("TRANSP:TRANSPARENT")
	w.line("END:VEVENT")
}

// location writes the pool's place. With coordinates, Apple Calendar shows a
// map. The travel advisory is ENABLED, the value Calendar itself writes
// when travel time is on: copies of an event drop AUTOMATIC.
func (w *icsWriter) location(p *Pool) {
	text := p.Name
	if p.Address != "" {
		text += "\n" + p.Address
	}
	w.prop("LOCATION", text)
	if len(p.Geo) != 2 {
		return
	}
	w.line(fmt.Sprintf("X-APPLE-STRUCTURED-LOCATION;VALUE=URI;X-ADDRESS=%s;X-APPLE-RADIUS=70;X-APPLE-REFERENCEFRAME=1;X-TITLE=%s:geo:%g,%g",
		paramValue(p.Address), paramValue(p.Name), p.Geo[0], p.Geo[1]))
	w.line("X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:ENABLED")
}

// pastEvent is a VEVENT read back from a previous feed.
type pastEvent struct {
	start time.Time
	raw   string // as written, folded and with CRLFs
}

// pastEvents extracts the events of a feed written by buildCalendar. Events
// whose start can't be read are dropped.
func pastEvents(feed []byte) []pastEvent {
	var out []pastEvent
	rest := string(feed)
	for {
		i := strings.Index(rest, "BEGIN:VEVENT\r\n")
		if i < 0 {
			return out
		}
		rest = rest[i:]
		j := strings.Index(rest, "END:VEVENT\r\n")
		if j < 0 {
			return out
		}
		raw := rest[:j+len("END:VEVENT\r\n")]
		rest = rest[len(raw):]
		unfolded := strings.ReplaceAll(raw, "\r\n ", "")
		for _, l := range strings.Split(unfolded, "\r\n") {
			v, ok := strings.CutPrefix(l, "DTSTART;TZID=Europe/Madrid:")
			if !ok {
				continue
			}
			if t, err := time.ParseInLocation("20060102T150405", v, loc); err == nil {
				out = append(out, pastEvent{t, raw})
			}
			break
		}
	}
}

// icsWriter writes content lines with CRLF endings, folded at 75 octets.
type icsWriter struct{ buf bytes.Buffer }

func (w *icsWriter) prop(name, text string) { w.line(name + ":" + escapeText(text)) }

func (w *icsWriter) line(l string) {
	// Continuation lines start with a space, which counts towards the limit.
	for limit := 75; len(l) > limit; limit = 74 {
		cut := limit
		// Never split a UTF-8 sequence.
		for l[cut]&0xC0 == 0x80 {
			cut--
		}
		w.buf.WriteString(l[:cut] + "\r\n ")
		l = l[cut:]
	}
	w.buf.WriteString(l + "\r\n")
}

var textEscaper = strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`)

func escapeText(s string) string { return textEscaper.Replace(s) }

// paramValue quotes a parameter value. Values can't contain double quotes.
func paramValue(s string) string { return `"` + strings.ReplaceAll(s, `"`, "'") + `"` }
