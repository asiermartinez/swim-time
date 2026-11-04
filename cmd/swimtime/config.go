package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	slotMinutes = 30
	slotsPerDay = 24 * 60 / slotMinutes
)

var dayKeys = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// Pool is one entry of pools.json.
type Pool struct {
	Code  string            `json:"code"`
	Slug  string            `json:"slug"`
	Name  string            `json:"name"`
	URL   string            `json:"url"`
	Hours map[string]string `json:"hours"`
	// Address and Geo (latitude, longitude) place the pool on the map in
	// the calendar feeds. Both are optional.
	Address string    `json:"address,omitempty"`
	Geo     []float64 `json:"geo,omitempty"`
	// Capacity is the "aforo total del espacio" published on the pool's
	// page, or 0 when unknown.
	Capacity int `json:"capacity,omitempty"`
	// Note is shown under the pool's chart; NoteEN is its English version,
	// and the English README falls back to Note without one.
	Note   string `json:"note,omitempty"`
	NoteEN string `json:"note_en,omitempty"`

	// week holds the opening spans per weekday, Monday first.
	week [7][]span
}

// span is an opening interval in minutes since midnight, [open, close).
type span struct{ open, close int }

func loadPools(path string) ([]*Pool, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	pools, err := parsePools(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return pools, nil
}

// parsePools decodes and validates the contents of pools.json.
func parsePools(b []byte) ([]*Pool, error) {
	var pools []*Pool
	if err := json.Unmarshal(b, &pools); err != nil {
		return nil, err
	}
	codes, slugs := map[string]bool{}, map[string]bool{}
	for _, p := range pools {
		switch {
		case p.Code == "" || p.Slug == "" || p.Name == "":
			return nil, fmt.Errorf("piscina incompleta: %+v", p)
		case codes[p.Code]:
			return nil, fmt.Errorf("código repetido %q", p.Code)
		case slugs[p.Slug]:
			return nil, fmt.Errorf("slug repetido %q", p.Slug)
		case len(p.Hours) == 0:
			return nil, fmt.Errorf("%s: sin horario", p.Slug)
		case p.Capacity < 0:
			return nil, fmt.Errorf("%s: aforo negativo", p.Slug)
		case p.Geo != nil && len(p.Geo) != 2:
			return nil, fmt.Errorf("%s: geo debe ser [latitud, longitud]", p.Slug)
		}
		codes[p.Code], slugs[p.Slug] = true, true
		for days, hours := range p.Hours {
			from, to, err := parseDays(days)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Slug, err)
			}
			s, err := parseSpan(hours)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Slug, err)
			}
			for d := from; d <= to; d++ {
				p.week[d] = append(p.week[d], s)
			}
		}
	}
	return pools, nil
}

// parseDays parses "mon-fri" or "sat" into an inclusive weekday range.
func parseDays(s string) (from, to int, err error) {
	a, b, found := strings.Cut(s, "-")
	if !found {
		b = a
	}
	from, to = dayIndex(a), dayIndex(b)
	if from < 0 || to < 0 || from > to {
		return 0, 0, fmt.Errorf("días no válidos %q", s)
	}
	return from, to, nil
}

func dayIndex(key string) int {
	for i, k := range dayKeys {
		if k == key {
			return i
		}
	}
	return -1
}

// parseSpan parses "08:00-21:30".
func parseSpan(s string) (span, error) {
	a, b, _ := strings.Cut(s, "-")
	open, err1 := parseClock(a)
	close, err2 := parseClock(b)
	if err1 != nil || err2 != nil || open >= close {
		return span{}, fmt.Errorf("horario no válido %q", s)
	}
	return span{open, close}, nil
}

// parseClock parses "08:00" into minutes since midnight. "24:00" is allowed
// as the end of the day.
func parseClock(s string) (int, error) {
	if s = strings.TrimSpace(s); s == "24:00" {
		return 24 * 60, nil
	}
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, err
	}
	return t.Hour()*60 + t.Minute(), nil
}

// weekday returns the weekday of t with Monday as 0.
func weekday(t time.Time) int { return (int(t.Weekday()) + 6) % 7 }

func minuteOfDay(t time.Time) int { return t.Hour()*60 + t.Minute() }

// OpenAt reports whether the pool is open at local time t.
func (p *Pool) OpenAt(t time.Time) bool {
	m := minuteOfDay(t)
	for _, s := range p.week[weekday(t)] {
		if s.open <= m && m < s.close {
			return true
		}
	}
	return false
}

// SlotOpen reports whether the 30-minute slot s of weekday d is fully within
// opening hours.
func (p *Pool) SlotOpen(d, s int) bool {
	start := s * slotMinutes
	for _, sp := range p.week[d] {
		if sp.open <= start && start+slotMinutes <= sp.close {
			return true
		}
	}
	return false
}

// SlotRange returns the first slot and one past the last slot the pool is
// open on any day of the week.
func (p *Pool) SlotRange() (lo, hi int) {
	lo, hi = slotsPerDay, 0
	for _, spans := range p.week {
		for _, sp := range spans {
			lo = min(lo, sp.open/slotMinutes)
			hi = max(hi, (sp.close+slotMinutes-1)/slotMinutes)
		}
	}
	if lo >= hi {
		return 0, 0
	}
	return lo, hi
}
