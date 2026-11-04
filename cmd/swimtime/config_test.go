package main

import (
	"strings"
	"testing"
)

func TestRepoPoolsJSON(t *testing.T) {
	pools, err := loadPools("../../" + poolsFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(pools) != 11 {
		t.Errorf("got %d pools, want 11", len(pools))
	}
	for _, p := range pools {
		if lo, hi := p.SlotRange(); lo >= hi {
			t.Errorf("%s: no opening hours", p.Slug)
		}
		if !strings.HasPrefix(p.URL, "https://") {
			t.Errorf("%s: bad url %q", p.Slug, p.URL)
		}
	}
}

func TestParsePoolsErrors(t *testing.T) {
	tests := []struct {
		name, doc, want string
	}{
		{"not json", `{`, "unexpected"},
		{"missing name", `[{"code":"1","slug":"a","hours":{"mon":"08:00-09:00"}}]`, "incompleta"},
		{"duplicate code", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"08:00-09:00"}},{"code":"1","slug":"b","name":"B","hours":{"mon":"08:00-09:00"}}]`, "código repetido"},
		{"duplicate slug", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"08:00-09:00"}},{"code":"2","slug":"a","name":"B","hours":{"mon":"08:00-09:00"}}]`, "slug repetido"},
		{"no hours", `[{"code":"1","slug":"a","name":"A"}]`, "sin horario"},
		{"negative capacity", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"08:00-09:00"},"capacity":-1}]`, "aforo negativo"},
		{"bad days", `[{"code":"1","slug":"a","name":"A","hours":{"fri-mon":"08:00-09:00"}}]`, "días no válidos"},
		{"bad day name", `[{"code":"1","slug":"a","name":"A","hours":{"lunes":"08:00-09:00"}}]`, "días no válidos"},
		{"bad geo", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"08:00-09:00"},"geo":[42.8]}]`, "geo debe ser"},
		{"bad span", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"09:00-08:00"}}]`, "horario no válido"},
		{"bad clock", `[{"code":"1","slug":"a","name":"A","hours":{"mon":"8h-9h"}}]`, "horario no válido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePools([]byte(tt.doc))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestParseDays(t *testing.T) {
	tests := []struct {
		in       string
		from, to int
	}{
		{"mon", 0, 0},
		{"mon-fri", 0, 4},
		{"sat-sun", 5, 6},
		{"mon-sun", 0, 6},
	}
	for _, tt := range tests {
		from, to, err := parseDays(tt.in)
		if err != nil || from != tt.from || to != tt.to {
			t.Errorf("parseDays(%q) = %d, %d, %v; want %d, %d", tt.in, from, to, err, tt.from, tt.to)
		}
	}
}

func TestParseSpanEndOfDay(t *testing.T) {
	s, err := parseSpan("22:00-24:00")
	if err != nil || s != (span{22 * 60, 24 * 60}) {
		t.Errorf("got %v, %v", s, err)
	}
}

func TestOpenAt(t *testing.T) {
	big := testPools(t, twoPools)[0]
	tests := []struct {
		name string
		h, m int
		day  int // September 2026: 21 is Monday, 26 is Saturday
		want bool
	}{
		{"weekday before opening", 5, 59, 21, false},
		{"weekday opening", 6, 0, 21, true},
		{"weekday last minute", 21, 29, 21, true},
		{"weekday closing is exclusive", 21, 30, 21, false},
		{"weekend early", 7, 0, 26, false},
		{"weekend opening", 8, 0, 26, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := big.OpenAt(at(2026, 9, tt.day, tt.h, tt.m)); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSlotOpenAndRange(t *testing.T) {
	big := testPools(t, twoPools)[0]
	if !big.SlotOpen(0, 12) { // Monday 06:00
		t.Error("Monday 06:00 should be open")
	}
	if big.SlotOpen(5, 12) { // Saturday 06:00
		t.Error("Saturday 06:00 should be closed")
	}
	if !big.SlotOpen(0, 42) || big.SlotOpen(0, 43) { // 21:00 open, 21:30 closed
		t.Error("last slot should be 21:00–21:30")
	}
	if lo, hi := big.SlotRange(); lo != 12 || hi != 43 {
		t.Errorf("SlotRange = %d, %d; want 12, 43", lo, hi)
	}
}

func TestSlotOpenPartialSlot(t *testing.T) {
	p := testPools(t, `[{"code":"1","slug":"a","name":"A","hours":{"mon":"08:15-09:45"}}]`)[0]
	// Only 08:30–09:30 is fully inside the opening hours.
	for s, want := range map[int]bool{16: false, 17: true, 18: true, 19: false} {
		if got := p.SlotOpen(0, s); got != want {
			t.Errorf("slot %s: got %v, want %v", clock(s), got, want)
		}
	}
}

func TestWeekday(t *testing.T) {
	for day, want := range map[int]int{21: 0, 22: 1, 26: 5, 27: 6} {
		if got := weekday(at(2026, 9, day, 12, 0)); got != want {
			t.Errorf("September %d: got %d, want %d", day, got, want)
		}
	}
}
