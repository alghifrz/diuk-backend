package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/ai/tools"
	"github.com/alghifrz/diuk-backend/internal/conversation"
	"github.com/alghifrz/diuk-backend/internal/usage"
)

func (s *Service) generate(ctx context.Context, businessID uuid.UUID, in GenerateReplyInput) (*GenerateReplyResult, error) {
	started := time.Now()
	if s.conversations != nil {
		conv, err := s.conversations.GetConversation(ctx, businessID, in.ConversationID)
		if err != nil {
			if isNotFound(err) {
				return nil, ErrConversationNotFound
			}
			return nil, fmt.Errorf("%w: load conversation", ErrContext)
		}
		if !conversation.AllowsAIGeneration(conv.HandlingState) {
			return nil, ErrHumanHandling
		}
	}
	settings, err := s.GetSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	if !settings.Enabled {
		return nil, ErrDisabled
	}
	if err := s.ensureProvider(settings.Provider); err != nil {
		return nil, err
	}

	if existing := s.existingReply(ctx, businessID, in.UsageSourceID); existing != nil {
		return existingReplyResult(existing, settings), nil
	}

	if in.PersistIncoming && in.Message != "" {
		content := in.Message
		if _, _, err := s.conversations.CreateMessage(ctx, businessID, in.ConversationID, conversation.CreateMessageInput{
			Direction:   string(conversation.DirectionInbound),
			SenderType:  string(conversation.SenderCustomer),
			MessageType: string(conversation.MessageTypeText),
			Content:     &content,
			Status:      string(conversation.MessageReceived),
		}); err != nil {
			if isNotFound(err) {
				return nil, ErrConversationNotFound
			}
			return nil, fmt.Errorf("%w: persist incoming", ErrContext)
		}
	}

	if err := s.checkAIQuota(ctx, businessID); err != nil {
		return nil, err
	}

	promptBody, err := s.activePromptBody(ctx, businessID)
	if err != nil {
		return nil, err
	}
	runtime, err := s.BuildContext(ctx, businessID, in.ConversationID, promptBody)
	if err != nil {
		return nil, err
	}

	systemPrompt := runtime.SystemPrompt
	if !in.DisableMutatingTools {
		if quotes := s.tools.Quotes(); quotes != nil {
			if pending, ok := quotes.Get(in.ConversationID); ok && pending.BusinessID == businessID {
				systemPrompt += "\n\n" + tools.PendingBookingBlock(pending)
			}
		}
	}
	messages := []provider.Message{{Role: provider.RoleSystem, Content: systemPrompt}}
	for _, msg := range runtime.History {
		if !includeHistory(msg) {
			continue
		}
		messages = append(messages, provider.Message{Role: historyRole(msg), Content: messageText(msg)})
	}
	if !in.PersistIncoming && in.Message != "" {
		messages = append(messages, provider.Message{Role: provider.RoleUser, Content: in.Message})
	}

	defs := s.tools.Definitions()
	toolDefs := make([]provider.ToolDefinition, 0, len(defs))
	for _, def := range defs {
		if in.DisableMutatingTools && mutatingTool(def.Name) {
			continue
		}
		toolDefs = append(toolDefs, provider.ToolDefinition{Name: def.Name, Description: def.Description, Parameters: def.Parameters})
	}

	turnID := in.UsageSourceID
	if turnID == uuid.Nil {
		turnID = uuid.New()
	}
	exec := tools.ExecContext{
		BusinessID:     businessID,
		UserID:         in.UserID,
		ConversationID: in.ConversationID,
		CustomerID:     runtime.CustomerID,
		TurnID:         turnID,
	}

	var (
		toolRounds int
		toolCalls  int
		seen       = map[string]json.RawMessage{}
		finalText  string
		model      = settings.Model
	)

	for round := 0; round <= MaxToolRounds; round++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		llmCtx, cancel := context.WithTimeout(ctx, s.defaults.Timeout)
		resp, err := s.llm.Generate(llmCtx, provider.GenerateRequest{
			Model:       settings.Model,
			Temperature: settings.Temperature,
			MaxTokens:   settings.MaxOutputTokens,
			Messages:    messages,
			Tools:       toolDefs,
		})
		cancel()
		if err != nil {
			s.log.Error("ai provider failed",
				"business_id", businessID,
				"conversation_id", in.ConversationID,
				"provider", settings.Provider,
				"model", settings.Model,
				"error", sanitizeProviderError(err),
			)
			if errors.Is(err, provider.ErrNotConfigured) {
				return nil, ErrProviderNotConfigured
			}
			if errors.Is(err, provider.ErrResponse) {
				return nil, ErrInvalidResponse
			}
			return nil, ErrProvider
		}
		if resp.Model != "" {
			model = resp.Model
		}
		if len(resp.ToolCalls) == 0 {
			finalText = resp.Content
			break
		}
		if round == MaxToolRounds {
			return nil, ErrToolLimit
		}
		toolRounds++
		messages = append(messages, provider.Message{Role: provider.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			toolCalls++
			if toolCalls > MaxToolCalls {
				return nil, ErrToolLimit
			}
			result, execErr := s.executeTool(ctx, exec, call, seen, in.DisableMutatingTools)
			s.log.Info("ai tool executed",
				"business_id", businessID,
				"conversation_id", in.ConversationID,
				"provider", settings.Provider,
				"model", settings.Model,
				"tool", call.Name,
				"success", result.Error == "" && execErr == nil,
				"error_code", result.Code,
				"error", result.Error,
			)
			payload, _ := json.Marshal(result)
			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				ToolCallID: call.ID,
				Content:    string(payload),
			})
		}
	}
	if finalText == "" {
		return nil, ErrInvalidResponse
	}

	if err := s.consumeAIQuota(ctx, businessID, in.UsageSourceID); err != nil {
		return nil, err
	}

	content := finalText
	msg, created, err := s.conversations.CreateMessage(ctx, businessID, in.ConversationID, conversation.CreateMessageInput{
		Direction:   string(conversation.DirectionOutbound),
		SenderType:  string(conversation.SenderAI),
		MessageType: string(conversation.MessageTypeText),
		Content:     &content,
		ExternalID:  aiReplyExternalID(in.UsageSourceID),
		Status:      string(conversation.MessageQueued),
	})
	if err != nil {
		if isNotFound(err) {
			return nil, ErrConversationNotFound
		}
		return nil, fmt.Errorf("%w: persist reply", ErrContext)
	}
	if !created {
		return existingReplyResult(msg, settings), nil
	}

	duration := time.Since(started)
	s.log.Info("ai generation completed",
		"business_id", businessID,
		"conversation_id", in.ConversationID,
		"provider", settings.Provider,
		"model", model,
		"tool_rounds", toolRounds,
		"tool_calls", toolCalls,
		"duration_ms", duration.Milliseconds(),
		"success", true,
	)
	return &GenerateReplyResult{
		Reply:      content,
		MessageID:  msg.ID,
		Provider:   settings.Provider,
		Model:      model,
		ToolRounds: toolRounds,
		ToolCalls:  toolCalls,
		Duration:   duration,
	}, nil
}

