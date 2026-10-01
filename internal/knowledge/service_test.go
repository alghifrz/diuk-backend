package knowledge

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/embedding"
)

func testService(enabled bool) (*Service, *memoryStore, *memoryObjects, *stubEmbedder, *recordingEnqueue) {
	store := newMemoryStore()
	objects := newMemoryObjects()
	embedder := &stubEmbedder{dim: 4}
	enqueue := &recordingEnqueue{}
	settings := DefaultSettings()
	settings.Enabled = enabled
	settings.EmbeddingDimensions = 4
	settings.MinSimilarity = 0.1
	settings.TopK = 3
	settings.MaxResultChars = 40
	svc := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store, objects, embedder, enqueue, settings)
	return svc, store, objects, embedder, enqueue
}

func TestCreateProcessRetrieveAndDelete(t *testing.T) {
	svc, store, _, _, enqueue := testService(true)
	ctx := context.Background()
	biz := uuid.New()
	other := uuid.New()

	doc, err := svc.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Cancellation Policy", Description: ptr("How to cancel"), SourceType: SourceText,
		Content:  []byte("Guests may cancel free of charge until 17:00. After that a fee applies."),
		Filename: "policy.txt", MimeType: "text/plain",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if doc.Status != StatusPending || enqueue.calls != 1 || enqueue.lastD != doc.ID {
		t.Fatalf("create state = %+v enqueue=%d", doc, enqueue.calls)
	}

	if err := svc.ProcessDocument(ctx, biz, doc.ID); err != nil {
		t.Fatalf("process: %v", err)
	}
	ready, err := svc.GetDocument(ctx, biz, doc.ID)
	if err != nil || ready.Status != StatusReady {
		t.Fatalf("ready = %+v err=%v", ready, err)
	}
	n, err := store.CountChunks(ctx, biz, doc.ID)
	if err != nil || n < 1 {
		t.Fatalf("chunks = %d err=%v", n, err)
	}

	updated, err := svc.UpdateDocument(ctx, biz, doc.ID, UpdateDocumentInput{Title: ptr("Updated Policy")})
	if err != nil || updated.Title != "Updated Policy" || updated.Status != StatusReady {
		t.Fatalf("update re-embedded? %+v err=%v", updated, err)
	}

	found, err := svc.SearchKnowledge(ctx, biz, "cancel fee", 5)
	if err != nil || !found.Available || len(found.Results) == 0 {
		t.Fatalf("search = %+v err=%v", found, err)
	}
	if _, err := svc.GetDocument(ctx, other, doc.ID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("cross tenant get = %v", err)
	}
	foreign, err := svc.SearchKnowledge(ctx, other, "cancel fee", 5)
	if err != nil || len(foreign.Results) != 0 {
		t.Fatalf("cross tenant search = %+v err=%v", foreign, err)
	}

	if err := svc.DeleteDocument(ctx, biz, doc.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.GetDocument(ctx, biz, doc.ID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("deleted still visible = %v", err)
	}
}

func TestProcessIdempotentAndDisabled(t *testing.T) {
	svc, _, _, embedder, _ := testService(true)
	ctx := context.Background()
	biz := uuid.New()
	doc, err := svc.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Hours", SourceType: SourceText, Content: []byte("We are open every day from 08:00 to 22:00."), Filename: "hours.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDocument(ctx, biz, doc.ID); err != nil {
		t.Fatal(err)
	}
	calls := embedder.calls
	if err := svc.ProcessDocument(ctx, biz, doc.ID); err != nil {
		t.Fatal(err)
	}
	if embedder.calls != calls {
		t.Fatalf("ready document was re-embedded, calls %d -> %d", calls, embedder.calls)
	}

	disabled, _, _, _, _ := testService(false)
	if _, err := disabled.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "FAQ", SourceType: SourceText, Content: []byte("Parking is available behind the shop."), Filename: "faq.txt",
	}); err != nil {
		t.Fatalf("disabled create: %v", err)
	}
}

