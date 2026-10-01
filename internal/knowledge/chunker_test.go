package knowledge

import (
	"errors"
	"strings"
	"testing"
)

func TestChunkTextParagraphsAndOverlap(t *testing.T) {
	text := "Cancellation is free until 24 hours before the booking.\n\nA late cancel may incur a fee.\n\nWalk-ins are welcome when tables are available."
	chunks, err := ChunkText(text, 80, 20)
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if chunk.Index != i {
			t.Fatalf("index = %d want %d", chunk.Index, i)
		}
		if strings.TrimSpace(chunk.Content) == "" {
			t.Fatal("empty chunk")
		}
		if chunk.TokenCount < 1 {
			t.Fatal("token count")
		}
	}
	again, err := ChunkText(text, 80, 20)
	if err != nil || len(again) != len(chunks) {
		t.Fatalf("deterministic count = %d err=%v", len(again), err)
	}
	for i := range chunks {
		if again[i].Content != chunks[i].Content {
			t.Fatalf("chunk %d not deterministic", i)
		}
	}
}

func TestChunkTextAvoidsWordSplitAndEmpty(t *testing.T) {
	if _, err := ChunkText("   \n\n  ", 80, 10); !errors.Is(err, ErrDocumentEmpty) {
		t.Fatalf("empty = %v", err)
	}
	chunks, err := ChunkText("Reservation policy allows guests to keep the same table for ninety minutes.", 40, 8)
	if err != nil {
		t.Fatalf("chunk: %v", err)
	}
	for _, chunk := range chunks {
		if strings.Contains(chunk.Content, "minut\n") {
			t.Fatalf("split word: %q", chunk.Content)
		}
	}
}
