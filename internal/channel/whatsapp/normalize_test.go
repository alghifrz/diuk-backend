package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNormalizeTextAndUnsupported(t *testing.T) {
	raw := []byte(`{
	  "object":"whatsapp_business_account",
	  "entry":[{"id":"waba","changes":[{"value":{
	    "metadata":{"phone_number_id":"pn-1","display_phone_number":"62800"},
	    "contacts":[{"wa_id":"628111","profile":{"name":"Budi"}}],
	    "messages":[{"id":"wamid.1","from":"628111","timestamp":"1710000000","type":"text","text":{"body":"Halo"}}]
	  }}]}]
	}`)
	incoming, statuses, err := Normalize(raw)
	if err != nil || len(incoming) != 1 || len(statuses) != 0 {
		t.Fatalf("normalize = %+v %+v err=%v", incoming, statuses, err)
	}
	if incoming[0].ExternalAccountID != "pn-1" || incoming[0].ExternalIdentityID != "628111" || incoming[0].Text != "Halo" || incoming[0].CustomerName != "Budi" {
		t.Fatalf("incoming = %+v", incoming[0])
	}

	image := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"pn-1"},"messages":[{"id":"wamid.img","from":"628111","type":"image"}]}}]}]}`)
	incoming, _, err = Normalize(image)
	if err != nil || incoming[0].MessageType != messageTypeUnsupported {
		t.Fatalf("image = %+v err=%v", incoming, err)
	}
}

func TestNormalizeMalformed(t *testing.T) {
	if _, _, err := Normalize([]byte(`{`)); err == nil {
		t.Fatal("malformed")
	}
	if _, _, err := Normalize([]byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"messages":[{"id":"x","from":"1","type":"text"}]}}]}]}`)); err == nil {
		t.Fatal("missing phone_number_id")
	}
	if _, _, err := Normalize([]byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"pn"},"messages":[{"id":"","from":"","type":"text"}]}}]}]}`)); err == nil {
		t.Fatal("missing sender/id")
	}
}

func TestNormalizeStatus(t *testing.T) {
	raw := []byte(`{"object":"whatsapp_business_account","entry":[{"changes":[{"value":{"metadata":{"phone_number_id":"pn-1"},"statuses":[{"id":"wamid.out","status":"delivered"}]}}]}]}`)
	_, statuses, err := Normalize(raw)
	if err != nil || len(statuses) != 1 || statuses[0].Status != "DELIVERED" {
		t.Fatalf("status = %+v err=%v", statuses, err)
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"object":"whatsapp_business_account"}`)
	header := "sha256=" + sign(body, "app-secret")
	if err := VerifySignature("app-secret", header, body); err != nil {
		t.Fatalf("valid: %v", err)
	}
	if err := VerifySignature("app-secret", "sha256="+sign(body, "wrong"), body); err == nil {
		t.Fatal("invalid signature")
	}
	if err := VerifySignature("app-secret", "", body); err == nil {
		t.Fatal("missing signature")
	}
	if err := VerifySignature("app-secret", header, []byte(`{"changed":true}`)); err == nil {
		t.Fatal("modified payload")
	}
}

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}
