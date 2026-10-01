package ai

import (
	"context"
	"strings"

	"github.com/alghifrz/diuk-backend/internal/ai/provider"
	"github.com/alghifrz/diuk-backend/internal/knowledge"
)

type knowledgeAnswerer struct {
	llm   provider.LLMProvider
	model string
}

// NewKnowledgeAnswerer lets the knowledge base turn retrieved passages into a
// short answer without importing provider types itself.
func NewKnowledgeAnswerer(llm provider.LLMProvider, model string) knowledge.AnswerGenerator {
	if llm == nil || strings.TrimSpace(model) == "" {
		return nil
	}
	return knowledgeAnswerer{llm: llm, model: model}
}

func (a knowledgeAnswerer) Answer(ctx context.Context, system, user string) (string, error) {
	temperature := 0.2
	maxTokens := 300
	resp, err := a.llm.Generate(ctx, provider.GenerateRequest{
		Model:       a.model,
		Temperature: &temperature,
		MaxTokens:   &maxTokens,
		Messages: []provider.Message{
			{Role: provider.RoleSystem, Content: system},
			{Role: provider.RoleUser, Content: user},
		},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}
