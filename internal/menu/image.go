package menu

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/google/uuid"
)

const (
	maxImageBytes    = 5 << 20
	maxUploadBytes   = maxImageBytes + 1<<20
	imageObjectName  = "original"
	thumbObjectName  = "thumb"
	thumbMaxEdge     = 512
	thumbJPEGQuality = 78
)

var allowedImageTypes = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

type validatedImage struct {
	ContentType string
	Extension   string
	Bytes       []byte
}

func validateImage(r io.Reader, declaredType string) (*validatedImage, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxImageBytes+1))
	if err != nil {
		return nil, ErrInvalidImageType
	}
	if len(data) == 0 {
		return nil, ErrInvalidImageType
	}
	if len(data) > maxImageBytes {
		return nil, ErrImageTooLarge
	}

	declaredType = strings.ToLower(strings.TrimSpace(declaredType))
	if i := strings.Index(declaredType, ";"); i >= 0 {
		declaredType = strings.TrimSpace(declaredType[:i])
	}
	detected := http.DetectContentType(data)

	contentType := detected
	if _, ok := allowedImageTypes[detected]; !ok {
		if _, declaredOK := allowedImageTypes[declaredType]; declaredOK {
			contentType = declaredType
		} else {
			return nil, ErrInvalidImageType
		}
	}
	if declaredType != "" && declaredType != "application/octet-stream" {
		if _, ok := allowedImageTypes[declaredType]; !ok {
			return nil, ErrInvalidImageType
		}
	}

	ext, ok := allowedImageTypes[contentType]
	if !ok {
		return nil, ErrInvalidImageType
	}

	if _, format, err := image.Decode(bytes.NewReader(data)); err != nil {
		return nil, ErrInvalidImageType
	} else if !imageFormatMatches(format, contentType) {
		return nil, ErrInvalidImageType
	}

	return &validatedImage{ContentType: contentType, Extension: ext, Bytes: data}, nil
}

func imageFormatMatches(format, contentType string) bool {
	switch contentType {
	case "image/jpeg":
		return format == "jpeg"
	case "image/png":
		return format == "png"
	case "image/webp":
		return format == "webp"
	default:
		return false
	}
}

func storagePath(businessID, itemID uuid.UUID, ext string) string {
	return businessID.String() + "/menu/" + itemID.String() + "/" + imageObjectName + "." + ext
}

func thumbStoragePath(businessID, itemID uuid.UUID) string {
	return businessID.String() + "/menu/" + itemID.String() + "/" + thumbObjectName + ".jpg"
}

// siblingThumbPath derives the card thumbnail key from an original object path.
func siblingThumbPath(originalPath string) string {
	i := strings.LastIndex(originalPath, "/")
	if i < 0 {
		return ""
	}
	return originalPath[:i+1] + thumbObjectName + ".jpg"
}

func pathOwnedByBusiness(businessID uuid.UUID, path string) bool {
	return strings.HasPrefix(path, businessID.String()+"/menu/")
}

// makeThumbnail downscales for menu cards and re-encodes as JPEG.
// JPEG avoids a WebP encoder dependency while staying small for grid loads.
func makeThumbnail(src *validatedImage) (*validatedImage, error) {
	img, _, err := image.Decode(bytes.NewReader(src.Bytes))
	if err != nil {
		return nil, ErrInvalidImageType
	}

	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return nil, ErrInvalidImageType
	}

	nw, nh := w, h
	if w > thumbMaxEdge || h > thumbMaxEdge {
		scale := float64(thumbMaxEdge) / float64(w)
		if h > w {
			scale = float64(thumbMaxEdge) / float64(h)
		}
		nw = int(float64(w)*scale + 0.5)
		nh = int(float64(h)*scale + 0.5)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: thumbJPEGQuality}); err != nil {
		return nil, ErrInvalidImageType
	}

	return &validatedImage{
		ContentType: "image/jpeg",
		Extension:   "jpg",
		Bytes:       buf.Bytes(),
	}, nil
}