func aiReplyExternalID(source uuid.UUID) *string {
	if source == uuid.Nil {
		return nil
	}
	id := "ai-reply:" + source.String()
	return &id
}

func (s *Service) existingReply(ctx context.Context, businessID, sourceID uuid.UUID) *conversation.Message {
	ext := aiReplyExternalID(sourceID)
	if ext == nil || s.conversations == nil {
		return nil
	}
	existing, err := s.conversations.FindMessageByExternalID(ctx, businessID, *ext)
	if err != nil || existing == nil {
		return nil
	}
	return existing
}

func existingReplyResult(msg *conversation.Message, settings *Settings) *GenerateReplyResult {
	reply := ""
	if msg.Content != nil {
		reply = *msg.Content
	}
	return &GenerateReplyResult{
		Reply:     reply,
		MessageID: msg.ID,
		Provider:  settings.Provider,
		Model:     settings.Model,
	}
}

func (s *Service) checkAIQuota(ctx context.Context, businessID uuid.UUID) error {
	if s.meter == nil {
		return nil
	}
	if err := s.meter.Check(ctx, businessID, usage.FeatureAIResponses); err != nil {
		return mapQuotaError(err)
	}
	return nil
}

func (s *Service) consumeAIQuota(ctx context.Context, businessID, sourceID uuid.UUID) error {
	if s.meter == nil {
		return nil
	}
	if sourceID == uuid.Nil {
		sourceID = uuid.New()
	}
	if _, err := s.meter.Consume(ctx, businessID, usage.ConsumeInput{
		Feature:    usage.FeatureAIResponses,
		Quantity:   1,
		SourceType: usage.SourceAIGeneration,
		SourceID:   sourceID,
	}); err != nil {
		return mapQuotaError(err)
	}
	return nil
}

