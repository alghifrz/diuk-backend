package knowledge

import (
	"errors"
	"strings"
	"testing"
)

func TestDocumentValidation(t *testing.T) {
	if _, err := normalizeTitle("  "); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty title = %v", err)
	}
	if _, err := normalizeTitle(strings.Repeat("a", DefaultMaxTitleRunes+1)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long title = %v", err)
	}
	if _, err := normalizeOptionalDescription(ptr(strings.Repeat("b", DefaultMaxDescriptionRunes+1))); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long description = %v", err)
	}
	if _, err := normalizeSourceType("CSV", "file.csv", "text/csv"); !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("csv = %v", err)
	}
	got, err := normalizeSourceType("", "policy.txt", "text/plain")
	if err != nil || got != SourceText {
		t.Fatalf("infer text = %s err=%v", got, err)
	}
	if err := validateSourceMatch(SourcePDF, "notes.txt", "text/plain"); !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("mismatch = %v", err)
	}
	for _, tc := range []struct{ name, mime string }{
		{"faq.md", "text/markdown"},
		{"faq.md", ""},
		{"faq.MD", "application/octet-stream"},
		{"faq.markdown", "text/x-markdown"},
	} {
		got, err := normalizeSourceType("", tc.name, tc.mime)
		if err != nil || got != SourceText {
			t.Fatalf("infer markdown %+v = %s err=%v", tc, got, err)
		}
		if err := validateSourceMatch(SourceText, tc.name, tc.mime); err != nil {
			t.Fatalf("markdown match %+v = %v", tc, err)
		}
	}
	if err := validateSourceMatch(SourcePDF, "faq.md", "text/markdown"); !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("pdf declared as markdown = %v", err)
	}
	if err := validateFile(nil, 10); !errors.Is(err, ErrDocumentEmpty) {
		t.Fatalf("empty file = %v", err)
	}
	if err := validateFile([]byte("hello"), 3); !errors.Is(err, ErrDocumentTooLarge) {
		t.Fatalf("too large = %v", err)
	}
	if _, err := normalizeQuery(""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty query = %v", err)
	}
	if _, err := normalizeQuery(strings.Repeat("q", DefaultMaxQueryRunes+1)); !errors.Is(err, ErrQueryTooLong) {
		t.Fatalf("long query = %v", err)
	}
	if n := normalizeTopK(100, 5, 20); n != 20 {
		t.Fatalf("top k = %d", n)
	}
	if _, _, err := normalizeChunkSettings(100, 100); !errors.Is(err, ErrInvalid) {
		t.Fatalf("overlap = %v", err)
	}
}

func ptr(v string) *string { return &v }

func TestSanitizeProcessingError(t *testing.T) {
	got := sanitizeProcessingError(errors.New("provider failed with Bearer sk-secret"))
	if strings.Contains(got, "sk-") || strings.Contains(strings.ToLower(got), "bearer") {
		t.Fatalf("leaked secret: %s", got)
	}
}
