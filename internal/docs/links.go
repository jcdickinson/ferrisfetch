package docs

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// ResolveDocLinks resolves rustdoc intra-doc links to rsdoc:// URIs.
// The item's Links field maps markdown target text (e.g. "Value::as_str") to
// item IDs in the crate index. We look up each ID in Paths to get the full
// Rust path and crate origin.
func ResolveDocLinks(item *RustdocItem, crate *RustdocCrate, crateName, version string) map[string]string {
	if len(item.Links) == 0 {
		return nil
	}

	resolved := make(map[string]string, len(item.Links))
	for markdownTarget, itemID := range item.Links {
		uri := ResolveItemURI(itemID, crate, crateName, version)
		if uri == "" {
			continue
		}
		resolved[markdownTarget] = uri
	}

	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// ResolveItemURI builds an rsdoc:// URI for a given rustdoc item ID.
// Returns "" if the item can't be resolved.
func ResolveItemURI(itemID int, crate *RustdocCrate, crateName, version string) string {
	idStr := strconv.Itoa(itemID)
	summary, ok := crate.Paths[idStr]
	if !ok {
		return ""
	}
	fullPath := strings.Join(summary.Path, "::")
	if summary.CrateID == 0 {
		return fmt.Sprintf("rsdoc://%s/%s/%s", crateName, version, fullPath)
	}
	depName := crate.ExternalCrateName(summary.CrateID)
	if depName == "" {
		return ""
	}
	return fmt.Sprintf("rsdoc://%s/latest/%s", depName, fullPath)
}

// ExternalCrateName looks up the Cargo package name for a dependency by crate_id.
// Prefers the name extracted from html_root_url (e.g. "https://docs.rs/tracing-core/0.1.36/...")
// since the Name field uses the Rust lib name (underscores) which may differ from the
// Cargo name (hyphens). Falls back to the lib name if no docs.rs URL is present.
func (c *RustdocCrate) ExternalCrateName(crateID int) string {
	ext, ok := c.ExternalCrates[strconv.Itoa(crateID)]
	if !ok {
		return ""
	}
	if name := extractDocsRsCrateName(ext.HTMLRootURL); name != "" {
		return name
	}
	return ext.Name
}

// docsRsCrateNameRe extracts the crate name from a docs.rs html_root_url.
// Example: "https://docs.rs/tracing-core/0.1.36/x86_64-unknown-linux-gnu/" → "tracing-core"
var docsRsCrateNameRe = regexp.MustCompile(`^https?://docs\.rs/([^/]+)/`)

func extractDocsRsCrateName(rootURL string) string {
	m := docsRsCrateNameRe.FindStringSubmatch(rootURL)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// CanonicalizePaths returns candidate canonical item paths to try for a
// user-supplied path. Always includes the original first. Adds normalized forms
// that translate docs.rs URL conventions back to Rust paths:
//
//   - `/` separators rewritten to `::`
//   - `kind.Name` segments (e.g. `enum.Foo`, `struct.Bar`) stripped to `Name`
//   - lib-name prefix (Cargo name with `-` → `_`) prepended when missing
//
// Examples for crate `openrouter-rs`:
//
//	types/stream/enum.StreamEvent → openrouter_rs::types::stream::StreamEvent
//	openrouter_rs::types::stream::StreamEvent → unchanged (returned first)
func CanonicalizePaths(path, crateName string) []string {
	candidates := []string{path}
	seen := map[string]bool{path: true}
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		candidates = append(candidates, p)
	}

	libName := strings.ReplaceAll(crateName, "-", "_")

	// Normalize: slashes → ::, then strip kind. prefix from each segment.
	normalized := strings.ReplaceAll(path, "/", "::")
	if normalized != path {
		add(normalized)
	}

	segments := strings.Split(normalized, "::")
	stripped := false
	for i, seg := range segments {
		if dot := strings.IndexByte(seg, '.'); dot >= 0 {
			segments[i] = seg[dot+1:]
			stripped = true
		}
	}
	if stripped {
		add(strings.Join(segments, "::"))
	}

	// Prepend lib name if missing.
	if libName != "" {
		joined := strings.Join(segments, "::")
		if joined != libName && !strings.HasPrefix(joined, libName+"::") {
			add(libName + "::" + joined)
		}
	}

	return candidates
}

// DocsRsURLToRsdocURI converts a docs.rs documentation URL into an rsdoc://
// URI suitable for `rsdoc get`. Forwards any fragment unchanged — if it
// doesn't match an rsdoc fragment, the lookup will 404 with a clear error.
//
// Returns "" if the input isn't a docs.rs URL.
func DocsRsURLToRsdocURI(input string) string {
	if !strings.HasPrefix(input, "https://docs.rs/") && !strings.HasPrefix(input, "http://docs.rs/") {
		return ""
	}
	base := docsRsToRsdoc(input)
	if base == "" {
		return ""
	}
	if hashIdx := strings.Index(input, "#"); hashIdx >= 0 && hashIdx < len(input)-1 {
		return base + "#" + input[hashIdx+1:]
	}
	return base
}

// docsRsRe matches docs.rs documentation URLs in markdown text.
// Captures everything up to whitespace or markdown link delimiters.
var docsRsRe = regexp.MustCompile(`https?://docs\.rs/[^\s)\]>]+`)

// ResolveDocsRsURLs scans doc text for docs.rs URLs and returns a mapping
// from each URL to its equivalent rsdoc:// URI.
func ResolveDocsRsURLs(docs string) map[string]string {
	matches := docsRsRe.FindAllString(docs, -1)
	if len(matches) == 0 {
		return nil
	}

	resolved := make(map[string]string)
	for _, fullURL := range matches {
		if uri := docsRsToRsdoc(fullURL); uri != "" {
			resolved[fullURL] = uri
		}
	}

	if len(resolved) == 0 {
		return nil
	}
	return resolved
}

// docsRsToRsdoc converts a single docs.rs URL to an rsdoc:// URI.
// Returns "" if the URL can't be converted (e.g. crate info pages).
func docsRsToRsdoc(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, "/")

	// Skip /crate/ info pages
	if strings.HasPrefix(path, "crate/") {
		return ""
	}

	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 3 {
		return ""
	}

	crateName := parts[0]
	version := parts[1]
	rest := parts[2]

	segments := strings.Split(rest, "/")
	// Trim empty trailing segments
	for len(segments) > 0 && segments[len(segments)-1] == "" {
		segments = segments[:len(segments)-1]
	}
	if len(segments) == 0 {
		return ""
	}

	// Handle last segment: index.html (module) or {kind}.{Name}.html (item)
	last := segments[len(segments)-1]
	if strings.HasSuffix(last, ".html") {
		if last == "index.html" {
			segments = segments[:len(segments)-1]
		} else {
			base := strings.TrimSuffix(last, ".html")
			if dotIdx := strings.Index(base, "."); dotIdx >= 0 {
				segments[len(segments)-1] = base[dotIdx+1:]
			}
		}
	}

	if len(segments) == 0 {
		return ""
	}

	rustPath := strings.Join(segments, "::")
	return fmt.Sprintf("rsdoc://%s/%s/%s", crateName, version, rustPath)
}
