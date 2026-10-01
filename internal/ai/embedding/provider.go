// Package embedding is the provider-independent embedding adapter.
// Domain packages depend on Provider; they never import OpenAI SDK types.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	ErrNotConfigured = errors.New("embedding provider is not configured")
	ErrRequest       = errors.New("embedding provider request failed")
	ErrResponse      = errors.New("embedding provider response is invalid")
	ErrPermanent     = errors.New("embedding provider rejected the request")
)

type Provider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type Config struct {
	Enabled    bool
	APIKey     string
	BaseURL    string
	Model      string
	Timeout    time.Duration
	Dimensions int
}

type OpenAI struct {
	apiKey     string
	baseURL    string
	model      string
	dimensions int
	client     *http.Client
}

func New(cfg Config) Provider {
	if !cfg.Enabled {
		return Disabled{}
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		return Disabled{}
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "text-embedding-3-small"
	}
	return &OpenAI{
		apiKey:     strings.TrimSpace(cfg.APIKey),
		baseURL:    base,
		model:      model,
		dimensions: cfg.Dimensions,
		client:     &http.Client{Timeout: timeout},
	}
}

type Disabled struct{}

func (Disabled) Embed(context.Context, []string) ([][]float32, error) {
	return nil, ErrNotConfigured
}

func (p *OpenAI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	body := embedRequest{Model: p.model, Input: texts}
	if p.dimensions > 0 {
		body.Dimensions = p.dimensions
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("%w: encode request", ErrRequest)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("%w: build request", ErrRequest)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: transport", ErrRequest)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("%w: read response", ErrRequest)
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("%w: http %d", ErrRequest, resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("%w: http 429", ErrRequest)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: http %d", ErrPermanent, resp.StatusCode)
	}
	var parsed embedResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("%w: decode response", ErrResponse)
	}
	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, got %d", ErrResponse, len(texts), len(parsed.Data))
	}
	out := make([][]float32, len(texts))
	for _, item := range parsed.Data {
		if item.Index < 0 || item.Index >= len(out) {
			return nil, fmt.Errorf("%w: invalid embedding index", ErrResponse)
		}
		if len(item.Embedding) == 0 {
			return nil, fmt.Errorf("%w: empty embedding", ErrResponse)
		}
		out[item.Index] = item.Embedding
	}
	for i, vec := range out {
		if len(vec) == 0 {
			return nil, fmt.Errorf("%w: missing embedding %d", ErrResponse, i)
		}
	}
	return out, nil
}

type embedRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embedResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}
