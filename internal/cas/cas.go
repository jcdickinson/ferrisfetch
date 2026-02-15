package cas

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jcdickinson/ferrisfetch/internal/config"
	"github.com/klauspost/compress/zstd"
)

// Dir returns the CAS directory path.
func Dir() string {
	return config.CASDir()
}

// path returns the sharded file path for a hash: cas/<first2>/<rest>.md.zst
func path(hash string) string {
	return filepath.Join(Dir(), hash[:2], hash[2:]+".md.zst")
}

// Write stores content in the CAS, returning its SHA-256 hash.
// If the content already exists, this is a no-op.
func Write(content string) (string, error) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(content)))

	p := path(hash)
	if _, err := os.Stat(p); err == nil {
		return hash, nil
	}

	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return "", fmt.Errorf("creating CAS directory: %w", err)
	}

	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		return "", fmt.Errorf("creating zstd writer: %w", err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		w.Close()
		return "", fmt.Errorf("compressing CAS content: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("closing zstd writer: %w", err)
	}

	if err := os.WriteFile(p, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("writing CAS file: %w", err)
	}

	return hash, nil
}

// Read retrieves content from the CAS by hash.
func Read(hash string) (string, error) {
	f, err := os.Open(path(hash))
	if err != nil {
		return "", fmt.Errorf("reading CAS file %s: %w", hash, err)
	}
	defer f.Close()

	r, err := zstd.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("creating zstd reader: %w", err)
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("decompressing CAS file %s: %w", hash, err)
	}
	return string(data), nil
}
