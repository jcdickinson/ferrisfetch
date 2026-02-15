package docs

import "testing"

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

func TestResolveDocsRsURLs(t *testing.T) {
	docs := `See the [Serialize](https://docs.rs/serde/latest/serde/ser/trait.Serialize.html) trait
and [serde](https://docs.rs/serde/latest/serde/) for more info.`

	got := ResolveDocsRsURLs(docs)
	if got == nil {
		t.Fatal("expected non-nil map")
	}

	want := map[string]string{
		"https://docs.rs/serde/latest/serde/ser/trait.Serialize.html": "rsdoc://serde/latest/serde::ser::Serialize",
		"https://docs.rs/serde/latest/serde/":                        "rsdoc://serde/latest/serde",
	}

	for k, v := range want {
		if got[k] != v {
			t.Errorf("ResolveDocsRsURLs[%q] = %q, want %q", k, got[k], v)
		}
	}
}
