package automation

import (
	"errors"
	"testing"
	"time"
)

func TestRenderTemplateVariables(t *testing.T) {
	vars := TemplateVars("Budi Santoso", &ReservationView{
		Status: "CONFIRMED", PartySize: 4, StartAt: time.Date(2026, 9, 25, 19, 30, 0, 0, time.UTC),
	}, "Warung DIUK", strPtr("0812"), time.UTC)
	out, err := RenderTemplate("Halo {{customer.first_name}}, {{reservation.start_time}} {{reservation.party_size}} di {{business.name}}", vars)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Halo Budi, 19:30 4 di Warung DIUK" {
		t.Fatalf("got %q", out)
	}
}

func TestRenderTemplateRejectsUnsafeAndUnknown(t *testing.T) {
	if _, err := RenderTemplate("{{customer.ssn}}", map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown var = %v", err)
	}
	if _, err := RenderTemplate("{{customer.first_name|upper}}", map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expression = %v", err)
	}
	if _, err := RenderTemplate("{{os.Exit(1)}}", map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("code = %v", err)
	}
	if _, err := RenderTemplate("Halo {{customer.first_name}", map[string]string{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("malformed = %v", err)
	}
	out, err := RenderTemplate("Halo {{customer.first_name}}", map[string]string{})
	if err != nil || out != "Halo" {
		t.Fatalf("missing value = %q %v", out, err)
	}
}

func strPtr(v string) *string { return &v }
