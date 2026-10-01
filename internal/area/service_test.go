package area

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
)

func TestCreateAreaValidation(t *testing.T) {
	biz := uuid.New()
	svc := NewService(newMemoryStore())

	got, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "  Indoor  ", Description: ptr(" Main hall ")})
	if err != nil {
		t.Fatalf("CreateArea() error = %v", err)
	}
	if got.Name != "Indoor" || got.Status != tenant.StatusActive || got.BusinessID != biz {
		t.Errorf("created = %+v", got)
	}
	if got.Description == nil || *got.Description != "Main hall" {
		t.Errorf("description = %v", got.Description)
	}

	tests := []struct {
		name string
		in   CreateAreaInput
		want string
	}{
		{name: "empty", in: CreateAreaInput{Name: "   "}, want: "name is required"},
		{name: "too long", in: CreateAreaInput{Name: strings.Repeat("a", 101)}, want: "at most 100"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateArea(context.Background(), biz, tt.in)
			if err == nil || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDuplicateAreaNameIsTenantScoped(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store)
	a := uuid.New()
	b := uuid.New()

	if _, err := svc.CreateArea(context.Background(), a, CreateAreaInput{Name: "Indoor"}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.CreateArea(context.Background(), a, CreateAreaInput{Name: "Indoor"}); !errors.Is(err, ErrAreaNameExists) {
		t.Fatalf("duplicate in A error = %v", err)
	}
	if _, err := svc.CreateArea(context.Background(), b, CreateAreaInput{Name: "Indoor"}); err != nil {
		t.Fatalf("same name in B should succeed: %v", err)
	}
}

func TestUpdateAreaPreservesOmittedFieldsAndRenames(t *testing.T) {
	biz := uuid.New()
	svc := NewService(newMemoryStore())
	created, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "Outdoor", Description: ptr("Garden")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := svc.UpdateArea(context.Background(), biz, created.ID, UpdateAreaInput{Name: ptr("Outdoor Garden")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Outdoor Garden" {
		t.Errorf("name = %q", got.Name)
	}
	if got.Description == nil || *got.Description != "Garden" {
		t.Errorf("description was overwritten: %v", got.Description)
	}
	if got.Status != tenant.StatusActive {
		t.Errorf("status changed: %s", got.Status)
	}

	_, err = svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "VIP"})
	if err != nil {
		t.Fatalf("create vip: %v", err)
	}
	_, err = svc.UpdateArea(context.Background(), biz, created.ID, UpdateAreaInput{Name: ptr("VIP")})
	if !errors.Is(err, ErrAreaNameExists) {
		t.Fatalf("duplicate rename error = %v", err)
	}
}

func TestDeactivateAreaRequiresInactiveTables(t *testing.T) {
	biz := uuid.New()
	svc := NewService(newMemoryStore())
	area, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("create area: %v", err)
	}
	table, err := svc.CreateTable(context.Background(), biz, area.ID, CreateTableInput{Name: "Table 01", Capacity: 4})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	_, err = svc.DeleteArea(context.Background(), biz, area.ID)
	if !errors.Is(err, ErrAreaHasActiveTables) {
		t.Fatalf("delete with active tables error = %v", err)
	}

	inactive := tenant.StatusInactive
	if _, err := svc.UpdateArea(context.Background(), biz, area.ID, UpdateAreaInput{Status: &inactive}); !errors.Is(err, ErrAreaHasActiveTables) {
		t.Fatalf("patch inactive with active tables error = %v", err)
	}

	if _, err := svc.DeleteTable(context.Background(), biz, area.ID, table.ID); err != nil {
		t.Fatalf("delete table: %v", err)
	}
	got, err := svc.DeleteArea(context.Background(), biz, area.ID)
	if err != nil {
		t.Fatalf("delete area: %v", err)
	}
	if got.Status != tenant.StatusInactive {
		t.Errorf("status = %s, want INACTIVE", got.Status)
	}

	listed, err := svc.ListAreas(context.Background(), biz, "")
	if err != nil {
		t.Fatalf("list default: %v", err)
	}
	if len(listed) != 0 {
		t.Errorf("default list = %d, want 0", len(listed))
	}
	inactiveList, err := svc.ListAreas(context.Background(), biz, "INACTIVE")
	if err != nil {
		t.Fatalf("list inactive: %v", err)
	}
	if len(inactiveList) != 1 || inactiveList[0].ID != area.ID {
		t.Errorf("inactive list = %+v", inactiveList)
	}
}

