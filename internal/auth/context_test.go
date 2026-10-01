package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestGetAuthContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())

	if _, ok := GetAuthContext(c); ok {
		t.Fatal("GetAuthContext() ok = true, want false when missing")
	}

	want := Context{
		AuthUserID: uuid.New(),
		UserID:     uuid.New(),
		FullName:   "Ada",
		Email:      "ada@example.com",
		BusinessID: uuid.New(),
		RoleID:     uuid.New(),
		RoleName:   "OWNER",
	}
	setAuthContext(c, want)

	got, ok := GetAuthContext(c)
	if !ok {
		t.Fatal("GetAuthContext() ok = false, want true")
	}
	if got != want {
		t.Errorf("GetAuthContext() = %+v, want %+v", got, want)
	}
	if !got.HasTenant() {
		t.Error("HasTenant() = false, want true")
	}
}

func TestHasTenantFalseWhenBusinessMissing(t *testing.T) {
	ac := Context{UserID: uuid.New()}
	if ac.HasTenant() {
		t.Error("HasTenant() = true, want false")
	}
}
