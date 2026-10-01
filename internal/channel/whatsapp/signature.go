package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

func VerifySignature(appSecret, header string, body []byte) error {
	if strings.TrimSpace(appSecret) == "" {
		return ErrInvalidSignature
	}
	provided := strings.TrimSpace(header)
	provided = strings.TrimPrefix(provided, "sha256=")
	if provided == "" {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, []byte(appSecret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(strings.ToLower(provided)), []byte(expected)) {
		return ErrInvalidSignature
	}
	return nil
}
