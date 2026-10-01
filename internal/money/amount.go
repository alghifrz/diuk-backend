// Package money stores currency amounts as integer minor units (1/100)
// so reservation deposits never use floating-point arithmetic.
package money

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type Amount int64

func FromMinor(minor int64) Amount { return Amount(minor) }

func FromMajor(major int64) Amount { return Amount(major * 100) }

func Parse(raw string) (Amount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("amount is required")
	}
	neg := false
	if strings.HasPrefix(raw, "-") {
		neg = true
		raw = raw[1:]
	}
	whole, frac, ok := strings.Cut(raw, ".")
	if !ok {
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("amount is invalid")
		}
		if neg {
			n = -n
		}
		return Amount(n * 100), nil
	}
	if frac == "" || len(frac) > 2 {
		return 0, fmt.Errorf("amount is invalid")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 {
		return 0, fmt.Errorf("amount is invalid")
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil || f < 0 {
		return 0, fmt.Errorf("amount is invalid")
	}
	n := w*100 + f
	if neg {
		n = -n
	}
	return Amount(n), nil
}

func (a Amount) Minor() int64 { return int64(a) }

func (a Amount) Negative() bool { return a < 0 }

func (a Amount) PercentOf(percent Amount) Amount {
	if a < 0 || percent < 0 {
		return 0
	}
	return Amount(int64(a) * int64(percent) / 10000)
}

func (a Amount) String() string {
	sign := ""
	v := int64(a)
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%02d", sign, v/100, v%100)
}

func (a Amount) MarshalJSON() ([]byte, error) {
	if a%100 == 0 {
		return []byte(strconv.FormatInt(int64(a)/100, 10)), nil
	}
	return []byte(a.String()), nil
}

func (a *Amount) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*a = 0
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(data, &n); err == nil {
		parsed, err := Parse(n.String())
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("amount is invalid")
	}
	parsed, err := Parse(s)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

func (a Amount) Value() (driver.Value, error) {
	return a.String(), nil
}

func (a *Amount) Scan(value any) error {
	if value == nil {
		*a = 0
		return nil
	}
	switch v := value.(type) {
	case string:
		parsed, err := Parse(v)
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case []byte:
		parsed, err := Parse(string(v))
		if err != nil {
			return err
		}
		*a = parsed
		return nil
	case int64:
		*a = FromMajor(v)
		return nil
	case pgtype.Numeric:
		if !v.Valid {
			*a = 0
			return nil
		}
		raw, err := v.Value()
		if err != nil {
			return err
		}
		return a.Scan(raw)
	case float64:
		return fmt.Errorf("amount must not be scanned from float64")
	default:
		return fmt.Errorf("amount is invalid")
	}
}

func MustParse(raw string) Amount {
	a, err := Parse(raw)
	if err != nil {
		panic(err)
	}
	return a
}

func ClampNonNegative(a Amount) Amount {
	if a < 0 {
		return 0
	}
	if int64(a) > math.MaxInt64/2 {
		return a
	}
	return a
}
