package knowledge

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

func ExtractText(sourceType string, data []byte) (string, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return "", ErrDocumentEmpty
	}
	switch sourceType {
	case SourceText:
		return extractPlainText(data)
	case SourcePDF:
		return extractPDF(data)
	case SourceDOCX:
		return extractDOCX(data)
	default:
		return "", ErrUnsupportedDocumentType
	}
}

func extractPlainText(data []byte) (string, error) {
	if !utf8.Valid(data) {
		return "", invalid("text document is not valid utf-8")
	}
	text := normalizeExtracted(string(data))
	if text == "" {
		return "", ErrDocumentEmpty
	}
	return text, nil
}

func extractPDF(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", ErrUnsupportedDocumentType
	}
	var b strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		plain, err := pdfPageText(page)
		if err != nil {
			return "", ErrDocumentProcessingFailed
		}
		if strings.TrimSpace(plain) == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(plain)
	}
	text := normalizeExtracted(b.String())
	if text == "" {
		return "", ErrDocumentEmpty
	}
	return text, nil
}

func extractDOCX(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", ErrUnsupportedDocumentType
	}
	var document io.ReadCloser
	for _, file := range reader.File {
		if file.Name == "word/document.xml" {
			document, err = file.Open()
			if err != nil {
				return "", ErrDocumentProcessingFailed
			}
			break
		}
	}
	if document == nil {
		return "", ErrUnsupportedDocumentType
	}
	defer document.Close()

	decoder := xml.NewDecoder(document)
	var b strings.Builder
	inTab := false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", ErrDocumentProcessingFailed
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if b.Len() > 0 {
					b.WriteString("\n")
				}
			case "tab":
				inTab = true
			}
		case xml.EndElement:
			if t.Name.Local == "tab" {
				inTab = false
			}
		case xml.CharData:
			text := string(t)
			if inTab {
				b.WriteByte('\t')
			}
			b.WriteString(text)
		}
	}
	text := normalizeExtracted(b.String())
	if text == "" {
		return "", ErrDocumentEmpty
	}
	return text, nil
}

func normalizeExtracted(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	prevSpace := false
	prevNewline := true
	for _, r := range raw {
		switch {
		case r == '\r':
			continue
		case r == '\n':
			if !prevNewline {
				b.WriteByte('\n')
			}
			prevNewline = true
			prevSpace = true
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			prevNewline = false
		default:
			b.WriteRune(r)
			prevSpace = false
			prevNewline = false
		}
	}
	return strings.TrimSpace(b.String())
}
