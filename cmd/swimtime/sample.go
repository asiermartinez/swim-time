package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const aforoURL = "https://sedeelectronica.vitoria-gasteiz.org/m01-10s/ND/consultaAforoAction.do"

func cmdSample(args []string) error {
	fs := flag.NewFlagSet("sample", flag.ExitOnError)
	dry := fs.Bool("dry", false, "muestra las lecturas sin guardarlas")
	fs.Parse(args)

	pools, err := loadPools(poolsFile)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	api := aforoAPI{URL: aforoURL, Client: &http.Client{Timeout: 10 * time.Second}}
	samples, missing, err := takeSample(pools, now, withRetry(api.Counts, 3*time.Second))
	if err != nil {
		return err
	}
	if samples == nil && missing == nil {
		fmt.Println("ninguna piscina abierta a las", now.Format("15:04"))
		return nil
	}
	for _, slug := range missing {
		fmt.Println("aviso: sin dato para", slug)
	}
	for _, s := range samples {
		fmt.Printf("%-14s %4d\n", s.Pool, s.Count)
	}
	if *dry {
		return nil
	}
	return appendSamples(dataDir, samples)
}

// fetchFunc returns the current occupancy of the given pool codes.
type fetchFunc func(codes []string) (map[string]int, error)

// takeSample reads the occupancy of the pools open at now. It returns no
// samples and no error when every pool is closed, and the slugs of open pools
// missing from the response.
func takeSample(pools []*Pool, now time.Time, fetch fetchFunc) (samples []Sample, missing []string, err error) {
	var open []*Pool
	var codes []string
	for _, p := range pools {
		if p.OpenAt(now) {
			open = append(open, p)
			codes = append(codes, p.Code)
		}
	}
	if len(open) == 0 {
		return nil, nil, nil
	}
	counts, err := fetch(codes)
	if err != nil {
		return nil, nil, err
	}
	for _, p := range open {
		if n, ok := counts[p.Code]; ok {
			samples = append(samples, Sample{T: now, Pool: p.Slug, Count: n})
		} else {
			missing = append(missing, p.Slug)
		}
	}
	if len(samples) == 0 {
		return nil, missing, fmt.Errorf("la respuesta no contiene ninguna piscina abierta")
	}
	return samples, missing, nil
}

// withRetry retries fetch once after wait.
func withRetry(fetch fetchFunc, wait time.Duration) fetchFunc {
	return func(codes []string) (map[string]int, error) {
		counts, err := fetch(codes)
		if err == nil {
			return counts, nil
		}
		time.Sleep(wait)
		return fetch(codes)
	}
}

// aforoAPI is the city council's real-time occupancy endpoint.
type aforoAPI struct {
	URL    string
	Client *http.Client
}

// Counts asks for the current occupancy of the given pool codes.
func (a aforoAPI) Counts(codes []string) (map[string]int, error) {
	type centro struct {
		Cod   string `json:"codCentro"`
		Aforo string `json:"aforoTReal,omitempty"`
	}
	req := make([]centro, len(codes))
	for i, c := range codes {
		req[i].Cod = c
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	q := url.Values{"codCentros": {string(body)}, "idiomaUsuario": {"es"}}

	resp, err := a.Client.Post(a.URL+"?"+q.Encode(), "", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("aforo: HTTP %d", resp.StatusCode)
	}
	var res struct {
		ListaCentros []centro `json:"listaCentros"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, fmt.Errorf("aforo: %w", err)
	}
	counts := make(map[string]int, len(res.ListaCentros))
	for _, c := range res.ListaCentros {
		n, err := strconv.Atoi(c.Aforo)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("aforo: valor no válido %q para el centro %s", c.Aforo, c.Cod)
		}
		counts[c.Cod] = n
	}
	return counts, nil
}
