package automation

import (
	"encoding/json"
	"strings"
	"unicode/utf8"
)

var allowedTriggerTypes = map[string]struct{}{
	TriggerReservationConfirmed: {},
	TriggerReservationCancelled: {},
	TriggerConversationCreated:  {},
	TriggerConversationInactive: {},
	TriggerCustomerCreated:      {},
	TriggerScheduled:            {},
}

var allowedActionTypes = map[string]struct{}{
	ActionSendMessage:       {},
	ActionAIGenerateMessage: {},
}

var allowedOperators = map[string]struct{}{
	OpEQ: {}, OpNEQ: {}, OpIN: {}, OpNOTIN: {},
	OpGT: {}, OpGTE: {}, OpLT: {}, OpLTE: {},
	OpEXISTS: {}, OpNOTExists: {},
}

var allowedConditionFields = map[string]struct{}{
	"reservation.status":     {},
	"reservation.party_size": {},
	"reservation.start_at":   {},
	"customer.status":        {},
	"conversation.status":    {},
	"conversation.channel":   {},
}

func normalizeCreate(in CreateAutomationInput) (CreateAutomationInput, error) {
	name, err := normalizeName(in.Name)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	description, err := normalizeDescription(in.Description)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	triggerType, err := normalizeTriggerType(in.TriggerType)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	triggerConfig, err := normalizeTriggerConfig(triggerType, in.TriggerConfig)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	conditions, err := normalizeConditions(in.Conditions)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	actionType, err := normalizeActionType(in.ActionType)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	actionConfig, err := normalizeActionConfig(actionType, in.ActionConfig)
	if err != nil {
		return CreateAutomationInput{}, err
	}
	return CreateAutomationInput{
		Name:          name,
		Description:   description,
		TriggerType:   triggerType,
		TriggerConfig: triggerConfig,
		Conditions:    conditions,
		ActionType:    actionType,
		ActionConfig:  actionConfig,
	}, nil
}

func normalizeName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" {
		return "", invalid("name is required")
	}
	if utf8.RuneCountInString(name) > MaxNameLen {
		return "", invalid("name is too long")
	}
	return name, nil
}

func normalizeDescription(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(trimmed) > MaxDescriptionLen {
		return nil, invalid("description is too long")
	}
	return &trimmed, nil
}

func normalizeTriggerType(value string) (string, error) {
	trigger := strings.ToUpper(strings.TrimSpace(value))
	if _, ok := allowedTriggerTypes[trigger]; !ok {
		return "", invalid("trigger_type is not supported")
	}
	return trigger, nil
}

func normalizeActionType(value string) (string, error) {
	action := strings.ToUpper(strings.TrimSpace(value))
	if _, ok := allowedActionTypes[action]; !ok {
		return "", invalid("action_type is not supported")
	}
	return action, nil
}

func normalizeTriggerConfig(triggerType string, raw json.RawMessage) (json.RawMessage, error) {
	if err := checkConfigSize(raw, "trigger_config"); err != nil {
		return nil, err
	}
	cfg, err := parseTriggerConfig(raw)
	if err != nil {
		return nil, err
	}
	switch triggerType {
	case TriggerScheduled:
		if strings.TrimSpace(cfg.Reference) == "" {
			cfg.Reference = RefReservationStartAt
		}
		if cfg.Reference != RefReservationStartAt {
			return nil, invalid("trigger_config.reference is not supported")
		}
		if cfg.OffsetMinutes == nil {
			return nil, invalid("trigger_config.offset_minutes is required")
		}
		if *cfg.OffsetMinutes < -10080 || *cfg.OffsetMinutes > 10080 {
			return nil, invalid("trigger_config.offset_minutes is out of range")
		}
		cfg.InactiveMinutes = nil
	case TriggerConversationInactive:
		if cfg.InactiveMinutes == nil {
			return nil, invalid("trigger_config.inactive_minutes is required")
		}
		if *cfg.InactiveMinutes < 1 || *cfg.InactiveMinutes > 10080 {
			return nil, invalid("trigger_config.inactive_minutes is out of range")
		}
		cfg.OffsetMinutes = nil
		cfg.Reference = ""
	default:
		if cfg.OffsetMinutes != nil || cfg.InactiveMinutes != nil || strings.TrimSpace(cfg.Reference) != "" {
			return nil, invalid("trigger_config is not valid for this trigger_type")
		}
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, invalid("trigger_config is invalid")
	}
	return out, nil
}