func TestCreateAndUpdateTableRules(t *testing.T) {
	store := newMemoryStore()
	svc := NewService(store)
	biz := uuid.New()
	indoor, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "Indoor"})
	if err != nil {
		t.Fatalf("indoor: %v", err)
	}
	outdoor, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "Outdoor"})
	if err != nil {
		t.Fatalf("outdoor: %v", err)
	}

	table, err := svc.CreateTable(context.Background(), biz, indoor.ID, CreateTableInput{Name: "  Table 01  ", Capacity: 4})
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	if table.Name != "Table 01" || table.Capacity != 4 || table.Status != tenant.StatusActive || table.AreaID != indoor.ID {
		t.Errorf("table = %+v", table)
	}

	if _, err := svc.CreateTable(context.Background(), biz, indoor.ID, CreateTableInput{Name: "Table 01", Capacity: 2}); !errors.Is(err, ErrTableNameExists) {
		t.Fatalf("duplicate same area error = %v", err)
	}
	if _, err := svc.CreateTable(context.Background(), biz, outdoor.ID, CreateTableInput{Name: "Table 01", Capacity: 2}); err != nil {
		t.Fatalf("same name in other area: %v", err)
	}
	if _, err := svc.CreateTable(context.Background(), biz, indoor.ID, CreateTableInput{Name: "Bad", Capacity: 0}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("capacity 0 error = %v", err)
	}

	got, err := svc.UpdateTable(context.Background(), biz, indoor.ID, table.ID, UpdateTableInput{Name: ptr("Table A")})
	if err != nil {
		t.Fatalf("rename: %v", err)
	}
	if got.Name != "Table A" || got.Capacity != 4 {
		t.Errorf("patch omitted capacity: %+v", got)
	}

	if _, err := svc.UpdateTable(context.Background(), biz, outdoor.ID, table.ID, UpdateTableInput{Name: ptr("Moved")}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move via other area path error = %v, want not found", err)
	}
	if store.tables[table.ID].AreaID != indoor.ID {
		t.Error("table moved between areas")
	}

	deactivated, err := svc.DeleteTable(context.Background(), biz, indoor.ID, table.ID)
	if err != nil {
		t.Fatalf("delete table: %v", err)
	}
	if deactivated.Status != tenant.StatusInactive {
		t.Errorf("status = %s", deactivated.Status)
	}

	active, err := svc.ListTables(context.Background(), biz, indoor.ID, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("active tables = %d", len(active))
	}
}

