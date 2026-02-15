package markdown

import (
	"fmt"
	"sort"
	"strings"

	gm "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	gmparser "github.com/gomarkdown/markdown/parser"
)

// RewriteLinks rewrites markdown link destinations using the provided link map.
// It parses the markdown to AST to find all link destinations, then performs
// targeted string replacements to preserve original formatting.
func RewriteLinks(src string, linkMap map[string]string) string {
	if len(linkMap) == 0 {
		return src
	}

	doc := gm.Parse([]byte(src), gmparser.NewWithExtensions(
		gmparser.CommonExtensions|gmparser.Autolink,
	))

	// Collect unique destinations that need replacement
	seen := make(map[string]bool)
	type replacement struct {
		oldDest string
		newDest string
	}
	var replacements []replacement

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		if link, ok := node.(*ast.Link); ok {
			dest := string(link.Destination)
			if newDest, ok := linkMap[dest]; ok && !seen[dest] {
				seen[dest] = true
				replacements = append(replacements, replacement{dest, newDest})
			}
		}
		return ast.GoToNext
	})

	if len(replacements) == 0 {
		return src
	}

	result := src

	// Inline links: [text](destination) — one pass per replacement
	for _, r := range replacements {
		result = strings.ReplaceAll(result, "]("+r.oldDest+")", "]("+r.newDest+")")
	}

	// Reference-style definitions: [ref]: destination — single pass over lines
	refMap := make(map[string]string, len(replacements))
	for _, r := range replacements {
		refMap["]: "+r.oldDest] = "]: " + r.newDest
	}
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for oldSuffix, newSuffix := range refMap {
			if strings.HasSuffix(trimmed, oldSuffix) {
				lines[i] = strings.Replace(line, oldSuffix, newSuffix, 1)
				break
			}
		}
	}
	result = strings.Join(lines, "\n")

	return result
}

// AddFrontMatter prepends a YAML front-matter block listing fragment URIs.
func AddFrontMatter(src string, fragments map[string]string) string {
	if len(fragments) == 0 {
		return src
	}

	keys := make([]string, 0, len(fragments))
	for k := range fragments {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("---\n")
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("%s: %s\n", k, fragments[k]))
	}
	b.WriteString("---\n\n")
	b.WriteString(src)
	return b.String()
}