func normalizeConditions(raw json.RawMessage) (json.RawMessage, error) {
	if err := checkConfigSize(raw, "conditions"); err != nil {
		return nil, err
	}
	conditions, err := parseConditions(raw)
	if err != nil {
		return nil, err
	}
	if len(conditions) > MaxConditions {
		return nil, invalid("too many conditions")
	}
	for i, condition := range conditions {
		field := strings.TrimSpace(condition.Field)
		if _, ok := allowedConditionFields[field]; !ok {
			return nil, invalid("conditions contain an unsupported field")
		}
		op := strings.ToUpper(strings.TrimSpace(condition.Operator))
		if _, ok := allowedOperators[op]; !ok {
			return nil, invalid("conditions contain an unsupported operator")
		}
		if (op == OpIN || op == OpNOTIN) && !isArray(condition.Value) {
			return nil, invalid("IN and NOT_IN require an array value")
		}
		conditions[i].Field = field
		conditions[i].Operator = op
	}
	out, err := json.Marshal(conditions)
	if err != nil {
		return nil, invalid("conditions are invalid")
	}
	return out, nil
}

func normalizeActionConfig(actionType string, raw json.RawMessage) (json.RawMessage, error) {
	if err := checkConfigSize(raw, "action_config"); err != nil {
		return nil, err
	}
	cfg, err := parseActionConfig(raw)
	if err != nil {
		return nil, err
	}
	switch actionType {
	case ActionSendMessage:
		template := strings.TrimSpace(cfg.Template)
		if template == "" {
			return nil, invalid("action_config.template is required")
		}
		if utf8.RuneCountInString(template) > MaxTemplateLen {
			return nil, invalid("action_config.template is too long")
		}
		if err := validateTemplate(template); err != nil {
			return nil, err
		}
		cfg.Template = template
		cfg.Instruction = ""
	case ActionAIGenerateMessage:
		instruction := strings.TrimSpace(cfg.Instruction)
		if utf8.RuneCountInString(instruction) > MaxTemplateLen {
			return nil, invalid("action_config.instruction is too long")
		}
		cfg.Instruction = instruction
		cfg.Template = ""
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return nil, invalid("action_config is invalid")
	}
	return out, nil
}

func parseTriggerConfig(raw json.RawMessage) (TriggerConfig, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return TriggerConfig{}, nil
	}
	var cfg TriggerConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return TriggerConfig{}, invalid("trigger_config is invalid")
	}
	return cfg, nil
}

func parseActionConfig(raw json.RawMessage) (ActionConfig, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return ActionConfig{}, nil
	}
	var cfg ActionConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return ActionConfig{}, invalid("action_config is invalid")
	}
	return cfg, nil
}

func parseConditions(raw json.RawMessage) ([]Condition, error) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || string(raw) == "null" {
		return []Condition{}, nil
	}
	var conditions []Condition
	if err := json.Unmarshal(raw, &conditions); err != nil {
		return nil, invalid("conditions are invalid")
	}
	if conditions == nil {
		return []Condition{}, nil
	}
	return conditions, nil
}

func checkConfigSize(raw json.RawMessage, field string) error {
	if len(raw) > MaxConfigBytes {
		return invalid(field + " is too large")
	}
	return nil
}

func isArray(value any) bool {
	switch value.(type) {
	case []any, []string:
		return true
	default:
		return false
	}
}

func MatchesTrigger(triggerType, eventType string) bool {
	switch triggerType {
	case TriggerReservationConfirmed:
		return eventType == EventReservationConfirmed
	case TriggerReservationCancelled:
		return eventType == EventReservationCancelled
	case TriggerConversationCreated:
		return eventType == EventConversationCreated
	case TriggerCustomerCreated:
		return eventType == EventCustomerCreated
	default:
		return false
	}
}
