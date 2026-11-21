package sqlcockpit

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

var allSentinels = []error{ErrUnauthenticated, ErrForbidden, ErrNotFound, ErrValidation, ErrRateLimited, ErrServer}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status   int
		code     string
		sentinel error
		is       func(error) bool
	}{
		{http.StatusBadRequest, "bad_request", ErrValidation, IsValidation},
		{http.StatusUnauthorized, "unauthenticated", ErrUnauthenticated, IsUnauthenticated},
		{http.StatusForbidden, "insufficient_scope", ErrForbidden, IsForbidden},
		{http.StatusForbidden, "plan_limit", ErrForbidden, IsForbidden},
		{http.StatusNotFound, "not_found", ErrNotFound, IsNotFound},
		{http.StatusUnprocessableEntity, "validation_failed", ErrValidation, IsValidation},
		{http.StatusTooManyRequests, "rate_limited", ErrRateLimited, IsRateLimited},
		{http.StatusInternalServerError, "server_error", ErrServer, IsServerError},
		{http.StatusServiceUnavailable, "server_error", ErrServer, IsServerError},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status)+"_"+tc.code, func(t *testing.T) {
			body := `{"ok":false,"error":{"code":"` + tc.code + `","message":"Nope.","fields":{"email":["Taken.","Too long."]}}}`
			p := newPortal(t, respond(tc.status, body))

			_, err := p.client(t, WithMaxRetries(0)).Account.Get(bg)
			e := asError(t, err)
			if e.Code != tc.code || e.Status != tc.status || e.Message != "Nope." {
				t.Errorf("error = %+v", e)
			}
			if got := e.Fields["email"]; len(got) != 2 || got[0] != "Taken." {
				t.Errorf("fields = %v", e.Fields)
			}
			if !errors.Is(err, tc.sentinel) || !tc.is(err) {
				t.Errorf("errors.Is(err, %v) = false", tc.sentinel)
			}
			for _, other := range allSentinels {
				if other != tc.sentinel && errors.Is(err, other) {
					t.Errorf("error also matches %v", other)
				}
			}
			want := "sqlcockpit: Nope. (" + tc.code + ", HTTP " + strconv.Itoa(tc.status) + ")"
			if err.Error() != want {
				t.Errorf("Error() = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestErrorWithoutEnvelope(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "<html>Bad gateway</html>")
	})

	_, err := p.client(t, WithMaxRetries(0)).Tenants.List(bg)
	e := asError(t, err)
	if e.Code != "server_error" || e.Status != http.StatusBadGateway || e.Message != "The portal request failed (HTTP 502)." || e.Fields != nil {
		t.Errorf("error = %+v", e)
	}
	if !IsServerError(err) {
		t.Error("IsServerError = false")
	}
}

func TestErrorFieldsAsEmptyArray(t *testing.T) {
	// PHP encodes an empty map as [].
	p := newPortal(t, respond(http.StatusNotFound, `{"ok":false,"error":{"code":"not_found","message":"Tenant not found.","fields":[]}}`))

	_, err := p.client(t).Team.List(bg, testTenantID)
	e := asError(t, err)
	if e.Fields != nil || !IsNotFound(err) {
		t.Errorf("error = %+v", e)
	}
	if p.count() != 1 {
		t.Errorf("got %d requests, want 1: a 404 is not retried", p.count())
	}
}

