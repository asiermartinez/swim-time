package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestTakeSample(t *testing.T) {
	pools := testPools(t, twoPools)
	monday7 := at(2026, 9, 21, 7, 0) // only "big" is open

	var asked []string
	fetch := func(codes []string) (map[string]int, error) {
		asked = codes
		return map[string]int{"1": 12, "2": 99}, nil
	}
	samples, missing, err := takeSample(pools, monday7, fetch)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(asked, []string{"1"}) {
		t.Errorf("asked for %v, want only the open pool", asked)
	}
	want := []Sample{{T: monday7, Pool: "big", Count: 12}}
	if !reflect.DeepEqual(samples, want) || missing != nil {
		t.Errorf("got %v, %v; want %v", samples, missing, want)
	}
}

func TestTakeSampleAllClosed(t *testing.T) {
	fetch := func([]string) (map[string]int, error) {
		t.Fatal("should not fetch when every pool is closed")
		return nil, nil
	}
	samples, missing, err := takeSample(testPools(t, twoPools), at(2026, 9, 21, 23, 0), fetch)
	if samples != nil || missing != nil || err != nil {
		t.Errorf("got %v, %v, %v", samples, missing, err)
	}
}

func TestTakeSampleMissing(t *testing.T) {
	pools := testPools(t, twoPools)
	noon := at(2026, 9, 21, 12, 0)

	fetch := func([]string) (map[string]int, error) { return map[string]int{"1": 5}, nil }
	samples, missing, err := takeSample(pools, noon, fetch)
	if err != nil || len(samples) != 1 || !reflect.DeepEqual(missing, []string{"small"}) {
		t.Errorf("got %v, %v, %v", samples, missing, err)
	}

	empty := func([]string) (map[string]int, error) { return map[string]int{}, nil }
	if _, _, err := takeSample(pools, noon, empty); err == nil {
		t.Error("want error when no open pool is in the response")
	}

	failing := func([]string) (map[string]int, error) { return nil, errors.New("caído") }
	if _, _, err := takeSample(pools, noon, failing); err == nil {
		t.Error("want fetch error")
	}
}

func TestWithRetry(t *testing.T) {
	calls := 0
	flaky := func([]string) (map[string]int, error) {
		if calls++; calls == 1 {
			return nil, errors.New("timeout")
		}
		return map[string]int{"1": 1}, nil
	}
	got, err := withRetry(flaky, 0)([]string{"1"})
	if err != nil || got["1"] != 1 || calls != 2 {
		t.Errorf("got %v, %v after %d calls", got, err, calls)
	}

	calls = 0
	down := func([]string) (map[string]int, error) { calls++; return nil, errors.New("down") }
	if _, err := withRetry(down, 0)(nil); err == nil || calls != 2 {
		t.Errorf("got %v after %d calls, want an error after 2", err, calls)
	}
}

func TestAforoAPI(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s, want POST", r.Method)
		}
		gotQuery = r.URL.Query().Get("codCentros")
		w.Header().Set("Content-Type", "text/json; charset=UTF-8")
		w.Write([]byte(`{"listaCentros":[{"aforoTReal":"125","codCentro":"2"},{"aforoTReal":"0","codCentro":"6"}]}`))
	}))
	defer srv.Close()

	got, err := aforoAPI{URL: srv.URL, Client: srv.Client()}.Counts([]string{"2", "6"})
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"2": 125, "6": 0}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	var codes []map[string]string
	if err := json.Unmarshal([]byte(gotQuery), &codes); err != nil {
		t.Fatalf("codCentros is not JSON: %q", gotQuery)
	}
	if want := []map[string]string{{"codCentro": "2"}, {"codCentro": "6"}}; !reflect.DeepEqual(codes, want) {
		t.Errorf("codCentros = %v, want %v", codes, want)
	}
}

func TestAforoAPIErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{"http error", http.StatusInternalServerError, "", "HTTP 500"},
		{"bad json", http.StatusOK, "<html>", "aforo:"},
		{"bad value", http.StatusOK, `{"listaCentros":[{"aforoTReal":"n/d","codCentro":"2"}]}`, "valor no válido"},
		{"negative", http.StatusOK, `{"listaCentros":[{"aforoTReal":"-1","codCentro":"2"}]}`, "valor no válido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			api := aforoAPI{URL: srv.URL, Client: &http.Client{Timeout: time.Second}}
			if _, err := api.Counts([]string{"2"}); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}
