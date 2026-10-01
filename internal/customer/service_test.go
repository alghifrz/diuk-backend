package customer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestCustomerValidation(t *testing.T) {
	svc := NewService(newMemoryStore())
	biz := uuid.New()

	got, err := svc.CreateCustomer(context.Background(), biz, CreateCustomerInput{
		Name: "  John Doe  ", Phone: ptr(" +628123456789 "), Email: ptr("  John@Example.COM "), Notes: ptr(" Regular "),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.Name != "John Doe" || got.Email == nil || *got.Email != "john@example.com" || got.Phone == nil || *got.Phone != "+628123456789" {
		t.Errorf("normalized = %+v", got)
	}

	tests := []struct {
		name string
		in   CreateCustomerInput
		want error
	}{
		{name: "empty", in: CreateCustomerInput{Name: "  "}, want: ErrInvalid},
		{name: "too long", in: CreateCustomerInput{Name: strings.Repeat("a", 151)}, want: ErrInvalid},
		{name: "bad email", in: CreateCustomerInput{Name: "A", Email: ptr("not-an-email")}, want: ErrInvalid},
		{name: "long phone", in: CreateCustomerInput{Name: "A", Phone: ptr(strings.Repeat("1", 31))}, want: ErrInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCustomer(context.Background(), biz, tt.in)
			if err == nil || !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestCustomerCRUDAndSearch(t *testing.T) {
	svc := NewService(newMemoryStore())
	biz := uuid.New()
	created, err := svc.CreateCustomer(context.Background(), biz, CreateCustomerInput{Name: "Jane", Phone: ptr("0811"), Email: ptr("jane@example.com")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.GetCustomer(context.Background(), biz, created.ID)
	if err != nil || got.Name != "Jane" {
		t.Fatalf("get = %+v err=%v", got, err)
	}
	updated, err := svc.UpdateCustomer(context.Background(), biz, created.ID, UpdateCustomerInput{Notes: ptr("VIP")})
	if err != nil || updated.Name != "Jane" || updated.Notes == nil || *updated.Notes != "VIP" {
		t.Fatalf("patch omitted: %+v err=%v", updated, err)
	}
	cleared, err := svc.UpdateCustomer(context.Background(), biz, created.ID, UpdateCustomerInput{Notes: ptr("")})
	if err != nil || cleared.Notes != nil {
		t.Fatalf("clear notes = %+v err=%v", cleared, err)
	}

	listed, err := svc.ListCustomers(context.Background(), biz, "", "jane", "", "")
	if err != nil || len(listed) != 1 {
		t.Fatalf("search = %d err=%v", len(listed), err)
	}
	deactivated, err := svc.DeleteCustomer(context.Background(), biz, created.ID)
	if err != nil || deactivated.Status != tenant.StatusInactive {
		t.Fatalf("delete = %+v err=%v", deactivated, err)
	}
	again, err := svc.DeleteCustomer(context.Background(), biz, created.ID)
	if err != nil || again.Status != tenant.StatusInactive {
		t.Fatalf("idempotent delete = %+v err=%v", again, err)
	}
	active, _ := svc.ListCustomers(context.Background(), biz, "", "", "", "")
	if len(active) != 0 {
		t.Errorf("active list = %d", len(active))
	}
}

func TestIdentityRules(t *testing.T) {
	svc := NewService(newMemoryStore())
	biz := uuid.New()
	cust, _ := svc.CreateCustomer(context.Background(), biz, CreateCustomerInput{Name: "John"})

	id, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "whatsapp", ExternalID: "  628123  ", DisplayName: ptr(" John ")})
	if err != nil {
		t.Fatalf("create identity: %v", err)
	}
	if id.Channel != ChannelWhatsApp || id.ExternalID != "628123" || id.DisplayName == nil || *id.DisplayName != "John" {
		t.Errorf("identity = %+v", id)
	}
	if _, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "628123"}); !errors.Is(err, ErrIdentityExists) {
		t.Fatalf("duplicate = %v", err)
	}
	if _, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "SMS", ExternalID: "1"}); !errors.Is(err, ErrInvalidChannel) {
		t.Fatalf("channel = %v", err)
	}
	if _, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "WEBSITE", ExternalID: ""}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty external = %v", err)
	}
	if _, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "WEBSITE", ExternalID: strings.Repeat("x", 256)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long external = %v", err)
	}

	patched, err := svc.UpdateIdentity(context.Background(), biz, cust.ID, id.ID, UpdateIdentityInput{DisplayName: ptr("Johnny")})
	if err != nil || patched.ExternalID != "628123" || patched.DisplayName == nil || *patched.DisplayName != "Johnny" {
		t.Fatalf("patch = %+v err=%v", patched, err)
	}
	off, err := svc.DeleteIdentity(context.Background(), biz, cust.ID, id.ID)
	if err != nil || off.Status != tenant.StatusInactive {
		t.Fatalf("soft delete identity = %+v err=%v", off, err)
	}

	if _, err := svc.DeleteCustomer(context.Background(), biz, cust.ID); err != nil {
		t.Fatalf("deactivate customer: %v", err)
	}
	if _, err := svc.CreateIdentity(context.Background(), biz, cust.ID, CreateIdentityInput{Channel: "WEBSITE", ExternalID: "web-1"}); !errors.Is(err, ErrCustomerInactive) {
		t.Fatalf("inactive customer identity = %v", err)
	}
}

