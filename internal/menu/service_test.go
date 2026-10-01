package menu

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/tenant"
	"github.com/alghifrz/diuk-backend/pkg/storage"
)

func testService() (*Service, *memoryStore, *memoryStorage) {
	store := newMemoryStore()
	objects := newMemoryStorage()
	return NewService(store, objects, slog.New(slog.NewTextHandler(io.Discard, nil))), store, objects
}

func TestCreateCategoryValidation(t *testing.T) {
	svc, _, _ := testService()
	biz := uuid.New()

	got, err := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "  Coffee  ", Description: ptr(" Drinks "), SortOrder: 2})
	if err != nil {
		t.Fatalf("CreateCategory() error = %v", err)
	}
	if got.Name != "Coffee" || got.Status != tenant.StatusActive || got.SortOrder != 2 || got.BusinessID != biz {
		t.Errorf("created = %+v", got)
	}

	tests := []struct {
		name string
		in   CreateCategoryInput
		want error
	}{
		{name: "empty", in: CreateCategoryInput{Name: "  "}, want: ErrInvalid},
		{name: "too long", in: CreateCategoryInput{Name: strings.Repeat("a", 101)}, want: ErrInvalid},
		{name: "negative sort", in: CreateCategoryInput{Name: "Tea", SortOrder: -1}, want: ErrInvalidSortOrder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateCategory(context.Background(), biz, tt.in)
			if err == nil || !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestDuplicateCategoryNameIsTenantScoped(t *testing.T) {
	svc, _, _ := testService()
	a, b := uuid.New(), uuid.New()
	if _, err := svc.CreateCategory(context.Background(), a, CreateCategoryInput{Name: "Coffee"}); err != nil {
		t.Fatalf("create A: %v", err)
	}
	if _, err := svc.CreateCategory(context.Background(), a, CreateCategoryInput{Name: "Coffee"}); !errors.Is(err, ErrCategoryNameExists) {
		t.Fatalf("duplicate A error = %v", err)
	}
	if _, err := svc.CreateCategory(context.Background(), b, CreateCategoryInput{Name: "Coffee"}); err != nil {
		t.Fatalf("same name in B: %v", err)
	}
}

func TestUpdateCategoryPreservesOmittedFields(t *testing.T) {
	svc, _, _ := testService()
	biz := uuid.New()
	created, err := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee", Description: ptr("Hot"), SortOrder: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := svc.UpdateCategory(context.Background(), biz, created.ID, UpdateCategoryInput{Name: ptr("Espresso Bar")})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "Espresso Bar" || got.SortOrder != 1 || got.Description == nil || *got.Description != "Hot" {
		t.Errorf("patched = %+v", got)
	}
	if _, err := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Food"}); err != nil {
		t.Fatalf("food: %v", err)
	}
	if _, err := svc.UpdateCategory(context.Background(), biz, created.ID, UpdateCategoryInput{Name: ptr("Food")}); !errors.Is(err, ErrCategoryNameExists) {
		t.Fatalf("duplicate rename = %v", err)
	}
}

func TestDeactivateCategoryRequiresInactiveItems(t *testing.T) {
	svc, _, _ := testService()
	biz := uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})
	item, err := svc.CreateItem(context.Background(), biz, CreateItemInput{CategoryID: cat.ID.String(), Name: "Americano", Price: "18000"})
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if _, err := svc.DeleteCategory(context.Background(), biz, cat.ID); !errors.Is(err, ErrCategoryHasActiveItems) {
		t.Fatalf("delete with items = %v", err)
	}
	inactive := tenant.StatusInactive
	if _, err := svc.UpdateCategory(context.Background(), biz, cat.ID, UpdateCategoryInput{Status: &inactive}); !errors.Is(err, ErrCategoryHasActiveItems) {
		t.Fatalf("patch inactive = %v", err)
	}
	if _, err := svc.DeleteItem(context.Background(), biz, item.ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
	got, err := svc.DeleteCategory(context.Background(), biz, cat.ID)
	if err != nil {
		t.Fatalf("delete category: %v", err)
	}
	if got.Status != tenant.StatusInactive {
		t.Errorf("status = %s", got.Status)
	}
	active, _ := svc.ListCategories(context.Background(), biz, "")
	if len(active) != 0 {
		t.Errorf("active list = %d", len(active))
	}
	inactiveList, _ := svc.ListCategories(context.Background(), biz, "INACTIVE")
	if len(inactiveList) != 1 {
		t.Errorf("inactive list = %d", len(inactiveList))
	}
}

