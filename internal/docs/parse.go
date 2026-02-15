package docs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Parse extracts items from rustdoc JSON bytes.
// crateName and version are used to build rsdoc:// URIs in resolved doc links.
func Parse(data []byte, crateName, version string) (*RustdocCrate, []ParsedItem, error) {
	var crate RustdocCrate
	if err := json.Unmarshal(data, &crate); err != nil {
		return nil, nil, fmt.Errorf("unmarshaling rustdoc JSON: %w", err)
	}

	var items []ParsedItem
	for id, item := range crate.Index {
		if item.CrateID != 0 {
			continue
		}
		parsed := parseItem(id, &item, &crate)
		if parsed == nil {
			continue
		}
		parsed.DocLinks = ResolveDocLinks(&item, &crate, crateName, version)
		for k, v := range ResolveDocsRsURLs(parsed.Docs) {
			if parsed.DocLinks == nil {
				parsed.DocLinks = make(map[string]string)
			}
			parsed.DocLinks[k] = v
		}
		items = append(items, *parsed)
	}

	// Generate fragments after all items are parsed (needs full crate context)
	for i, parsed := range items {
		item, ok := crate.Index[parsed.RustdocID]
		if !ok {
			continue
		}
		items[i].Fragments = GenerateFragments(&item, &crate, crateName, version)
	}

	return &crate, items, nil
}

func parseItem(id string, item *RustdocItem, crate *RustdocCrate) *ParsedItem {
	if item.Name == nil {
		return nil
	}

	name := *item.Name

	// Resolve full path and kind from the paths map
	path := name
	kind := innerKind(item.Inner)
	if summary, ok := crate.Paths[id]; ok {
		path = strings.Join(summary.Path, "::")
		kind = summary.Kind
	}

	// Skip impl blocks — they don't have meaningful standalone docs
	if kind == "impl" {
		return nil
	}

	var docs string
	if item.Docs != nil {
		docs = *item.Docs
	}

	sig := extractSignature(item.Inner, kind)

	return &ParsedItem{
		RustdocID: id,
		Name:      name,
		Path:      path,
		Kind:      kind,
		Docs:      docs,
		Signature: sig,
	}
}

// innerKind extracts the kind from the inner JSON's single key.
func innerKind(inner json.RawMessage) string {
	if len(inner) == 0 {
		return "unknown"
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(inner, &outer); err != nil {
		return "unknown"
	}
	for k := range outer {
		return k
	}
	return "unknown"
}

// extractSignature pulls a useful signature from the inner JSON based on kind.
func extractSignature(inner json.RawMessage, kind string) string {
	if len(inner) == 0 {
		return ""
	}

	var outer map[string]json.RawMessage
	if err := json.Unmarshal(inner, &outer); err != nil {
		return ""
	}

	// The inner field is a map with a single key matching the kind
	kindData, ok := outer[kind]
	if !ok {
		// Try common alternatives
		for _, alt := range []string{"function", "method", "struct", "enum", "trait", "type_alias"} {
			if d, found := outer[alt]; found {
				kindData = d
				ok = true
				kind = alt
				break
			}
		}
		if !ok {
			return ""
		}
	}

	switch kind {
	case "function", "method":
		return extractFnSignature(kindData)
	case "struct", "enum", "trait", "type_alias":
		return extractTypeSignature(kindData)
	default:
		return ""
	}
}

func extractFnSignature(data json.RawMessage) string {
	var fn struct {
		Sig struct {
			Header string `json:"header"`
		} `json:"sig"`
		Header string `json:"header"`
	}
	if err := json.Unmarshal(data, &fn); err == nil {
		if fn.Sig.Header != "" {
			return fn.Sig.Header
		}
		if fn.Header != "" {
			return fn.Header
		}
	}

	var fnDecl struct {
		Decl string `json:"decl"`
	}
	if err := json.Unmarshal(data, &fnDecl); err == nil && fnDecl.Decl != "" {
		return fnDecl.Decl
	}

	return ""
}

func extractTypeSignature(data json.RawMessage) string {
	var t struct {
		Sig struct {
			Header string `json:"header"`
		} `json:"sig"`
		Header string `json:"header"`
	}
	if err := json.Unmarshal(data, &t); err == nil {
		if t.Sig.Header != "" {
			return t.Sig.Header
		}
		if t.Header != "" {
			return t.Header
		}
	}
	return ""
}