func TestTagAndAssignmentRules(t *testing.T) {
	svc := NewService(newMemoryStore())
	biz := uuid.New()
	cust, _ := svc.CreateCustomer(context.Background(), biz, CreateCustomerInput{Name: "John"})
	tag, err := svc.CreateTag(context.Background(), biz, CreateTagInput{Name: "  VIP  ", Description: ptr(" High value ")})
	if err != nil || tag.Name != "VIP" {
		t.Fatalf("create tag = %+v err=%v", tag, err)
	}
	if _, err := svc.CreateTag(context.Background(), biz, CreateTagInput{Name: "VIP"}); !errors.Is(err, ErrTagNameExists) {
		t.Fatalf("duplicate tag = %v", err)
	}
	if _, err := svc.CreateTag(context.Background(), biz, CreateTagInput{Name: strings.Repeat("t", 51)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("long tag = %v", err)
	}

	assigned, created, err := svc.AssignTag(context.Background(), biz, cust.ID, tag.ID)
	if err != nil || !created || assigned.ID != tag.ID {
		t.Fatalf("assign = %+v created=%v err=%v", assigned, created, err)
	}
	_, created, err = svc.AssignTag(context.Background(), biz, cust.ID, tag.ID)
	if err != nil || created {
		t.Fatalf("idempotent assign created=%v err=%v", created, err)
	}
	tags, err := svc.ListCustomerTags(context.Background(), biz, cust.ID)
	if err != nil || len(tags) != 1 {
		t.Fatalf("customer tags = %d err=%v", len(tags), err)
	}

	if _, err := svc.DeleteTag(context.Background(), biz, tag.ID); err != nil {
		t.Fatalf("soft delete tag: %v", err)
	}
	other, _ := svc.CreateCustomer(context.Background(), biz, CreateCustomerInput{Name: "Other"})
	if _, _, err := svc.AssignTag(context.Background(), biz, other.ID, tag.ID); !errors.Is(err, ErrTagInactive) {
		t.Fatalf("inactive tag assign = %v", err)
	}

	if err := svc.UnassignTag(context.Background(), biz, cust.ID, tag.ID); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if err := svc.UnassignTag(context.Background(), biz, cust.ID, tag.ID); err != nil {
		t.Fatalf("idempotent unassign: %v", err)
	}

	fresh, _ := svc.CreateTag(context.Background(), biz, CreateTagInput{Name: "New"})
	if _, err := svc.DeleteCustomer(context.Background(), biz, cust.ID); err != nil {
		t.Fatalf("deactivate customer: %v", err)
	}
	if _, _, err := svc.AssignTag(context.Background(), biz, cust.ID, fresh.ID); !errors.Is(err, ErrCustomerInactive) {
		t.Fatalf("assign inactive customer = %v", err)
	}
}

type recordingCustomerEvents struct {
	count int
	last  *Customer
}

func (r *recordingCustomerEvents) CustomerCreated(_ context.Context, cust *Customer) error {
	r.count++
	r.last = cust
	return nil
}

func TestEnsureIdentityPublishesOnce(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store)
	events := &recordingCustomerEvents{}
	svc.SetEvents(events)
	biz := uuid.New()

	first, created, err := svc.EnsureIdentity(context.Background(), biz, EnsureIdentityInput{
		Channel: "WHATSAPP", ExternalID: "628111", Name: "Alghifif", Phone: ptr("628111"),
	})
	if err != nil || !created || first == nil {
		t.Fatalf("first = %+v created=%v err=%v", first, created, err)
	}
	if events.count != 1 {
		t.Fatalf("events after first = %d", events.count)
	}

	second, created, err := svc.EnsureIdentity(context.Background(), biz, EnsureIdentityInput{
		Channel: "WHATSAPP", ExternalID: "628111", Name: "Alghifif", Phone: ptr("628111"),
	})
	if err != nil || created || second == nil || second.ID != first.ID {
		t.Fatalf("second = %+v created=%v err=%v", second, created, err)
	}
	if events.count != 1 {
		t.Fatalf("events after reuse = %d", events.count)
	}
}

func TestCrossTenantIsolation(t *testing.T) {
	svc := NewService(newMemoryStore())
	a, b := uuid.New(), uuid.New()
	custA, _ := svc.CreateCustomer(context.Background(), a, CreateCustomerInput{Name: "A"})
	tagB, _ := svc.CreateTag(context.Background(), b, CreateTagInput{Name: "VIP"})
	if _, err := svc.GetCustomer(context.Background(), b, custA.ID); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("get A as B = %v", err)
	}
	if _, _, err := svc.AssignTag(context.Background(), a, custA.ID, tagB.ID); !errors.Is(err, ErrTagNotFound) {
		t.Fatalf("cross tag assign = %v", err)
	}
	if _, err := svc.CreateIdentity(context.Background(), b, custA.ID, CreateIdentityInput{Channel: "WHATSAPP", ExternalID: "1"}); !errors.Is(err, ErrCustomerNotFound) {
		t.Fatalf("identity on foreign customer = %v", err)
	}
}
