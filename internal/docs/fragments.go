package docs

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	FragFields          = "fields"
	FragVariants        = "variants"
	FragImplementations = "implementations"
	FragImplementors    = "implementors"
	FragRequiredMethods = "required-methods"
	FragProvidedMethods = "provided-methods"
)

// GenerateFragments creates sub-documents for an item based on its kind.
// Fragment names match docs.rs sections: #fields, #variants, #implementors,
// #required-methods, #provided-methods, #implementations.
func GenerateFragments(item *RustdocItem, crate *RustdocCrate, crateName, version string) []Fragment {
	kind := innerKind(item.Inner)
	switch kind {
	case "struct":
		return generateStructFragments(item, crate, crateName, version)
	case "enum":
		return generateEnumFragments(item, crate, crateName, version)
	case "trait":
		return generateTraitFragments(item, crate, crateName, version)
	default:
		return nil
	}
}

func generateStructFragments(item *RustdocItem, crate *RustdocCrate, crateName, version string) []Fragment {
	inner := unwrapInner(item.Inner, "struct")
	if inner == nil {
		return nil
	}

	var fragments []Fragment

	if f := fieldsFragment(inner, crate); f != nil {
		fragments = append(fragments, *f)
	}
	if f := implsFragment(inner, crate, item, crateName, version); f != nil {
		fragments = append(fragments, *f)
	}

	return fragments
}

func generateEnumFragments(item *RustdocItem, crate *RustdocCrate, crateName, version string) []Fragment {
	inner := unwrapInner(item.Inner, "enum")
	if inner == nil {
		return nil
	}

	var fragments []Fragment

	if f := variantsFragment(inner, crate); f != nil {
		fragments = append(fragments, *f)
	}
	if f := implsFragment(inner, crate, item, crateName, version); f != nil {
		fragments = append(fragments, *f)
	}

	return fragments
}

func generateTraitFragments(item *RustdocItem, crate *RustdocCrate, crateName, version string) []Fragment {
	inner := unwrapInner(item.Inner, "trait")
	if inner == nil {
		return nil
	}

	var fragments []Fragment

	frags := traitMethodFragments(inner, crate, crateName, version)
	fragments = append(fragments, frags...)

	if f := traitImplementorsFragment(inner, crate, crateName, version); f != nil {
		fragments = append(fragments, *f)
	}
	if f := implsFragment(inner, crate, item, crateName, version); f != nil {
		fragments = append(fragments, *f)
	}

	return fragments
}

// fieldsFragment generates a #fields fragment for a struct.
func fieldsFragment(structData json.RawMessage, crate *RustdocCrate) *Fragment {
	var s struct {
		Kind json.RawMessage `json:"kind"`
	}
	if err := json.Unmarshal(structData, &s); err != nil {
		return nil
	}

	// StructKind::Plain has fields; Tuple and Unit don't have named fields worth listing.
	var kind map[string]json.RawMessage
	if err := json.Unmarshal(s.Kind, &kind); err != nil {
		return nil
	}

	plainData, ok := kind["plain"]
	if !ok {
		return nil
	}

	var plain struct {
		Fields []int `json:"fields"`
	}
	if err := json.Unmarshal(plainData, &plain); err != nil || len(plain.Fields) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("# Fields\n\n")
	for _, fieldID := range plain.Fields {
		fieldItem, ok := crate.Index[strconv.Itoa(fieldID)]
		if !ok {
			continue
		}
		name := "<unnamed>"
		if fieldItem.Name != nil {
			name = *fieldItem.Name
		}
		b.WriteString(fmt.Sprintf("- **%s**", name))
		if fieldItem.Docs != nil && *fieldItem.Docs != "" {
			first := strings.SplitN(*fieldItem.Docs, "\n", 2)[0]
			b.WriteString(": " + first)
		}
		b.WriteString("\n")
	}

	content := b.String()
	if content == "# Fields\n\n" {
		return nil
	}
	return &Fragment{Name: FragFields, Content: content}
}

// variantsFragment generates a #variants fragment for an enum.
func variantsFragment(enumData json.RawMessage, crate *RustdocCrate) *Fragment {
	var e struct {
		Variants []int `json:"variants"`
	}
	if err := json.Unmarshal(enumData, &e); err != nil || len(e.Variants) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("# Variants\n\n")
	for _, variantID := range e.Variants {
		variantItem, ok := crate.Index[strconv.Itoa(variantID)]
		if !ok {
			continue
		}
		name := "<unnamed>"
		if variantItem.Name != nil {
			name = *variantItem.Name
		}
		b.WriteString(fmt.Sprintf("- **%s**", name))
		if variantItem.Docs != nil && *variantItem.Docs != "" {
			first := strings.SplitN(*variantItem.Docs, "\n", 2)[0]
			b.WriteString(": " + first)
		}
		b.WriteString("\n")
	}

	content := b.String()
	if content == "# Variants\n\n" {
		return nil
	}
	return &Fragment{Name: FragVariants, Content: content}
}

