package docs

import (
	"encoding/json"
	"strings"
	"testing"
)

func strPtr(s string) *string { return &s }

// makeCrateWithItems builds a minimal RustdocCrate with the given items in its index.
func makeCrateWithItems(items map[string]RustdocItem) *RustdocCrate {
	return &RustdocCrate{
		Index:          items,
		Paths:          map[string]RustdocSummary{},
		ExternalCrates: map[string]ExternalCrate{},
	}
}

func TestGenerateFragments_Struct(t *testing.T) {
	t.Parallel()

	// Struct with two fields and one impl block with a method
	items := map[string]RustdocItem{
		"1": {ID: 1, Name: strPtr("x"), Docs: strPtr("The x coordinate")},
		"2": {ID: 2, Name: strPtr("y"), Docs: strPtr("The y coordinate")},
		"3": {ID: 3, Name: strPtr("len"), Docs: strPtr("Returns length"),
			Inner: json.RawMessage(`{"function":{"sig":{"inputs":[],"output":null},"generics":{"params":[]},"header":{}}}`)},
		"10": {ID: 10, Inner: json.RawMessage(`{"impl":{"trait":null,"for":null,"items":[3]}}`)},
	}
	crate := makeCrateWithItems(items)

	item := &RustdocItem{
		ID:   0,
		Name: strPtr("Point"),
		Inner: json.RawMessage(`{"struct":{"kind":{"plain":{"fields":[1,2]}},"impls":[10]}}`),
	}

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")
	if len(fragments) == 0 {
		t.Fatal("expected fragments")
	}

	var foundFields, foundImpls bool
	for _, f := range fragments {
		switch f.Name {
		case FragFields:
			foundFields = true
			if !strings.Contains(f.Content, "**x**") || !strings.Contains(f.Content, "**y**") {
				t.Errorf("fields fragment missing field names: %s", f.Content)
			}
		case FragImplementations:
			foundImpls = true
			if !strings.Contains(f.Content, "`len`") {
				t.Errorf("implementations fragment missing method: %s", f.Content)
			}
		}
	}
	if !foundFields {
		t.Error("expected fields fragment")
	}
	if !foundImpls {
		t.Error("expected implementations fragment")
	}
}

func TestGenerateFragments_Enum(t *testing.T) {
	t.Parallel()

	items := map[string]RustdocItem{
		"1": {ID: 1, Name: strPtr("A"), Docs: strPtr("Variant A")},
		"2": {ID: 2, Name: strPtr("B"), Docs: strPtr("Variant B")},
	}
	crate := makeCrateWithItems(items)

	item := &RustdocItem{
		ID:    0,
		Name:  strPtr("MyEnum"),
		Inner: json.RawMessage(`{"enum":{"variants":[1,2],"impls":[]}}`),
	}

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")

	var foundVariants bool
	for _, f := range fragments {
		if f.Name == FragVariants {
			foundVariants = true
			if !strings.Contains(f.Content, "**A**") || !strings.Contains(f.Content, "**B**") {
				t.Errorf("variants fragment missing variant names: %s", f.Content)
			}
		}
	}
	if !foundVariants {
		t.Error("expected variants fragment")
	}
}

func TestGenerateFragments_Trait(t *testing.T) {
	t.Parallel()

	items := map[string]RustdocItem{
		// Required method (no body)
		"1": {ID: 1, Name: strPtr("required_fn"), Docs: strPtr("Must implement"),
			Inner: json.RawMessage(`{"function":{"has_body":false,"sig":{"inputs":[],"output":null},"generics":{"params":[]},"header":{}}}`)},
		// Provided method (has body)
		"2": {ID: 2, Name: strPtr("provided_fn"), Docs: strPtr("Default impl"),
			Inner: json.RawMessage(`{"function":{"has_body":true,"sig":{"inputs":[],"output":null},"generics":{"params":[]},"header":{}}}`)},
		// Implementor impl block
		"20": {ID: 20, Inner: json.RawMessage(`{"impl":{"for":{"resolved_path":{"name":"Foo","id":30}},"trait":null,"items":[]}}`)},
	}
	crate := makeCrateWithItems(items)
	crate.Paths["30"] = RustdocSummary{CrateID: 0, Path: []string{"mycrate", "Foo"}, Kind: "struct"}

	item := &RustdocItem{
		ID:   0,
		Name: strPtr("MyTrait"),
		Inner: json.RawMessage(`{"trait":{"items":[1,2],"implementations":[20],"impls":[]}}`),
	}

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")

	names := map[string]bool{}
	for _, f := range fragments {
		names[f.Name] = true
	}

	if !names[FragRequiredMethods] {
		t.Error("expected required-methods fragment")
	}
	if !names[FragProvidedMethods] {
		t.Error("expected provided-methods fragment")
	}
	if !names[FragImplementors] {
		t.Error("expected implementors fragment")
	}
}

func TestGenerateFragments_UnknownKind(t *testing.T) {
	t.Parallel()

	item := &RustdocItem{
		ID:    0,
		Name:  strPtr("my_fn"),
		Inner: json.RawMessage(`{"function":{}}`),
	}
	crate := makeCrateWithItems(nil)

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")
	if fragments != nil {
		t.Errorf("expected nil for function kind, got %v", fragments)
	}
}
