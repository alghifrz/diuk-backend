package automation

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func EvaluateConditions(raw []byte, ctx map[string]any) (bool, error) {
	conditions, err := parseConditions(raw)
	if err != nil {
		return false, err
	}
	for _, condition := range conditions {
		ok, err := evaluateCondition(condition, ctx)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

func evaluateCondition(condition Condition, ctx map[string]any) (bool, error) {
	value, exists := lookupField(ctx, condition.Field)
	switch condition.Operator {
	case OpEXISTS:
		return exists && !isEmpty(value), nil
	case OpNOTExists:
		return !exists || isEmpty(value), nil
	}
	if !exists {
		return false, nil
	}
	switch condition.Operator {
	case OpEQ:
		return compareEqual(value, condition.Value), nil
	case OpNEQ:
		return !compareEqual(value, condition.Value), nil
	case OpIN:
		return containsValue(condition.Value, value), nil
	case OpNOTIN:
		return !containsValue(condition.Value, value), nil
	case OpGT, OpGTE, OpLT, OpLTE:
		cmp, ok := compareOrdered(value, condition.Value)
		if !ok {
			return false, nil
		}
		switch condition.Operator {
		case OpGT:
			return cmp > 0, nil
		case OpGTE:
			return cmp >= 0, nil
		case OpLT:
			return cmp < 0, nil
		case OpLTE:
			return cmp <= 0, nil
		}
	}
	return false, invalid("unsupported operator")
}

func lookupField(ctx map[string]any, path string) (any, bool) {
	current := any(ctx)
	for _, part := range strings.Split(path, ".") {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := asMap[part]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

func compareEqual(left, right any) bool {
	if leftTime, ok := asTime(left); ok {
		if rightTime, ok := asTime(right); ok {
			return leftTime.Equal(rightTime)
		}
	}
	if leftNum, ok := asFloat(left); ok {
		if rightNum, ok := asFloat(right); ok {
			return leftNum == rightNum
		}
	}
	return strings.EqualFold(fmt.Sprint(left), fmt.Sprint(right))
}

func containsValue(haystack any, needle any) bool {
	switch items := haystack.(type) {
	case []any:
		for _, item := range items {
			if compareEqual(needle, item) {
				return true
			}
		}
	case []string:
		for _, item := range items {
			if compareEqual(needle, item) {
				return true
			}
		}
	default:
		rv := reflect.ValueOf(haystack)
		if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
			return false
		}
		for i := 0; i < rv.Len(); i++ {
			if compareEqual(needle, rv.Index(i).Interface()) {
				return true
			}
		}
	}
	return false
}

func compareOrdered(left, right any) (int, bool) {
	if leftTime, ok := asTime(left); ok {
		if rightTime, ok := asTime(right); ok {
			switch {
			case leftTime.Before(rightTime):
				return -1, true
			case leftTime.After(rightTime):
				return 1, true
			default:
				return 0, true
			}
		}
		return 0, false
	}
	leftNum, okLeft := asFloat(left)
	rightNum, okRight := asFloat(right)
	if !okLeft || !okRight {
		return 0, false
	}
	switch {
	case leftNum < rightNum:
		return -1, true
	case leftNum > rightNum:
		return 1, true
	default:
		return 0, true
	}
}

func asFloat(value any) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case jsonNumber:
		f, err := n.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func asTime(value any) (time.Time, bool) {
	switch t := value.(type) {
	case time.Time:
		return t, true
	case *time.Time:
		if t == nil {
			return time.Time{}, false
		}
		return *t, true
	case string:
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(t))
		if err != nil {
			return time.Time{}, false
		}
		return parsed, true
	default:
		return time.Time{}, false
	}
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v) == ""
	case *string:
		return v == nil || strings.TrimSpace(*v) == ""
	default:
		return false
	}
}

type jsonNumber interface {
	Float64() (float64, error)
}
