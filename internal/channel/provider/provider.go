package provider

import "context"

type MessagingProvider interface {
	SendText(ctx context.Context, input SendTextInput) (SendResult, error)
	SendTemplate(ctx context.Context, input SendTemplateInput) (SendResult, error)
	MarkRead(ctx context.Context, input MarkReadInput) error
}

type SendTextInput struct {
	PhoneNumberID string
	To            string
	Body          string
	AccessToken   string
	BaseURL       string
	APIVersion    string
}

type SendTemplateInput struct {
	PhoneNumberID string
	To            string
	AccessToken   string
	BaseURL       string
	APIVersion    string
	TemplateName  string
	Language      string
	Parameters    []string
}

type SendResult struct {
	ProviderMessageID string
}

type MarkReadInput struct {
	PhoneNumberID string
	MessageID     string
	AccessToken   string
	BaseURL       string
	APIVersion    string
}
