package menu

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

var (
	ErrInvalid                 = errors.New("invalid request")
	ErrCategoryNotFound        = errors.New("category not found")
	ErrItemNotFound            = errors.New("menu item not found")
	ErrCategoryNameExists      = errors.New("category name already exists")
	ErrCategoryHasActiveItems  = errors.New("category has active menu items")
	ErrCategoryInactive        = errors.New("category is not active")
	ErrInvalidPrice            = errors.New("price is invalid")
	ErrInvalidSortOrder        = errors.New("sort_order is invalid")
	ErrInvalidImageType        = errors.New("image type is invalid")
	ErrImageTooLarge           = errors.New("image is too large")
	ErrImageUploadFailed       = errors.New("image upload failed")
	ErrImageDeleteFailed       = errors.New("image delete failed")
	ErrSignedURLFailed         = errors.New("signed url failed")
	ErrMenuItemCategoryInvalid = errors.New("menu item category is invalid")
)

type validationError struct {
	msg string
}

func (e validationError) Error() string { return e.msg }
func (e validationError) Unwrap() error { return ErrInvalid }

func invalid(msg string) error {
	return validationError{msg: msg}
}

func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": gin.H{
			"code":    code,
			"message": message,
		},
	})
}

func writeServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidPrice):
		writeError(c, http.StatusBadRequest, "INVALID_PRICE", "price is invalid")
	case errors.Is(err, ErrInvalidSortOrder):
		writeError(c, http.StatusBadRequest, "INVALID_SORT_ORDER", "sort_order is invalid")
	case errors.Is(err, ErrInvalidImageType):
		writeError(c, http.StatusBadRequest, "INVALID_IMAGE_TYPE", "image type is invalid")
	case errors.Is(err, ErrImageTooLarge):
		writeError(c, http.StatusBadRequest, "IMAGE_TOO_LARGE", "image is too large")
	case errors.Is(err, ErrInvalid):
		writeError(c, http.StatusBadRequest, "BAD_REQUEST", err.Error())
	case errors.Is(err, ErrCategoryNotFound):
		writeError(c, http.StatusNotFound, "CATEGORY_NOT_FOUND", "category not found")
	case errors.Is(err, ErrItemNotFound):
		writeError(c, http.StatusNotFound, "MENU_ITEM_NOT_FOUND", "menu item not found")
	case errors.Is(err, ErrCategoryNameExists):
		writeError(c, http.StatusConflict, "CATEGORY_NAME_ALREADY_EXISTS", "category name already exists")
	case errors.Is(err, ErrCategoryHasActiveItems):
		writeError(c, http.StatusConflict, "CATEGORY_HAS_ACTIVE_ITEMS", "category has active menu items")
	case errors.Is(err, ErrCategoryInactive), errors.Is(err, ErrMenuItemCategoryInvalid):
		writeError(c, http.StatusConflict, "MENU_ITEM_CATEGORY_INVALID", "menu item category is invalid")
	case errors.Is(err, ErrImageUploadFailed):
		writeError(c, http.StatusInternalServerError, "IMAGE_UPLOAD_FAILED", "image upload failed")
	case errors.Is(err, ErrImageDeleteFailed):
		writeError(c, http.StatusInternalServerError, "IMAGE_DELETE_FAILED", "image delete failed")
	case errors.Is(err, ErrSignedURLFailed):
		writeError(c, http.StatusInternalServerError, "IMAGE_SIGNED_URL_FAILED", "signed url failed")
	default:
		writeError(c, http.StatusInternalServerError, "INTERNAL", "internal server error")
	}
}

func isClientError(err error) bool {
	return errors.Is(err, ErrInvalid) ||
		errors.Is(err, ErrInvalidPrice) ||
		errors.Is(err, ErrInvalidSortOrder) ||
		errors.Is(err, ErrInvalidImageType) ||
		errors.Is(err, ErrImageTooLarge) ||
		errors.Is(err, ErrCategoryNotFound) ||
		errors.Is(err, ErrItemNotFound) ||
		errors.Is(err, ErrCategoryNameExists) ||
		errors.Is(err, ErrCategoryHasActiveItems) ||
		errors.Is(err, ErrCategoryInactive) ||
		errors.Is(err, ErrMenuItemCategoryInvalid)
}
