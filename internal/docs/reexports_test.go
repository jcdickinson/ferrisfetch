package docs

import (
	"encoding/json"
	"testing"
)

func TestCollectReexports_Individual(t *testing.T) {
	t.Parallel()

	// Root module re-exports dep::Thing as mycrate::Thing
	crate := &RustdocCrate{
		Root: 0,
		Index: map[string]RustdocItem{
			"0": {ID: 0, Name: strPtr("mycrate"), Inner: json.RawMessage(`{"module":{"items":[1]}}`)},
			"1": {ID: 1, Name: strPtr("Thing"),
				Inner: json.RawMessage(`{"use":{"name":"Thing","id":100,"is_glob":false}}`)},
		},
		Paths: map[string]RustdocSummary{
			"0":   {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
			"100": {CrateID: 5, Path: []string{"dep", "Thing"}, Kind: "struct"},
		},
		ExternalCrates: map[string]ExternalCrate{
			"5": {Name: "dep"},
		},
	}

	reexports := CollectReexports(crate, "mycrate")
	if len(reexports) != 1 {
		t.Fatalf("expected 1 reexport, got %d", len(reexports))
	}
	r := reexports[0]
	if r.LocalPrefix != "mycrate::Thing" {
		t.Errorf("LocalPrefix = %s", r.LocalPrefix)
	}
	if r.SourceCrate != "dep" {
		t.Errorf("SourceCrate = %s", r.SourceCrate)
	}
	if r.SourcePrefix != "dep::Thing" {
		t.Errorf("SourcePrefix = %s", r.SourcePrefix)
	}
}

func TestCollectReexports_Glob(t *testing.T) {
	t.Parallel()

	// mycrate::prelude glob re-exports dep::types::*
	crate := &RustdocCrate{
		Root: 0,
		Index: map[string]RustdocItem{
			"0": {ID: 0, Name: strPtr("mycrate"), Inner: json.RawMessage(`{"module":{"items":[1]}}`)},
			"1": {ID: 1, Name: strPtr("prelude"), Inner: json.RawMessage(`{"module":{"items":[2]}}`)},
			"2": {ID: 2, Name: strPtr("*"),
				Inner: json.RawMessage(`{"use":{"name":"*","id":50,"is_glob":true}}`)},
		},
		Paths: map[string]RustdocSummary{
			"0":  {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
			"1":  {CrateID: 0, Path: []string{"mycrate", "prelude"}, Kind: "module"},
			"50": {CrateID: 5, Path: []string{"dep", "types"}, Kind: "module"},
		},
		ExternalCrates: map[string]ExternalCrate{
			"5": {Name: "dep"},
		},
	}

	reexports := CollectReexports(crate, "mycrate")
	if len(reexports) != 1 {
		t.Fatalf("expected 1 reexport, got %d", len(reexports))
	}
	r := reexports[0]
	if r.LocalPrefix != "mycrate::prelude" {
		t.Errorf("LocalPrefix = %s", r.LocalPrefix)
	}
	if r.SourceCrate != "dep" || r.SourcePrefix != "dep::types" {
		t.Errorf("got src=%s prefix=%s", r.SourceCrate, r.SourcePrefix)
	}
}

func TestCollectReexports_SkipsSelfGlob(t *testing.T) {
	t.Parallel()

	// Glob re-export from self module should be skipped
	crate := &RustdocCrate{
		Root: 0,
		Index: map[string]RustdocItem{
			"0": {ID: 0, Name: strPtr("mycrate"), Inner: json.RawMessage(`{"module":{"items":[1]}}`)},
			"1": {ID: 1, Name: strPtr("*"),
				Inner: json.RawMessage(`{"use":{"name":"*","id":0,"is_glob":true}}`)},
		},
		Paths: map[string]RustdocSummary{
			"0": {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
		},
		ExternalCrates: map[string]ExternalCrate{},
	}

	reexports := CollectReexports(crate, "mycrate")
	if len(reexports) != 0 {
		t.Errorf("expected 0 reexports for self-glob, got %d: %v", len(reexports), reexports)
	}
}

func TestCollectReexports_Empty(t *testing.T) {
	t.Parallel()

	crate := &RustdocCrate{
		Root: 0,
		Index: map[string]RustdocItem{
			"0": {ID: 0, Name: strPtr("mycrate"), Inner: json.RawMessage(`{"module":{"items":[]}}`)},
		},
		Paths: map[string]RustdocSummary{
			"0": {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
		},
		ExternalCrates: map[string]ExternalCrate{},
	}

	reexports := CollectReexports(crate, "mycrate")
	if len(reexports) != 0 {
		t.Errorf("expected 0 reexports, got %d", len(reexports))
	}
}
