package knowledge

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type TextChunk struct {
	Index      int
	Content    string
	TokenCount int
}

func ChunkText(text string, size, overlap int) ([]TextChunk, error) {
	size, overlap, err := normalizeChunkSettings(size, overlap)
	if err != nil {
		return nil, err
	}
	normalized := normalizeExtracted(text)
	if normalized == "" {
		return nil, ErrDocumentEmpty
	}

	out := make([]TextChunk, 0)
	for _, section := range splitSections(splitParagraphs(normalized)) {
		// Each chunk repeats its heading path so "- Dogs allowed" is still
		// retrievable as an answer about pets, not just a stray bullet.
		prefix := section.heading
		bodySize := size
		if prefix != "" {
			bodySize = size - utf8.RuneCountInString(prefix) - 1
			if bodySize < size/2 {
				bodySize = size / 2
			}
		}
		var previous string
		for _, part := range packParagraphs(glueQuestions(section.lines), bodySize) {
			content := part
			if overlap > 0 && previous != "" {
				tail := overlapTail(previous, overlap)
				if tail != "" && !strings.HasPrefix(content, tail) {
					content = strings.TrimSpace(tail + " " + content)
				}
			}
			content = strings.TrimSpace(content)
			if content == "" {
				continue
			}
			if prefix != "" {
				content = prefix + "\n" + content
			}
			out = append(out, TextChunk{
				Index:      len(out),
				Content:    content,
				TokenCount: estimateTokens(content),
			})
			previous = part
		}
	}
	if len(out) == 0 {
		return nil, ErrDocumentEmpty
	}
	return out, nil
}

// glueQuestions keeps a line that ends in "?" together with the line after it,
// so FAQ-style "question / answer" pairs are never split across two chunks.
func glueQuestions(lines []string) []string {
	out := make([]string, 0, len(lines))
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasSuffix(line, "?") && i+1 < len(lines) {
			line += "\n" + lines[i+1]
			i++
		}
		out = append(out, line)
	}
	return out
}

type textSection struct {
	heading string
	lines   []string
}

// splitSections groups lines under the nearest Markdown heading (as a path such
// as "Pets › Dogs"). Text without headings becomes one section with no heading,
// which keeps plain .txt behaviour unchanged.
func splitSections(lines []string) []textSection {
	var (
		sections []textSection
		stack    []string
		current  textSection
		inFence  bool
	)
	flush := func() {
		if len(current.lines) > 0 {
			sections = append(sections, current)
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			inFence = !inFence
			current.lines = append(current.lines, line)
			continue
		}
		level, title, ok := parseHeading(line)
		if inFence || !ok {
			current.lines = append(current.lines, line)
			continue
		}
		flush()
		if level > len(stack) {
			for len(stack) < level-1 {
				stack = append(stack, "")
			}
			stack = append(stack, title)
		} else {
			stack = append(stack[:level-1], title)
		}
		current = textSection{heading: headingPath(stack)}
	}
	flush()
	if len(sections) == 0 {
		// Only headings, no body: index the headings themselves.
		return []textSection{{lines: lines}}
	}
	return sections
}

func parseHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || len(line) <= level || line[level] != ' ' {
		return 0, "", false
	}
	title := strings.TrimSpace(strings.TrimRight(strings.TrimSpace(line[level:]), "#"))
	if title == "" {
		return 0, "", false
	}
	return level, title, true
}

func headingPath(stack []string) string {
	parts := make([]string, 0, len(stack))
	for _, part := range stack {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, " \u203a ")
}
func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return []string{strings.TrimSpace(text)}
	}
	return out
}

func packParagraphs(paragraphs []string, size int) []string {
	out := make([]string, 0)
	var current strings.Builder
	for _, para := range paragraphs {
		if utf8.RuneCountInString(para) <= size {
			if current.Len() > 0 && utf8.RuneCountInString(current.String()+"\n\n"+para) > size {
				out = append(out, strings.TrimSpace(current.String()))
				current.Reset()
			}
			if current.Len() > 0 {
				current.WriteString("\n\n")
			}
			current.WriteString(para)
			continue
		}
		if current.Len() > 0 {
			out = append(out, strings.TrimSpace(current.String()))
			current.Reset()
		}
		out = append(out, splitLong(para, size)...)
	}
	if current.Len() > 0 {
		out = append(out, strings.TrimSpace(current.String()))
	}
	return out
}

func splitLong(text string, size int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	out := make([]string, 0)
	var current strings.Builder
	for _, word := range words {
		if current.Len() == 0 {
			current.WriteString(word)
			continue
		}
		if utf8.RuneCountInString(current.String()+" "+word) <= size {
			current.WriteByte(' ')
			current.WriteString(word)
			continue
		}
		out = append(out, current.String())
		current.Reset()
		if utf8.RuneCountInString(word) <= size {
			current.WriteString(word)
			continue
		}
		out = append(out, splitRunes(word, size)...)
	}
	if current.Len() > 0 {
		out = append(out, current.String())
	}
	return out
}

func splitRunes(text string, size int) []string {
	runes := []rune(text)
	out := make([]string, 0, (len(runes)+size-1)/size)
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		out = append(out, string(runes[i:end]))
	}
	return out
}

func overlapTail(text string, overlap int) string {
	runes := []rune(text)
	if overlap <= 0 || len(runes) == 0 {
		return ""
	}
	if overlap >= len(runes) {
		return strings.TrimSpace(text)
	}
	start := len(runes) - overlap
	for start < len(runes) && !unicode.IsSpace(runes[start]) && start > 0 {
		if unicode.IsSpace(runes[start-1]) {
			break
		}
		start++
	}
	if start >= len(runes) {
		start = len(runes) - overlap
	}
	return strings.TrimSpace(string(runes[start:]))
}

func estimateTokens(text string) int {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return 0
	}
	return len(fields)
}
