package main

import (
	"testing"
	"time"
)

// testPools parses a pools.json document or fails the test.
func testPools(t *testing.T, doc string) []*Pool {
	t.Helper()
	pools, err := parsePools([]byte(doc))
	if err != nil {
		t.Fatalf("parsePools: %v", err)
	}
	return pools
}

// twoPools is a small config: "big" opens earlier on weekdays, like
// Mendizorrotza, and "small" has a known capacity.
const twoPools = `[
  {"code": "1", "slug": "big", "name": "Grande", "hours": {"mon-fri": "06:00-21:30", "sat-sun": "08:00-21:30"}},
  {"code": "2", "slug": "small", "name": "Pequeña", "hours": {"mon-sun": "08:00-21:30"}, "capacity": 100}
]`

// at returns a local time. 2026-09-21 is a Monday.
func at(year int, month time.Month, day, hour, min int) time.Time {
	return time.Date(year, month, day, hour, min, 0, 0, loc)
}
