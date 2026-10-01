package money

import (
	"encoding/json"
	"testing"
)

func TestAmountParseAndJSON(t *testing.T) {
	got, err := Parse("100000")
	if err != nil || got != FromMajor(100000) {
		t.Fatalf("parse 100000 = %v err=%v", got, err)
	}
	got, err = Parse("100000.50")
	if err != nil || got != FromMajor(100000)+50 {
		t.Fatalf("parse cents = %v err=%v", got, err)
	}
	raw, err := json.Marshal(FromMajor(100000))
	if err != nil || string(raw) != "100000" {
		t.Fatalf("marshal = %s err=%v", raw, err)
	}
	var a Amount
	if err := json.Unmarshal([]byte("100000"), &a); err != nil || a != FromMajor(100000) {
		t.Fatalf("unmarshal = %v err=%v", a, err)
	}
	if err := a.Scan(float64(1)); err == nil {
		t.Fatal("float64 scan must fail")
	}
}

func TestPercentOf(t *testing.T) {
	base := FromMajor(500000)
	got := base.PercentOf(FromMajor(20))
	if got != FromMajor(100000) {
		t.Fatalf("20%% of 500000 = %v", got)
	}
}
