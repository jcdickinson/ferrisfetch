package embeddings

import (
	"strings"

	gm "github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	gmparser "github.com/gomarkdown/markdown/parser"
)

type Chunk struct {
	Text  string
	Index int
}

// ChunkSections splits markdown into semantically meaningful chunks using
// AST-based heading detection. Each chunk gets the preamble prepended so
// every chunk carries the item's identity (path + signature).
//
// Additionally:
// - The first paragraph (summary line) is emitted as a standalone chunk
//   for double representation in vector space.
// - Fenced code blocks >= 80 chars are extracted as standalone chunks.
//
// No max size enforcement — Voyage.ai truncates if needed.
func ChunkSections(preamble, markdown string) []Chunk {
	markdown = strings.TrimSpace(markdown)
	if markdown == "" {
		return []Chunk{{Text: preamble, Index: 0}}
	}

	doc := gm.Parse([]byte(markdown), gmparser.NewWithExtensions(
		gmparser.CommonExtensions|gmparser.Autolink,
	))

	// Walk top-level children to find section boundaries and extract code blocks.
	sections, summary, codeBlocks := splitSections(doc, []byte(markdown))

	var chunks []Chunk
	idx := 0

	// Summary chunk (first paragraph before any heading, if doc has more content)
	if summary != "" && len(sections) > 1 {
		chunks = append(chunks, Chunk{Text: preamble + "\n\n" + summary, Index: idx})
		idx++
	}

	// Section chunks
	for _, sec := range sections {
		text := strings.TrimSpace(sec)
		if text == "" {
			continue
		}
		chunks = append(chunks, Chunk{Text: preamble + "\n\n" + text, Index: idx})
		idx++
	}

	// Code block chunks
	for _, code := range codeBlocks {
		chunks = append(chunks, Chunk{Text: preamble + "\n\n```\n" + code + "\n```", Index: idx})
		idx++
	}

	if len(chunks) == 0 {
		chunks = append(chunks, Chunk{Text: preamble, Index: 0})
	}

	return chunks
}

// splitSections walks the AST and splits text into heading-delimited sections.
// Returns the sections, an optional summary (first paragraph text), and
// extracted code blocks (>= 80 chars).
func splitSections(doc ast.Node, source []byte) (sections []string, summary string, codeBlocks []string) {
	children := doc.GetChildren()
	if len(children) == 0 {
		return []string{string(source)}, "", nil
	}

	var headingOffsets []int
	var firstParagraph *ast.Paragraph
	foundHeading := false

	for _, child := range children {
		switch n := child.(type) {
		case *ast.Heading:
			foundHeading = true
			offset := findHeadingOffset(source, n, headingOffsets)
			if offset >= 0 {
				headingOffsets = append(headingOffsets, offset)
			}
		case *ast.Paragraph:
			if !foundHeading && firstParagraph == nil {
				firstParagraph = n
			}
		}

		// Extract code blocks
		if cb, ok := child.(*ast.CodeBlock); ok {
			code := strings.TrimSpace(string(cb.Literal))
			if len(code) >= 80 {
				codeBlocks = append(codeBlocks, code)
			}
		}
	}

	// Extract summary from first paragraph
	if firstParagraph != nil {
		summary = extractNodeText(firstParagraph)
	}

	// Split source on heading offsets
	if len(headingOffsets) == 0 {
		return []string{string(source)}, summary, codeBlocks
	}

	src := string(source)
	for i, offset := range headingOffsets {
		if i == 0 && offset > 0 {
			// Content before first heading = intro section
			intro := strings.TrimSpace(src[:offset])
			if intro != "" {
				sections = append(sections, intro)
			}
		}
		end := len(src)
		if i+1 < len(headingOffsets) {
			end = headingOffsets[i+1]
		}
		sec := strings.TrimSpace(src[offset:end])
		if sec != "" {
			sections = append(sections, sec)
		}
	}

	// If first heading is at offset 0, there's no intro before it
	if headingOffsets[0] == 0 {
		return sections, summary, codeBlocks
	}

	return sections, summary, codeBlocks
}

// findHeadingOffset finds the byte offset in source where a heading starts.
// It searches for lines starting with '#' characters, skipping previously found offsets.
func findHeadingOffset(source []byte, heading *ast.Heading, found []int) int {
	src := string(source)
	prefix := strings.Repeat("#", heading.Level) + " "
	searchFrom := 0
	if len(found) > 0 {
		searchFrom = found[len(found)-1] + 1
	}

	for i := searchFrom; i < len(src); i++ {
		// Must be at line start
		if i > 0 && src[i-1] != '\n' {
			continue
		}
		if strings.HasPrefix(src[i:], prefix) {
			// Verify this offset isn't already found
			alreadyFound := false
			for _, f := range found {
				if f == i {
					alreadyFound = true
					break
				}
			}
			if !alreadyFound {
				return i
			}
		}
	}
	return -1
}

// extractNodeText recursively extracts text content from an AST node.
func extractNodeText(node ast.Node) string {
	var b strings.Builder
	ast.WalkFunc(node, func(n ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}
		if leaf := n.AsLeaf(); leaf != nil && leaf.Literal != nil {
			b.Write(leaf.Literal)
		}
		return ast.GoToNext
	})
	return strings.TrimSpace(b.String())
}
