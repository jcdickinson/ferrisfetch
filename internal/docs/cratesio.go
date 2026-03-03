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

// CrateInfo holds metadata for a crate from crates.io.
type CrateInfo struct {
	Name        string
	Description string
	Homepage    string
	Repository  string
	License     string
	Downloads   int
	Keywords    []string
	Versions    []VersionInfo
}

// VersionInfo holds metadata for a single crate version.
type VersionInfo struct {
	Num    string
	Yanked bool
	MSRV   string
}

// FetchCrateInfo fetches crate metadata from crates.io.
func FetchCrateInfo(name string) (*CrateInfo, error) {
	u := fmt.Sprintf("https://crates.io/api/v1/crates/%s", url.PathEscape(name))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "ferrisfetch/0.1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching crate info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("crates.io returned %d for %s: %s", resp.StatusCode, name, string(body))
	}

	var payload struct {
		Crate struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Homepage    string `json:"homepage"`
			Repository  string `json:"repository"`
			Downloads   int    `json:"downloads"`
		} `json:"crate"`
		Keywords []struct {
			Keyword string `json:"keyword"`
		} `json:"keywords"`
		Versions []struct {
			Num         string  `json:"num"`
			Yanked      bool    `json:"yanked"`
			License     string  `json:"license"`
			RustVersion *string `json:"rust_version"`
		} `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding crate info: %w", err)
	}

	info := &CrateInfo{
		Name:        payload.Crate.Name,
		Description: payload.Crate.Description,
		Homepage:    payload.Crate.Homepage,
		Repository:  payload.Crate.Repository,
		Downloads:   payload.Crate.Downloads,
	}

	for _, kw := range payload.Keywords {
		info.Keywords = append(info.Keywords, kw.Keyword)
	}

	for _, v := range payload.Versions {
		vi := VersionInfo{
			Num:    v.Num,
			Yanked: v.Yanked,
		}
		if v.RustVersion != nil {
			vi.MSRV = *v.RustVersion
		}
		// Grab license from the latest version
		if len(info.Versions) == 0 && v.License != "" {
			info.License = v.License
		}
		info.Versions = append(info.Versions, vi)
	}

	return info, nil
}

// Dependency holds a single dependency from a crate version.
type Dependency struct {
	Name     string `json:"crate_id"`
	Req      string `json:"req"`
	Kind     string `json:"kind"` // "normal", "dev", "build"
	Optional bool   `json:"optional"`
}

// FetchVersionDeps fetches dependencies for a specific crate version from crates.io.
func FetchVersionDeps(name, version string) ([]Dependency, error) {
	u := fmt.Sprintf("https://crates.io/api/v1/crates/%s/%s/dependencies",
		url.PathEscape(name), url.PathEscape(version))

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "ferrisfetch/0.1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching dependencies: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("crates.io returned %d for %s@%s deps: %s", resp.StatusCode, name, version, string(body))
	}

	var payload struct {
		Dependencies []Dependency `json:"dependencies"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding dependencies: %w", err)
	}

	return payload.Dependencies, nil
}
