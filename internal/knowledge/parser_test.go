package knowledge

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
)

func TestExtractPlainText(t *testing.T) {
	got, err := ExtractText(SourceText, []byte("  FAQ\n\nWe open at 08:00.  "))
	if err != nil || got != "FAQ\nWe open at 08:00." {
		t.Fatalf("text = %q err=%v", got, err)
	}
	if _, err := ExtractText(SourceText, []byte("   ")); !errors.Is(err, ErrDocumentEmpty) {
		t.Fatalf("empty = %v", err)
	}
	if _, err := ExtractText("CSV", []byte("a,b")); !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("unsupported = %v", err)
	}
}

func TestExtractDOCX(t *testing.T) {
	data := minimalDOCX(t, "Guests may cancel until 17:00.")
	got, err := ExtractText(SourceDOCX, data)
	if err != nil || got != "Guests may cancel until 17:00." {
		t.Fatalf("docx = %q err=%v", got, err)
	}
	if _, err := ExtractText(SourceDOCX, []byte("not-a-zip")); !errors.Is(err, ErrUnsupportedDocumentType) {
		t.Fatalf("bad docx = %v", err)
	}
}

func TestExtractPDFEmptyFails(t *testing.T) {
	if _, err := ExtractText(SourcePDF, []byte("%PDF-1.4 empty")); !errors.Is(err, ErrUnsupportedDocumentType) && !errors.Is(err, ErrDocumentEmpty) && !errors.Is(err, ErrDocumentProcessingFailed) {
		t.Fatalf("invalid pdf = %v", err)
	}
}

func minimalDOCX(t *testing.T, text string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	fw, err := w.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
