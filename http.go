package sqlcockpit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// MaxResponseBytes is the largest response body the client reads (2 MiB). A larger response is
// refused with code CodeResponseTooLarge.
const MaxResponseBytes = 2 << 20

// MaxRetryDelay is the longest the client waits before a retry, whatever Retry-After says.
const MaxRetryDelay = 30 * time.Second

// retryBaseDelay is the first backoff when the portal sends no Retry-After. Tests shorten it.
var retryBaseDelay = 500 * time.Millisecond

// do sends one API call, retrying GETs on 429 and 5xx, and decodes the response's data into out.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	if c == nil || c.cfg == nil {
		return configError("The client is not initialised. Create it with NewClient.")
	}
	if ctx == nil {
		return &Error{Code: CodeInvalidArgument, Message: "ctx must not be nil."}
	}
	cfg := c.cfg

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return &Error{Code: CodeInvalidArgument, Message: "The request body could not be encoded.", err: err}
		}
		payload = encoded
	}

	target := cfg.baseURL + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}

	retries := 0
	if method == http.MethodGet {
		retries = cfg.maxRetries
	}
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.attempt(ctx, method, target, payload, out)
		if err == nil {
			return nil
		}
		var apiErr *Error
		if attempt >= retries || !errors.As(err, &apiErr) || !apiErr.retry {
			return err
		}
		timer := time.NewTimer(retryDelay(apiErr, attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) attempt(ctx context.Context, method, target string, payload []byte, out any) error {
	cfg := c.cfg
	attemptCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	var reqBody io.Reader
	if payload != nil {
		reqBody = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(attemptCtx, method, target, reqBody)
	if err != nil {
		return &Error{Code: CodeInvalidConfig, Message: "The request could not be built. Check the portal URL.", err: err}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	req.Header.Set("User-Agent", cfg.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := cfg.httpClient.Do(req)
	if err != nil {
		return transportError(ctx, attemptCtx, cfg.timeout, err)
	}
	defer resp.Body.Close()

	// The http.Client is set up to hand back a 3xx instead of following it.
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		drain(resp.Body)
		return &Error{
			Code:    CodeUnexpectedRedirect,
			Status:  resp.StatusCode,
			Message: "The portal answered with a redirect, which the client does not follow. Check the portal URL.",
		}
	}

	if resp.ContentLength > MaxResponseBytes {
		return tooLarge(resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return transportError(ctx, attemptCtx, cfg.timeout, err)
	}
	if len(data) > MaxResponseBytes {
		return tooLarge(resp.StatusCode)
	}

	var envelope struct {
		OK    *bool           `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error json.RawMessage `json:"error"`
	}
	parsed := json.Unmarshal(data, &envelope) == nil

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if !parsed || envelope.OK == nil || !*envelope.OK {
			return &Error{
				Code:    CodeInvalidResponse,
				Status:  resp.StatusCode,
				Message: fmt.Sprintf("The portal returned an unexpected response (HTTP %d).", resp.StatusCode),
			}
		}
		if out != nil && len(envelope.Data) > 0 {
			if err := json.Unmarshal(envelope.Data, out); err != nil {
				return &Error{
					Code:    CodeInvalidResponse,
					Status:  resp.StatusCode,
					Message: "The portal returned data the client could not read.",
					err:     err,
				}
			}
		}
		return nil
	}

	var rawError json.RawMessage
	if parsed {
		rawError = envelope.Error
	}
	return apiError(resp, rawError, cfg.apiKey)
}

func transportError(ctx, attemptCtx context.Context, timeout time.Duration, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) {
		return &Error{
			Code:    CodeTimeout,
			Message: fmt.Sprintf("The SQL Cockpit portal did not answer within %s.", timeout),
			err:     context.DeadlineExceeded,
		}
	}
	// The cause describes the network failure. It never holds the request headers, and
	// Error() does not repeat it.
	return &Error{
		Code:    CodeConnectionError,
		Message: "Could not reach the SQL Cockpit portal. Check the portal URL and your connection.",
		err:     cause,
	}
}

func apiError(resp *http.Response, rawError json.RawMessage, apiKey string) *Error {
	status := resp.StatusCode
	var body struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Fields  json.RawMessage `json:"fields"`
	}
	if len(rawError) > 0 {
		// A field of the wrong type is skipped; the others are still read.
		_ = json.Unmarshal(rawError, &body)
	}

	code := body.Code
	if code == "" {
		code = codeForStatus(status)
	}
	message := body.Message
	if message == "" {
		message = fmt.Sprintf("The portal request failed (HTTP %d).", status)
	}

	e := &Error{
		Code:    redact(code, apiKey),
		Status:  status,
		Message: redact(message, apiKey),
		Fields:  parseFields(body.Fields, apiKey),
		retry:   status == http.StatusTooManyRequests || status >= 500,
	}
	if wait, ok := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
		e.RetryAfter = wait
		e.hasRetryAfter = true
	}
	return e
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthenticated"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusMethodNotAllowed:
		return "method_not_allowed"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnprocessableEntity:
		return "validation_failed"
	case http.StatusTooManyRequests:
		return "rate_limited"
	}
	if status >= 500 {
		return "server_error"
	}
	return CodeInvalidResponse
}

func parseFields(raw json.RawMessage, apiKey string) map[string][]string {
	var byName map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &byName) != nil || len(byName) == 0 {
		return nil
	}
	fields := make(map[string][]string, len(byName))
	for name, value := range byName {
		var items []json.RawMessage
		if json.Unmarshal(value, &items) != nil {
			continue
		}
		messages := make([]string, 0, len(items))
		for _, item := range items {
			var text string
			if json.Unmarshal(item, &text) == nil {
				messages = append(messages, redact(text, apiKey))
			}
		}
		fields[redact(name, apiKey)] = messages
	}
	return fields
}

// parseRetryAfter reads Retry-After as a number of seconds or an HTTP date.
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseUint(value, 10, 64); err == nil {
		// Anything past a day is as good as a day; the wait is capped at MaxRetryDelay anyway.
		if seconds > 86400 {
			seconds = 86400
		}
		return time.Duration(seconds) * time.Second, true
	}
	when, err := http.ParseTime(value)
	if err != nil {
		return 0, false
	}
	wait := when.Sub(now)
	if wait < 0 {
		return 0, true
	}
	return wait.Round(time.Second), true
}

func retryDelay(e *Error, attempt int) time.Duration {
	if e.hasRetryAfter {
		return min(e.RetryAfter, MaxRetryDelay)
	}
	backoff := retryBaseDelay << attempt
	if jitter := retryBaseDelay / 2; jitter > 0 {
		backoff += rand.N(jitter)
	}
	return min(backoff, MaxRetryDelay)
}

func tooLarge(status int) *Error {
	return &Error{
		Code:    CodeResponseTooLarge,
		Status:  status,
		Message: "The portal response was larger than 2 MiB and was not read.",
	}
}

func drain(body io.Reader) {
	_, _ = io.Copy(io.Discard, io.LimitReader(body, 64<<10))
}

func redact(text, apiKey string) string {
	if apiKey == "" || !strings.Contains(text, apiKey) {
		return text
	}
	return strings.ReplaceAll(text, apiKey, "[redacted]")
}
