package docs

import (
	"slices"
	"testing"
)

func TestCanonicalizePaths(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		crateName string
		wantHas   []string // candidates that must be present (order-insensitive)
		wantFirst string   // first candidate (always the original)
	}{
		{
			name:      "already canonical",
			path:      "openrouter_rs::types::stream::StreamEvent",
			crateName: "openrouter-rs",
			wantFirst: "openrouter_rs::types::stream::StreamEvent",
			wantHas:   []string{"openrouter_rs::types::stream::StreamEvent"},
		},
		{
			name:      "docs.rs style: slashes + kind prefix + no lib prefix",
			path:      "types/stream/enum.StreamEvent",
			crateName: "openrouter-rs",
			wantFirst: "types/stream/enum.StreamEvent",
			wantHas: []string{
				"types::stream::enum.StreamEvent",
				"types::stream::StreamEvent",
				"openrouter_rs::types::stream::StreamEvent",
			},
		},
		{
			name:      "slashes with lib prefix",
			path:      "openrouter_rs/types/stream/enum.StreamEvent",
			crateName: "openrouter-rs",
			wantHas:   []string{"openrouter_rs::types::stream::StreamEvent"},
		},
		{
			name:      "just the item name",
			path:      "Serialize",
			crateName: "serde",
			wantHas:   []string{"Serialize", "serde::Serialize"},
		},
		{
			name:      "lib name matches cargo name",
			path:      "value/enum.Value",
			crateName: "serde_json",
			wantHas: []string{
				"value::enum.Value",
				"value::Value",
				"serde_json::value::Value",
			},
		},
		{
			name:      "crate root only",
			path:      "openrouter_rs",
			crateName: "openrouter-rs",
			wantHas:   []string{"openrouter_rs"},
		},
		{
			name:      "fn with lowercase name",
			path:      "tokio/fn.spawn",
			crateName: "tokio",
			wantHas: []string{
				"tokio::fn.spawn",
				"tokio::spawn",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalizePaths(tt.path, tt.crateName)
			if len(got) == 0 || got[0] != tt.path {
				t.Errorf("CanonicalizePaths(%q, %q): first candidate = %q, want %q",
					tt.path, tt.crateName, got[0], tt.path)
			}
			if tt.wantFirst != "" && got[0] != tt.wantFirst {
				t.Errorf("first = %q, want %q", got[0], tt.wantFirst)
			}
			for _, want := range tt.wantHas {
				if !slices.Contains(got, want) {
					t.Errorf("CanonicalizePaths(%q, %q) = %v, missing %q",
						tt.path, tt.crateName, got, want)
				}
			}
		})
	}
}

func TestDocsRsToRsdoc(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// Item URLs
		{
			"https://docs.rs/serde/latest/serde/ser/trait.Serialize.html",
			"rsdoc://serde/latest/serde::ser::Serialize",
		},
		{
			"https://docs.rs/serde/1.0.210/serde/de/trait.Deserialize.html",
			"rsdoc://serde/1.0.210/serde::de::Deserialize",
		},
		{
			"https://docs.rs/tokio/latest/tokio/sync/struct.Mutex.html",
			"rsdoc://tokio/latest/tokio::sync::Mutex",
		},
		{
			"https://docs.rs/serde/latest/serde/ser/fn.impossible.html",
			"rsdoc://serde/latest/serde::ser::impossible",
		},
		// Item URL with fragment (fragment ignored)
		{
			"https://docs.rs/serde/latest/serde/ser/trait.Serialize.html#method.serialize",
			"rsdoc://serde/latest/serde::ser::Serialize",
		},
		// Module via index.html
		{
			"https://docs.rs/serde/latest/serde/ser/index.html",
			"rsdoc://serde/latest/serde::ser",
		},
		// Module via trailing slash
		{
			"https://docs.rs/serde/latest/serde/ser/",
			"rsdoc://serde/latest/serde::ser",
		},
		// Crate root
		{
			"https://docs.rs/serde/latest/serde/",
			"rsdoc://serde/latest/serde",
		},
		// Crate root without trailing slash
		{
			"https://docs.rs/serde/latest/serde",
			"rsdoc://serde/latest/serde",
		},
		// Crate info page — not convertible
		{"https://docs.rs/crate/serde/latest", ""},
		// Too few path segments
		{"https://docs.rs/serde/latest", ""},
		{"https://docs.rs/serde", ""},
		// HTTP variant
		{
			"http://docs.rs/serde/latest/serde/ser/trait.Serialize.html",
			"rsdoc://serde/latest/serde::ser::Serialize",
		},
	}

	for _, tt := range tests {
		got := docsRsToRsdoc(tt.url)
		if got != tt.want {
			t.Errorf("docsRsToRsdoc(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

func TestDocsRsURLToRsdocURI(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "not a docs.rs URL",
			in:   "https://example.com/foo",
			want: "",
		},
		{
			name: "rsdoc URI passes through unchanged (returns empty)",
			in:   "rsdoc://serde/latest/serde::Serialize",
			want: "",
		},
		{
			name: "basic item URL",
			in:   "https://docs.rs/openrouter-rs/0.9.0/openrouter_rs/types/stream/enum.StreamEvent.html",
			want: "rsdoc://openrouter-rs/0.9.0/openrouter_rs::types::stream::StreamEvent",
		},
		{
			name: "preserves simple fragment",
			in:   "https://docs.rs/serde/latest/serde/ser/trait.Serialize.html#required-methods",
			want: "rsdoc://serde/latest/serde::ser::Serialize#required-methods",
		},
		{
			name: "preserves #variants",
			in:   "https://docs.rs/openrouter-rs/0.9.0/openrouter_rs/types/stream/enum.StreamEvent.html#variants",
			want: "rsdoc://openrouter-rs/0.9.0/openrouter_rs::types::stream::StreamEvent#variants",
		},
		{
			// Forward verbatim — lookup will 404 if there's no match, which
			// is clearer than guessing whether the fragment is mappable.
			name: "forwards method anchor unchanged",
			in:   "https://docs.rs/serde/latest/serde/ser/trait.Serialize.html#method.serialize",
			want: "rsdoc://serde/latest/serde::ser::Serialize#method.serialize",
		},
		{
			name: "forwards impl anchor unchanged",
			in:   "https://docs.rs/serde/latest/serde/struct.Foo.html#impl-Display-for-Foo",
			want: "rsdoc://serde/latest/serde::Foo#impl-Display-for-Foo",
		},
		{
			name: "module URL",
			in:   "https://docs.rs/serde/latest/serde/ser/index.html",
			want: "rsdoc://serde/latest/serde::ser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DocsRsURLToRsdocURI(tt.in)
			if got != tt.want {
				t.Errorf("DocsRsURLToRsdocURI(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDocsRsURLs(t *testing.T) {
	docs := `See the [Serialize](https://docs.rs/serde/latest/serde/ser/trait.Serialize.html) trait
and [serde](https://docs.rs/serde/latest/serde/) for more info.`

	got := ResolveDocsRsURLs(docs)
	if got == nil {
		t.Fatal("expected non-nil map")
	}

	want := map[string]string{
		"https://docs.rs/serde/latest/serde/ser/trait.Serialize.html": "rsdoc://serde/latest/serde::ser::Serialize",
		"https://docs.rs/serde/latest/serde/":                         "rsdoc://serde/latest/serde",
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("ResolveDocsRsURLs[%q] = %q, want %q", k, got[k], v)
		}
	}
}