func TestProcessPermanentAndRetryable(t *testing.T) {
	svc, store, _, embedder, _ := testService(true)
	ctx := context.Background()
	biz := uuid.New()
	doc, err := svc.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Empty", SourceType: SourceText, Content: []byte("visible"), Filename: "ok.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	embedder.err = embedding.ErrRequest
	if err := svc.ProcessDocument(ctx, biz, doc.ID); !errors.Is(err, ErrEmbeddingFailed) {
		t.Fatalf("retryable = %v", err)
	}
	pending, _ := store.GetDocument(ctx, biz, doc.ID)
	if pending.Status != StatusPending {
		t.Fatalf("retryable should release to pending, got %s", pending.Status)
	}

	empty, err := svc.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Whitespace", SourceType: SourceText, Content: []byte("x"), Filename: "empty.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	empty.StoragePath = pending.StoragePath
	if _, err := store.UpdateDocumentStorage(ctx, biz, empty.ID, "missing/path.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	embedder.err = nil
	if err := svc.ProcessDocument(ctx, biz, empty.ID); !errors.Is(err, ErrDocumentEmpty) && !errors.Is(err, ErrStorageFailed) {
		t.Fatalf("missing source = %v", err)
	}

	disabled := NewService(slog.New(slog.NewTextHandler(io.Discard, nil)), store, newMemoryObjects(), &stubEmbedder{dim: 4}, nil, DefaultSettings())
	doc2, err := disabled.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Policy", SourceType: SourceText, Content: []byte("No pets allowed in the dining room."), Filename: "pets.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := disabled.ProcessDocument(ctx, biz, doc2.ID); !errors.Is(err, ErrEmbeddingNotConfigured) {
		t.Fatalf("disabled process = %v", err)
	}
	failed, _ := store.GetDocument(ctx, biz, doc2.ID)
	if failed.Status != StatusFailed {
		t.Fatalf("disabled status = %s", failed.Status)
	}

	unavailable, err := disabled.SearchKnowledge(ctx, biz, "pets", 3)
	if err != nil || unavailable.Available {
		t.Fatalf("disabled search = %+v err=%v", unavailable, err)
	}
}

func TestReprocessReplacesChunks(t *testing.T) {
	svc, store, objects, _, enqueue := testService(true)
	ctx := context.Background()
	biz := uuid.New()
	doc, err := svc.CreateDocument(ctx, biz, CreateDocumentInput{
		Title: "Menu notes", SourceType: SourceText, Content: []byte("The signature drink is not on the printed menu."), Filename: "notes.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ProcessDocument(ctx, biz, doc.ID); err != nil {
		t.Fatal(err)
	}
	if err := objects.Upload(ctx, *mustPath(store, biz, doc.ID), "text/plain", bytes.NewReader([]byte("The signature drink is now listed as seasonal."))); err != nil {
		t.Fatal(err)
	}
	re, err := svc.ReprocessDocument(ctx, biz, doc.ID, nil)
	if err != nil || re.Status != StatusPending || re.Version < 2 || enqueue.calls < 2 {
		t.Fatalf("reprocess = %+v err=%v calls=%d", re, err, enqueue.calls)
	}
	if err := svc.ProcessDocument(ctx, biz, doc.ID); err != nil {
		t.Fatal(err)
	}
	n, _ := store.CountChunks(ctx, biz, doc.ID)
	if n < 1 {
		t.Fatal("reprocess left no chunks")
	}
	ready, _ := svc.GetDocument(ctx, biz, doc.ID)
	if ready.Status != StatusReady {
		t.Fatalf("status = %s", ready.Status)
	}
}

func mustPath(store *memoryStore, biz, id uuid.UUID) *string {
	doc, _ := store.GetDocument(context.Background(), biz, id)
	return doc.StoragePath
}
