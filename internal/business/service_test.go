package business

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/money"
	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestUpdateProfilePatchesOnlyProvidedFields(t *testing.T) {
	id := uuid.New()
	store := newMemoryStore(id)
	svc := NewService(store)
	desc := "Specialty coffee"
	phone := "08123456789"

	got, err := svc.UpdateProfile(context.Background(), id, UpdateProfileInput{
		Name:        ptr("Kasiko Coffee Jakarta"),
		Description: &desc,
		Phone:       &phone,
	})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if got.Name != "Kasiko Coffee Jakarta" {
		t.Errorf("name = %q", got.Name)
	}
	if got.Slug != "kasiko-coffee" {
		t.Errorf("slug changed to %q", got.Slug)
	}
	if got.Type != tenant.BusinessTypeFNB || got.Status != tenant.StatusActive {
		t.Errorf("identity fields changed: type=%s status=%s", got.Type, got.Status)
	}
	if got.Description == nil || *got.Description != desc {
		t.Errorf("description = %v", got.Description)
	}
	if got.Phone == nil || *got.Phone != phone {
		t.Errorf("phone = %v", got.Phone)
	}
	if got.Timezone != "Asia/Jakarta" {
		t.Errorf("timezone was overwritten: %q", got.Timezone)
	}
}

func TestUpdateProfileClearsOptionalField(t *testing.T) {
	id := uuid.New()
	store := newMemoryStore(id)
	empty := ""
	svc := NewService(store)

	if _, err := svc.UpdateProfile(context.Background(), id, UpdateProfileInput{Description: ptr("keep")}); err != nil {
		t.Fatalf("seed description: %v", err)
	}
	got, err := svc.UpdateProfile(context.Background(), id, UpdateProfileInput{Description: &empty})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if got.Description != nil {
		t.Errorf("description = %v, want nil", got.Description)
	}
}

func TestUpdateProfileValidation(t *testing.T) {
	id := uuid.New()
	svc := NewService(newMemoryStore(id))

	tests := []struct {
		name string
		in   UpdateProfileInput
		want string
	}{
		{name: "empty name", in: UpdateProfileInput{Name: ptr("  ")}, want: "name is required"},
		{name: "invalid email", in: UpdateProfileInput{Email: ptr("not-an-email")}, want: "email is invalid"},
		{name: "invalid url", in: UpdateProfileInput{WebsiteURL: ptr("kasiko")}, want: "website_url is invalid"},
		{name: "invalid logo scheme", in: UpdateProfileInput{LogoURL: ptr("ftp://files.example.com/logo.png")}, want: "logo_url is invalid"},
		{name: "invalid timezone", in: UpdateProfileInput{Timezone: ptr("Jakarta")}, want: "timezone is invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateProfile(context.Background(), id, tt.in)
			if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			got, _ := svc.Profile(context.Background(), id)
			if got.Name != "Kasiko Coffee" {
				t.Errorf("rejected update mutated the profile: %+v", got)
			}
		})
	}
}

func TestUpdateFNBSettingsPatchesOnlyProvidedFields(t *testing.T) {
	id := uuid.New()
	svc := NewService(newMemoryStore(id))
	slot := 60
	maxParty := 10

	got, err := svc.UpdateFNBSettings(context.Background(), id, UpdateFNBSettingsInput{
		ReservationSlotDurationMinutes: &slot,
		ReservationMaxPartySize:        &maxParty,
	})
	if err != nil {
		t.Fatalf("UpdateFNBSettings() error = %v", err)
	}
	if got.ReservationSlotDurationMinutes != 60 || got.ReservationMaxPartySize != 10 {
		t.Errorf("patched fields = %+v", got)
	}
	if got.Currency != "IDR" || got.ReservationMinPartySize != 1 || got.ReservationMaxAdvanceDays != 30 {
		t.Errorf("unspecified fields changed: %+v", got)
	}
	if got.DepositEnabled || got.DepositType != depositTypeFixed {
		t.Errorf("deposit defaults changed: %+v", got)
	}
}

