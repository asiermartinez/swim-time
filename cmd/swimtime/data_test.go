package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAppendAndLoadSamples(t *testing.T) {
	dir := t.TempDir()
	first := []Sample{
		{T: at(2026, 8, 31, 20, 15), Pool: "big", Count: 40},
		{T: at(2026, 9, 1, 8, 0), Pool: "big", Count: 3},
	}
	second := []Sample{{T: at(2026, 9, 1, 8, 15), Pool: "small", Count: 7}}
	if err := appendSamples(dir, first); err != nil {
		t.Fatal(err)
	}
	if err := appendSamples(dir, second); err != nil {
		t.Fatal(err)
	}

	sep, err := os.ReadFile(filepath.Join(dir, "2026-09.csv"))
	if err != nil {
		t.Fatal(err)
	}
	want := "ts,pool,count\n2026-09-01T08:00:00+02:00,big,3\n2026-09-01T08:15:00+02:00,small,7\n"
	if string(sep) != want {
		t.Errorf("2026-09.csv =\n%s\nwant\n%s", sep, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "2026-08.csv")); err != nil {
		t.Errorf("samples should be split per month: %v", err)
	}

	got, err := loadSamples(dir, at(2026, 1, 1, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	all := append(first, second...)
	if len(got) != len(all) {
		t.Fatalf("loaded %d samples, want %d", len(got), len(all))
	}
	for i := range all {
		if !got[i].T.Equal(all[i].T) || got[i].Pool != all[i].Pool || got[i].Count != all[i].Count {
			t.Errorf("sample %d = %+v, want %+v", i, got[i], all[i])
		}
		if got[i].T.Location() != loc {
			t.Errorf("sample %d not in local time", i)
		}
	}
}

func TestAppendTruncatesToMinute(t *testing.T) {
	dir := t.TempDir()
	ts := at(2026, 9, 1, 8, 0).Add(42 * time.Second)
	if err := appendSamples(dir, []Sample{{T: ts, Pool: "big", Count: 1}}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "2026-09.csv"))
	if !strings.Contains(string(b), "2026-09-01T08:00:00+02:00") {
		t.Errorf("got %q", b)
	}
}

func TestLoadSamplesSince(t *testing.T) {
	dir := t.TempDir()
	samples := []Sample{
		{T: at(2026, 7, 30, 10, 0), Pool: "big", Count: 1}, // older month, skipped by file name
		{T: at(2026, 8, 2, 10, 0), Pool: "big", Count: 2},  // same month, before since
		{T: at(2026, 8, 3, 0, 0), Pool: "big", Count: 3},   // exactly since
		{T: at(2026, 9, 1, 10, 0), Pool: "big", Count: 4},
	}
	if err := appendSamples(dir, samples); err != nil {
		t.Fatal(err)
	}
	got, err := loadSamples(dir, at(2026, 8, 3, 0, 0))
	if err != nil {
		t.Fatal(err)
	}
	var counts []int
	for _, s := range got {
		counts = append(counts, s.Count)
	}
	if !reflect.DeepEqual(counts, []int{3, 4}) {
		t.Errorf("got counts %v, want [3 4]", counts)
	}
}

func TestLoadSamplesEmptyDir(t *testing.T) {
	got, err := loadSamples(filepath.Join(t.TempDir(), "missing"), at(2026, 1, 1, 0, 0))
	if err != nil || len(got) != 0 {
		t.Errorf("got %v, %v", got, err)
	}
}

func TestLoadSamplesBadRow(t *testing.T) {
	tests := map[string]string{
		"bad time":   "ts,pool,count\nayer,big,1\n",
		"bad count":  "ts,pool,count\n2026-09-01T08:00:00+02:00,big,muchos\n",
		"bad fields": "ts,pool,count\n2026-09-01T08:00:00+02:00,big\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "2026-09.csv"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := loadSamples(dir, at(2026, 1, 1, 0, 0)); err == nil {
				t.Error("want error")
			}
		})
	}
}
