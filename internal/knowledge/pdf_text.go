package knowledge

import (
	"math"
	"sort"
	"strings"

	"github.com/ledongthuc/pdf"
)

// pdfPageText rebuilds readable text from positioned glyph runs.
//
// pdf.Page.GetPlainText starts a new line at every text object, and many
// exporters (Google Docs, Skia/Chrome) emit one object per word, which turns a
// paragraph into one word per line. That wrecks both chunking and embeddings,
// so we group runs into lines by their position and join wrapped lines back
// into paragraphs instead.
func pdfPageText(page pdf.Page) (string, error) {
	var texts []pdf.Text
	func() {
		// The library panics on some malformed content streams.
		defer func() { _ = recover() }()
		texts = page.Content().Text
	}()
	if len(texts) == 0 {
		return legacyPDFText(page)
	}
	text := layoutPDFText(texts)
	if strings.TrimSpace(text) == "" {
		return legacyPDFText(page)
	}
	return text, nil
}

func legacyPDFText(page pdf.Page) (string, error) {
	return page.GetPlainText(nil)
}

type pdfLine struct {
	y        float64
	fontSize float64
	runs     []pdf.Text
}

func layoutPDFText(texts []pdf.Text) string {
	sorted := make([]pdf.Text, 0, len(texts))
	for _, t := range texts {
		// Bare newlines are text-object markers, not content; spaces are kept.
		if strings.Trim(t.S, "\r\n") != "" {
			sorted = append(sorted, t)
		}
	}
	// Top to bottom; left to right within a line.
	sort.SliceStable(sorted, func(i, j int) bool {
		if math.Abs(sorted[i].Y-sorted[j].Y) > 0.5 {
			return sorted[i].Y > sorted[j].Y
		}
		return sorted[i].X < sorted[j].X
	})

	var lines []*pdfLine
	for _, t := range sorted {
		size := t.FontSize
		if size <= 0 {
			size = 10
		}
		if n := len(lines); n > 0 && math.Abs(lines[n-1].y-t.Y) <= math.Max(size*0.4, 1.5) {
			lines[n-1].runs = append(lines[n-1].runs, t)
			if size > lines[n-1].fontSize {
				lines[n-1].fontSize = size
			}
			continue
		}
		lines = append(lines, &pdfLine{y: t.Y, fontSize: size, runs: []pdf.Text{t}})
	}

	var out strings.Builder
	var prev *pdfLine
	for _, line := range lines {
		sort.SliceStable(line.runs, func(i, j int) bool { return line.runs[i].X < line.runs[j].X })
		text := strings.TrimSpace(joinRuns(line.runs))
		if text == "" {
			continue
		}
		switch {
		case prev == nil:
		case startsNewParagraph(prev, line):
			out.WriteByte('\n')
		default:
			out.WriteByte(' ')
		}
		out.WriteString(text)
		prev = line
	}
	return out.String()
}

func joinRuns(runs []pdf.Text) string {
	var b strings.Builder
	for i, run := range runs {
		if i > 0 {
			prev := runs[i-1]
			gap := run.X - (prev.X + prev.W)
			size := math.Max(prev.FontSize, 1)
			needsSpace := gap > size*0.3
			if prev.W <= 0 {
				// No width information: fall back to treating each run as a word.
				needsSpace = true
			}
			if needsSpace && !strings.HasSuffix(b.String(), " ") && !strings.HasPrefix(run.S, " ") {
				b.WriteByte(' ')
			}
		}
		b.WriteString(run.S)
	}
	return b.String()
}

// startsNewParagraph reports whether `line` begins a new block rather than
// continuing the wrapped paragraph above it: a larger vertical gap, a change in
// font size (headings), or a list/question marker.
func startsNewParagraph(prev, line *pdfLine) bool {
	gap := prev.y - line.y
	if gap > prev.fontSize*1.7 {
		return true
	}
	if math.Abs(prev.fontSize-line.fontSize) > 1 {
		return true
	}
	first := strings.TrimSpace(line.runs[0].S)
	switch {
	case strings.HasPrefix(first, "•"), strings.HasPrefix(first, "-"), strings.HasPrefix(first, "–"):
		return true
	case strings.HasPrefix(first, "Q:"), strings.HasPrefix(first, "Q."), first == "Q":
		return true
	}
	return false
}