func TestCreateAndUpdateItemRules(t *testing.T) {
	svc, _, _ := testService()
	biz := uuid.New()
	coffee, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})
	nonCoffee, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Non Coffee", SortOrder: 1})

	item, err := svc.CreateItem(context.Background(), biz, CreateItemInput{CategoryID: coffee.ID.String(), Name: "  Americano  ", Price: "18000", Description: ptr(" Espresso ")})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if item.Name != "Americano" || item.Price != "18000.00" || item.ImagePath != nil || item.ImageURL != nil {
		t.Errorf("item = %+v", item)
	}

	if _, err := svc.CreateItem(context.Background(), biz, CreateItemInput{CategoryID: nonCoffee.ID.String(), Name: "Americano", Price: "20000"}); err != nil {
		t.Fatalf("same name other category: %v", err)
	}

	tests := []struct {
		name string
		in   CreateItemInput
		want error
	}{
		{name: "empty name", in: CreateItemInput{CategoryID: coffee.ID.String(), Name: " ", Price: "1"}, want: ErrInvalid},
		{name: "long name", in: CreateItemInput{CategoryID: coffee.ID.String(), Name: strings.Repeat("a", 151), Price: "1"}, want: ErrInvalid},
		{name: "negative price", in: CreateItemInput{CategoryID: coffee.ID.String(), Name: "X", Price: "-1"}, want: ErrInvalidPrice},
		{name: "negative sort", in: CreateItemInput{CategoryID: coffee.ID.String(), Name: "X", Price: "1", SortOrder: -1}, want: ErrInvalidSortOrder},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.CreateItem(context.Background(), biz, tt.in)
			if err == nil || !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}

	patched, err := svc.UpdateItem(context.Background(), biz, item.ID, UpdateItemInput{Name: ptr("House Americano")})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.Name != "House Americano" || patched.Price != "18000.00" {
		t.Errorf("omitted price changed: %+v", patched)
	}

	deactivated, err := svc.DeleteItem(context.Background(), biz, item.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deactivated.Status != tenant.StatusInactive {
		t.Errorf("status = %s", deactivated.Status)
	}
}

func TestCreateItemRequiresActiveOwnedCategory(t *testing.T) {
	svc, _, _ := testService()
	a, b := uuid.New(), uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), a, CreateCategoryInput{Name: "Coffee"})
	if _, err := svc.CreateItem(context.Background(), b, CreateItemInput{CategoryID: cat.ID.String(), Name: "Americano", Price: "18000"}); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("cross-tenant category = %v", err)
	}
	if _, err := svc.DeleteCategory(context.Background(), a, cat.ID); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if _, err := svc.CreateItem(context.Background(), a, CreateItemInput{CategoryID: cat.ID.String(), Name: "Americano", Price: "18000"}); !errors.Is(err, ErrMenuItemCategoryInvalid) {
		t.Fatalf("inactive category = %v", err)
	}
}

