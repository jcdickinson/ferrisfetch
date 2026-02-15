package docs

import "encoding/json"

// RustdocCrate is the top-level structure of rustdoc JSON output.
type RustdocCrate struct {
	Root           int                        `json:"root"`
	CrateVersion   *string                    `json:"crate_version"`
	Index          map[string]RustdocItem     `json:"index"`
	Paths          map[string]RustdocSummary  `json:"paths"`
	ExternalCrates map[string]ExternalCrate   `json:"external_crates"`
	FormatVersion  int                        `json:"format_version"`
}

// ExternalCrate identifies a dependency crate by name.
type ExternalCrate struct {
	Name        string `json:"name"`
	HTMLRootURL string `json:"html_root_url"`
}

// RustdocItem is a single item in the rustdoc index.
type RustdocItem struct {
	ID      int             `json:"id"`
	CrateID int             `json:"crate_id"`
	Name    *string         `json:"name"`
	Docs    *string         `json:"docs"`
	Links   map[string]int  `json:"links"` // markdown text → item ID (u32)
	Inner   json.RawMessage `json:"inner"`
}

// RustdocSummary provides the path and kind for an item.
type RustdocSummary struct {
	CrateID int      `json:"crate_id"`
	Path    []string `json:"path"`
	Kind    string   `json:"kind"`
}

// Fragment is a sub-document generated from an item (e.g. #fields, #variants).
type Fragment struct {
	Name    string
	Content string
}

// ParsedItem is a processed doc item ready for indexing.
type ParsedItem struct {
	RustdocID string
	Name      string
	Path      string
	Kind      string
	Docs      string
	Signature string
	DocLinks  map[string]string // resolved: markdown target → rsdoc URI
	Fragments []Fragment
}
