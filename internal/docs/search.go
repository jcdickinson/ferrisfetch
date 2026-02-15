package docs

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

type CratesIOResult struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	MaxVersion  string `json:"max_version"`
	Downloads   int    `json:"downloads"`
}

// SearchCratesIO searches crates.io for crates matching the query.
func SearchCratesIO(query string, limit int) ([]CratesIOResult, error) {
	if limit <= 0 {
		limit = 20
	}

	u := fmt.Sprintf("https://crates.io/api/v1/crates?q=%s&per_page=%s",
		url.QueryEscape(query), strconv.Itoa(limit))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "ferrisfetch/0.1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("searching crates.io: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("crates.io returned %d: %s", resp.StatusCode, string(body))
	}

	var payload struct {
		Crates []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			MaxVersion  string `json:"max_version"`
			Downloads   int    `json:"downloads"`
		} `json:"crates"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding crates.io response: %w", err)
	}

	results := make([]CratesIOResult, len(payload.Crates))
	for i, c := range payload.Crates {
		results[i] = CratesIOResult{
			Name:        c.Name,
			Description: c.Description,
			MaxVersion:  c.MaxVersion,
			Downloads:   c.Downloads,
		}
	}
	return results, nil
}
