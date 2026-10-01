package knowledge

import (
	"strings"
	"testing"
)

const headingSep = " \u203a "

func TestChunkTextMarkdownKeepsHeadingContext(t *testing.T) {
	md := "# House rules\n\nIntro line about the venue.\n\n## Pets\n\n- Small dogs are welcome on the terrace.\n- No pets inside.\n\n## Parking\n\nFree parking behind the building.\n\n```\n# not a heading\n```\n"
	chunks, err := ChunkText(md, 200, 20)
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	var pets, parking *TextChunk
	for i := range chunks {
		switch {
		case strings.Contains(chunks[i].Content, "Small dogs"):
			pets = &chunks[i]
		case strings.Contains(chunks[i].Content, "Free parking"):
			parking = &chunks[i]
		}
	}
	if pets == nil || !strings.HasPrefix(pets.Content, "House rules"+headingSep+"Pets\n") {
		t.Fatalf("pets chunk = %+v", pets)
	}
	if parking == nil || !strings.HasPrefix(parking.Content, "House rules"+headingSep+"Parking\n") {
		t.Fatalf("parking chunk = %+v", parking)
	}
	if strings.Contains(pets.Content, "Parking") {
		t.Fatalf("sections leaked: %q", pets.Content)
	}
	if !strings.Contains(parking.Content, "# not a heading") {
		t.Fatalf("fenced code lost: %q", parking.Content)
	}
}

func TestChunkTextHeadingsOnlyAndPlainText(t *testing.T) {
	chunks, err := ChunkText("# Title\n## Subtitle", 80, 10)
	if err != nil || len(chunks) == 0 {
		t.Fatalf("headings only = %v err=%v", chunks, err)
	}
	plain, err := ChunkText("No headings here.\nJust two lines.", 80, 10)
	if err != nil || len(plain) != 1 || strings.Contains(plain[0].Content, strings.TrimSpace(headingSep)) {
		t.Fatalf("plain = %+v err=%v", plain, err)
	}
}