func TestRateLimitedErrorCarriesRetryAfter(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Retry-After", "7")
		writeJSON(w, http.StatusTooManyRequests, `{"ok":false,"error":{"code":"rate_limited","message":"Slow down."}}`)
	})

	_, err := p.client(t, WithMaxRetries(0)).Account.Get(bg)
	e := asError(t, err)
	if e.RetryAfter != 7*time.Second || !IsRateLimited(err) {
		t.Errorf("error = %+v", e)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		value string
		want  time.Duration
		ok    bool
	}{
		{"", 0, false},
		{"0", 0, true},
		{" 12 ", 12 * time.Second, true},
		{"999999999999", 24 * time.Hour, true},
		{"Fri, 20 Nov 2026 12:00:10 GMT", 10 * time.Second, true},
		{"Fri, 20 Nov 2026 11:00:00 GMT", 0, true},
		{"soon", 0, false},
		{"-5", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseRetryAfter(tc.value, now)
		if got != tc.want || ok != tc.ok {
			t.Errorf("parseRetryAfter(%q) = %v, %v; want %v, %v", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	if got := retryDelay(&Error{RetryAfter: 2 * time.Minute, hasRetryAfter: true}, 0); got != MaxRetryDelay {
		t.Errorf("a 2 minute Retry-After waits %v, want %v", got, MaxRetryDelay)
	}
	if got := retryDelay(&Error{RetryAfter: 3 * time.Second, hasRetryAfter: true}, 1); got != 3*time.Second {
		t.Errorf("a 3 second Retry-After waits %v", got)
	}
	if got := retryDelay(&Error{hasRetryAfter: true}, 1); got != 0 {
		t.Errorf("Retry-After: 0 waits %v", got)
	}
	for attempt := 0; attempt < 2; attempt++ {
		low := retryBaseDelay << attempt
		high := low + retryBaseDelay/2
		if got := retryDelay(&Error{}, attempt); got < low || got >= high {
			t.Errorf("backoff for attempt %d = %v, want in [%v, %v)", attempt, got, low, high)
		}
	}
}

func TestInvalidResponse(t *testing.T) {
	for _, body := range []string{"not json", `{"ok":false}`, `{"data":{}}`, `[]`} {
		p := newPortal(t, respond(http.StatusOK, body))
		_, err := p.client(t).Account.Get(bg)
		if e := asError(t, err); e.Code != CodeInvalidResponse || e.Status != http.StatusOK {
			t.Errorf("body %q: error = %+v", body, e)
		}
		if p.count() != 1 {
			t.Errorf("body %q: got %d requests, want 1", body, p.count())
		}
	}
}

func TestDataOfTheWrongShape(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"tenants":"none"}}`))
	_, err := p.client(t).Tenants.List(bg)
	if e := asError(t, err); e.Code != CodeInvalidResponse || errors.Unwrap(err) == nil {
		t.Errorf("error = %+v", e)
	}
}

func TestGetRetriesOn429ThenSucceeds(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, n int) {
		if n <= 2 {
			w.Header().Set("Retry-After", "0")
			writeJSON(w, http.StatusTooManyRequests, `{"ok":false,"error":{"code":"rate_limited","message":"Slow down."}}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"ok":true,"data":{"tenants":[]}}`)
	})

	if _, err := p.client(t).Tenants.List(bg); err != nil {
		t.Fatal(err)
	}
	if p.count() != 3 {
		t.Errorf("got %d requests, want 3", p.count())
	}
}

func TestGetRetriesOn5xxAtMostTwice(t *testing.T) {
	saved := retryBaseDelay
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() { retryBaseDelay = saved })

	p := newPortal(t, respond(http.StatusServiceUnavailable, `{"ok":false,"error":{"code":"server_error","message":"Down."}}`))

	_, err := p.client(t).Account.Usage(bg)
	if !IsServerError(err) {
		t.Fatalf("error = %v, want a server error", err)
	}
	if p.count() != 3 {
		t.Errorf("got %d requests, want 3 (1 + 2 retries)", p.count())
	}
}

func TestMaxRetriesLimitsRetries(t *testing.T) {
	for retries := 0; retries <= 2; retries++ {
		p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Retry-After", "0")
			writeJSON(w, http.StatusTooManyRequests, `{"ok":false,"error":{"code":"rate_limited","message":"Slow down."}}`)
		})
		_, err := p.client(t, WithMaxRetries(retries)).Agents.List(bg, nil)
		if !IsRateLimited(err) {
			t.Fatalf("error = %v, want rate limited", err)
		}
		if p.count() != retries+1 {
			t.Errorf("WithMaxRetries(%d): got %d requests, want %d", retries, p.count(), retries+1)
		}
	}
}

func TestWritesAreNeverRetried(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Retry-After", "0")
		writeJSON(w, http.StatusServiceUnavailable, `{"ok":false,"error":{"code":"server_error","message":"Down."}}`)
	})
	c := p.client(t)

	_, err := c.Support.Create(bg, CreateTicketParams{Subject: "Agent offline", Message: "The Agent stopped reporting."})
	if !IsServerError(err) {
		t.Fatalf("Support.Create error = %v", err)
	}
	_, err = c.Team.Add(bg, testTenantID, AddMemberParams{Name: "Sam Lee", Email: "sam@example.com", Role: RoleDeveloper})
	if !IsServerError(err) {
		t.Fatalf("Team.Add error = %v", err)
	}
	_, err = c.Team.ChangeRole(bg, testTenantID, testMembershipID, RoleAdmin)
	if !IsServerError(err) {
		t.Fatalf("Team.ChangeRole error = %v", err)
	}
	_, err = c.Team.Remove(bg, testTenantID, testMembershipID)
	if !IsServerError(err) {
		t.Fatalf("Team.Remove error = %v", err)
	}
	_, err = c.Team.ResendInvite(bg, testTenantID, testMembershipID)
	if !IsServerError(err) {
		t.Fatalf("Team.ResendInvite error = %v", err)
	}
	if p.count() != 5 {
		t.Errorf("got %d requests for 5 writes, want 5", p.count())
	}
}

func TestRetryWaitStopsWhenContextEnds(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		w.Header().Set("Retry-After", "20")
		writeJSON(w, http.StatusTooManyRequests, `{"ok":false,"error":{"code":"rate_limited","message":"Slow down."}}`)
	})

	ctx, cancel := context.WithTimeout(bg, 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := p.client(t).Account.Get(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v; the retry wait should stop with the context", elapsed)
	}
	if p.count() != 1 {
		t.Errorf("got %d requests, want 1", p.count())
	}
}

func TestCanceledContextSendsNothing(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	ctx, cancel := context.WithCancel(bg)
	cancel()

	_, err := p.client(t).Account.Get(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if p.count() != 0 {
		t.Errorf("got %d requests, want 0", p.count())
	}
}

func TestTimeout(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})

	_, err := p.client(t, WithTimeout(50*time.Millisecond), WithMaxRetries(0)).Account.Get(bg)
	e := asError(t, err)
	if e.Code != CodeTimeout || e.Status != 0 {
		t.Errorf("error = %+v", e)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Error("a timeout should match context.DeadlineExceeded")
	}
}

func TestConnectionError(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	c := p.client(t, WithMaxRetries(0))
	p.server.Close()

	_, err := c.Account.Get(bg)
	e := asError(t, err)
	if e.Code != CodeConnectionError || e.Status != 0 || errors.Unwrap(err) == nil {
		t.Errorf("error = %+v", e)
	}
}

func TestRedirectIsRefused(t *testing.T) {
	var p *portal
	p = newPortal(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.URL.Path == "/elsewhere" {
			writeJSON(w, http.StatusOK, `{"ok":true,"data":{"name":"Followed"}}`)
			return
		}
		http.Redirect(w, r, p.server.URL+"/elsewhere", http.StatusFound)
	})

	own := &http.Client{}
	for _, c := range []*Client{p.client(t), p.client(t, WithHTTPClient(own))} {
		before := p.count()
		_, err := c.Account.Get(bg)
		e := asError(t, err)
		if e.Code != CodeUnexpectedRedirect || e.Status != http.StatusFound {
			t.Errorf("error = %+v", e)
		}
		if got := p.count() - before; got != 1 {
			t.Errorf("got %d requests, want 1: the redirect must not be followed", got)
		}
	}
	if own.CheckRedirect != nil {
		t.Error("WithHTTPClient changed the caller's http.Client")
	}
}

func TestRedirectOnWriteIsRefused(t *testing.T) {
	p := newPortal(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		http.Redirect(w, r, "/elsewhere", http.StatusTemporaryRedirect)
	})

	_, err := p.client(t).Support.Create(bg, CreateTicketParams{Subject: "Agent offline", Message: "The Agent stopped reporting."})
	if e := asError(t, err); e.Code != CodeUnexpectedRedirect || e.Status != http.StatusTemporaryRedirect {
		t.Errorf("error = %+v", e)
	}
	if p.count() != 1 {
		t.Errorf("got %d requests, want 1", p.count())
	}
}

func accountOfSize(size int) string {
	prefix := `{"ok":true,"data":{"name":"`
	suffix := `"}}`
	return prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix
}

func TestResponseAtTheCapIsRead(t *testing.T) {
	body := accountOfSize(MaxResponseBytes)
	p := newPortal(t, respond(http.StatusOK, body))

	account, err := p.client(t).Account.Get(bg)
	if err != nil {
		t.Fatal(err)
	}
	if len(account.Name) != MaxResponseBytes-len(`{"ok":true,"data":{"name":""}}`) {
		t.Errorf("name has %d bytes", len(account.Name))
	}
}

func TestResponseOverTheCapIsRefused(t *testing.T) {
	body := accountOfSize(MaxResponseBytes + 1)

	t.Run("streamed", func(t *testing.T) {
		p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Write in pieces with flushes so no Content-Length is sent.
			for i := 0; i < len(body); i += 64 << 10 {
				end := min(i+64<<10, len(body))
				if _, err := io.WriteString(w, body[i:end]); err != nil {
					return
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		})
		_, err := p.client(t).Account.Get(bg)
		if e := asError(t, err); e.Code != CodeResponseTooLarge || e.Status != http.StatusOK {
			t.Errorf("error = %+v", e)
		}
	})

	t.Run("declared", func(t *testing.T) {
		p := newPortal(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, body)
		})
		_, err := p.client(t).Account.Get(bg)
		if e := asError(t, err); e.Code != CodeResponseTooLarge {
			t.Errorf("error = %+v", e)
		}
	})

	t.Run("not retried", func(t *testing.T) {
		p := newPortal(t, respond(http.StatusServiceUnavailable, body))
		_, err := p.client(t).Account.Get(bg)
		if e := asError(t, err); e.Code != CodeResponseTooLarge {
			t.Errorf("error = %+v", e)
		}
		if p.count() != 1 {
			t.Errorf("got %d requests, want 1", p.count())
		}
	})
}

