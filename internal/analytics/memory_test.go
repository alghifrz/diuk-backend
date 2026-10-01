package analytics

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	overview      map[uuid.UUID]Overview
	conversations map[uuid.UUID]ConversationReport
	customers     map[uuid.UUID]CustomerReport
	reservations  map[uuid.UUID]ReservationReport
	conversion    map[uuid.UUID]ConversionReport
	revenue       map[uuid.UUID]PaymentReport
	ai            map[uuid.UUID]AIReport
	handover      map[uuid.UUID]HandoverReport
	automation    map[uuid.UUID]AutomationReport
	messaging     map[uuid.UUID]MessagingReport
	campaigns     map[uuid.UUID]CampaignReport
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		overview:      map[uuid.UUID]Overview{},
		conversations: map[uuid.UUID]ConversationReport{},
		customers:     map[uuid.UUID]CustomerReport{},
		reservations:  map[uuid.UUID]ReservationReport{},
		conversion:    map[uuid.UUID]ConversionReport{},
		revenue:       map[uuid.UUID]PaymentReport{},
		ai:            map[uuid.UUID]AIReport{},
		handover:      map[uuid.UUID]HandoverReport{},
		automation:    map[uuid.UUID]AutomationReport{},
		messaging:     map[uuid.UUID]MessagingReport{},
		campaigns:     map[uuid.UUID]CampaignReport{},
	}
}

func (m *memoryStore) Overview(_ context.Context, businessID uuid.UUID, _, _ time.Time) (Overview, error) {
	return m.overview[businessID], nil
}
func (m *memoryStore) Conversations(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (ConversationReport, error) {
	return m.conversations[businessID], nil
}
func (m *memoryStore) Customers(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (CustomerReport, error) {
	return m.customers[businessID], nil
}
func (m *memoryStore) Reservations(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (ReservationReport, error) {
	return m.reservations[businessID], nil
}
func (m *memoryStore) Conversion(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (ConversionReport, error) {
	return m.conversion[businessID], nil
}
func (m *memoryStore) Revenue(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (PaymentReport, error) {
	return m.revenue[businessID], nil
}
func (m *memoryStore) AI(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (AIReport, error) {
	return m.ai[businessID], nil
}
func (m *memoryStore) Handover(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (HandoverReport, error) {
	return m.handover[businessID], nil
}
func (m *memoryStore) Automation(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (AutomationReport, error) {
	return m.automation[businessID], nil
}
func (m *memoryStore) Messaging(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (MessagingReport, error) {
	return m.messaging[businessID], nil
}
func (m *memoryStore) Campaigns(_ context.Context, businessID uuid.UUID, _, _ time.Time, _ string) (CampaignReport, error) {
	return m.campaigns[businessID], nil
}

type stubTZ struct{ tz string }

func (s stubTZ) Timezone(context.Context, uuid.UUID) (string, error) { return s.tz, nil }
