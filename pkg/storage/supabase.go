package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHTTPTimeout = 30 * time.Second

// Supabase talks to the Supabase Storage REST API with the service role key.
// The key is never logged or returned.
type Supabase struct {
	baseURL    string
	serviceKey string
	bucket     string
	http       *http.Client
}

// NewSupabase returns a Storage client. An empty service key yields a client
// whose operations fail with ErrNotConfigured so the API can start without
// image support misconfigured as a hard crash.
func NewSupabase(projectURL, serviceRoleKey, bucket string) *Supabase {
	if bucket == "" {
		bucket = "diuk-menu-images"
	}
	return &Supabase{
		baseURL:    strings.TrimRight(strings.TrimSpace(projectURL), "/"),
		serviceKey: strings.TrimSpace(serviceRoleKey),
		bucket:     bucket,
		http:       &http.Client{Timeout: defaultHTTPTimeout},
	}
}

func (s *Supabase) configured() error {
	if s == nil || s.baseURL == "" || s.serviceKey == "" || s.bucket == "" {
		return ErrNotConfigured
	}
	return nil
}

func (s *Supabase) Upload(ctx context.Context, path string, contentType string, data io.Reader) error {
	if err := s.configured(); err != nil {
		return err
	}
	path = strings.TrimPrefix(path, "/")
	endpoint := s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + encodeObjectPath(path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, data)
	if err != nil {
		return ErrUploadFailed
	}
	s.authorize(req)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	resp, err := s.http.Do(req)
	if err != nil {
		return ErrUploadFailed
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return ErrUploadFailed
}

func (s *Supabase) Download(ctx context.Context, path string) ([]byte, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	path = strings.TrimPrefix(path, "/")
	endpoint := s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket) + "/" + encodeObjectPath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, ErrDownloadFailed
	}
	s.authorize(req)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, ErrDownloadFailed
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, ErrDownloadFailed
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrDownloadFailed
	}
	return raw, nil
}

func (s *Supabase) Delete(ctx context.Context, path string) error {
	if err := s.configured(); err != nil {
		return err
	}
	path = strings.TrimPrefix(path, "/")
	endpoint := s.baseURL + "/storage/v1/object/" + url.PathEscape(s.bucket)

	body, err := json.Marshal(map[string][]string{"prefixes": {path}})
	if err != nil {
		return ErrDeleteFailed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, bytes.NewReader(body))
	if err != nil {
		return ErrDeleteFailed
	}
	s.authorize(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return ErrDeleteFailed
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return ErrDeleteFailed
}

func (s *Supabase) CreateSignedURL(ctx context.Context, path string, expiresIn time.Duration) (string, error) {
	urls, err := s.CreateSignedURLs(ctx, []string{path}, expiresIn)
	if err != nil {
		return "", err
	}
	signed, ok := urls[path]
	if !ok || signed == "" {
		return "", ErrSignFailed
	}
	return signed, nil
}

func (s *Supabase) CreateSignedURLs(ctx context.Context, paths []string, expiresIn time.Duration) (map[string]string, error) {
	if err := s.configured(); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	if expiresIn <= 0 {
		expiresIn = DefaultSignedURLTTL
	}

	clean := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		p = strings.TrimPrefix(p, "/")
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return map[string]string{}, nil
	}

	payload, err := json.Marshal(map[string]any{
		"expiresIn": int(expiresIn.Seconds()),
		"paths":     clean,
	})
	if err != nil {
		return nil, ErrSignFailed
	}

	endpoint := s.baseURL + "/storage/v1/object/sign/" + url.PathEscape(s.bucket)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrSignFailed
	}
	s.authorize(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.http.Do(req)
	if err != nil {
		return nil, ErrSignFailed
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, ErrSignFailed
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrSignFailed
	}

	out, err := parseSignedURLResponse(s.baseURL, raw)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Supabase) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("apikey", s.serviceKey)
}

func encodeObjectPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

type signedEntry struct {
	Path         string `json:"path"`
	SignedURL    string `json:"signedURL"`
	SignedURLAlt string `json:"signedUrl"`
	Error        any    `json:"error"`
}

func parseSignedURLResponse(baseURL string, raw []byte) (map[string]string, error) {
	var list []signedEntry
	if err := json.Unmarshal(raw, &list); err != nil {
		var wrapped struct {
			Data []signedEntry `json:"data"`
		}
		if err2 := json.Unmarshal(raw, &wrapped); err2 != nil || len(wrapped.Data) == 0 {
			return nil, ErrSignFailed
		}
		list = wrapped.Data
	}

	out := make(map[string]string, len(list))
	for _, entry := range list {
		if entry.Error != nil && fmt.Sprint(entry.Error) != "" && fmt.Sprint(entry.Error) != "<nil>" {
			continue
		}
		signed := entry.SignedURL
		if signed == "" {
			signed = entry.SignedURLAlt
		}
		if signed == "" || entry.Path == "" {
			continue
		}
		out[entry.Path] = absolutizeSignedURL(baseURL, signed)
	}
	if len(out) == 0 {
		return nil, ErrSignFailed
	}
	return out, nil
}

func absolutizeSignedURL(baseURL, signed string) string {
	if strings.HasPrefix(signed, "http://") || strings.HasPrefix(signed, "https://") {
		return signed
	}
	if !strings.HasPrefix(signed, "/") {
		signed = "/" + signed
	}
	if strings.HasPrefix(signed, "/storage/v1/") {
		return baseURL + signed
	}
	return baseURL + "/storage/v1" + signed
}
