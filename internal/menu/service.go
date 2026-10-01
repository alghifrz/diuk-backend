package menu

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/pkg/storage"
)

// Reuse signed URLs until this much TTL remains, so list refreshes keep the
// same browser-cacheable image URLs instead of re-signing every request.
const signedURLReuseFloor = 2 * time.Minute

type cachedSignedURL struct {
	url       string
	expiresAt time.Time
}

// Service applies menu rules. The business id must come from tenant context.
type Service struct {
	store   Store
	objects storage.ObjectStorage
	log     *slog.Logger

	signedMu    sync.Mutex
	signedCache map[string]cachedSignedURL
}

// NewService returns a service backed by store and objects.
func NewService(store Store, objects storage.ObjectStorage, log *slog.Logger) *Service {
	if log == nil {
		log = slog.Default()
	}
	return &Service{
		store:       store,
		objects:     objects,
		log:         log,
		signedCache: make(map[string]cachedSignedURL),
	}
}

func (s *Service) ListCategories(ctx context.Context, businessID uuid.UUID, statusRaw string) ([]Category, error) {
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	return s.store.ListCategories(ctx, businessID, status)
}

func (s *Service) GetCategory(ctx context.Context, businessID, categoryID uuid.UUID) (*Category, error) {
	return s.store.GetCategory(ctx, businessID, categoryID)
}

func (s *Service) CreateCategory(ctx context.Context, businessID uuid.UUID, in CreateCategoryInput) (*Category, error) {
	name, err := normalizeCategoryName(in.Name)
	if err != nil {
		return nil, err
	}
	if err := validateSortOrder(in.SortOrder); err != nil {
		return nil, err
	}
	return s.store.CreateCategory(ctx, businessID, name, normalizeOptionalDescription(in.Description), in.SortOrder)
}

func (s *Service) UpdateCategory(ctx context.Context, businessID, categoryID uuid.UUID, in UpdateCategoryInput) (*Category, error) {
	current, err := s.store.GetCategory(ctx, businessID, categoryID)
	if err != nil {
		return nil, err
	}

	next := *current
	if in.Name != nil {
		name, err := normalizeCategoryName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		next.Description = normalizeOptionalDescription(in.Description)
	}
	if in.SortOrder != nil {
		if err := validateSortOrder(*in.SortOrder); err != nil {
			return nil, err
		}
		next.SortOrder = *in.SortOrder
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		if *in.Status == tenant.StatusInactive && current.Status != tenant.StatusInactive {
			if err := s.ensureNoActiveItems(ctx, businessID, categoryID); err != nil {
				return nil, err
			}
		}
		next.Status = *in.Status
	}
	next.BusinessID = current.BusinessID
	return s.store.UpdateCategory(ctx, businessID, next)
}

func (s *Service) DeleteCategory(ctx context.Context, businessID, categoryID uuid.UUID) (*Category, error) {
	current, err := s.store.GetCategory(ctx, businessID, categoryID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureNoActiveItems(ctx, businessID, categoryID); err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return current, nil
	}
	next := *current
	next.Status = tenant.StatusInactive
	return s.store.UpdateCategory(ctx, businessID, next)
}

func (s *Service) ListItems(ctx context.Context, businessID uuid.UUID, statusRaw string, categoryID *uuid.UUID) ([]Item, error) {
	if categoryID != nil {
		if _, err := s.store.GetCategory(ctx, businessID, *categoryID); err != nil {
			return nil, err
		}
	}
	status, err := parseStatusFilter(statusRaw)
	if err != nil {
		return nil, err
	}
	items, err := s.store.ListItems(ctx, businessID, status, categoryID)
	if err != nil {
		return nil, err
	}
	return s.attachURLs(ctx, businessID, items)
}

