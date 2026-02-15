package markdown

import (
	"strings"
	"testing"
)

func TestRewriteLinks_InlineLinks(t *testing.T) {
	t.Parallel()
	src := "See [Foo](old/path) for details."
	got := RewriteLinks(src, map[string]string{"old/path": "rsdoc://crate/1.0/Foo"})
	want := "See [Foo](rsdoc://crate/1.0/Foo) for details."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRewriteLinks_ReferenceStyleLinks(t *testing.T) {
	t.Parallel()
	src := "See [Foo][ref] for details.\n\n[ref]: old/path"
	got := RewriteLinks(src, map[string]string{"old/path": "rsdoc://new"})
	if !strings.Contains(got, "[ref]: rsdoc://new") {
		t.Errorf("reference link not rewritten: %q", got)
	}
}

func TestRewriteLinks_EmptyMap(t *testing.T) {
	t.Parallel()
	src := "Hello [world](url)."
	got := RewriteLinks(src, nil)
	if got != src {
		t.Errorf("expected unchanged, got %q", got)
	}
	got = RewriteLinks(src, map[string]string{})
	if got != src {
		t.Errorf("expected unchanged for empty map, got %q", got)
	}
}

func TestRewriteLinks_NoMatchingLinks(t *testing.T) {
	t.Parallel()
	src := "Check [this](keep-me) out."
	got := RewriteLinks(src, map[string]string{"other": "rsdoc://x"})
	if got != src {
		t.Errorf("expected unchanged, got %q", got)
	}
}

func TestRewriteLinks_MultipleLinks(t *testing.T) {
	t.Parallel()
	src := "[A](a-dest) and [B](b-dest) together."
	got := RewriteLinks(src, map[string]string{
		"a-dest": "rsdoc://a",
		"b-dest": "rsdoc://b",
	})
	if !strings.Contains(got, "(rsdoc://a)") {
		t.Error("link A not rewritten")
	}
	if !strings.Contains(got, "(rsdoc://b)") {
		t.Error("link B not rewritten")
	}
}

func TestAddFrontMatter(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		got := AddFrontMatter("# Doc", map[string]string{"fields": "rsdoc://x#fields"})
		if !strings.HasPrefix(got, "---\n") {
			t.Error("missing opening ---")
		}
		if !strings.Contains(got, "fields: rsdoc://x#fields") {
			t.Error("missing fragment entry")
		}
		if !strings.HasSuffix(got, "# Doc") {
			t.Error("original content missing")
		}
	})

	t.Run("sorted_keys", func(t *testing.T) {
		got := AddFrontMatter("body", map[string]string{
			"z-frag": "rsdoc://z",
			"a-frag": "rsdoc://a",
		})
		aIdx := strings.Index(got, "a-frag")
		zIdx := strings.Index(got, "z-frag")
		if aIdx > zIdx {
			t.Error("keys not sorted alphabetically")
		}
	})

	t.Run("empty_map", func(t *testing.T) {
		got := AddFrontMatter("body", nil)
		if got != "body" {
			t.Errorf("expected unchanged for empty map, got %q", got)
		}
	})
}
