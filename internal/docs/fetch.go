package docs

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/klauspost/compress/zstd"
)

var httpClient = &http.Client{Timeout: 60 * time.Second}

// FetchRustdocJSON downloads and decompresses rustdoc JSON from docs.rs.
// The version "latest" is resolved by docs.rs via redirect.
func FetchRustdocJSON(name, version string) ([]byte, error) {
	if version == "" {
		version = "latest"
	}

	url := fmt.Sprintf("https://docs.rs/crate/%s/%s/json", name, version)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "ferrisfetch/0.1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("docs.rs returned %d for %s/%s: %s", resp.StatusCode, name, version, string(body))
	}

	// docs.rs returns zstd-compressed JSON
	decoder, err := zstd.NewReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("creating zstd decoder: %w", err)
	}
	defer decoder.Close()

	data, err := io.ReadAll(decoder)
	if err != nil {
		return nil, fmt.Errorf("decompressing rustdoc JSON: %w", err)
	}

	return data, nil
}
