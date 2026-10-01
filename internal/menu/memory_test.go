package menu

import (
	"bytes"
	"context"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/pkg/storage"
)

type memoryStore struct {
	mu         sync.Mutex
	categories map[uuid.UUID]*Category
	items      map[uuid.UUID]*Item
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		categories: map[uuid.UUID]*Category{},
		items:      map[uuid.UUID]*Item{},
	}
}

func (m *memoryStore) ListCategories(_ context.Context, businessID uuid.UUID, status tenant.Status) ([]Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Category, 0)
	for _, c := range m.categories {
		if c.BusinessID == businessID && c.Status == status {
			copy := m.withCounts(c)
			out = append(out, copy)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *memoryStore) GetCategory(_ context.Context, businessID, categoryID uuid.UUID) (*Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.categories[categoryID]
	if !ok || c.BusinessID != businessID {
		return nil, ErrCategoryNotFound
	}
	copy := m.withCounts(c)
	return &copy, nil
}

func (m *memoryStore) CreateCategory(_ context.Context, businessID uuid.UUID, name string, description *string, sortOrder int) (*Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := m.uniqueCategoryName(businessID, name, uuid.Nil); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	c := &Category{
		ID:          uuid.New(),
		BusinessID:  businessID,
		Name:        name,
		Description: description,
		Status:      tenant.StatusActive,
		SortOrder:   sortOrder,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.categories[c.ID] = c
	copy := *c
	return &copy, nil
}

func (m *memoryStore) UpdateCategory(_ context.Context, businessID uuid.UUID, category Category) (*Category, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.categories[category.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrCategoryNotFound
	}
	if err := m.uniqueCategoryName(businessID, category.Name, category.ID); err != nil {
		return nil, err
	}
	current.Name = category.Name
	current.Description = category.Description
	current.Status = category.Status
	current.SortOrder = category.SortOrder
	current.UpdatedAt = time.Now().UTC()
	copy := m.withCounts(current)
	return &copy, nil
}

func (m *memoryStore) CountActiveItems(_ context.Context, businessID, categoryID uuid.UUID) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, item := range m.items {
		if item.BusinessID == businessID && item.CategoryID == categoryID && item.Status == tenant.StatusActive {
			n++
		}
	}
	return n, nil
}

func (m *memoryStore) ListItems(_ context.Context, businessID uuid.UUID, status tenant.Status, categoryID *uuid.UUID) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Item, 0)
	for _, item := range m.items {
		if item.BusinessID != businessID || item.Status != status {
			continue
		}
		if categoryID != nil && item.CategoryID != *categoryID {
			continue
		}
		copy := *item
		if cat, ok := m.categories[item.CategoryID]; ok {
			copy.CategoryName = cat.Name
		}
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		ci, cj := m.categories[out[i].CategoryID], m.categories[out[j].CategoryID]
		if ci != nil && cj != nil && ci.SortOrder != cj.SortOrder {
			return ci.SortOrder < cj.SortOrder
		}
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (m *memoryStore) GetItem(_ context.Context, businessID, itemID uuid.UUID) (*Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[itemID]
	if !ok || item.BusinessID != businessID {
		return nil, ErrItemNotFound
	}
	copy := *item
	if cat, ok := m.categories[item.CategoryID]; ok {
		copy.CategoryName = cat.Name
	}
	return &copy, nil
}

func (m *memoryStore) CreateItem(_ context.Context, item Item) (*Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cat, ok := m.categories[item.CategoryID]
	if !ok || cat.BusinessID != item.BusinessID {
		return nil, ErrCategoryNotFound
	}
	now := time.Now().UTC()
	stored := item
	stored.CreatedAt = now
	stored.UpdatedAt = now
	stored.Status = tenant.StatusActive
	stored.CategoryName = cat.Name
	m.items[stored.ID] = &stored
	copy := stored
	return &copy, nil
}

func (m *memoryStore) UpdateItem(_ context.Context, businessID uuid.UUID, item Item) (*Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.items[item.ID]
	if !ok || current.BusinessID != businessID {
		return nil, ErrItemNotFound
	}
	current.Name = item.Name
	current.Description = item.Description
	current.Price = item.Price
	current.ImagePath = item.ImagePath
	current.Status = item.Status
	current.SortOrder = item.SortOrder
	current.UpdatedAt = time.Now().UTC()
	copy := *current
	if cat, ok := m.categories[current.CategoryID]; ok {
		copy.CategoryName = cat.Name
	}
	return &copy, nil
}

func (m *memoryStore) uniqueCategoryName(businessID uuid.UUID, name string, except uuid.UUID) error {
	for _, c := range m.categories {
		if c.BusinessID == businessID && c.Name == name && c.ID != except {
			return ErrCategoryNameExists
		}
	}
	return nil
}

func (m *memoryStore) withCounts(c *Category) Category {
	copy := *c
	copy.ItemCount = 0
	copy.ActiveItemCount = 0
	for _, item := range m.items {
		if item.CategoryID != c.ID {
			continue
		}
		copy.ItemCount++
		if item.Status == tenant.StatusActive {
			copy.ActiveItemCount++
		}
	}
	return copy
}

type memoryStorage struct {
	mu        sync.Mutex
	objects   map[string][]byte
	types     map[string]string
	uploadErr error
	deleteErr error
	signErr   error
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{objects: map[string][]byte{}, types: map[string]string{}}
}

func (m *memoryStorage) Upload(_ context.Context, path, contentType string, data io.Reader) error {
	if m.uploadErr != nil {
		return m.uploadErr
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := bytes.Clone(b)
	m.objects[path] = cp
	m.types[path] = contentType
	return nil
}

func (m *memoryStorage) Download(_ context.Context, path string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.objects[path]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return bytes.Clone(data), nil
}

func (m *memoryStorage) Delete(_ context.Context, path string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.objects[path]; !ok {
		return storage.ErrNotFound
	}
	delete(m.objects, path)
	delete(m.types, path)
	return nil
}

func (m *memoryStorage) CreateSignedURL(_ context.Context, path string, _ time.Duration) (string, error) {
	urls, err := m.CreateSignedURLs(context.Background(), []string{path}, 0)
	if err != nil {
		return "", err
	}
	return urls[path], nil
}

func (m *memoryStorage) CreateSignedURLs(_ context.Context, paths []string, _ time.Duration) (map[string]string, error) {
	if m.signErr != nil {
		return nil, m.signErr
	}
	out := map[string]string{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, path := range paths {
		if _, ok := m.objects[path]; !ok {
			continue
		}
		out[path] = "https://signed.example/" + path
	}
	if len(paths) > 0 && len(out) == 0 {
		return nil, storage.ErrSignFailed
	}
	return out, nil
}

func ptr(v string) *string { return &v }
func ptrInt(v int) *int    { return &v }