func TestCreateTableRequiresActiveOwnedArea(t *testing.T) {
	svc := NewService(newMemoryStore())
	biz := uuid.New()
	other := uuid.New()
	area, err := svc.CreateArea(context.Background(), biz, CreateAreaInput{Name: "VIP"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.DeleteArea(context.Background(), biz, area.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.CreateTable(context.Background(), biz, area.ID, CreateTableInput{Name: "T1", Capacity: 2}); !errors.Is(err, ErrAreaInactive) {
		t.Fatalf("inactive area error = %v", err)
	}
	if _, err := svc.CreateTable(context.Background(), other, area.ID, CreateTableInput{Name: "T1", Capacity: 2}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant create error = %v", err)
	}
}

func TestGetAreaAndTableIsolation(t *testing.T) {
	svc := NewService(newMemoryStore())
	a := uuid.New()
	b := uuid.New()
	areaA, _ := svc.CreateArea(context.Background(), a, CreateAreaInput{Name: "Indoor"})
	tableA, _ := svc.CreateTable(context.Background(), a, areaA.ID, CreateTableInput{Name: "Table 01", Capacity: 4})
	areaB, _ := svc.CreateArea(context.Background(), b, CreateAreaInput{Name: "Indoor"})
	tableB, _ := svc.CreateTable(context.Background(), b, areaB.ID, CreateTableInput{Name: "Table 01", Capacity: 2})

	if _, err := svc.GetArea(context.Background(), b, areaA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant get area error = %v", err)
	}
	if _, err := svc.GetTable(context.Background(), a, areaA.ID, tableB.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign table in area A error = %v", err)
	}
	if _, err := svc.GetTable(context.Background(), a, areaB.ID, tableB.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("table via other-business area error = %v", err)
	}
	got, err := svc.GetArea(context.Background(), a, areaA.ID)
	if err != nil || got.TableCount != 1 || got.ActiveTableCount != 1 {
		t.Fatalf("get A = %+v err=%v", got, err)
	}
	if _, err := svc.GetTable(context.Background(), b, areaB.ID, tableA.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("table A via business B error = %v", err)
	}
}

type memoryStore struct {
	areas  map[uuid.UUID]*Area
	tables map[uuid.UUID]*Table
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		areas:  map[uuid.UUID]*Area{},
		tables: map[uuid.UUID]*Table{},
	}
}

func (m *memoryStore) ListAreas(_ context.Context, businessID uuid.UUID, status tenant.Status) ([]Area, error) {
	out := make([]Area, 0)
	for _, a := range m.areas {
		if a.BusinessID == businessID && a.Status == status {
			copy := m.withCounts(a)
			out = append(out, copy)
		}
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Name < out[i].Name {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (m *memoryStore) GetArea(_ context.Context, businessID, areaID uuid.UUID) (*Area, error) {
	a, ok := m.areas[areaID]
	if !ok || a.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := m.withCounts(a)
	return &copy, nil
}

func (m *memoryStore) CreateArea(_ context.Context, businessID uuid.UUID, name string, description *string) (*Area, error) {
	if err := m.uniqueAreaName(businessID, name, uuid.Nil); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	a := &Area{
		ID:          uuid.New(),
		BusinessID:  businessID,
		Name:        name,
		Description: description,
		Status:      tenant.StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.areas[a.ID] = a
	copy := *a
	return &copy, nil
}

func (m *memoryStore) UpdateArea(_ context.Context, businessID uuid.UUID, area Area) (*Area, error) {
	current, ok := m.areas[area.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrNotFound
	}
	if err := m.uniqueAreaName(businessID, area.Name, area.ID); err != nil {
		return nil, err
	}
	current.Name = area.Name
	current.Description = area.Description
	current.Status = area.Status
	current.UpdatedAt = time.Now().UTC()
	copy := m.withCounts(current)
	return &copy, nil
}

func (m *memoryStore) CountActiveTables(_ context.Context, businessID, areaID uuid.UUID) (int, error) {
	n := 0
	for _, t := range m.tables {
		area, ok := m.areas[t.AreaID]
		if !ok || area.BusinessID != businessID || t.AreaID != areaID {
			continue
		}
		if t.Status == tenant.StatusActive {
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) ListTables(_ context.Context, businessID, areaID uuid.UUID, status tenant.Status) ([]Table, error) {
	out := make([]Table, 0)
	for _, t := range m.tables {
		area, ok := m.areas[t.AreaID]
		if !ok || area.BusinessID != businessID || t.AreaID != areaID || t.Status != status {
			continue
		}
		copy := *t
		out = append(out, copy)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].Name < out[i].Name {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

func (m *memoryStore) GetTable(_ context.Context, businessID, areaID, tableID uuid.UUID) (*Table, error) {
	t, ok := m.tables[tableID]
	if !ok || t.AreaID != areaID {
		return nil, ErrNotFound
	}
	area, ok := m.areas[t.AreaID]
	if !ok || area.BusinessID != businessID {
		return nil, ErrNotFound
	}
	copy := *t
	return &copy, nil
}

func (m *memoryStore) CreateTable(_ context.Context, businessID, areaID uuid.UUID, name string, capacity int) (*Table, error) {
	area, ok := m.areas[areaID]
	if !ok || area.BusinessID != businessID {
		return nil, ErrNotFound
	}
	if err := m.uniqueTableName(areaID, name, uuid.Nil); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	t := &Table{
		ID:        uuid.New(),
		AreaID:    areaID,
		Name:      name,
		Capacity:  capacity,
		Status:    tenant.StatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	m.tables[t.ID] = t
	copy := *t
	return &copy, nil
}

func (m *memoryStore) UpdateTable(_ context.Context, businessID, areaID uuid.UUID, table Table) (*Table, error) {
	current, ok := m.tables[table.ID]
	if !ok || current.AreaID != areaID {
		return nil, ErrNotFound
	}
	area, ok := m.areas[current.AreaID]
	if !ok || area.BusinessID != businessID {
		return nil, ErrNotFound
	}
	if err := m.uniqueTableName(areaID, table.Name, table.ID); err != nil {
		return nil, err
	}
	current.Name = table.Name
	current.Capacity = table.Capacity
	current.Status = table.Status
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	return &copy, nil
}

func (m *memoryStore) uniqueAreaName(businessID uuid.UUID, name string, except uuid.UUID) error {
	for _, a := range m.areas {
		if a.BusinessID == businessID && a.Name == name && a.ID != except {
			return ErrAreaNameExists
		}
	}
	return nil
}

func (m *memoryStore) uniqueTableName(areaID uuid.UUID, name string, except uuid.UUID) error {
	for _, t := range m.tables {
		if t.AreaID == areaID && t.Name == name && t.ID != except {
			return ErrTableNameExists
		}
	}
	return nil
}

func (m *memoryStore) withCounts(a *Area) Area {
	copy := *a
	copy.TableCount = 0
	copy.ActiveTableCount = 0
	for _, t := range m.tables {
		if t.AreaID != a.ID {
			continue
		}
		copy.TableCount++
		if t.Status == tenant.StatusActive {
			copy.ActiveTableCount++
		}
	}
	return copy
}

func ptr(v string) *string { return &v }
