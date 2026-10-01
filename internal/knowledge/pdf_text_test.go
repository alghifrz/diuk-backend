package knowledge

import (
	"strings"
	"testing"

	"github.com/ledongthuc/pdf"
)

func glyphs(s string, x, y, size float64) []pdf.Text {
	out := make([]pdf.Text, 0, len(s))
	for _, r := range s {
		w := size * 0.5
		out = append(out, pdf.Text{S: string(r), X: x, Y: y, W: w, FontSize: size})
		x += w
	}
	return out
}

func TestLayoutPDFTextRebuildsLinesAndParagraphs(t *testing.T) {
	var runs []pdf.Text
	// Heading, then a two-line wrapped paragraph, with bare-newline markers
	// between text objects the way some exporters emit them.
	runs = append(runs, glyphs("Parking", 50, 700, 20)...)
	runs = append(runs, pdf.Text{S: "\n", X: 120, Y: 700, FontSize: 20})
	runs = append(runs, glyphs("Free parking for", 50, 670, 10)...)
	runs = append(runs, pdf.Text{S: "\n", X: 120, Y: 670, FontSize: 10})
	runs = append(runs, glyphs("cars and bikes.", 50, 658, 10)...)

	got := layoutPDFText(runs)
	want := "Parking\nFree parking for cars and bikes."
	if got != want {
		t.Fatalf("layout = %q want %q", got, want)
	}
}

func TestLayoutPDFTextInsertsWordGaps(t *testing.T) {
	var runs []pdf.Text
	runs = append(runs, glyphs("Hello", 10, 100, 10)...)
	runs = append(runs, glyphs("world", 10+5*5+6, 100, 10)...) // 6pt gap
	if got := layoutPDFText(runs); got != "Hello world" {
		t.Fatalf("layout = %q", got)
	}
}

func TestGlueQuestionsKeepsAnswerWithQuestion(t *testing.T) {
	got := glueQuestions([]string{"Intro", "Is parking free?", "Yes, behind the cafe.", "Other"})
	if len(got) != 3 || got[1] != "Is parking free?\nYes, behind the cafe." {
		t.Fatalf("glued = %q", got)
	}
	chunks, err := ChunkText("Intro text here that is fairly long to push chunks.\n\nIs parking free?\nYes, behind the cafe.", 60, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range chunks {
		if strings.Contains(c.Content, "Is parking free?") && !strings.Contains(c.Content, "Yes, behind") {
			t.Fatalf("question split from answer: %q", c.Content)
		}
	}
}
