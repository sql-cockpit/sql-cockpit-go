package sqlcockpit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewClientReadsEnvironment(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"name":"Ada"}}`))
	t.Setenv(EnvAPIKey, testKey)
	t.Setenv(EnvPortalURL, p.server.URL+"/api/portal/v1/")

	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if c.PortalURL() != p.server.URL {
		t.Errorf("PortalURL() = %q, want %q", c.PortalURL(), p.server.URL)
	}
	if _, err := c.Account.Get(bg); err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/account", "", "")
}

func TestOptionsOverrideEnvironment(t *testing.T) {
	t.Setenv(EnvAPIKey, "sco_from_environment_0000000000")
	t.Setenv(EnvPortalURL, "https://env.example.com")
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))

	c := p.client(t)
	if c.PortalURL() != p.server.URL {
		t.Errorf("PortalURL() = %q", c.PortalURL())
	}
	if _, err := c.Account.Get(bg); err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/account", "", "")
}

func TestDefaults(t *testing.T) {
	t.Setenv(EnvPortalURL, "")
	c, err := NewClient(WithAPIKey(testKey))
	if err != nil {
		t.Fatal(err)
	}
	if c.PortalURL() != DefaultPortalURL || c.Timeout() != DefaultTimeout || c.MaxRetries() != DefaultMaxRetries {
		t.Errorf("client = %v", c)
	}
}

func TestNewClientRequiresAKey(t *testing.T) {
	t.Setenv(EnvAPIKey, "")
	_, err := NewClient()
	if e := asError(t, err); e.Code != CodeInvalidConfig || !strings.Contains(e.Message, "SQL_COCKPIT_API_KEY") {
		t.Errorf("error = %+v", e)
	}
}

func TestOptionValidation(t *testing.T) {
	cases := map[string]Option{
		"short key":        WithAPIKey("short"),
		"key with a space": WithAPIKey("sco_test_0123456789 abcdefghij"),
		"key with newline": WithAPIKey("sco_test_0123456789\nabcdefghij"),
		"zero timeout":     WithTimeout(0),
		"long timeout":     WithTimeout(11 * time.Minute),
		"negative retries": WithMaxRetries(-1),
		"too many retries": WithMaxRetries(3),
		"bad user agent":   WithUserAgent("acme/1.0\r\nX-Evil: 1"),
	}
	for name, opt := range cases {
		opts := []Option{WithAPIKey(testKey), WithPortalURL("https://portal.example.com"), opt}
		_, err := NewClient(opts...)
		if e := asError(t, err); e.Code != CodeInvalidConfig {
			t.Errorf("%s: error = %+v", name, e)
		}
	}
}

func TestPortalURLRules(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", DefaultPortalURL},
		{"https://portal.sql-cockpit.com", "https://portal.sql-cockpit.com"},
		{" https://portal.sql-cockpit.com/ ", "https://portal.sql-cockpit.com"},
		{"https://portal.sql-cockpit.com/api/portal/v1", "https://portal.sql-cockpit.com"},
		{"https://portal.sql-cockpit.com/API/Portal/V1/", "https://portal.sql-cockpit.com"},
		{"https://example.com/portal", "https://example.com/portal"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://127.0.0.1:9000", "http://127.0.0.1:9000"},
		{"http://[::1]:9000", "http://[::1]:9000"},
	}
	for _, tc := range cases {
		got, err := normalizePortalURL(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("normalizePortalURL(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}

	refused := []string{
		"http://portal.sql-cockpit.com",
		"http://localhost.example.com",
		"http://10.0.0.1",
		"https://user:pass@portal.sql-cockpit.com",
		"https://user@portal.sql-cockpit.com",
		"https://portal.sql-cockpit.com?x=1",
		"https://portal.sql-cockpit.com/?",
		"https://portal.sql-cockpit.com/#top",
		"ftp://portal.sql-cockpit.com",
		"portal.sql-cockpit.com",
		"https://",
		"://nothing",
	}
	for _, in := range refused {
		_, err := NewClient(WithAPIKey(testKey), WithPortalURL(in))
		if e := asError(t, err); e.Code != CodeInvalidConfig {
			t.Errorf("WithPortalURL(%q): error = %+v", in, e)
		}
	}
}

func TestUserAgent(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	if _, err := p.client(t, WithUserAgent("acme-sync/1.2")).Account.Get(bg); err != nil {
		t.Fatal(err)
	}
	want := "acme-sync/1.2 sqlcockpit-go/" + Version
	if got := p.all()[0].Header.Get("User-Agent"); got != want {
		t.Errorf("User-Agent = %q, want %q", got, want)
	}
}

type countingTransport struct {
	calls int
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c.calls++
	return http.DefaultTransport.RoundTrip(r)
}

func TestWithHTTPClientIsUsed(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	transport := &countingTransport{}
	own := &http.Client{Transport: transport}

	if _, err := p.client(t, WithHTTPClient(own)).Account.Get(bg); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Errorf("the given http.Client sent %d requests, want 1", transport.calls)
	}
	if own.CheckRedirect != nil {
		t.Error("WithHTTPClient changed the caller's http.Client")
	}
}

func TestKeyNeverInFormattedOutput(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	c := p.client(t)

	var outputs []string
	outputs = append(outputs,
		fmt.Sprint(c),
		fmt.Sprintf("%v|%+v|%#v|%s|%q", c, c, c, c, c),
		fmt.Sprintf("%v|%+v|%#v", *c, *c, *c),
		fmt.Sprintf("%v|%+v|%#v", c.Account, c.Team, *c.Support),
		fmt.Sprintf("%+v", []any{c, c.APIKeys, c.Agents, c.Instances, c.Tenants}),
	)
	for _, v := range []any{c, *c, c.Team} {
		encoded, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, string(encoded))
	}

	var logs bytes.Buffer
	slog.New(slog.NewTextHandler(&logs, nil)).Info("client", "client", c, "team", c.Team)
	slog.New(slog.NewJSONHandler(&logs, nil)).Info("client", "client", c, "value", *c)
	outputs = append(outputs, logs.String())

	for _, out := range outputs {
		if strings.Contains(out, testKey) {
			t.Errorf("output contains the API key: %s", out)
		}
	}
	if !strings.Contains(outputs[0], p.server.URL) {
		t.Errorf("String() = %q, want the portal URL in it", outputs[0])
	}
}

func TestKeyNeverInErrors(t *testing.T) {
	var errs []error

	_, err := NewClient(WithAPIKey(testKey), WithPortalURL("https://user:"+testKey+"@portal.example.com"))
	errs = append(errs, err)
	_, err = NewClient(WithAPIKey(testKey+" extra"), WithPortalURL("https://portal.example.com"))
	errs = append(errs, err)

	// The portal echoes the key back in its message and fields.
	p := newPortal(t, respond(http.StatusUnauthorized, `{"ok":false,"error":{"code":"unauthenticated","message":"Key `+testKey+` is revoked.","fields":{"key":["`+testKey+`"]}}}`))
	_, err = p.client(t).Account.Get(bg)
	e := asError(t, err)
	if !strings.Contains(e.Message, "[redacted]") || e.Fields["key"][0] != "[redacted]" {
		t.Errorf("error = %+v, want the key redacted", e)
	}
	errs = append(errs, err)

	closed := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	c := closed.client(t, WithMaxRetries(0))
	closed.server.Close()
	_, err = c.Account.Get(bg)
	errs = append(errs, err, errors.Unwrap(err))

	slow := newPortal(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		<-r.Context().Done()
	})
	_, err = slow.client(t, WithTimeout(20*time.Millisecond), WithMaxRetries(0)).Account.Get(bg)
	errs = append(errs, err)

	for i, err := range errs {
		if err == nil {
			t.Errorf("error %d is nil", i)
			continue
		}
		for _, out := range []string{err.Error(), fmt.Sprintf("%v %+v %#v", err, err, err)} {
			if strings.Contains(out, testKey) {
				t.Errorf("error %d contains the API key: %s", i, out)
			}
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	e := &Error{Code: CodeInvalidConfig, Message: "Bad."}
	if got := e.Error(); got != "sqlcockpit: Bad. (invalid_config)" {
		t.Errorf("Error() = %q", got)
	}
	e = &Error{Code: "not_found", Status: 404, Message: "Gone."}
	if got := e.Error(); got != "sqlcockpit: Gone. (not_found, HTTP 404)" {
		t.Errorf("Error() = %q", got)
	}
	wrapped := fmt.Errorf("loading team: %w", e)
	if !IsNotFound(wrapped) || !errors.Is(wrapped, ErrNotFound) {
		t.Error("a wrapped error should still match ErrNotFound")
	}
	if IsNotFound(errors.New("other")) || IsNotFound(nil) {
		t.Error("IsNotFound matched an unrelated error")
	}
}
