package knowledge

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryKnowledge(t *testing.T) {
	tx := newKnowledgeTx(t)
	ctx := context.Background()
	repo := NewRepository(tx)
	bizRepo := tenant.NewBusinessRepository(tx)
	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "KB A", Slug: "kb-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "KB B", Slug: "kb-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	docA, err := repo.CreateDocument(ctx, &Document{BusinessID: a.ID, Title: "Policy A", SourceType: SourceText, Status: StatusPending})
	if err != nil {
		t.Fatalf("create doc A: %v", err)
	}
	docB, err := repo.CreateDocument(ctx, &Document{BusinessID: b.ID, Title: "Policy B", SourceType: SourceText, Status: StatusPending})
	if err != nil {
		t.Fatalf("create doc B: %v", err)
	}
	if _, err := repo.GetDocument(ctx, b.ID, docA.ID); !errors.Is(err, ErrDocumentNotFound) {
		t.Fatalf("cross tenant get = %v", err)
	}

	if _, err := repo.UpdateDocumentMeta(ctx, a.ID, docA.ID, "Updated A", nil, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	listed, err := repo.ListDocuments(ctx, a.ID, "")
	if err != nil || len(listed) != 1 || listed[0].Title != "Updated A" {
		t.Fatalf("list = %+v err=%v", listed, err)
	}

	vec := make([]float32, DefaultEmbeddingDimensions)
	vec[0] = 1
	if err := repo.ReplaceChunks(ctx, a.ID, docA.ID, []Chunk{{
		BusinessID: a.ID, DocumentID: docA.ID, ChunkIndex: 0, Content: "cancel free until 17:00", Embedding: vec,
	}}); err != nil {
		t.Fatalf("replace A: %v", err)
	}
	foreign := make([]float32, DefaultEmbeddingDimensions)
	foreign[1] = 1
	if err := repo.ReplaceChunks(ctx, b.ID, docB.ID, []Chunk{{
		BusinessID: b.ID, DocumentID: docB.ID, ChunkIndex: 0, Content: "other tenant secret", Embedding: foreign,
	}}); err != nil {
		t.Fatalf("replace B: %v", err)
	}
	if _, err := repo.MarkReady(ctx, a.ID, docA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.MarkReady(ctx, b.ID, docB.ID); err != nil {
		t.Fatal(err)
	}

	hits, err := repo.SearchChunks(ctx, a.ID, vec, 0.2, 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].DocumentID != docA.ID {
		t.Fatalf("tenant isolation hits = %+v", hits)
	}
	unrelated := make([]float32, DefaultEmbeddingDimensions)
	unrelated[len(unrelated)-1] = 1
	none, err := repo.SearchChunks(ctx, a.ID, unrelated, 0.5, 5)
	if err != nil || len(none) != 0 {
		t.Fatalf("threshold = %+v err=%v", none, err)
	}

	if err := repo.DeleteDocument(ctx, a.ID, docA.ID); err != nil {
		t.Fatal(err)
	}
	n, err := repo.CountChunks(ctx, a.ID, docA.ID)
	if err != nil || n != 0 {
		t.Fatalf("cascade chunks = %d err=%v", n, err)
	}
}

func newKnowledgeTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping knowledge integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector')`).Scan(&ok); err != nil || !ok {
		t.Skip("pgvector is not available; skipping knowledge integration tests")
	}
	var table string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('public.knowledge_documents')`).Scan(&table); err != nil || table == "" {
		t.Skip("knowledge tables are not migrated; skipping knowledge integration tests")
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
