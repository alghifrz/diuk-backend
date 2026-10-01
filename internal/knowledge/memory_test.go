package knowledge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/pkg/storage"
)

type memoryStore struct {
	mu        sync.Mutex
	documents map[uuid.UUID]*Document
	chunks    map[uuid.UUID][]Chunk
}

func newMemoryStore() *memoryStore {
	return &memoryStore{documents: map[uuid.UUID]*Document{}, chunks: map[uuid.UUID][]Chunk{}}
}

func (m *memoryStore) CreateDocument(_ context.Context, doc *Document) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}
	now := time.Now().UTC()
	doc.CreatedAt = now
	doc.UpdatedAt = now
	copy := *doc
	m.documents[doc.ID] = &copy
	out := copy
	return &out, nil
}

func (m *memoryStore) GetDocument(_ context.Context, businessID, documentID uuid.UUID) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) ListDocuments(_ context.Context, businessID uuid.UUID, status string) ([]Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Document, 0)
	for _, doc := range m.documents {
		if doc.BusinessID != businessID {
			continue
		}
		if status != "" && doc.Status != status {
			continue
		}
		out = append(out, *doc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (m *memoryStore) UpdateDocumentMeta(_ context.Context, businessID, documentID uuid.UUID, title string, description *string, updatedBy *uuid.UUID) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	doc.Title = title
	doc.Description = description
	doc.UpdatedBy = updatedBy
	doc.UpdatedAt = time.Now().UTC()
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) UpdateDocumentStorage(_ context.Context, businessID, documentID uuid.UUID, path, mime string) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	doc.StoragePath = &path
	doc.MimeType = &mime
	doc.UpdatedAt = time.Now().UTC()
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) MarkPending(_ context.Context, businessID, documentID uuid.UUID, updatedBy *uuid.UUID) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	doc.Status = StatusPending
	doc.ProcessingError = nil
	doc.Version++
	doc.UpdatedBy = updatedBy
	doc.UpdatedAt = time.Now().UTC()
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) ReleaseProcessing(_ context.Context, businessID, documentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return ErrDocumentNotFound
	}
	if doc.Status == StatusProcessing {
		doc.Status = StatusPending
		doc.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (m *memoryStore) ClaimProcessing(_ context.Context, businessID, documentID uuid.UUID, staleAfter time.Duration) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	stale := staleAfter > 0 && time.Since(doc.UpdatedAt) >= staleAfter
	if doc.Status == StatusPending || (doc.Status == StatusProcessing && stale) {
		doc.Status = StatusProcessing
		doc.ProcessingError = nil
		doc.UpdatedAt = time.Now().UTC()
		copy := *doc
		return &copy, nil
	}
	return nil, ErrDocumentAlreadyProcessing
}

func (m *memoryStore) MarkReady(_ context.Context, businessID, documentID uuid.UUID) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	doc.Status = StatusReady
	doc.ProcessingError = nil
	doc.UpdatedAt = time.Now().UTC()
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) MarkFailed(_ context.Context, businessID, documentID uuid.UUID, processingError string) (*Document, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return nil, ErrDocumentNotFound
	}
	doc.Status = StatusFailed
	doc.ProcessingError = &processingError
	doc.UpdatedAt = time.Now().UTC()
	copy := *doc
	return &copy, nil
}

func (m *memoryStore) DeleteDocument(_ context.Context, businessID, documentID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	doc, ok := m.documents[documentID]
	if !ok || doc.BusinessID != businessID {
		return ErrDocumentNotFound
	}
	delete(m.documents, documentID)
	delete(m.chunks, documentID)
	return nil
}

func (m *memoryStore) ReplaceChunks(_ context.Context, businessID, documentID uuid.UUID, chunks []Chunk) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if doc, ok := m.documents[documentID]; !ok || doc.BusinessID != businessID {
		return ErrDocumentNotFound
	}
	copied := make([]Chunk, len(chunks))
	for i, chunk := range chunks {
		if chunk.ID == uuid.Nil {
			chunk.ID = uuid.New()
		}
		chunk.BusinessID = businessID
		chunk.DocumentID = documentID
		copied[i] = chunk
	}
	m.chunks[documentID] = copied
	return nil
}

func (m *memoryStore) SearchChunks(_ context.Context, businessID uuid.UUID, embedding []float32, minSimilarity float64, limit int) ([]SearchHit, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]SearchHit, 0)
	for docID, chunks := range m.chunks {
		doc, ok := m.documents[docID]
		if !ok || doc.BusinessID != businessID || doc.Status != StatusReady {
			continue
		}
		for _, chunk := range chunks {
			if chunk.BusinessID != businessID {
				continue
			}
			sim := cosineSimilarity(embedding, chunk.Embedding)
			if sim < minSimilarity {
				continue
			}
			out = append(out, SearchHit{
				ChunkID: chunk.ID, DocumentID: doc.ID, Title: doc.Title,
				Content: chunk.Content, Similarity: sim, Metadata: chunk.Metadata,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (m *memoryStore) CountChunks(_ context.Context, businessID, documentID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, chunk := range m.chunks[documentID] {
		if chunk.BusinessID == businessID {
			n++
		}
	}
	return n, nil
}

func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i] * b[i])
		na += float64(a[i] * a[i])
		nb += float64(b[i] * b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (sqrt(na) * sqrt(nb))
}

func sqrt(v float64) float64 {
	if v <= 0 {
		return 0
	}
	x := v
	for i := 0; i < 12; i++ {
		x = 0.5 * (x + v/x)
	}
	return x
}

type memoryObjects struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newMemoryObjects() *memoryObjects {
	return &memoryObjects{objects: map[string][]byte{}}
}

func (m *memoryObjects) Upload(_ context.Context, path, _ string, data io.Reader) error {
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[path] = bytes.Clone(b)
	return nil
}

func (m *memoryObjects) Download(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[path]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return bytes.Clone(data), nil
}

func (m *memoryObjects) Delete(_ context.Context, path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[path]; !ok {
		return storage.ErrNotFound
	}
	delete(m.objects, path)
	return nil
}

type stubEmbedder struct {
	dim     int
	err     error
	calls   int
	lastLen int
}

func (s *stubEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	s.calls++
	s.lastLen = len(texts)
	if s.err != nil {
		return nil, s.err
	}
	dim := s.dim
	if dim < 1 {
		dim = 4
	}
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec := make([]float32, dim)
		if len(text) > 0 {
			vec[0] = float32(len(text)%10) / 10
			vec[1] = 0.8
		}
		if errors.Is(s.err, nil) && text == "policy" {
			vec[1] = 0.99
		}
		out[i] = vec
	}
	return out, nil
}

type recordingEnqueue struct {
	calls int
	lastB uuid.UUID
	lastD uuid.UUID
	err   error
}

func (r *recordingEnqueue) EnqueueKnowledgeIngest(_ context.Context, businessID, documentID uuid.UUID) error {
	r.calls++
	r.lastB = businessID
	r.lastD = documentID
	return r.err
}
