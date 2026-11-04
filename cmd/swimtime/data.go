package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

var csvHeader = []string{"ts", "pool", "count"}

// Sample is one occupancy reading of one pool.
type Sample struct {
	T     time.Time
	Pool  string
	Count int
}

// appendSamples appends samples to the monthly CSV files in dir, creating
// them with a header when needed.
func appendSamples(dir string, samples []Sample) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	byFile := map[string][]Sample{}
	var files []string
	for _, s := range samples {
		name := filepath.Join(dir, s.T.In(loc).Format("2006-01")+".csv")
		if byFile[name] == nil {
			files = append(files, name)
		}
		byFile[name] = append(byFile[name], s)
	}
	for _, name := range files {
		if err := appendFile(name, byFile[name]); err != nil {
			return err
		}
	}
	return nil
}

func appendFile(name string, samples []Sample) error {
	_, err := os.Stat(name)
	isNew := errors.Is(err, os.ErrNotExist)
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w := csv.NewWriter(f)
	if isNew {
		w.Write(csvHeader)
	}
	for _, s := range samples {
		w.Write([]string{
			s.T.In(loc).Truncate(time.Minute).Format(time.RFC3339),
			s.Pool,
			strconv.Itoa(s.Count),
		})
	}
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// loadSamples reads every sample taken at or after since, in local time.
func loadSamples(dir string, since time.Time) ([]Sample, error) {
	names, err := filepath.Glob(filepath.Join(dir, "*.csv"))
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	firstMonth := since.In(loc).Format("2006-01") + ".csv"
	var out []Sample
	for _, name := range names {
		if filepath.Base(name) < firstMonth {
			continue
		}
		if out, err = readFile(name, since, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func readFile(name string, since time.Time, out []Sample) ([]Sample, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = len(csvHeader)
	r.ReuseRecord = true
	for line := 1; ; line++ {
		rec, err := r.Read()
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if line == 1 && rec[0] == csvHeader[0] {
			continue
		}
		t, err := time.Parse(time.RFC3339, rec[0])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, line, err)
		}
		n, err := strconv.Atoi(rec[2])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", name, line, err)
		}
		if t.Before(since) {
			continue
		}
		out = append(out, Sample{T: t.In(loc), Pool: rec[1], Count: n})
	}
}
