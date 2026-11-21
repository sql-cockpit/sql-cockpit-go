package sqlcockpit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const testKey = "sco_test_0123456789abcdefghijklmnop"

const (
	testTenantID     = "3f2504e0-4f89-41d3-9a0c-0305e82c3301"
	testMembershipID = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	testTicketID     = "9b2f6c1e-2c4d-4b8a-9f3e-1a2b3c4d5e6f"
	testAgentID      = "1c9d2e3f-4a5b-4c6d-8e7f-9a0b1c2d3e4f"
)

var bg = context.Background()

// recorded is one request the stand-in portal received.
type recorded struct {
	Method string
	Path   string
	Query  string
	Body   string
	Header http.Header
}

// portal is a stand-in for the SQL Cockpit portal that records every request.
type portal struct {
	server   *httptest.Server
	mu       sync.Mutex
	requests []recorded
}

// newPortal starts a stand-in portal. handler gets the 1-based number of the request.
func newPortal(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, n int)) *portal {
	t.Helper()
	p := &portal{}
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.requests = append(p.requests, recorded{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Body:   string(body),
			Header: r.Header.Clone(),
		})
		n := len(p.requests)
		p.mu.Unlock()
		handler(w, r, n)
	}))
	t.Cleanup(p.server.Close)
	return p
}

// respond answers every request with status and body.
func respond(status int, body string) func(w http.ResponseWriter, r *http.Request, n int) {
	return func(w http.ResponseWriter, _ *http.Request, _ int) {
		writeJSON(w, status, body)
	}
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func (p *portal) client(t *testing.T, opts ...Option) *Client {
	t.Helper()
	all := append([]Option{WithAPIKey(testKey), WithPortalURL(p.server.URL)}, opts...)
	c, err := NewClient(all...)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func (p *portal) all() []recorded {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]recorded(nil), p.requests...)
}

func (p *portal) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

// expect checks that exactly one request was sent, and what it was.
func (p *portal) expect(t *testing.T, method, path, query, body string) {
	t.Helper()
	requests := p.all()
	if len(requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(requests))
	}
	r := requests[0]
	if r.Method != method {
		t.Errorf("method = %q, want %q", r.Method, method)
	}
	if want := "/api/portal/v1" + path; r.Path != want {
		t.Errorf("path = %q, want %q", r.Path, want)
	}
	if r.Query != query {
		t.Errorf("query = %q, want %q", r.Query, query)
	}
	if r.Body != body {
		t.Errorf("body = %q, want %q", r.Body, body)
	}
	if got := r.Header.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization header = %q", got)
	}
	if got := r.Header.Get("Accept"); got != "application/json" {
		t.Errorf("Accept = %q, want application/json", got)
	}
	if got, want := r.Header.Get("User-Agent"), "sqlcockpit-go/"+Version; got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
	wantType := ""
	if body != "" {
		wantType = "application/json"
	}
	if got := r.Header.Get("Content-Type"); got != wantType {
		t.Errorf("Content-Type = %q, want %q", got, wantType)
	}
}

// asError returns err as an *Error, failing the test when it is not one.
func asError(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatal("got no error, want one")
	}
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error %T (%v) is not an *Error", err, err)
	}
	return e
}
