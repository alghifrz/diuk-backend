package tools

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

type Schema struct {
	Type                 string            `json:"type"`
	Properties           map[string]Schema `json:"properties,omitempty"`
	Required             []string          `json:"required,omitempty"`
	AdditionalProperties *bool             `json:"additionalProperties,omitempty"`
	Description          string            `json:"description,omitempty"`
	Minimum              *float64          `json:"minimum,omitempty"`
	Format               string            `json:"format,omitempty"`
	Items                *Schema           `json:"items,omitempty"`
}

var (
	datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	timePattern = regexp.MustCompile(`^\d{2}:\d{2}(:\d{2})?$`)
)

func objectSchema(properties map[string]Schema, required []string) Schema {
	additional := false
	return Schema{
		Type:                 "object",
		Properties:           properties,
		Required:             required,
		AdditionalProperties: &additional,
	}
}

func (s Schema) MarshalJSON() ([]byte, error) {
	type alias Schema
	return json.Marshal(alias(s))
}

func Validate(schema Schema, raw json.RawMessage) (map[string]any, error) {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("arguments are not valid json")
	}
	if err := validateValue(schema, payload, ""); err != nil {
		return nil, err
	}
	obj, _ := payload.(map[string]any)
	if obj == nil {
		obj = map[string]any{}
	}
	return obj, nil
}

func validateValue(schema Schema, value any, path string) error {
	switch schema.Type {
	case "", "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", label(path))
		}
		required := map[string]struct{}{}
		for _, key := range schema.Required {
			required[key] = struct{}{}
		}
		for key := range required {
			if _, ok := obj[key]; !ok {
				return fmt.Errorf("%s is required", label(join(path, key)))
			}
		}
		if schema.AdditionalProperties != nil && !*schema.AdditionalProperties {
			for key := range obj {
				if _, ok := schema.Properties[key]; !ok {
					return fmt.Errorf("%s is not allowed", label(join(path, key)))
				}
			}
		}
		for key, child := range schema.Properties {
			item, ok := obj[key]
			if !ok {
				continue
			}
			if err := validateValue(child, item, join(path, key)); err != nil {
				return err
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", label(path))
		}
		if strings.TrimSpace(text) == "" && path != "" {
			return fmt.Errorf("%s must not be empty", label(path))
		}
		switch schema.Format {
		case "date":
			if !datePattern.MatchString(text) {
				return fmt.Errorf("%s must be a date (YYYY-MM-DD)", label(path))
			}
		case "time":
			if !timePattern.MatchString(text) {
				return fmt.Errorf("%s must be a time (HH:MM)", label(path))
			}
		}
	case "integer":
		n, ok := asInt(value)
		if !ok {
			return fmt.Errorf("%s must be an integer", label(path))
		}
		if schema.Minimum != nil && float64(n) < *schema.Minimum {
			return fmt.Errorf("%s must be at least %v", label(path), *schema.Minimum)
		}
	case "number":
		n, ok := asFloat(value)
		if !ok {
			return fmt.Errorf("%s must be a number", label(path))
		}
		if schema.Minimum != nil && n < *schema.Minimum {
			return fmt.Errorf("%s must be at least %v", label(path), *schema.Minimum)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", label(path))
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", label(path))
		}
		if schema.Items != nil {
			for i, el := range arr {
				if err := validateValue(*schema.Items, el, fmt.Sprintf("%s[%d]", path, i)); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported schema type %q", schema.Type)
	}
	return nil
}

func asInt(value any) (int, bool) {
	switch n := value.(type) {
	case float64:
		if n != math.Trunc(n) {
			return 0, false
		}
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

func asFloat(value any) (float64, bool) {
	switch n := value.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func label(path string) string {
	if path == "" {
		return "arguments"
	}
	return path
}

func intSchema(minimum float64, description string) Schema {
	return Schema{Type: "integer", Minimum: &minimum, Description: description}
}

func stringSchema(description, format string) Schema {
	return Schema{Type: "string", Description: description, Format: format}
}

func arraySchema(item Schema, description string) Schema {
	return Schema{Type: "array", Description: description, Items: &item}
}
