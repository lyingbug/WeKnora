package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
)

// DefaultMaxRetries is the default maximum number of retries for API requests.
const DefaultMaxRetries = 3

// PostJSONWithRetry sends a POST request with JSON body and exponential backoff retry.
// Returns the raw response body on success. On HTTP error (non-200), returns an *APIError.
func PostJSONWithRetry(ctx context.Context, client *http.Client, url, apiKey string,
	body any, headers map[string]string, maxRetries int) ([]byte, error) {

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	return PostRawWithRetry(ctx, client, url, apiKey, jsonData, headers, maxRetries)
}

// PostRawWithRetry sends a POST request with pre-marshaled JSON data and exponential backoff retry.
func PostRawWithRetry(ctx context.Context, client *http.Client, url, apiKey string,
	jsonData []byte, headers map[string]string, maxRetries int) ([]byte, error) {

	var lastErr error
	for i := 0; i <= maxRetries; i++ {
		if i > 0 {
			backoff := min(time.Duration(1<<uint(i-1))*time.Second, 10*time.Second)
			logger.Infof(ctx, "httputil retrying request (%d/%d), waiting %v", i, maxRetries, backoff)

			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
		if err != nil {
			lastErr = fmt.Errorf("create request: %w", err)
			continue
		}

		req.Header.Set("Content-Type", "application/json")
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send request: %w", err)
			logger.Errorf(ctx, "httputil request failed (attempt %d/%d): %v", i+1, maxRetries+1, err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}

		return respBody, nil
	}

	return nil, fmt.Errorf("max retries (%d) exceeded: %w", maxRetries, lastErr)
}
