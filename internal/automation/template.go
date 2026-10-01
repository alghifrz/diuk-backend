package automation

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	templateToken = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
	allowedVars   = map[string]struct{}{
		"customer.first_name":    {},
		"customer.full_name":     {},
		"reservation.start_time": {},
		"reservation.start_date": {},
		"reservation.party_size": {},
		"reservation.status":     {},
		"business.name":          {},
		"business.phone":         {},
	}
)

func validateTemplate(template string) error {
	if strings.Contains(template, "{{") && !templateToken.MatchString(template) {
		return invalid("action_config.template has malformed syntax")
	}
	matches := templateToken.FindAllStringSubmatch(template, -1)
	for _, match := range matches {
		name := strings.TrimSpace(match[1])
		if strings.ContainsAny(name, "()|+*/") || strings.Contains(name, " ") {
			return invalid("action_config.template cannot execute expressions")
		}
		if _, ok := allowedVars[name]; !ok {
			return invalid("action_config.template contains an unsupported variable")
		}
	}
	open := strings.Count(template, "{{")
	close := strings.Count(template, "}}")
	if open != close {
		return invalid("action_config.template has malformed syntax")
	}
	return nil
}

func RenderTemplate(template string, vars map[string]string) (string, error) {
	if err := validateTemplate(template); err != nil {
		return "", err
	}
	out := templateToken.ReplaceAllStringFunc(template, func(token string) string {
		match := templateToken.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		name := strings.TrimSpace(match[1])
		if value, ok := vars[name]; ok {
			return value
		}
		return ""
	})
	return strings.TrimSpace(out), nil
}

func TemplateVars(customerName string, reservation *ReservationView, businessName string, businessPhone *string, loc *time.Location) map[string]string {
	if loc == nil {
		loc = time.UTC
	}
	vars := map[string]string{
		"customer.full_name":  strings.TrimSpace(customerName),
		"customer.first_name": firstName(customerName),
		"business.name":       strings.TrimSpace(businessName),
		"business.phone":      stringOrEmpty(businessPhone),
	}
	if reservation != nil {
		start := reservation.StartAt.In(loc)
		vars["reservation.start_time"] = start.Format("15:04")
		vars["reservation.start_date"] = start.Format("2006-01-02")
		vars["reservation.party_size"] = fmt.Sprintf("%d", reservation.PartySize)
		vars["reservation.status"] = reservation.Status
	}
	return vars
}

func firstName(name string) string {
	fields := strings.Fields(strings.TrimSpace(name))
	if len(fields) == 0 {
		return ""
	}
	runes := []rune(fields[0])
	if len(runes) == 0 {
		return ""
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

func stringOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

type ReservationView struct {
	Status    string
	PartySize int
	StartAt   time.Time
}
