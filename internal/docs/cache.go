package docs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/klauspost/compress/zstd"
)

func crateCachePath(name, version string) string {
	return filepath.Join(config.JSONCacheDir(), name+"_"+version+".json.zst")
}

// SaveCrateCache compresses and saves rustdoc JSON bytes to disk.
func SaveCrateCache(data []byte, name, version string) error {
	dir := config.JSONCacheDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating json cache dir: %w", err)
	}

	f, err := os.Create(crateCachePath(name, version))
	if err != nil {
		return fmt.Errorf("creating cache file: %w", err)
	}
	defer f.Close()

	w, err := zstd.NewWriter(f)
	if err != nil {
		return fmt.Errorf("creating zstd writer: %w", err)
	}

	if _, err := w.Write(data); err != nil {
		w.Close()
		return fmt.Errorf("writing compressed data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing zstd writer: %w", err)
	}
	return nil
}

// LoadCrateCache loads and decompresses cached rustdoc JSON from disk.
func LoadCrateCache(name, version string) (*RustdocCrate, error) {
	f, err := os.Open(crateCachePath(name, version))
	if err != nil {
		return nil, fmt.Errorf("opening cache file: %w", err)
	}
	defer f.Close()

	r, err := zstd.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("creating zstd reader: %w", err)
	}
	defer r.Close()

	var crate RustdocCrate
	if err := json.NewDecoder(r).Decode(&crate); err != nil {
		return nil, fmt.Errorf("decoding cached rustdoc JSON: %w", err)
	}
	return &crate, nil
}

// HasCrateCache checks whether a cached rustdoc JSON file exists on disk.
func HasCrateCache(name, version string) bool {
	_, err := os.Stat(crateCachePath(name, version))
	return err == nil
}
