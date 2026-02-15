package docs

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// resolveTypeName extracts a type name from a rustdoc Type JSON, formatted as a
// markdown link if the type can be resolved to an rsdoc:// URI.
func resolveTypeName(typeJSON json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(typeJSON, &outer); err != nil {
		return ""
	}

	if resolved, ok := outer["resolved_path"]; ok {
		return formatResolvedPath(resolved, crate, crateName, version)
	}

	if prim, ok := outer["primitive"]; ok {
		var name string
		if err := json.Unmarshal(prim, &name); err == nil {
			return name
		}
	}

	if dt, ok := outer["dyn_trait"]; ok {
		return formatDynTrait(dt, crate, crateName, version)
	}

	if br, ok := outer["borrowed_ref"]; ok {
		return formatBorrowedRef(br, crate, crateName, version)
	}

	if sl, ok := outer["slice"]; ok {
		inner := resolveTypeName(sl, crate, crateName, version)
		if inner != "" {
			return "[" + inner + "]"
		}
	}

	if g, ok := outer["generic"]; ok {
		var name string
		if err := json.Unmarshal(g, &name); err == nil {
			return name
		}
	}

	if qp, ok := outer["qualified_path"]; ok {
		return formatQualifiedPath(qp, crate, crateName, version)
	}

	if tp, ok := outer["tuple"]; ok {
		return formatTuple(tp, crate, crateName, version)
	}

	return ""
}

func formatResolvedPath(resolved json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var rp struct {
		Name string           `json:"name"`
		ID   int              `json:"id"`
		Args *json.RawMessage `json:"args"`
	}
	if err := json.Unmarshal(resolved, &rp); err != nil {
		return ""
	}

	// Name can be empty in rustdoc JSON — fall back to paths lookup
	name := rp.Name
	if name == "" {
		if summary, ok := crate.Paths[strconv.Itoa(rp.ID)]; ok && len(summary.Path) > 0 {
			name = summary.Path[len(summary.Path)-1]
		}
	}
	if name == "" {
		return ""
	}

	// Build base name (with optional link)
	base := name
	if uri := ResolveItemURI(rp.ID, crate, crateName, version); uri != "" {
		base = fmt.Sprintf("[%s](%s)", name, uri)
	}

	// Append generic args
	if rp.Args != nil {
		args := formatGenericArgs(*rp.Args, crate, crateName, version)
		if args != "" {
			base += args
		}
	}

	return base
}

func formatGenericArgs(argsJSON json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var args struct {
		AngleBracketed *struct {
			Args []json.RawMessage `json:"args"`
		} `json:"angle_bracketed"`
	}
	if err := json.Unmarshal(argsJSON, &args); err != nil || args.AngleBracketed == nil {
		return ""
	}

	var parts []string
	for _, arg := range args.AngleBracketed.Args {
		var a map[string]json.RawMessage
		if err := json.Unmarshal(arg, &a); err != nil {
			continue
		}
		if typeData, ok := a["type"]; ok {
			if t := resolveTypeName(typeData, crate, crateName, version); t != "" {
				parts = append(parts, t)
			}
		} else if lifetime, ok := a["lifetime"]; ok {
			var lt string
			if json.Unmarshal(lifetime, &lt) == nil {
				parts = append(parts, lt)
			}
		}
	}

	if len(parts) == 0 {
		return ""
	}
	return "\\<" + strings.Join(parts, ", ") + ">"
}

func formatDynTrait(dt json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var d struct {
		Traits []struct {
			Trait struct {
				Name string `json:"name"`
				Path string `json:"path"`
				ID   int    `json:"id"`
			} `json:"trait"`
		} `json:"traits"`
		Lifetime *string `json:"lifetime"`
	}
	if err := json.Unmarshal(dt, &d); err != nil || len(d.Traits) == 0 {
		return ""
	}

	parts := make([]string, 0, len(d.Traits)+1)
	for _, t := range d.Traits {
		name := t.Trait.Name
		if name == "" {
			name = t.Trait.Path
		}
		if uri := ResolveItemURI(t.Trait.ID, crate, crateName, version); uri != "" {
			parts = append(parts, fmt.Sprintf("[%s](%s)", name, uri))
		} else {
			parts = append(parts, name)
		}
	}
	if d.Lifetime != nil && *d.Lifetime != "" {
		parts = append(parts, *d.Lifetime)
	}

	return "dyn " + strings.Join(parts, " + ")
}

func formatBorrowedRef(br json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var r struct {
		Lifetime  *string         `json:"lifetime"`
		IsMutable bool            `json:"is_mutable"`
		Type      json.RawMessage `json:"type"`
	}
	if err := json.Unmarshal(br, &r); err != nil {
		return ""
	}

	inner := resolveTypeName(r.Type, crate, crateName, version)
	if inner == "" {
		return ""
	}

	prefix := "&"
	if r.Lifetime != nil && *r.Lifetime != "" {
		prefix += *r.Lifetime + " "
	}
	if r.IsMutable {
		prefix += "mut "
	}
	return prefix + inner
}

func formatQualifiedPath(qp json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var q struct {
		Name     string          `json:"name"`
		SelfType json.RawMessage `json:"self_type"`
		Trait    *struct {
			Name string `json:"name"`
			ID   int    `json:"id"`
		} `json:"trait"`
	}
	if err := json.Unmarshal(qp, &q); err != nil {
		return ""
	}
	selfType := resolveTypeName(q.SelfType, crate, crateName, version)
	if selfType == "" {
		return ""
	}
	if q.Trait != nil && q.Trait.Name != "" {
		return fmt.Sprintf("<%s as %s>::%s", selfType, q.Trait.Name, q.Name)
	}
	return fmt.Sprintf("%s::%s", selfType, q.Name)
}

func formatTuple(tp json.RawMessage, crate *RustdocCrate, crateName, version string) string {
	var types []json.RawMessage
	if err := json.Unmarshal(tp, &types); err != nil {
		return ""
	}
	var parts []string
	for _, t := range types {
		if name := resolveTypeName(t, crate, crateName, version); name != "" {
			parts = append(parts, name)
		}
	}
	return "(" + strings.Join(parts, ", ") + ")"
}
