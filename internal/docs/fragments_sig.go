package docs

import (
	"encoding/json"
	"regexp"
	"strings"
)

// renderFnSig builds a plain-text Rust function signature from structured rustdoc JSON.
// Example output: "fn record_debug(&mut self, field: &Field, value: &dyn Debug)"
func renderFnSig(name string, fnData json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var fn struct {
		Sig struct {
			Inputs      []json.RawMessage `json:"inputs"`
			Output      json.RawMessage   `json:"output"`
			IsCVariadic bool              `json:"is_c_variadic"`
		} `json:"sig"`
		Generics struct {
			Params []struct {
				Name string          `json:"name"`
				Kind json.RawMessage `json:"kind"`
			} `json:"params"`
			WherePredicates []json.RawMessage `json:"where_predicates"`
		} `json:"generics"`
		Header struct {
			IsConst  bool `json:"is_const"`
			IsUnsafe bool `json:"is_unsafe"`
			IsAsync  bool `json:"is_async"`
		} `json:"header"`
	}
	if err := json.Unmarshal(fnData, &fn); err != nil {
		return ""
	}

	var b strings.Builder

	// Header qualifiers
	if fn.Header.IsConst {
		b.WriteString("const ")
	}
	if fn.Header.IsUnsafe {
		b.WriteString("unsafe ")
	}
	if fn.Header.IsAsync {
		b.WriteString("async ")
	}

	b.WriteString("fn ")
	b.WriteString(name)

	// Generic params
	var genericNames []string
	for _, p := range fn.Generics.Params {
		if p.Name != "" {
			genericNames = append(genericNames, p.Name)
		}
	}
	if len(genericNames) > 0 {
		b.WriteString("<")
		b.WriteString(strings.Join(genericNames, ", "))
		b.WriteString(">")
	}

	// Parameters
	b.WriteString("(")
	var params []string
	for _, input := range fn.Sig.Inputs {
		var pair []json.RawMessage
		if err := json.Unmarshal(input, &pair); err != nil || len(pair) < 2 {
			continue
		}
		var paramName string
		json.Unmarshal(pair[0], &paramName)

		typeStr := plainType(resolveTypeName(pair[1], crate, crateName, version))

		// Render self params with Rust shorthand
		if paramName == "self" {
			params = append(params, selfShorthand(pair[1]))
		} else {
			params = append(params, paramName+": "+typeStr)
		}
	}
	b.WriteString(strings.Join(params, ", "))
	b.WriteString(")")

	// Return type
	if fn.Sig.Output != nil {
		var null interface{}
		json.Unmarshal(fn.Sig.Output, &null)
		if null != nil {
			retType := plainType(resolveTypeName(fn.Sig.Output, crate, crateName, version))
			if retType != "" {
				b.WriteString(" -> ")
				b.WriteString(retType)
			}
		}
	}

	return b.String()
}

// selfShorthand converts a rustdoc self-parameter type to Rust shorthand.
// {"generic": "Self"} → "self", {"borrowed_ref": {is_mutable: false, type: {generic: Self}}} → "&self", etc.
func selfShorthand(typeJSON json.RawMessage) string {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(typeJSON, &outer); err != nil {
		return "self"
	}
	if _, ok := outer["generic"]; ok {
		return "self"
	}
	if br, ok := outer["borrowed_ref"]; ok {
		var r struct {
			Lifetime  *string `json:"lifetime"`
			IsMutable bool    `json:"is_mutable"`
		}
		json.Unmarshal(br, &r)
		prefix := "&"
		if r.Lifetime != nil && *r.Lifetime != "" {
			prefix += *r.Lifetime + " "
		}
		if r.IsMutable {
			prefix += "mut "
		}
		return prefix + "self"
	}
	return "self"
}

// mdLinkRe matches markdown links like [Text](url).
var mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)

// plainType strips markdown formatting from a type string, converting it to plain Rust syntax.
func plainType(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, `\<`, `<`)
	return s
}
