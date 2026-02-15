package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheBase_XDGSet(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/custom/cache")
	got := cacheBase()
	want := filepath.Join("/custom/cache", "ferrisfetch")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCacheBase_HomeDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	got := cacheBase()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home dir")
	}
	want := filepath.Join(home, ".cache", "ferrisfetch")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestCacheBase_TmpFallback(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")
	got := cacheBase()
	// Should use os.TempDir() when HOME is unset
	if !strings.Contains(got, "ferrisfetch") {
		t.Errorf("expected ferrisfetch in path, got %q", got)
	}
}