func (s *Service) GetItem(ctx context.Context, businessID, itemID uuid.UUID) (*Item, error) {
	item, err := s.store.GetItem(ctx, businessID, itemID)
	if err != nil {
		return nil, err
	}
	items, err := s.attachURLs(ctx, businessID, []Item{*item})
	if err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (s *Service) CreateItem(ctx context.Context, businessID uuid.UUID, in CreateItemInput) (*Item, error) {
	categoryID, err := uuid.Parse(in.CategoryID)
	if err != nil {
		return nil, invalid("category_id is invalid")
	}
	category, err := s.store.GetCategory(ctx, businessID, categoryID)
	if err != nil {
		return nil, err
	}
	if category.Status != tenant.StatusActive {
		return nil, ErrMenuItemCategoryInvalid
	}

	name, err := normalizeItemName(in.Name)
	if err != nil {
		return nil, err
	}
	price, err := parsePrice(in.Price)
	if err != nil {
		return nil, err
	}
	if err := validateSortOrder(in.SortOrder); err != nil {
		return nil, err
	}

	item := Item{
		ID:          uuid.New(),
		BusinessID:  businessID,
		CategoryID:  category.ID,
		Name:        name,
		Description: normalizeOptionalDescription(in.Description),
		Price:       price,
		Status:      tenant.StatusActive,
		SortOrder:   in.SortOrder,
	}

	if in.Image != nil {
		path, err := s.uploadImagePair(ctx, businessID, item.ID, in.Image)
		if err != nil {
			return nil, err
		}
		item.ImagePath = &path
		created, err := s.store.CreateItem(ctx, item)
		if err != nil {
			s.cleanupImagePair(ctx, path)
			return nil, err
		}
		return s.GetItem(ctx, businessID, created.ID)
	}

	created, err := s.store.CreateItem(ctx, item)
	if err != nil {
		return nil, err
	}
	return s.GetItem(ctx, businessID, created.ID)
}

func (s *Service) UpdateItem(ctx context.Context, businessID, itemID uuid.UUID, in UpdateItemInput) (*Item, error) {
	current, err := s.store.GetItem(ctx, businessID, itemID)
	if err != nil {
		return nil, err
	}

	next := *current
	next.ImageURL = nil
	next.ImageThumbURL = nil
	if in.Name != nil {
		name, err := normalizeItemName(*in.Name)
		if err != nil {
			return nil, err
		}
		next.Name = name
	}
	if in.Description != nil {
		next.Description = normalizeOptionalDescription(in.Description)
	}
	if in.Price != nil {
		price, err := parsePrice(*in.Price)
		if err != nil {
			return nil, err
		}
		next.Price = price
	}
	if in.SortOrder != nil {
		if err := validateSortOrder(*in.SortOrder); err != nil {
			return nil, err
		}
		next.SortOrder = *in.SortOrder
	}
	if in.Status != nil {
		if *in.Status != tenant.StatusActive && *in.Status != tenant.StatusInactive {
			return nil, invalid("status is invalid")
		}
		next.Status = *in.Status
	}

	oldPath := current.ImagePath
	newPath := ""

	if in.Image != nil {
		path, err := s.uploadImagePair(ctx, businessID, current.ID, in.Image)
		if err != nil {
			return nil, err
		}
		newPath = path
		next.ImagePath = &newPath
	} else if in.RemoveImage && oldPath != nil {
		if err := s.deleteImagePair(ctx, *oldPath); err != nil {
			return nil, err
		}
		next.ImagePath = nil
		oldPath = nil
	}

	updated, err := s.store.UpdateItem(ctx, businessID, next)
	if err != nil {
		if newPath != "" {
			s.cleanupImagePair(ctx, newPath)
		}
		return nil, err
	}

	if newPath != "" && oldPath != nil && *oldPath != newPath {
		if err := s.deleteReplacedOriginal(ctx, *oldPath, newPath); err != nil && !errors.Is(err, storage.ErrNotFound) {
			s.log.Error("menu image cleanup failed", "op", "replace", "item_id", itemID.String())
		}
	}

	return s.GetItem(ctx, businessID, updated.ID)
}

func (s *Service) DeleteItem(ctx context.Context, businessID, itemID uuid.UUID) (*Item, error) {
	current, err := s.store.GetItem(ctx, businessID, itemID)
	if err != nil {
		return nil, err
	}
	if current.Status == tenant.StatusInactive {
		return s.GetItem(ctx, businessID, itemID)
	}
	next := *current
	next.Status = tenant.StatusInactive
	next.ImageURL = nil
	next.ImageThumbURL = nil
	if _, err := s.store.UpdateItem(ctx, businessID, next); err != nil {
		return nil, err
	}
	return s.GetItem(ctx, businessID, itemID)
}

func (s *Service) ensureNoActiveItems(ctx context.Context, businessID, categoryID uuid.UUID) error {
	n, err := s.store.CountActiveItems(ctx, businessID, categoryID)
	if err != nil {
		return err
	}
	if n > 0 {
		return ErrCategoryHasActiveItems
	}
	return nil
}

func (s *Service) upload(ctx context.Context, path string, img *validatedImage) error {
	if s.objects == nil {
		return ErrImageUploadFailed
	}
	if err := s.objects.Upload(ctx, path, img.ContentType, bytes.NewReader(img.Bytes)); err != nil {
		return ErrImageUploadFailed
	}
	return nil
}

func (s *Service) uploadImagePair(ctx context.Context, businessID, itemID uuid.UUID, img *validatedImage) (string, error) {
	originalPath := storagePath(businessID, itemID, img.Extension)
	thumb, err := makeThumbnail(img)
	if err != nil {
		return "", ErrImageUploadFailed
	}
	thumbPath := thumbStoragePath(businessID, itemID)

	if err := s.upload(ctx, originalPath, img); err != nil {
		return "", err
	}
	if err := s.upload(ctx, thumbPath, thumb); err != nil {
		s.cleanup(ctx, originalPath)
		return "", err
	}
	return originalPath, nil
}

func (s *Service) deleteObject(ctx context.Context, path string) error {
	s.forgetSignedURL(path)
	if s.objects == nil {
		return ErrImageDeleteFailed
	}
	err := s.objects.Delete(ctx, path)
	if err == nil || errors.Is(err, storage.ErrNotFound) {
		return nil
	}
	return ErrImageDeleteFailed
}

func (s *Service) deleteImagePair(ctx context.Context, originalPath string) error {
	if err := s.deleteObject(ctx, originalPath); err != nil {
		return err
	}
	if thumb := siblingThumbPath(originalPath); thumb != "" {
		if err := s.deleteObject(ctx, thumb); err != nil && !errors.Is(err, storage.ErrNotFound) {
			// Best-effort: original is already gone; orphan thumb must not block remove.
			s.log.Error("menu thumb cleanup failed", "op", "delete_pair")
		}
	}
	return nil
}

// deleteReplacedOriginal removes the previous original after a successful
// replace. Thumb keys are shared per item (.../thumb.jpg), so the new thumb
// must not be deleted when cleaning up the old original.
func (s *Service) deleteReplacedOriginal(ctx context.Context, oldPath, newPath string) error {
	if err := s.deleteObject(ctx, oldPath); err != nil {
		return err
	}
	oldThumb := siblingThumbPath(oldPath)
	newThumb := siblingThumbPath(newPath)
	if oldThumb != "" && oldThumb != newThumb {
		if err := s.deleteObject(ctx, oldThumb); err != nil && !errors.Is(err, storage.ErrNotFound) {
			s.log.Error("menu thumb cleanup failed", "op", "replace")
		}
	}
	return nil
}

func (s *Service) cleanup(ctx context.Context, path string) {
	s.forgetSignedURL(path)
	if s.objects == nil || path == "" {
		return
	}
	if err := s.objects.Delete(ctx, path); err != nil && !errors.Is(err, storage.ErrNotFound) {
		s.log.Error("menu image cleanup failed", "op", "compensate")
	}
}

func (s *Service) cleanupImagePair(ctx context.Context, originalPath string) {
	s.cleanup(ctx, originalPath)
	if thumb := siblingThumbPath(originalPath); thumb != "" {
		s.cleanup(ctx, thumb)
	}
}

func (s *Service) forgetSignedURL(path string) {
	if path == "" {
		return
	}
	s.signedMu.Lock()
	delete(s.signedCache, path)
	s.signedMu.Unlock()
}

func (s *Service) attachURLs(ctx context.Context, businessID uuid.UUID, items []Item) ([]Item, error) {
	originals := make([]string, 0)
	thumbs := make([]string, 0)
	seenOrig := map[string]struct{}{}
	seenThumb := map[string]struct{}{}

	for i := range items {
		items[i].ImageURL = nil
		items[i].ImageThumbURL = nil
		if items[i].ImagePath == nil || *items[i].ImagePath == "" {
			items[i].ImagePath = nil
			continue
		}
		if !pathOwnedByBusiness(businessID, *items[i].ImagePath) {
			return nil, ErrItemNotFound
		}
		orig := *items[i].ImagePath
		if _, ok := seenOrig[orig]; !ok {
			seenOrig[orig] = struct{}{}
			originals = append(originals, orig)
		}
		if thumb := siblingThumbPath(orig); thumb != "" {
			if _, ok := seenThumb[thumb]; !ok {
				seenThumb[thumb] = struct{}{}
				thumbs = append(thumbs, thumb)
			}
		}
	}
	if len(originals) == 0 {
		return items, nil
	}
	if s.objects == nil {
		return nil, ErrSignedURLFailed
	}

	// Originals are required. Missing thumbs (legacy uploads) must not fail the list —
	// Supabase rejects the whole sign batch when any path is absent.
	urls, err := s.resolveSignedURLs(ctx, originals)
	if err != nil {
		return nil, ErrSignedURLFailed
	}

	thumbURLs := map[string]string{}
	if len(thumbs) > 0 {
		if signed, thumbErr := s.resolveSignedURLs(ctx, thumbs); thumbErr == nil {
			thumbURLs = signed
		}
	}

	for i := range items {
		if items[i].ImagePath == nil {
			continue
		}
		if signed, ok := urls[*items[i].ImagePath]; ok && signed != "" {
			u := signed
			items[i].ImageURL = &u
		}
		if thumbPath := siblingThumbPath(*items[i].ImagePath); thumbPath != "" {
			if signed, ok := thumbURLs[thumbPath]; ok && signed != "" {
				u := signed
				items[i].ImageThumbURL = &u
				continue
			}
		}
		// Legacy uploads without a thumb: fall back to the original.
		if items[i].ImageURL != nil {
			u := *items[i].ImageURL
			items[i].ImageThumbURL = &u
		}
	}
	return items, nil
}

func (s *Service) resolveSignedURLs(ctx context.Context, paths []string) (map[string]string, error) {
	now := time.Now()
	out := make(map[string]string, len(paths))
	missing := make([]string, 0, len(paths))

	s.signedMu.Lock()
	for _, path := range paths {
		if cached, ok := s.signedCache[path]; ok && cached.url != "" && cached.expiresAt.After(now.Add(signedURLReuseFloor)) {
			out[path] = cached.url
			continue
		}
		missing = append(missing, path)
	}
	s.signedMu.Unlock()

	if len(missing) == 0 {
		return out, nil
	}

	fresh, err := s.objects.CreateSignedURLs(ctx, missing, storage.DefaultSignedURLTTL)
	if err != nil {
		return nil, err
	}

	expiresAt := now.Add(storage.DefaultSignedURLTTL)
	s.signedMu.Lock()
	for path, url := range fresh {
		if url == "" {
			continue
		}
		s.signedCache[path] = cachedSignedURL{url: url, expiresAt: expiresAt}
		out[path] = url
	}
	s.signedMu.Unlock()

	return out, nil
}
