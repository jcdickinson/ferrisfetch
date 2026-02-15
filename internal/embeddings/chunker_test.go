package embeddings

import (
	"strings"
	"testing"
)

func TestChunkSections_EmptyMarkdown(t *testing.T) {
	chunks := ChunkSections("serde::Serialize", "")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Text != "serde::Serialize" {
		t.Errorf("expected preamble only, got %q", chunks[0].Text)
	}
}

func TestChunkSections_SingleParagraph(t *testing.T) {
	chunks := ChunkSections("my_crate::Foo", "A simple struct for doing things.")
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if !strings.HasPrefix(chunks[0].Text, "my_crate::Foo\n\n") {
		t.Errorf("chunk should start with preamble, got %q", chunks[0].Text)
	}
	if !strings.Contains(chunks[0].Text, "A simple struct") {
		t.Errorf("chunk should contain the paragraph")
	}
}

func TestChunkSections_PreambleOnEveryChunk(t *testing.T) {
	md := `Summary line.

# Section One

Content of section one.

# Section Two

Content of section two.
`
	chunks := ChunkSections("tokio::spawn\npub fn spawn<F>(f: F)", md)
	if len(chunks) < 3 {
		t.Fatalf("expected at least 3 chunks (summary + 2 sections + intro), got %d", len(chunks))
	}
	for i, c := range chunks {
		if !strings.HasPrefix(c.Text, "tokio::spawn\npub fn spawn<F>(f: F)\n\n") {
			t.Errorf("chunk %d missing preamble: %q", i, c.Text[:min(80, len(c.Text))])
		}
	}
}

func TestChunkSections_SummaryExtraction(t *testing.T) {
	md := `This is the summary line.

Some more intro text.

# Details

The details section.
`
	chunks := ChunkSections("path", md)

	// First chunk should be the summary (double-represented)
	if !strings.Contains(chunks[0].Text, "This is the summary line.") {
		t.Errorf("first chunk should be summary, got %q", chunks[0].Text)
	}

	// There should be an intro section that also contains the summary
	foundIntro := false
	for _, c := range chunks {
		if strings.Contains(c.Text, "Some more intro text") {
			foundIntro = true
			if !strings.Contains(c.Text, "This is the summary line.") {
				t.Error("intro section should also contain the summary line")
			}
		}
	}
	if !foundIntro {
		t.Error("should have an intro section with the pre-heading content")
	}
}

func TestChunkSections_NoSummaryForSingleSection(t *testing.T) {
	md := "Just one paragraph with no headings."
	chunks := ChunkSections("path", md)
	// Single section = no separate summary chunk (would be redundant)
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk for single section, got %d", len(chunks))
	}
}

func TestChunkSections_CodeBlockExtraction(t *testing.T) {
	longCode := strings.Repeat("let x = foo();\n", 10) // > 80 chars
	md := "Some text.\n\n```rust\n" + longCode + "```\n"
	chunks := ChunkSections("path", md)

	foundCodeChunk := false
	for _, c := range chunks {
		if strings.Contains(c.Text, "```\n"+strings.TrimSpace(longCode)+"\n```") {
			foundCodeChunk = true
			if !strings.HasPrefix(c.Text, "path\n\n") {
				t.Error("code block chunk should have preamble")
			}
		}
	}
	if !foundCodeChunk {
		t.Errorf("should extract code block as standalone chunk, got chunks: %v", chunkTexts(chunks))
	}
}

func TestChunkSections_SmallCodeBlockNotExtracted(t *testing.T) {
	md := "Text.\n\n```rust\nlet x = 1;\n```\n"
	chunks := ChunkSections("path", md)
	for _, c := range chunks {
		// The small code block should only appear within a section, not as standalone
		if strings.HasPrefix(strings.TrimPrefix(c.Text, "path\n\n"), "```") {
			t.Error("small code block should not be extracted as standalone chunk")
		}
	}
}

func TestChunkSections_HeadingSplitting(t *testing.T) {
	md := `# First

Content one.

## Second

Content two.

# Third

Content three.
`
	chunks := ChunkSections("p", md)

	// Should have separate chunks for each heading section
	sectionTexts := chunkTexts(chunks)
	foundFirst := false
	foundSecond := false
	foundThird := false
	for _, t := range sectionTexts {
		if strings.Contains(t, "# First") && strings.Contains(t, "Content one") {
			foundFirst = true
		}
		if strings.Contains(t, "## Second") && strings.Contains(t, "Content two") {
			foundSecond = true
		}
		if strings.Contains(t, "# Third") && strings.Contains(t, "Content three") {
			foundThird = true
		}
	}
	if !foundFirst || !foundSecond || !foundThird {
		t.Errorf("should split on headings. Found: first=%v second=%v third=%v\nChunks: %v",
			foundFirst, foundSecond, foundThird, sectionTexts)
	}
}

func TestChunkSections_IndexesAreSequential(t *testing.T) {
	md := `Summary.

# A

text

# B

text
`
	chunks := ChunkSections("p", md)
	for i, c := range chunks {
		if c.Index != i {
			t.Errorf("chunk %d has Index %d, expected sequential", i, c.Index)
		}
	}
}

func TestChunkSections_OnlyHeadingsNoContent(t *testing.T) {
	md := "# Heading One\n\n# Heading Two\n\n# Heading Three\n"
	chunks := ChunkSections("p", md)
	// Each heading becomes a section even without body text
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks for headings-only, got %d", len(chunks))
	}
}

func TestChunkSections_CodeBlockAtBoundary(t *testing.T) {
	// Code block that is exactly 80 chars (>= 80 should be extracted)
	code := strings.Repeat("x", 80)
	md := "Text.\n\n```\n" + code + "\n```\n"
	chunks := ChunkSections("p", md)
	foundCodeChunk := false
	for _, c := range chunks {
		if strings.Contains(c.Text, code) && strings.Contains(c.Text, "```") {
			text := strings.TrimPrefix(c.Text, "p\n\n")
			if strings.HasPrefix(text, "```") {
				foundCodeChunk = true
			}
		}
	}
	if !foundCodeChunk {
		t.Error("code block at exactly 80 chars should be extracted")
	}
}

func TestChunkSections_MultipleCodeBlocks(t *testing.T) {
	code1 := strings.Repeat("let a = 1;\n", 10)
	code2 := strings.Repeat("let b = 2;\n", 10)
	md := "Intro.\n\n```rust\n" + code1 + "```\n\nMiddle.\n\n```rust\n" + code2 + "```\n"
	chunks := ChunkSections("p", md)

	codeChunks := 0
	for _, c := range chunks {
		text := strings.TrimPrefix(c.Text, "p\n\n")
		if strings.HasPrefix(text, "```") {
			codeChunks++
		}
	}
	if codeChunks < 2 {
		t.Errorf("expected at least 2 code block chunks, got %d", codeChunks)
	}
}

func chunkTexts(chunks []Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}
	return texts
}