func mapQuotaError(err error) error {
	if errors.Is(err, usage.ErrQuotaExceeded) || errors.Is(err, usage.ErrFeatureDisabled) || errors.Is(err, usage.ErrNotEntitled) || errors.Is(err, usage.ErrPlanInactive) {
		return ErrQuotaExceeded
	}
	return err
}

func (s *Service) executeTool(ctx context.Context, exec tools.ExecContext, call provider.ToolCall, seen map[string]json.RawMessage, readOnly bool) (tools.Result, error) {
	if readOnly && mutatingTool(call.Name) {
		result := tools.Result{Error: "tool is not available", Code: "AI_TOOL_NOT_FOUND"}
		cacheTool(seen, call.ID, result)
		return result, nil
	}
	if call.ID != "" {
		if cached, ok := seen[call.ID]; ok {
			var data any
			_ = json.Unmarshal(cached, &data)
			if asMap, ok := data.(map[string]any); ok {
				if errMsg, _ := asMap["error"].(string); errMsg != "" {
					code, _ := asMap["code"].(string)
					return tools.Result{Error: errMsg, Code: code}, nil
				}
			}
			return tools.Result{Data: data}, nil
		}
	}
	tool, err := s.tools.Get(call.Name)
	if err != nil {
		result := tools.Result{Error: "unknown tool", Code: "AI_TOOL_NOT_FOUND"}
		cacheTool(seen, call.ID, result)
		return result, tools.ErrUnknownTool
	}
	input, err := tools.Validate(tool.InputSchema(), call.Arguments)
	if err != nil {
		result := tools.Result{Error: err.Error(), Code: "AI_TOOL_INVALID_ARGUMENTS"}
		cacheTool(seen, call.ID, result)
		return result, tools.ErrArguments
	}
	result, err := tool.Execute(ctx, exec, input)
	if err != nil && result.Error == "" {
		result = tools.Result{Error: "tool execution failed", Code: "AI_TOOL_EXECUTION_FAILED"}
	}
	cacheTool(seen, call.ID, result)
	return result, err
}

func cacheTool(seen map[string]json.RawMessage, id string, result tools.Result) {
	if id == "" {
		return
	}
	raw, _ := json.Marshal(result)
	seen[id] = raw
}

func (s *Service) ensureProvider(name string) error {
	if s.llm == nil {
		return ErrProviderNotConfigured
	}
	switch name {
	case ProviderOpenAI, ProviderGroq, ProviderOllama:
		if name != ProviderOllama && s.defaults.APIKey == "" {
			return ErrProviderNotConfigured
		}
		return nil
	default:
		return ErrProviderNotConfigured
	}
}

func mutatingTool(name string) bool {
	switch name {
	case "quote_reservation", "create_reservation", "confirm_reservation", "cancel_reservation", "request_human_handover":
		return true
	default:
		return false
	}
}

func sanitizeProviderError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
