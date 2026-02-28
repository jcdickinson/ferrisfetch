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
			if !strings.Contains(f.Content, "`fn len()`") {
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

func TestGenerateFragments_TypesUsed(t *testing.T) {
	t.Parallel()

	// Struct with an impl block whose method references resolvable types in params and return.
	items := map[string]RustdocItem{
		// Method: fn get(&self, key: Key) -> Value
		"3": {ID: 3, Name: strPtr("get"),
			Inner: json.RawMessage(`{"function":{"sig":{"inputs":[["self",{"borrowed_ref":{"type":{"generic":"Self"}}}],["key",{"resolved_path":{"name":"Key","id":50}}]],"output":{"resolved_path":{"name":"Value","id":51}}},"generics":{"params":[]},"header":{}}}`)},
		// Impl block
		"10": {ID: 10, Inner: json.RawMessage(`{"impl":{"trait":null,"for":null,"items":[3]}}`)},
	}
	crate := makeCrateWithItems(items)
	crate.Paths["50"] = RustdocSummary{CrateID: 0, Path: []string{"mycrate", "Key"}, Kind: "struct"}
	crate.Paths["51"] = RustdocSummary{CrateID: 0, Path: []string{"mycrate", "Value"}, Kind: "struct"}

	item := &RustdocItem{
		ID:    0,
		Name:  strPtr("MyMap"),
		Inner: json.RawMessage(`{"struct":{"kind":{"plain":{"fields":[]}},"impls":[10]}}`),
	}

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")

	var implFrag *Fragment
	for i := range fragments {
		if fragments[i].Name == FragImplementations {
			implFrag = &fragments[i]
		}
	}
	if implFrag == nil {
		t.Fatal("expected implementations fragment")
	}

	if !strings.Contains(implFrag.Content, "## Types Used") {
		t.Errorf("expected Types Used section, got:\n%s", implFrag.Content)
	}
	if !strings.Contains(implFrag.Content, "rsdoc://mycrate/1.0.0/mycrate::Key") {
		t.Errorf("expected Key URI in Types Used, got:\n%s", implFrag.Content)
	}
	if !strings.Contains(implFrag.Content, "rsdoc://mycrate/1.0.0/mycrate::Value") {
		t.Errorf("expected Value URI in Types Used, got:\n%s", implFrag.Content)
	}
}

func TestGenerateFragments_TraitTypesUsed(t *testing.T) {
	t.Parallel()

	items := map[string]RustdocItem{
		// Required method: fn process(&self, input: Input) -> Output
		"1": {ID: 1, Name: strPtr("process"),
			Inner: json.RawMessage(`{"function":{"has_body":false,"sig":{"inputs":[["self",{"borrowed_ref":{"type":{"generic":"Self"}}}],["input",{"resolved_path":{"name":"Input","id":60}}]],"output":{"resolved_path":{"name":"Output","id":61}}},"generics":{"params":[]},"header":{}}}`)},
	}
	crate := makeCrateWithItems(items)
	crate.Paths["60"] = RustdocSummary{CrateID: 0, Path: []string{"mycrate", "Input"}, Kind: "struct"}
	crate.Paths["61"] = RustdocSummary{CrateID: 0, Path: []string{"mycrate", "Output"}, Kind: "struct"}

	item := &RustdocItem{
		ID:    0,
		Name:  strPtr("Processor"),
		Inner: json.RawMessage(`{"trait":{"items":[1],"implementations":[],"impls":[]}}`),
	}

	fragments := GenerateFragments(item, crate, "mycrate", "1.0.0")

	var reqFrag *Fragment
	for i := range fragments {
		if fragments[i].Name == FragRequiredMethods {
			reqFrag = &fragments[i]
		}
	}
	if reqFrag == nil {
		t.Fatal("expected required-methods fragment")
	}

	if !strings.Contains(reqFrag.Content, "## Types Used") {
		t.Errorf("expected Types Used section, got:\n%s", reqFrag.Content)
	}
	if !strings.Contains(reqFrag.Content, "rsdoc://mycrate/1.0.0/mycrate::Input") {
		t.Errorf("expected Input URI in Types Used, got:\n%s", reqFrag.Content)
	}
	if !strings.Contains(reqFrag.Content, "rsdoc://mycrate/1.0.0/mycrate::Output") {
		t.Errorf("expected Output URI in Types Used, got:\n%s", reqFrag.Content)
	}
}

func TestGenerateFragments_Module(t *testing.T) {
	t.Parallel()

	items := map[string]RustdocItem{
		// Module item (the module under test)
		"0": {ID: 0, Name: strPtr("mymod"),
			Inner: json.RawMessage(`{"module":{"items":[1,2,3,4,5,6]}}`)},
		// Child struct
		"1": {ID: 1, Name: strPtr("Foo"), Docs: strPtr("A foo struct"),
			Inner: json.RawMessage(`{"struct":{}}`)},
		// Child enum
		"2": {ID: 2, Name: strPtr("Bar"), Docs: strPtr("A bar enum"),
			Inner: json.RawMessage(`{"enum":{}}`)},
		// Child function
		"3": {ID: 3, Name: strPtr("baz"),
			Inner: json.RawMessage(`{"function":{}}`)},
		// Child impl (should be skipped)
		"4": {ID: 4, Inner: json.RawMessage(`{"impl":{}}`)},
		// Child use (should be skipped)
		"5": {ID: 5, Name: strPtr("reexport"),
			Inner: json.RawMessage(`{"use":{}}`)},
		// Child submodule
		"6": {ID: 6, Name: strPtr("sub"), Docs: strPtr("A submodule"),
			Inner: json.RawMessage(`{"module":{"items":[]}}`)},
	}
	crate := &RustdocCrate{
		Index:          items,
		ExternalCrates: map[string]ExternalCrate{},
		Paths: map[string]RustdocSummary{
			"1": {CrateID: 0, Path: []string{"mycrate", "mymod", "Foo"}, Kind: "struct"},
			"2": {CrateID: 0, Path: []string{"mycrate", "mymod", "Bar"}, Kind: "enum"},
			"3": {CrateID: 0, Path: []string{"mycrate", "mymod", "baz"}, Kind: "function"},
			"6": {CrateID: 0, Path: []string{"mycrate", "mymod", "sub"}, Kind: "module"},
		},
	}

	item := items["0"]
	fragments := GenerateFragments(&item, crate, "mycrate", "1.0.0")

	fragsByName := map[string]string{}
	for _, f := range fragments {
		fragsByName[f.Name] = f.Content
	}

	// Should have modules, structs, enums, functions
	if _, ok := fragsByName["modules"]; !ok {
		t.Error("expected modules fragment")
	}
	if _, ok := fragsByName["structs"]; !ok {
		t.Error("expected structs fragment")
	}
	if _, ok := fragsByName["enums"]; !ok {
		t.Error("expected enums fragment")
	}
	if _, ok := fragsByName["functions"]; !ok {
		t.Error("expected functions fragment")
	}

	// Should NOT have impl or use fragments
	if len(fragments) != 4 {
		t.Errorf("expected 4 fragments, got %d: %v", len(fragments), fragsByName)
	}

	// Check content format: links with rsdoc URIs
	structs := fragsByName["structs"]
	if !strings.Contains(structs, "[Foo](rsdoc://mycrate/1.0.0/mycrate::mymod::Foo)") {
		t.Errorf("structs fragment missing Foo link: %s", structs)
	}
	if !strings.Contains(structs, ": A foo struct") {
		t.Errorf("structs fragment missing Foo docs: %s", structs)
	}

	// Check ordering: modules should come before structs
	var moduleIdx, structIdx int
	for i, f := range fragments {
		if f.Name == "modules" {
			moduleIdx = i
		}
		if f.Name == "structs" {
			structIdx = i
		}
	}
	if moduleIdx > structIdx {
		t.Errorf("modules fragment should come before structs")
	}
}

func TestGenerateFragments_Module_ResolvesUseItems(t *testing.T) {
	t.Parallel()

	// Root module re-exports items via `pub use`.
	items := map[string]RustdocItem{
		"0": {ID: 0, Name: strPtr("mycrate"),
			Inner: json.RawMessage(`{"module":{"items":[1,2]}}`)},
		// use item: pub use internal::Foo;
		"1": {ID: 1, Name: strPtr("Foo"),
			Inner: json.RawMessage(`{"use":{"id":10,"name":"Foo","is_glob":false}}`)},
		// use item: pub use internal::bar;
		"2": {ID: 2, Name: strPtr("bar"),
			Inner: json.RawMessage(`{"use":{"id":11,"name":"bar","is_glob":false}}`)},
		// Target items (in submodule)
		"10": {ID: 10, Name: strPtr("Foo"), Docs: strPtr("A foo struct"),
			Inner: json.RawMessage(`{"struct":{}}`)},
		"11": {ID: 11, Name: strPtr("bar"), Docs: strPtr("A bar function"),
			Inner: json.RawMessage(`{"function":{}}`)},
	}
	crate := &RustdocCrate{
		Index:          items,
		ExternalCrates: map[string]ExternalCrate{},
		Paths: map[string]RustdocSummary{
			"0":  {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
			"10": {CrateID: 0, Path: []string{"mycrate", "Foo"}, Kind: "struct"},
			"11": {CrateID: 0, Path: []string{"mycrate", "bar"}, Kind: "function"},
		},
	}

	item := items["0"]
	fragments := GenerateFragments(&item, crate, "mycrate", "1.0.0")

	fragsByName := map[string]string{}
	for _, f := range fragments {
		fragsByName[f.Name] = f.Content
	}

	if _, ok := fragsByName["structs"]; !ok {
		t.Error("expected structs fragment from resolved use item")
	}
	if _, ok := fragsByName["functions"]; !ok {
		t.Error("expected functions fragment from resolved use item")
	}
	// URI is built from module path + use name (local path)
	if !strings.Contains(fragsByName["structs"], "[Foo](rsdoc://mycrate/1.0.0/mycrate::Foo)") {
		t.Errorf("expected Foo link in structs: %s", fragsByName["structs"])
	}
	if !strings.Contains(fragsByName["structs"], ": A foo struct") {
		t.Errorf("expected Foo docs in structs: %s", fragsByName["structs"])
	}
}

func TestGenerateFragments_Module_ResolvesExternalUseItems(t *testing.T) {
	t.Parallel()

	// Root module re-exports an item from an external crate (not in Index).
	items := map[string]RustdocItem{
		"2": {ID: 2, Name: strPtr("mycrate"),
			Inner: json.RawMessage(`{"module":{"items":[0]}}`)},
		// use item: pub use dep_macro::my_macro; (Name is nil on the index entry)
		"0": {ID: 0,
			Inner: json.RawMessage(`{"use":{"source":"dep_macro::my_macro","name":"my_macro","id":1,"is_glob":false}}`)},
		// Target item 1 is NOT in index (external crate item)
	}
	crate := &RustdocCrate{
		Index:          items,
		ExternalCrates: map[string]ExternalCrate{"20": {Name: "dep_macro"}},
		Paths: map[string]RustdocSummary{
			"2": {CrateID: 0, Path: []string{"mycrate"}, Kind: "module"},
			"1": {CrateID: 20, Path: []string{"dep_macro", "my_macro"}, Kind: "proc_attribute"},
		},
	}

	item := items["2"]
	fragments := GenerateFragments(&item, crate, "mycrate", "1.0.0")

	if len(fragments) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(fragments))
	}
	f := fragments[0]
	if f.Name != "attribute-macros" {
		t.Errorf("expected attribute-macros fragment, got %s", f.Name)
	}
	// URI should use local path, not external crate path
	if !strings.Contains(f.Content, "[my_macro](rsdoc://mycrate/1.0.0/mycrate::my_macro)") {
		t.Errorf("expected local URI for re-exported macro: %s", f.Content)
	}
	// Should indicate source with a link
	if !strings.Contains(f.Content, "(from [dep_macro::my_macro](rsdoc://dep_macro/latest/dep_macro::my_macro))") {
		t.Errorf("expected source annotation: %s", f.Content)
	}
}

