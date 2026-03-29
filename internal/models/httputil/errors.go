package httputil

import "fmt"

// APIError represents an HTTP API error with status code and response body.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	body := e.Body
	if len(body) > 1000 {
		body = body[:1000] + "... (truncated)"
	}
	return fmt.Sprintf("API error: HTTP %d: %s", e.StatusCode, body)
}
