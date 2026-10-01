package knowledge

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

func normalizeTitle(raw string) (string, error) {
	title := strings.TrimSpace(raw)
	if title == "" {
		return "", invalid("title is required")
	}
	if utf8.RuneCountInString(title) > DefaultMaxTitleRunes {
		return "", invalid("title is too long")
	}
	return title, nil
}

func normalizeOptionalDescription(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(text) > DefaultMaxDescriptionRunes {
		return nil, invalid("description is too long")
	}
	return &text, nil
}

func normalizeSourceType(raw, filename, mime string) (string, error) {
	source := strings.ToUpper(strings.TrimSpace(raw))
	if source == "" {
		source = inferSourceType(filename, mime)
	}
	switch source {
	case SourceText, SourcePDF, SourceDOCX:
		return source, nil
	default:
		return "", ErrUnsupportedDocumentType
	}
}

func inferSourceType(filename, mime string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".txt", ".md", ".markdown":
		return SourceText
	case ".pdf":
		return SourcePDF
	case ".docx":
		return SourceDOCX
	}
	switch strings.ToLower(strings.TrimSpace(mime)) {
	case "text/plain", "text/markdown", "text/x-markdown":
		return SourceText
	case "application/pdf":
		return SourcePDF
	case "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return SourceDOCX
	}
	return ""
}

func validateSourceMatch(source, filename, mime string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext != "" {
		// Markdown is plain text with light syntax, so it shares the TEXT pipeline.
		want := map[string][]string{
			SourceText: {".txt", ".md", ".markdown"},
			SourcePDF:  {".pdf"},
			SourceDOCX: {".docx"},
		}
		if !slices.Contains(want[source], ext) {
			return ErrUnsupportedDocumentType
		}
	}
	normalized := strings.ToLower(strings.TrimSpace(mime))
	if normalized == "" || normalized == "application/octet-stream" {
		return nil
	}
	allowed := map[string][]string{
		SourceText: {"text/plain", "text/markdown", "text/x-markdown"},
		SourcePDF:  {"application/pdf"},
		SourceDOCX: {"application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	}
	for _, item := range allowed[source] {
		if normalized == item {
			return nil
		}
	}
	return ErrUnsupportedDocumentType
}

func extensionFor(source string) string {
	switch source {
	case SourcePDF:
		return "pdf"
	case SourceDOCX:
		return "docx"
	default:
		return "txt"
	}
}

func defaultMIME(source string) string {
	switch source {
	case SourcePDF:
		return "application/pdf"
	case SourceDOCX:
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "text/plain"
	}
}

func validateFile(content []byte, maxBytes int64) error {
	if len(content) == 0 {
		return ErrDocumentEmpty
	}
	if maxBytes > 0 && int64(len(content)) > maxBytes {
		return ErrDocumentTooLarge
	}
	return nil
}

func normalizeQuery(raw string) (string, error) {
	query := strings.TrimSpace(raw)
	if query == "" {
		return "", invalid("query is required")
	}
	if utf8.RuneCountInString(query) > DefaultMaxQueryRunes {
		return "", ErrQueryTooLong
	}
	return query, nil
}

func normalizeTopK(n, fallback, max int) int {
	if n < 1 {
		n = fallback
	}
	if max < 1 {
		max = DefaultMaxTopK
	}
	if n > max {
		return max
	}
	return n
}

func normalizeChunkSettings(size, overlap int) (int, int, error) {
	if size < 1 {
		size = DefaultChunkSize
	}
	if size > 8000 {
		return 0, 0, invalid("chunk size is too large")
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		return 0, 0, invalid("chunk overlap must be smaller than chunk size")
	}
	return size, overlap, nil
}

func objectPath(businessID, documentID, source string) string {
	return businessID + "/knowledge/" + documentID + "/source." + extensionFor(source)
}