func TestItemAndCategoryIsolation(t *testing.T) {
	svc, _, objects := testService()
	a, b := uuid.New(), uuid.New()
	catA, _ := svc.CreateCategory(context.Background(), a, CreateCategoryInput{Name: "Coffee"})
	itemA, err := svc.CreateItem(context.Background(), a, CreateItemInput{
		CategoryID: catA.ID.String(), Name: "Americano", Price: "18000", Image: mustJPEG(t),
	})
	if err != nil {
		t.Fatalf("item A: %v", err)
	}
	catB, _ := svc.CreateCategory(context.Background(), b, CreateCategoryInput{Name: "Coffee"})
	itemB, _ := svc.CreateItem(context.Background(), b, CreateItemInput{CategoryID: catB.ID.String(), Name: "Americano", Price: "19000"})

	if _, err := svc.GetCategory(context.Background(), b, catA.ID); !errors.Is(err, ErrCategoryNotFound) {
		t.Fatalf("get category A as B = %v", err)
	}
	if _, err := svc.GetItem(context.Background(), b, itemA.ID); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("get item A as B = %v", err)
	}
	if _, err := svc.GetItem(context.Background(), a, itemB.ID); !errors.Is(err, ErrItemNotFound) {
		t.Fatalf("get item B as A = %v", err)
	}
	if itemA.ImagePath == nil || !strings.Contains(*itemA.ImagePath, a.String()) || !strings.Contains(*itemA.ImagePath, itemA.ID.String()) {
		t.Fatalf("image path = %v", itemA.ImagePath)
	}
	if itemA.ImageURL == nil || !strings.HasPrefix(*itemA.ImageURL, "https://signed.example/") {
		t.Fatalf("signed url = %v", itemA.ImageURL)
	}
	if itemA.ImageThumbURL == nil || !strings.HasPrefix(*itemA.ImageThumbURL, "https://signed.example/") {
		t.Fatalf("thumb url = %v", itemA.ImageThumbURL)
	}
	if _, ok := objects.objects[*itemA.ImagePath]; !ok {
		t.Fatal("image was not stored")
	}
	thumb := siblingThumbPath(*itemA.ImagePath)
	if _, ok := objects.objects[thumb]; !ok {
		t.Fatal("thumb was not stored")
	}
}

func TestImageLifecycle(t *testing.T) {
	svc, store, objects := testService()
	biz := uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})

	created, err := svc.CreateItem(context.Background(), biz, CreateItemInput{
		CategoryID: cat.ID.String(), Name: "Latte", Price: "25000", Image: mustPNG(t),
	})
	if err != nil {
		t.Fatalf("create with png: %v", err)
	}
	old := *created.ImagePath
	oldThumb := siblingThumbPath(old)
	if !strings.HasSuffix(old, ".png") {
		t.Fatalf("path = %s", old)
	}
	if _, ok := objects.objects[oldThumb]; !ok {
		t.Fatal("png thumb missing")
	}

	replaced, err := svc.UpdateItem(context.Background(), biz, created.ID, UpdateItemInput{Image: mustJPEG(t)})
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	if replaced.ImagePath == nil || !strings.HasSuffix(*replaced.ImagePath, ".jpg") {
		t.Fatalf("new path = %v", replaced.ImagePath)
	}
	if _, ok := objects.objects[old]; ok {
		t.Error("old image was not removed")
	}
	newThumb := siblingThumbPath(*replaced.ImagePath)
	if oldThumb != newThumb {
		if _, ok := objects.objects[oldThumb]; ok {
			t.Error("old thumb was not removed")
		}
	}
	if _, ok := objects.objects[*replaced.ImagePath]; !ok {
		t.Error("new image missing")
	}
	if _, ok := objects.objects[newThumb]; !ok {
		t.Error("new thumb missing")
	}

	removed, err := svc.UpdateItem(context.Background(), biz, created.ID, UpdateItemInput{RemoveImage: true})
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed.ImagePath != nil || removed.ImageURL != nil || removed.ImageThumbURL != nil {
		t.Errorf("removed still has image: %+v", removed)
	}

	// Image path is never persisted as a URL.
	if strings.Contains(store.items[created.ID].Price, "http") {
		t.Fatal("unexpected url in price")
	}
	if store.items[created.ID].ImagePath != nil {
		t.Fatal("image_path should be null after remove")
	}

	before := len(store.items)
	objects.uploadErr = storage.ErrUploadFailed
	_, err = svc.CreateItem(context.Background(), biz, CreateItemInput{
		CategoryID: cat.ID.String(), Name: "Mocha", Price: "22000", Image: mustJPEG(t),
	})
	if !errors.Is(err, ErrImageUploadFailed) {
		t.Fatalf("upload failure = %v", err)
	}
	if len(store.items) != before {
		t.Fatalf("failed upload inserted a row: %d", len(store.items))
	}
}

