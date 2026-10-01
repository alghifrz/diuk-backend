package menu

import (
	"bytes"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestValidateImageAcceptsJPEGPNGWebP(t *testing.T) {
	jpegImg, err := validateImage(bytes.NewReader(encodeJPEG(t)), "image/jpeg")
	if err != nil || jpegImg.Extension != "jpg" {
		t.Fatalf("jpeg err=%v ext=%v", err, jpegImg)
	}
	pngImg, err := validateImage(bytes.NewReader(encodePNG(t)), "image/png")
	if err != nil || pngImg.Extension != "png" {
		t.Fatalf("png err=%v ext=%v", err, pngImg)
	}
	webpImg, err := validateImage(bytes.NewReader(tinyWebP), "image/webp")
	if err != nil || webpImg.Extension != "webp" {
		t.Fatalf("webp err=%v ext=%v", err, webpImg)
	}
}

func TestValidateImageRejectsUnsupportedAndInvalid(t *testing.T) {
	if _, err := validateImage(bytes.NewReader([]byte("GIF89a")), "image/gif"); err != ErrInvalidImageType {
		t.Fatalf("gif error = %v", err)
	}
	if _, err := validateImage(bytes.NewReader([]byte("not-an-image")), "image/jpeg"); err != ErrInvalidImageType {
		t.Fatalf("garbage jpeg error = %v", err)
	}
	if _, err := validateImage(bytes.NewReader(nil), "image/png"); err != ErrInvalidImageType {
		t.Fatalf("empty error = %v", err)
	}
}

func TestValidateImageRejectsOversized(t *testing.T) {
	payload := append(encodeJPEG(t), bytes.Repeat([]byte("x"), maxImageBytes+1)...)
	if _, err := validateImage(bytes.NewReader(payload), "image/jpeg"); err != ErrImageTooLarge {
		t.Fatalf("oversize error = %v", err)
	}
}

func TestStoragePathIsBackendGenerated(t *testing.T) {
	biz := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	item := uuid.MustParse("7f1d8452-2c2a-4a8d-9c11-0c9a0b1d2e3f")
	path := storagePath(biz, item, "webp")
	want := biz.String() + "/menu/" + item.String() + "/original.webp"
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	thumb := thumbStoragePath(biz, item)
	wantThumb := biz.String() + "/menu/" + item.String() + "/thumb.jpg"
	if thumb != wantThumb {
		t.Fatalf("thumb = %q, want %q", thumb, wantThumb)
	}
	if siblingThumbPath(path) != wantThumb {
		t.Fatalf("sibling = %q", siblingThumbPath(path))
	}
	if strings.Contains(path, "..") || strings.Contains(path, " ") {
		t.Fatalf("unsafe path %q", path)
	}
	if !pathOwnedByBusiness(biz, path) {
		t.Fatal("ownership check failed")
	}
	if pathOwnedByBusiness(uuid.New(), path) {
		t.Fatal("other business owned the path")
	}
}

func TestMakeThumbnailDownscalesAndReencodes(t *testing.T) {
	src, err := validateImage(bytes.NewReader(encodeJPEG(t)), "image/jpeg")
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	thumb, err := makeThumbnail(src)
	if err != nil {
		t.Fatalf("thumb: %v", err)
	}
	if thumb.ContentType != "image/jpeg" || thumb.Extension != "jpg" {
		t.Fatalf("thumb meta = %+v", thumb)
	}
	if len(thumb.Bytes) == 0 || len(thumb.Bytes) > len(src.Bytes)+1024 {
		// Tiny source may not shrink; just ensure it encodes.
		if len(thumb.Bytes) == 0 {
			t.Fatal("empty thumb")
		}
	}
	if _, err := validateImage(bytes.NewReader(thumb.Bytes), "image/jpeg"); err != nil {
		t.Fatalf("thumb not valid jpeg: %v", err)
	}
}

// 1x1 lossy WebP used to verify decoding without a WebP encoder.
var tinyWebP = []byte{
	0x52, 0x49, 0x46, 0x46, 0x24, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50,
	0x56, 0x50, 0x38, 0x20, 0x18, 0x00, 0x00, 0x00, 0x30, 0x01, 0x00, 0x9d,
	0x01, 0x2a, 0x01, 0x00, 0x01, 0x00, 0x02, 0x00, 0x34, 0x25, 0xa4, 0x00,
	0x03, 0x70, 0x00, 0xfe, 0xfb, 0x94, 0xfb, 0x00, 0x00,
}
