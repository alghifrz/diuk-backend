package conversation

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/alghifrz/diuk-backend/internal/customer"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestRepositoryConversationFoundation(t *testing.T) {
	tx := newTx(t)
	ctx := context.Background()
	bizRepo := tenant.NewBusinessRepository(tx)
	userRepo := tenant.NewUserRepository(tx)
	custSvc := customer.NewService(customer.NewRepository(tx))
	svc := NewService(NewRepository(tx), userRepo, tenant.NewBusinessUserRepository(tx))

	a, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Conv A", Slug: "conv-a-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Jakarta",
	})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := bizRepo.Create(ctx, tenant.CreateBusinessParams{
		Name: "Conv B", Slug: "conv-b-" + uuid.NewString()[:8], Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar",
	})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	custA, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "John", Phone: ptr("0811")})
	if err != nil {
		t.Fatalf("customer A: %v", err)
	}
	custA2, err := custSvc.CreateCustomer(ctx, a.ID, customer.CreateCustomerInput{Name: "Pat"})
	if err != nil {
		t.Fatalf("customer A2: %v", err)
	}
	custB, err := custSvc.CreateCustomer(ctx, b.ID, customer.CreateCustomerInput{Name: "John"})
	if err != nil {
		t.Fatalf("customer B: %v", err)
	}
	identA, err := custSvc.CreateIdentity(ctx, a.ID, custA.ID, customer.CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "6281"})
	if err != nil {
		t.Fatalf("identity A: %v", err)
	}
	identB, err := custSvc.CreateIdentity(ctx, b.ID, custB.ID, customer.CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "6281"})
	if err != nil {
		t.Fatalf("identity B: %v", err)
	}

	conv, created, err := svc.CreateConversation(ctx, a.ID, CreateConversationInput{
		CustomerID: custA.ID.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(identA.ID.String()),
	})
	if err != nil || !created {
		t.Fatalf("create conversation: created=%v err=%v", created, err)
	}
	if conv.HandlingState != HandlingAIActive {
		t.Fatalf("new conversation handling = %s", conv.HandlingState)
	}
	reused, created, err := svc.CreateConversation(ctx, a.ID, CreateConversationInput{
		CustomerID: custA.ID.String(), Channel: "WHATSAPP", CustomerIdentityID: ptr(identA.ID.String()),
	})
	if err != nil || created || reused.ID != conv.ID {
		t.Fatalf("reuse open: created=%v err=%v", created, err)
	}
	if _, err := svc.GetConversation(ctx, b.ID, conv.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get = %v", err)
	}

	msg, created, err := svc.CreateMessage(ctx, a.ID, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("halo"), ExternalID: ptr("wa-100"),
	})
	if err != nil || !created {
		t.Fatalf("message: created=%v err=%v", created, err)
	}
	again, created, err := svc.CreateMessage(ctx, a.ID, conv.ID, CreateMessageInput{
		Direction: "INBOUND", SenderType: "CUSTOMER", Content: ptr("halo"), ExternalID: ptr("wa-100"),
	})
	if err != nil || created || again.ID != msg.ID {
		t.Fatalf("duplicate external: created=%v err=%v", created, err)
	}

	convB, created, err := svc.CreateConversation(ctx, b.ID, CreateConversationInput{
		CustomerID: custB.ID.String(), Channel: "WEBSITE",
	})
	if err != nil || !created {
		t.Fatalf("conversation B: created=%v err=%v", created, err)
	}

	t.Run("reject conversation business A + customer B", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO conversations (business_id, customer_id, channel)
			VALUES ($1, $2, 'WEBSITE')`, a.ID, custB.ID)
	})
	t.Run("reject conversation business A + identity B", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO conversations (business_id, customer_id, channel, customer_identity_id)
			VALUES ($1, $2, 'WHATSAPP', $3)`, a.ID, custA.ID, identB.ID)
	})
	t.Run("reject conversation identity belonging to another customer", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO conversations (business_id, customer_id, channel, customer_identity_id)
			VALUES ($1, $2, 'WHATSAPP', $3)`, a.ID, custA2.ID, identA.ID)
	})
	t.Run("reject message business A + conversation B", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, content)
			VALUES ($1, $2, 'INBOUND', 'CUSTOMER', 'x')`, a.ID, convB.ID)
	})
	t.Run("reject invalid conversation status", func(t *testing.T) {
		mustReject(t, tx, `UPDATE conversations SET status = 'ARCHIVED' WHERE id = $1`, conv.ID)
	})
	t.Run("reject invalid handling state", func(t *testing.T) {
		mustReject(t, tx, `UPDATE conversations SET handling_state = 'ASSIGNED' WHERE id = $1`, conv.ID)
	})
	t.Run("accept internal note", func(t *testing.T) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, message_type, status, content)
			VALUES ($1, $2, 'OUTBOUND', 'USER', 'INTERNAL_NOTE', 'INTERNAL', 'private')`, a.ID, conv.ID); err != nil {
			t.Fatalf("internal note insert: %v", err)
		}
	})
	t.Run("reject invalid conversation channel", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO conversations (business_id, customer_id, channel)
			VALUES ($1, $2, 'SMS')`, a.ID, custA.ID)
	})
	t.Run("reject invalid message direction", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, content)
			VALUES ($1, $2, 'SIDEWAYS', 'CUSTOMER', 'x')`, a.ID, conv.ID)
	})
	t.Run("reject invalid sender type", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, content)
			VALUES ($1, $2, 'INBOUND', 'BOT', 'x')`, a.ID, conv.ID)
	})
	t.Run("reject invalid message type", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, message_type, content)
			VALUES ($1, $2, 'INBOUND', 'CUSTOMER', 'TEMPLATE', 'x')`, a.ID, conv.ID)
	})
	t.Run("reject invalid message status", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, status, content)
			VALUES ($1, $2, 'INBOUND', 'CUSTOMER', 'PENDING', 'x')`, a.ID, conv.ID)
	})
	t.Run("reject duplicate open conversation", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO conversations (business_id, customer_id, channel, customer_identity_id, status)
			VALUES ($1, $2, 'WHATSAPP', $3, 'OPEN')`, a.ID, custA.ID, identA.ID)
	})
	t.Run("reject duplicate external message id", func(t *testing.T) {
		mustReject(t, tx, `
			INSERT INTO messages (business_id, conversation_id, direction, sender_type, content, external_id)
			VALUES ($1, $2, 'INBOUND', 'CUSTOMER', 'x', 'wa-100')`, a.ID, conv.ID)
	})
}

func mustReject(t *testing.T, tx pgx.Tx, query string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if _, err := tx.Exec(ctx, "SAVEPOINT reject_check"); err != nil {
		t.Fatalf("savepoint: %v", err)
	}
	_, err := tx.Exec(ctx, query, args...)
	if _, rbErr := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT reject_check"); rbErr != nil {
		t.Fatalf("rollback savepoint: %v", rbErr)
	}
	if err == nil {
		t.Fatal("expected constraint rejection")
	}
}

func newTx(t *testing.T) pgx.Tx {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping conversation integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	return tx
}
