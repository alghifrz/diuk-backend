package knowledge

import "time"

type CreateDocumentInput struct {
	Title       string
	Description *string
	SourceType  string
	Content     []byte
	Filename    string
	MimeType    string
	CreatedBy   *string
}

type UpdateDocumentInput struct {
	Title       *string
	Description *string
	UpdatedBy   *string
}

type documentJSON struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	SourceType      string  `json:"source_type"`
	MimeType        *string `json:"mime_type"`
	Status          string  `json:"status"`
	Version         int     `json:"version"`
	ProcessingError *string `json:"processing_error,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type createDocumentJSON struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	SourceType  string  `json:"source_type"`
	Content     string  `json:"content"`
}

type updateDocumentJSON struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
}

type statusJSON struct {
	Enabled       bool    `json:"enabled"`
	MaxFileBytes  int64   `json:"max_file_bytes"`
	MinSimilarity float64 `json:"min_similarity"`
}

type searchRequestJSON struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type searchHitJSON struct {
	ChunkID    string  `json:"chunk_id"`
	DocumentID string  `json:"document_id"`
	Title      string  `json:"title"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
}

type askRequestJSON struct {
	Question string `json:"question"`
}

type askResultJSON struct {
	Available bool            `json:"available"`
	Found     bool            `json:"found"`
	Answer    string          `json:"answer,omitempty"`
	Sources   []searchHitJSON `json:"sources"`
}

func askResponse(out *AskResult) askResultJSON {
	res := askResultJSON{Available: out.Available, Found: out.Found, Answer: out.Answer, Sources: make([]searchHitJSON, 0, len(out.Sources))}
	for _, hit := range out.Sources {
		res.Sources = append(res.Sources, searchHitJSON{
			ChunkID: hit.ChunkID.String(), DocumentID: hit.DocumentID.String(),
			Title: hit.Title, Content: hit.Content, Similarity: hit.Similarity,
		})
	}
	return res
}

type searchResultJSON struct {
	Available bool            `json:"available"`
	Reason    string          `json:"reason,omitempty"`
	Results   []searchHitJSON `json:"results"`
}

func searchResponse(out *SearchResult) searchResultJSON {
	res := searchResultJSON{Available: out.Available, Reason: out.Reason, Results: make([]searchHitJSON, 0, len(out.Results))}
	for _, hit := range out.Results {
		res.Results = append(res.Results, searchHitJSON{
			ChunkID: hit.ChunkID.String(), DocumentID: hit.DocumentID.String(),
			Title: hit.Title, Content: hit.Content, Similarity: hit.Similarity,
		})
	}
	return res
}

func documentResponse(doc *Document) documentJSON {
	return documentJSON{
		ID:              doc.ID.String(),
		Title:           doc.Title,
		Description:     doc.Description,
		SourceType:      doc.SourceType,
		MimeType:        doc.MimeType,
		Status:          doc.Status,
		Version:         doc.Version,
		ProcessingError: doc.ProcessingError,
		CreatedAt:       doc.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       doc.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func documentsResponse(items []Document) []documentJSON {
	out := make([]documentJSON, 0, len(items))
	for i := range items {
		out = append(out, documentResponse(&items[i]))
	}
	return out
}