func TestInvalidIDsSendNothing(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{}}`))
	c := p.client(t)

	check := func(err error, field string) {
		t.Helper()
		e := asError(t, err)
		if e.Code != CodeInvalidArgument || e.Status != 0 || len(e.Fields[field]) != 1 {
			t.Errorf("error = %+v, want invalid_argument for %s", e, field)
		}
		if !IsValidation(err) {
			t.Errorf("IsValidation(%v) = false", err)
		}
	}

	_, err := c.Team.List(bg, "not-a-uuid")
	check(err, "tenantID")
	_, err = c.Team.List(bg, testTenantID+"\n")
	check(err, "tenantID")
	_, err = c.Team.List(bg, "")
	check(err, "tenantID")
	_, err = c.Team.Add(bg, "../admin", AddMemberParams{Name: "Sam Lee", Email: "sam@example.com", Role: RoleDeveloper})
	check(err, "tenantID")
	_, err = c.Team.ChangeRole(bg, testTenantID, "x", RoleAdmin)
	check(err, "membershipID")
	_, err = c.Team.Remove(bg, "x", testMembershipID)
	check(err, "tenantID")
	_, err = c.Team.ResendInvite(bg, testTenantID, testMembershipID+"0")
	check(err, "membershipID")
	_, err = c.Agents.List(bg, &ListAgentsParams{TenantID: "../tenants"})
	check(err, "tenantID")
	_, err = c.Instances.List(bg, &ListInstancesParams{TenantID: "x"})
	check(err, "tenantID")
	_, err = c.Support.Get(bg, "../../account")
	check(err, "ticketID")
	_, err = c.Support.Create(bg, CreateTicketParams{Subject: "Agent offline", Message: "The Agent stopped reporting.", TenantID: "nope"})
	check(err, "tenantID")
	_, err = c.Support.Create(bg, CreateTicketParams{Subject: "Agent offline", Message: "The Agent stopped reporting.", AgentID: "nope"})
	check(err, "agentID")

	if p.count() != 0 {
		t.Errorf("got %d requests, want 0", p.count())
	}
}