// implsFragment generates a #implementations fragment listing methods from impl blocks.
func implsFragment(typeData json.RawMessage, crate *RustdocCrate, item *RustdocItem, crateName, version string) *Fragment {
	var t struct {
		Impls []int `json:"impls"`
	}
	if err := json.Unmarshal(typeData, &t); err != nil || len(t.Impls) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("# Implementations\n\n")
	count := 0
	var allURIs []string

	for _, implID := range t.Impls {
		implItem, ok := crate.Index[strconv.Itoa(implID)]
		if !ok {
			continue
		}
		implInner := unwrapInner(implItem.Inner, "impl")
		if implInner == nil {
			continue
		}

		var impl struct {
			Trait *json.RawMessage `json:"trait"`
			Items []int            `json:"items"`
		}
		if err := json.Unmarshal(implInner, &impl); err != nil {
			continue
		}

		// Group header: "impl Type" or "impl Trait for Type"
		header := "impl"
		if impl.Trait != nil {
			var traitPath struct {
				Name string `json:"name"`
				ID   int    `json:"id"`
			}
			if err := json.Unmarshal(*impl.Trait, &traitPath); err == nil && traitPath.Name != "" {
				if uri := ResolveItemURI(traitPath.ID, crate, crateName, version); uri != "" {
					header = fmt.Sprintf("impl [%s](%s)", traitPath.Name, uri)
					allURIs = append(allURIs, uri)
				} else {
					header = fmt.Sprintf("impl %s", traitPath.Name)
				}
			}
		}

		methods := listMethodSummaries(impl.Items, crate, crateName, version)
		if len(methods) == 0 {
			continue
		}

		b.WriteString(fmt.Sprintf("## %s\n\n", header))
		for _, m := range methods {
			display := m.name
			if m.sig != "" {
				display = m.sig
			}
			b.WriteString(fmt.Sprintf("- `%s`", display))
			if m.docs != "" {
				b.WriteString(": " + m.docs)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		count++

		for _, id := range impl.Items {
			methodItem, ok := crate.Index[strconv.Itoa(id)]
			if !ok {
				continue
			}
			if fnData := unwrapInner(methodItem.Inner, "function"); fnData != nil {
				allURIs = append(allURIs, collectFnURIs(fnData, crate, crateName, version)...)
			}
		}
	}

	if count == 0 {
		return nil
	}
	appendTypesUsed(&b, allURIs)
	return &Fragment{Name: FragImplementations, Content: b.String()}
}

// traitImplementorsFragment generates a #implementors fragment listing types that implement this trait.
func traitImplementorsFragment(traitData json.RawMessage, crate *RustdocCrate, crateName, version string) *Fragment {
	var t struct {
		Implementations []int `json:"implementations"`
	}
	if err := json.Unmarshal(traitData, &t); err != nil || len(t.Implementations) == 0 {
		return nil
	}

	var b strings.Builder
	b.WriteString("# Implementors\n\n")
	count := 0

	for _, implID := range t.Implementations {
		implItem, ok := crate.Index[strconv.Itoa(implID)]
		if !ok {
			continue
		}
		implInner := unwrapInner(implItem.Inner, "impl")
		if implInner == nil {
			continue
		}

		var impl struct {
			For json.RawMessage `json:"for"`
		}
		if err := json.Unmarshal(implInner, &impl); err != nil {
			continue
		}

		typeName := resolveTypeName(impl.For, crate, crateName, version)
		if typeName == "" {
			continue
		}

		b.WriteString(fmt.Sprintf("- %s\n", typeName))
		count++
	}

	if count == 0 {
		return nil
	}
	return &Fragment{Name: FragImplementors, Content: b.String()}
}

// traitMethodFragments generates #required-methods and/or #provided-methods fragments.
func traitMethodFragments(traitData json.RawMessage, crate *RustdocCrate, crateName, version string) []Fragment {
	var t struct {
		Items []int `json:"items"`
	}
	if err := json.Unmarshal(traitData, &t); err != nil || len(t.Items) == 0 {
		return nil
	}

	var required, provided []traitMethodInfo
	var requiredURIs, providedURIs []string
	for _, id := range t.Items {
		item, ok := crate.Index[strconv.Itoa(id)]
		if !ok || item.Name == nil {
			continue
		}

		m := traitMethodInfo{name: *item.Name}
		if item.Docs != nil {
			m.docs = *item.Docs
		}

		fnData := unwrapInner(item.Inner, "function")
		if fnData != nil {
			var fn struct {
				HasBody bool `json:"has_body"`
			}
			if err := json.Unmarshal(fnData, &fn); err != nil {
				continue
			}
			m.sig = renderFnSig(*item.Name, fnData, crate, crateName, version)
			uris := collectFnURIs(fnData, crate, crateName, version)
			if fn.HasBody {
				provided = append(provided, m)
				providedURIs = append(providedURIs, uris...)
			} else {
				required = append(required, m)
				requiredURIs = append(requiredURIs, uris...)
			}
		} else {
			// Associated types, constants — put with required
			m.sig = extractAssociatedItemSig(&item)
			required = append(required, m)
		}
	}

	var fragments []Fragment

	if len(required) > 0 {
		var b strings.Builder
		b.WriteString("# Required Methods\n\n")
		writeTraitMethods(&b, required)
		appendTypesUsed(&b, requiredURIs)
		fragments = append(fragments, Fragment{Name: FragRequiredMethods, Content: b.String()})
	}

	if len(provided) > 0 {
		var b strings.Builder
		b.WriteString("# Provided Methods\n\n")
		writeTraitMethods(&b, provided)
		appendTypesUsed(&b, providedURIs)
		fragments = append(fragments, Fragment{Name: FragProvidedMethods, Content: b.String()})
	}

	return fragments
}

type traitMethodInfo struct {
	name string
	sig  string
	docs string
}

func writeTraitMethods(b *strings.Builder, methods []traitMethodInfo) {
	for _, m := range methods {
		b.WriteString(fmt.Sprintf("## %s\n\n", m.name))
		if m.sig != "" {
			b.WriteString(fmt.Sprintf("```rust\n%s\n```\n\n", m.sig))
		}
		if m.docs != "" {
			b.WriteString(m.docs)
			b.WriteString("\n\n")
		}
	}
}

// listMethodSummaries returns brief method info (signature + first line of docs) for impl block listings.
func listMethodSummaries(itemIDs []int, crate *RustdocCrate, crateName, version string) []methodSummary {
	var methods []methodSummary
	for _, id := range itemIDs {
		methodItem, ok := crate.Index[strconv.Itoa(id)]
		if !ok || methodItem.Name == nil {
			continue
		}
		docs := ""
		if methodItem.Docs != nil && *methodItem.Docs != "" {
			docs = strings.SplitN(*methodItem.Docs, "\n", 2)[0]
		}
		sig := ""
		if fnData := unwrapInner(methodItem.Inner, "function"); fnData != nil {
			sig = renderFnSig(*methodItem.Name, fnData, crate, crateName, version)
		}
		methods = append(methods, methodSummary{name: *methodItem.Name, sig: sig, docs: docs})
	}
	return methods
}

type methodSummary struct {
	name string
	sig  string
	docs string
}

func extractAssociatedItemSig(item *RustdocItem) string {
	if data := unwrapInner(item.Inner, "type_alias"); data != nil {
		return "type " + *item.Name
	}
	if data := unwrapInner(item.Inner, "constant"); data != nil {
		return "const " + *item.Name
	}
	return ""
}

// unwrapInner extracts the inner data for a given kind from a rustdoc Item's Inner field.
// Inner is shaped like {"struct": {...}} or {"enum": {...}}.
func unwrapInner(inner json.RawMessage, kind string) json.RawMessage {
	if len(inner) == 0 {
		return nil
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(inner, &outer); err != nil {
		return nil
	}
	data, ok := outer[kind]
	if !ok {
		return nil
	}
	return data
}

// rsdocLinkRe matches markdown links with rsdoc:// URIs, capturing the URI.
var rsdocLinkRe = regexp.MustCompile(`\]\((rsdoc://[^)]+)\)`)

// extractRsdocURIs extracts rsdoc:// URIs from markdown link syntax.
func extractRsdocURIs(markdown string) []string {
	matches := rsdocLinkRe.FindAllStringSubmatch(markdown, -1)
	uris := make([]string, 0, len(matches))
	for _, m := range matches {
		uris = append(uris, m[1])
	}
	return uris
}

// collectFnURIs extracts rsdoc:// URIs from a function's parameter and return types.
func collectFnURIs(fnData json.RawMessage, crate *RustdocCrate, crateName, version string) []string {
	var fn struct {
		Sig struct {
			Inputs []json.RawMessage `json:"inputs"`
			Output json.RawMessage   `json:"output"`
		} `json:"sig"`
	}
	if err := json.Unmarshal(fnData, &fn); err != nil {
		return nil
	}

	var uris []string
	for _, input := range fn.Sig.Inputs {
		var pair []json.RawMessage
		if err := json.Unmarshal(input, &pair); err != nil || len(pair) < 2 {
			continue
		}
		var paramName string
		json.Unmarshal(pair[0], &paramName)
		if paramName == "self" {
			continue
		}
		uris = append(uris, extractRsdocURIs(resolveTypeName(pair[1], crate, crateName, version))...)
	}
	if fn.Sig.Output != nil {
		uris = append(uris, extractRsdocURIs(resolveTypeName(fn.Sig.Output, crate, crateName, version))...)
	}
	return uris
}

// appendTypesUsed appends a "## Types Used" section with deduplicated bare rsdoc:// URIs.
func appendTypesUsed(b *strings.Builder, uris []string) {
	seen := make(map[string]bool)
	var unique []string
	for _, uri := range uris {
		if !seen[uri] {
			seen[uri] = true
			unique = append(unique, uri)
		}
	}
	if len(unique) == 0 {
		return
	}
	b.WriteString("## Types Used\n\n")
	for _, uri := range unique {
		b.WriteString("- ")
		b.WriteString(uri)
		b.WriteString("\n")
	}
}