func TestStorageFailureDoesNotDropExistingImageOnReplace(t *testing.T) {
	svc, _, objects := testService()
	biz := uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})
	created, err := svc.CreateItem(context.Background(), biz, CreateItemInput{
		CategoryID: cat.ID.String(), Name: "Cappuccino", Price: "23000", Image: mustJPEG(t),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	old := *created.ImagePath
	objects.uploadErr = storage.ErrUploadFailed
	if _, err := svc.UpdateItem(context.Background(), biz, created.ID, UpdateItemInput{Image: mustPNG(t)}); !errors.Is(err, ErrImageUploadFailed) {
		t.Fatalf("replace error = %v", err)
	}
	got, _ := svc.GetItem(context.Background(), biz, created.ID)
	if got.ImagePath == nil || *got.ImagePath != old {
		t.Fatalf("path changed after failed replace: %v", got.ImagePath)
	}
	if _, ok := objects.objects[old]; !ok {
		t.Fatal("original object was deleted")
	}
}

func TestRemoveImageDeleteFailureKeepsPath(t *testing.T) {
	svc, _, objects := testService()
	biz := uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})
	created, _ := svc.CreateItem(context.Background(), biz, CreateItemInput{
		CategoryID: cat.ID.String(), Name: "Matcha", Price: "28000", Image: mustJPEG(t),
	})
	objects.deleteErr = storage.ErrDeleteFailed
	if _, err := svc.UpdateItem(context.Background(), biz, created.ID, UpdateItemInput{RemoveImage: true}); !errors.Is(err, ErrImageDeleteFailed) {
		t.Fatalf("remove error = %v", err)
	}
	got, _ := svc.GetItem(context.Background(), biz, created.ID)
	if got.ImagePath == nil {
		t.Fatal("image_path was cleared after failed delete")
	}
}

func TestCategoryItemCounts(t *testing.T) {
	svc, _, _ := testService()
	biz := uuid.New()
	cat, _ := svc.CreateCategory(context.Background(), biz, CreateCategoryInput{Name: "Coffee"})
	first, _ := svc.CreateItem(context.Background(), biz, CreateItemInput{CategoryID: cat.ID.String(), Name: "A", Price: "1"})
	_, _ = svc.CreateItem(context.Background(), biz, CreateItemInput{CategoryID: cat.ID.String(), Name: "B", Price: "2"})
	_, _ = svc.DeleteItem(context.Background(), biz, first.ID)
	got, err := svc.GetCategory(context.Background(), biz, cat.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ItemCount != 2 || got.ActiveItemCount != 1 {
		t.Errorf("counts = %d/%d", got.ItemCount, got.ActiveItemCount)
	}
}

func mustJPEG(t *testing.T) *validatedImage {
	t.Helper()
	img, err := validateImage(bytes.NewReader(encodeJPEG(t)), "image/jpeg")
	if err != nil {
		t.Fatalf("jpeg: %v", err)
	}
	return img
}

func mustPNG(t *testing.T) *validatedImage {
	t.Helper()
	img, err := validateImage(bytes.NewReader(encodePNG(t)), "image/png")
	if err != nil {
		t.Fatalf("png: %v", err)
	}
	return img
}

func encodeJPEG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := jpeg.Encode(&buf, m, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}

func encodePNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&buf, m); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
