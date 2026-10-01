package calendar

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

type memoryStore struct {
	mu           sync.Mutex
	timezone     map[uuid.UUID]string
	hours        map[uuid.UUID][]Hour
	exceptions   map[uuid.UUID][]ScheduleException
	areas        map[uuid.UUID]uuid.UUID
	areaNames    map[uuid.UUID]string
	tables       map[uuid.UUID]uuid.UUID
	tableNames   map[uuid.UUID]string
	tableAreas   map[uuid.UUID]uuid.UUID
	blocks       map[uuid.UUID]*Block
	reservations []ReservationEvent
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		timezone:   map[uuid.UUID]string{},
		hours:      map[uuid.UUID][]Hour{},
		exceptions: map[uuid.UUID][]ScheduleException{},
		areas:      map[uuid.UUID]uuid.UUID{},
		areaNames:  map[uuid.UUID]string{},
		tables:     map[uuid.UUID]uuid.UUID{},
		tableNames: map[uuid.UUID]string{},
		tableAreas: map[uuid.UUID]uuid.UUID{},
		blocks:     map[uuid.UUID]*Block{},
	}
}

func (m *memoryStore) seedBusiness(id uuid.UUID) {
	m.timezone[id] = "Asia/Jakarta"
	m.hours[id] = []Hour{{DayOfWeek: 4, OpenTime: "09:00", CloseTime: "22:00"}}
}

func (m *memoryStore) addArea(businessID uuid.UUID, name string) uuid.UUID {
	id := uuid.New()
	m.areas[id] = businessID
	m.areaNames[id] = name
	return id
}

func (m *memoryStore) addTable(businessID, areaID uuid.UUID, name string) uuid.UUID {
	id := uuid.New()
	m.tables[id] = businessID
	m.tableNames[id] = name
	m.tableAreas[id] = areaID
	return id
}

func (m *memoryStore) Timezone(_ context.Context, businessID uuid.UUID) (string, error) {
	return m.timezone[businessID], nil
}

func (m *memoryStore) ListHours(_ context.Context, businessID uuid.UUID) ([]Hour, error) {
	return append([]Hour{}, m.hours[businessID]...), nil
}

func (m *memoryStore) ListExceptions(_ context.Context, businessID uuid.UUID) ([]ScheduleException, error) {
	return append([]ScheduleException{}, m.exceptions[businessID]...), nil
}

func (m *memoryStore) ResolveScope(_ context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID) (string, string, error) {
	if areaID == nil && tableID == nil {
		return "", "", nil
	}
	if tableID != nil && areaID == nil {
		return "", "", ErrInvalidScope
	}
	if areaID != nil && m.areas[*areaID] != businessID {
		return "", "", ErrAreaMissing
	}
	if tableID != nil {
		if m.tables[*tableID] != businessID || m.tableAreas[*tableID] != *areaID {
			return "", "", ErrTableMissing
		}
		return m.areaNames[*areaID], m.tableNames[*tableID], nil
	}
	return m.areaNames[*areaID], "", nil
}

func (m *memoryStore) GetBlock(_ context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blocks[blockID]
	if !ok || b.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *b
	return &copy, nil
}

func (m *memoryStore) InsertBlock(_ context.Context, b Block) (*Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.blocks {
		if existing.BusinessID != b.BusinessID || existing.Status != StatusActive {
			continue
		}
		if sameScope(*existing, b) && overlaps(existing.StartAt, existing.EndAt, b.StartAt, b.EndAt) {
			return nil, ErrConflict
		}
	}
	now := time.Now().UTC()
	b.ID = uuid.New()
	b.CreatedAt = now
	b.UpdatedAt = now
	m.blocks[b.ID] = &b
	copy := b
	return &copy, nil
}

func (m *memoryStore) UpdateBlock(_ context.Context, businessID uuid.UUID, b Block) (*Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.blocks[b.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrNotFound
	}
	current.Title = b.Title
	current.Description = b.Description
	current.StartAt = b.StartAt
	current.EndAt = b.EndAt
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	return &copy, nil
}

func (m *memoryStore) CancelBlock(_ context.Context, businessID, blockID uuid.UUID) (*Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blocks[blockID]
	if !ok || b.BusinessID != businessID {
		return nil, ErrNotFound
	}
	b.Status = StatusCancelled
	b.UpdatedAt = time.Now().UTC()
	copy := *b
	return &copy, nil
}

func (m *memoryStore) ListBlocks(_ context.Context, businessID uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Block, 0)
	for _, b := range m.blocks {
		if b.BusinessID != businessID || !overlaps(b.StartAt, b.EndAt, from, to) {
			continue
		}
		if status != "" && b.Status != status {
			continue
		}
		if areaID != nil && (b.AreaID == nil || *b.AreaID != *areaID) && (b.TableID == nil || m.tableAreas[*b.TableID] != *areaID) {
			continue
		}
		if tableID != nil && (b.TableID == nil || *b.TableID != *tableID) {
			continue
		}
		copy := *b
		out = append(out, copy)
	}
	return out, nil
}

func (m *memoryStore) OverlappingReservations(_ context.Context, businessID uuid.UUID, areaID, tableID *uuid.UUID, start, end time.Time) ([]ReservationConflict, error) {
	out := make([]ReservationConflict, 0)
	for _, r := range m.reservations {
		if !overlaps(r.StartAt, r.EndAt, start, end) || (r.Status != "PENDING" && r.Status != "CONFIRMED") {
			continue
		}
		if tableID != nil && (r.TableID == nil || *r.TableID != *tableID) {
			continue
		}
		if tableID == nil && areaID != nil && (r.AreaID == nil || *r.AreaID != *areaID) {
			continue
		}
		out = append(out, ReservationConflict{ReservationID: r.ID, StartAt: r.StartAt, EndAt: r.EndAt, Status: r.Status})
	}
	_ = businessID
	return out, nil
}

func (m *memoryStore) ListReservationEvents(_ context.Context, _ uuid.UUID, from, to time.Time, areaID, tableID *uuid.UUID, status string) ([]ReservationEvent, error) {
	out := make([]ReservationEvent, 0)
	for _, r := range m.reservations {
		if !overlaps(r.StartAt, r.EndAt, from, to) {
			continue
		}
		if status != "" && r.Status != status {
			continue
		}
		if status == "" && r.Status == "CANCELLED" {
			continue
		}
		if areaID != nil && (r.AreaID == nil || *r.AreaID != *areaID) {
			continue
		}
		if tableID != nil && (r.TableID == nil || *r.TableID != *tableID) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func sameScope(a, b Block) bool {
	return eqID(a.AreaID, b.AreaID) && eqID(a.TableID, b.TableID)
}

func eqID(a, b *uuid.UUID) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func overlaps(aStart, aEnd, bStart, bEnd time.Time) bool {
	return aStart.Before(bEnd) && bStart.Before(aEnd)
}