func TestUpdateFNBSettingsValidation(t *testing.T) {
	id := uuid.New()
	svc := NewService(newMemoryStore(id))

	tests := []struct {
		name string
		in   UpdateFNBSettingsInput
		want string
	}{
		{name: "currency", in: UpdateFNBSettingsInput{Currency: ptr("id")}, want: "currency is invalid"},
		{name: "slot", in: UpdateFNBSettingsInput{ReservationSlotDurationMinutes: ptrInt(0)}, want: "reservation_slot_duration_minutes"},
		{name: "min party", in: UpdateFNBSettingsInput{ReservationMinPartySize: ptrInt(0)}, want: "reservation_min_party_size"},
		{name: "max < min", in: UpdateFNBSettingsInput{ReservationMinPartySize: ptrInt(20), ReservationMaxPartySize: ptrInt(10)}, want: "reservation_max_party_size"},
		{name: "negative advance", in: UpdateFNBSettingsInput{ReservationMinAdvanceMinutes: ptrInt(-1)}, want: "reservation_min_advance_minutes"},
		{name: "max days", in: UpdateFNBSettingsInput{ReservationMaxAdvanceDays: ptrInt(0)}, want: "reservation_max_advance_days"},
		{name: "deposit type", in: UpdateFNBSettingsInput{DepositType: ptr("SPLIT")}, want: "deposit_type is invalid"},
		{name: "percentage range", in: UpdateFNBSettingsInput{DepositType: ptr("PERCENTAGE"), DepositValue: ptrAmount(money.FromMajor(101))}, want: "deposit_value must be at most 100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UpdateFNBSettings(context.Background(), id, tt.in)
			if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestServiceUsesCallerBusinessID(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	store := newMemoryStore(a)
	store.profiles[b] = &Profile{ID: b, Name: "Other", Slug: "other", Type: tenant.BusinessTypeFNB, Timezone: "Asia/Makassar", Status: tenant.StatusActive}
	store.settings[b] = defaultSettings(b)
	svc := NewService(store)

	got, err := svc.Profile(context.Background(), b)
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if got.ID != b || got.Name != "Other" {
		t.Errorf("Profile() returned %+v", got)
	}

	updated, err := svc.UpdateProfile(context.Background(), a, UpdateProfileInput{Name: ptr("Only A")})
	if err != nil {
		t.Fatalf("UpdateProfile() error = %v", err)
	}
	if updated.ID != a {
		t.Errorf("updated id = %s, want %s", updated.ID, a)
	}
	if store.profiles[b].Name != "Other" {
		t.Error("update leaked across businesses")
	}
}

type memoryStore struct {
	profiles map[uuid.UUID]*Profile
	settings map[uuid.UUID]*FNBSettings
}

func newMemoryStore(id uuid.UUID) *memoryStore {
	return &memoryStore{
		profiles: map[uuid.UUID]*Profile{
			id: {
				ID:       id,
				Name:     "Kasiko Coffee",
				Slug:     "kasiko-coffee",
				Type:     tenant.BusinessTypeFNB,
				Timezone: "Asia/Jakarta",
				Status:   tenant.StatusActive,
			},
		},
		settings: map[uuid.UUID]*FNBSettings{
			id: defaultSettings(id),
		},
	}
}

func defaultSettings(id uuid.UUID) *FNBSettings {
	return &FNBSettings{
		BusinessID:                     id,
		Currency:                       defaultCurrency,
		ReservationEnabled:             true,
		ReservationSlotDurationMinutes: defaultReservationSlotMinutes,
		ReservationMinPartySize:        defaultReservationMinPartySize,
		ReservationMaxPartySize:        defaultReservationMaxPartySize,
		ReservationMinAdvanceMinutes:   defaultReservationMinAdvanceMinutes,
		ReservationMaxAdvanceDays:      defaultReservationMaxAdvanceDays,
		DepositEnabled:                 false,
		DepositType:                    depositTypeFixed,
		DepositValue:                   0,
	}
}

func (m *memoryStore) GetProfile(_ context.Context, id uuid.UUID) (*Profile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *p
	return &copy, nil
}

func (m *memoryStore) UpdateProfile(_ context.Context, id uuid.UUID, profile Profile) (*Profile, error) {
	if _, ok := m.profiles[id]; !ok {
		return nil, ErrNotFound
	}
	copy := profile
	copy.ID = id
	m.profiles[id] = &copy
	return &copy, nil
}

func (m *memoryStore) GetFNBSettings(_ context.Context, id uuid.UUID) (*FNBSettings, error) {
	s, ok := m.settings[id]
	if !ok {
		return nil, ErrNotFound
	}
	copy := *s
	return &copy, nil
}

func (m *memoryStore) UpdateFNBSettings(_ context.Context, id uuid.UUID, settings FNBSettings) (*FNBSettings, error) {
	if _, ok := m.settings[id]; !ok {
		return nil, ErrNotFound
	}
	copy := settings
	copy.BusinessID = id
	m.settings[id] = &copy
	return &copy, nil
}

func ptr(v string) *string                   { return &v }
func ptrInt(v int) *int                      { return &v }
func ptrAmount(v money.Amount) *money.Amount { return &v }
