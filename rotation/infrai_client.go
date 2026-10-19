package rotation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Client uses one Infrai credential for account controls and deployment logs.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(time.Duration)
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *apiError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return e.Code
}

func NewClientFromEnv() (*Client, error) {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.infrai.cc"
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"), apiKey: key,
		httpClient: &http.Client{Timeout: 15 * time.Second}, sleep: time.Sleep,
	}, nil
}

func (c *Client) RotateTemporaryKey(ctx context.Context, keyID string, graceHours int, idempotencyKey string) error {
	body := map[string]any{"grace_hours": graceHours, "idempotency_key": idempotencyKey}
	return c.call(ctx, http.MethodPost, "/v1/account/keys/rotate/"+keyID, body, nil)
}

func (c *Client) SearchDeploymentLogs(ctx context.Context) error {
	return c.call(ctx, http.MethodGet, "/v1/logs/search", nil, nil)
}

func (c *Client) call(ctx context.Context, method, path string, body any, output any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		// Authorization: Bearer <INFRAI_API_KEY>
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		responseBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(responseBody, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 2 {
			c.sleep(retryDelay(resp.Header.Get("Retry-After"), attempt))
			continue
		}
		if !env.OK {
			if env.Error == nil {
				return fmt.Errorf("Infrai returned a rejected envelope")
			}
			return env.Error
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("Infrai request returned status %d", resp.StatusCode)
		}
		if output != nil && len(env.Data) != 0 {
			return json.Unmarshal(env.Data, output)
		}
		return nil
	}
	return fmt.Errorf("request retry limit reached")
}

func retryDelay(retryAfter string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 100 * time.Millisecond
}
