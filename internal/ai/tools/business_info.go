package tools

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

type BusinessInfo struct {
	Business BusinessLookup
	// Hours is optional; when set, weekly opening hours are included.
	Hours HoursLookup
}

func (t BusinessInfo) Name() string { return "get_business_info" }

func (t BusinessInfo) Description() string {
	return "Retrieve customer-facing business profile and reservation settings. Use this for name, contact, timezone, and booking rules. Never invent this information."
}

func (t BusinessInfo) InputSchema() Schema {
	return objectSchema(map[string]Schema{}, nil)
}

func (t BusinessInfo) Execute(ctx context.Context, exec ExecContext, _ map[string]any) (Result, error) {
	if exec.BusinessID == uuid.Nil {
		return fail("AI_CONTEXT_ERROR", "business context is missing")
	}
	profile, err := t.Business.Profile(ctx, exec.BusinessID)
	if err != nil {
		return fail("AI_CONTEXT_ERROR", "business profile is unavailable")
	}
	settings, err := t.Business.FNBSettings(ctx, exec.BusinessID)
	if err != nil {
		return fail("AI_CONTEXT_ERROR", "business settings are unavailable")
	}
	nowLocal := time.Now().UTC()
	if loc, err := time.LoadLocation(profile.Timezone); err == nil {
		nowLocal = time.Now().In(loc)
	}
	openingHours := t.openingHours(ctx, exec.BusinessID)
	gaps := profileGaps(profile.Address, profile.Phone, len(openingHours) > 0)
	return success(map[string]any{
		"profile_gaps":       gaps,
		"profile_gaps_hint":  "Fields listed in profile_gaps are not set in the profile. Call search_knowledge for them before saying they are unavailable.",
		"name":               profile.Name,
		"description":        profile.Description,
		"phone":              profile.Phone,
		"email":              profile.Email,
		"address":            profile.Address,
		"website":            profile.WebsiteURL,
		"opening_hours":      openingHours,
		"timezone":           profile.Timezone,
		"current_local_date": nowLocal.Format("2006-01-02"),
		"current_local_time": nowLocal.Format("15:04"),
		"currency":           settings.Currency,
		"reservations": map[string]any{
			"enabled":             settings.ReservationEnabled,
			"slot_minutes":        settings.ReservationSlotDurationMinutes,
			"min_party_size":      settings.ReservationMinPartySize,
			"max_party_size":      settings.ReservationMaxPartySize,
			"min_advance_minutes": settings.ReservationMinAdvanceMinutes,
			"max_advance_days":    settings.ReservationMaxAdvanceDays,
			"deposit_enabled":     settings.DepositEnabled,
			"deposit_type":        settings.DepositType,
			"deposit_value":       settings.DepositValue,
			"bank_name":           settings.BankName,
			"bank_account_number": settings.BankAccountNumber,
			"bank_account_holder": settings.BankAccountHolder,
		},
	})
}

// openingHours renders the weekly schedule saved in settings as readable
// "Monday: 09:00-22:00" entries. Days with no schedule are marked closed.
func (t BusinessInfo) openingHours(ctx context.Context, businessID uuid.UUID) []string {
	if t.Hours == nil {
		return nil
	}
	hours, err := t.Hours.ListHours(ctx, businessID)
	if err != nil || len(hours) == 0 {
		return nil
	}
	byDay := make(map[int][]string, 7)
	for _, h := range hours {
		byDay[h.DayOfWeek] = append(byDay[h.DayOfWeek], trimClock(h.OpenTime)+"-"+trimClock(h.CloseTime))
	}
	out := make([]string, 0, 7)
	for d := 1; d <= 7; d++ {
		day := d % 7 // Monday first, Sunday last
		label := time.Weekday(day).String()
		if slots, ok := byDay[day]; ok {
			out = append(out, label+": "+strings.Join(slots, ", "))
		} else {
			out = append(out, label+": closed")
		}
	}
	return out
}

func trimClock(v string) string {
	if len(v) >= 5 {
		return v[:5]
	}
	return v
}

// profileGaps lists customer-facing facts that are missing from the saved
// profile and settings so the model falls back to the knowledge base instead
// of answering "unavailable".
func profileGaps(address, phone *string, hasHours bool) []string {
	gaps := []string{}
	if !hasHours {
		gaps = append(gaps, "opening_hours")
	}
	if address == nil || strings.TrimSpace(*address) == "" {
		gaps = append(gaps, "address")
	}
	if phone == nil || strings.TrimSpace(*phone) == "" {
		gaps = append(gaps, "phone")
	}
	return gaps
}
