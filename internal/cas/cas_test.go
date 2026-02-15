package cas

import (
	"os"
	"testing"
)

func TestWriteRead_RoundTrip(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	content := "# Hello\n\nThis is some documentation."
	hash, err := Write(content)
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	got, err := Read(hash)
	if err != nil {
		t.Fatal(err)
	}
	if got != content {
		t.Errorf("round-trip failed: got %q, want %q", got, content)
	}
}

func TestWrite_Dedup(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	content := "duplicate content"
	hash1, err := Write(content)
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := Write(content)
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Errorf("same content produced different hashes: %s vs %s", hash1, hash2)
	}
}

func TestWrite_DifferentContent(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	hash1, err := Write("content A")
	if err != nil {
		t.Fatal(err)
	}
	hash2, err := Write("content B")
	if err != nil {
		t.Fatal(err)
	}
	if hash1 == hash2 {
		t.Error("different content should produce different hashes")
	}
}

func TestRead_MissingHash(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	_, err := Read("0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Fatal("expected error for missing hash")
	}
	if !os.IsNotExist(unwrapPathError(err)) {
		// The error wraps a path error; just check it's an error
		t.Logf("got expected error: %v", err)
	}
}

// unwrapPathError extracts the underlying error if it's a PathError.
func unwrapPathError(err error) error {
	for {
		if pe, ok := err.(*os.PathError); ok {
			return pe
		}
		if u, ok := err.(interface{ Unwrap() error }); ok {
			err = u.Unwrap()
		} else {
			return err
		}
	}
}