func TestGenerateFragments_Module_SkipsExternal(t *testing.T) {
	t.Parallel()

	items := map[string]RustdocItem{
		"0": {ID: 0, Name: strPtr("mymod"),
			Inner: json.RawMessage(`{"module":{"items":[1,2]}}`)},
		// Local item
		"1": {ID: 1, Name: strPtr("Local"),
			Inner: json.RawMessage(`{"struct":{}}`)},
		// External item (CrateID != 0)
		"2": {ID: 2, Name: strPtr("External"),
			Inner: json.RawMessage(`{"struct":{}}`)},
	}
	crate := &RustdocCrate{
		Index:          items,
		ExternalCrates: map[string]ExternalCrate{},
		Paths: map[string]RustdocSummary{
			"1": {CrateID: 0, Path: []string{"mycrate", "mymod", "Local"}, Kind: "struct"},
			"2": {CrateID: 5, Path: []string{"othercrate", "External"}, Kind: "struct"},
		},
	}

	item := items["0"]
	fragments := GenerateFragments(&item, crate, "mycrate", "1.0.0")

	if len(fragments) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(fragments))
	}
	if !strings.Contains(fragments[0].Content, "Local") {
		t.Errorf("expected Local in fragment: %s", fragments[0].Content)
	}
	if strings.Contains(fragments[0].Content, "External") {
		t.Errorf("external item should be excluded: %s", fragments[0].Content)
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
