package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/alghifrz/diuk-backend/internal/channel/provider"
)

type Client struct {
	http *http.Client
}

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{http: &http.Client{Timeout: timeout}}
}

func NewClientWithHTTP(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{http: httpClient}
}

func (c *Client) SendText(ctx context.Context, input provider.SendTextInput) (provider.SendResult, error) {
	if strings.TrimSpace(input.PhoneNumberID) == "" || strings.TrimSpace(input.To) == "" || strings.TrimSpace(input.Body) == "" {
		return provider.SendResult{}, ErrInvalidPayload
	}
	base := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}
	version := strings.Trim(strings.TrimSpace(input.APIVersion), "/")
	if version == "" {
		version = "v21.0"
	}
	var body sendTextRequest
	body.MessagingProduct = "whatsapp"
	body.To = strings.TrimSpace(input.To)
	body.Type = "text"
	body.Text.Body = input.Body
	payload, err := json.Marshal(body)
	if err != nil {
		return provider.SendResult{}, fmt.Errorf("%w: encode", ErrUnavailable)
	}

	return c.postMessage(ctx, input.PhoneNumberID, input.AccessToken, input.BaseURL, input.APIVersion, payload)
}

func (c *Client) SendTemplate(ctx context.Context, input provider.SendTemplateInput) (provider.SendResult, error) {
	if strings.TrimSpace(input.PhoneNumberID) == "" || strings.TrimSpace(input.To) == "" ||
		strings.TrimSpace(input.TemplateName) == "" || strings.TrimSpace(input.Language) == "" {
		return provider.SendResult{}, ErrInvalidPayload
	}
	var body sendTemplateRequest
	body.MessagingProduct = "whatsapp"
	body.To = strings.TrimSpace(input.To)
	body.Type = "template"
	body.Template.Name = strings.TrimSpace(input.TemplateName)
	body.Template.Language.Code = strings.TrimSpace(input.Language)
	if len(input.Parameters) > 0 {
		params := make([]sendTemplateParam, 0, len(input.Parameters))
		for _, p := range input.Parameters {
			params = append(params, sendTemplateParam{Type: "text", Text: p})
		}
		body.Template.Components = []sendTemplateComponent{{Type: "body", Parameters: params}}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return provider.SendResult{}, fmt.Errorf("%w: encode", ErrUnavailable)
	}
	return c.postMessage(ctx, input.PhoneNumberID, input.AccessToken, input.BaseURL, input.APIVersion, payload)
}

func (c *Client) MarkRead(ctx context.Context, input provider.MarkReadInput) error {
	if strings.TrimSpace(input.PhoneNumberID) == "" || strings.TrimSpace(input.MessageID) == "" {
		return ErrInvalidPayload
	}
	payload, err := json.Marshal(markReadRequest{
		MessagingProduct: "whatsapp",
		Status:           "read",
		MessageID:        strings.TrimSpace(input.MessageID),
	})
	if err != nil {
		return fmt.Errorf("%w: encode", ErrUnavailable)
	}
	raw, err := c.post(ctx, input.PhoneNumberID, input.AccessToken, input.BaseURL, input.APIVersion, payload)
	if err != nil {
		return err
	}
	var parsed markReadResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || !parsed.Success {
		return ErrUnavailable
	}
	return nil
}

func (c *Client) postMessage(ctx context.Context, phoneNumberID, accessToken, baseURL, apiVersion string, payload []byte) (provider.SendResult, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}
	version := strings.Trim(strings.TrimSpace(apiVersion), "/")
	if version == "" {
		version = "v21.0"
	}
	raw, err := c.post(ctx, phoneNumberID, accessToken, baseURL, apiVersion, payload)
	if err != nil {
		return provider.SendResult{}, err
	}
	var parsed sendTextResponse
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Messages) == 0 || parsed.Messages[0].ID == "" {
		return provider.SendResult{}, ErrUnavailable
	}
	return provider.SendResult{ProviderMessageID: parsed.Messages[0].ID}, nil
}

func (c *Client) post(ctx context.Context, phoneNumberID, accessToken, baseURL, apiVersion string, payload []byte) ([]byte, error) {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = "https://graph.facebook.com"
	}
	version := strings.Trim(strings.TrimSpace(apiVersion), "/")
	if version == "" {
		version = "v21.0"
	}
	url := base + "/" + version + "/" + strings.TrimSpace(phoneNumberID) + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrUnavailable)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := strings.TrimSpace(accessToken); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrUnavailable
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, ErrUnavailable
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, RateLimitedError{After: parseRetryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode >= 500 {
		return nil, ErrUnavailable
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: http %d", ErrRejected, resp.StatusCode)
	}
	return raw, nil
}

func parseRetryAfter(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	if when, err := http.ParseTime(raw); err == nil {
		d := time.Until(when)
		if d > 0 {
			return d
		}
	}
	return 0
}
