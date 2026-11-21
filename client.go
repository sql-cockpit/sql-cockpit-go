package sqlcockpit

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Defaults and the environment variables NewClient reads.
const (
	DefaultPortalURL  = "https://portal.sql-cockpit.com"
	DefaultTimeout    = 20 * time.Second
	DefaultMaxRetries = 2
	EnvAPIKey         = "SQL_COCKPIT_API_KEY"
	EnvPortalURL      = "SQL_COCKPIT_PORTAL_URL"
)

const (
	apiBasePath = "/api/portal/v1"
	maxTimeout  = 10 * time.Minute
)

// Client calls the SQL Cockpit portal API. Create one with [NewClient] and share it: it is safe
// for concurrent use.
//
// Each group of operations is a service:
//
//   - Account: your account and plan usage
//   - APIKeys: your portal API keys (list only)
//   - Tenants: the tenants you own or belong to
//   - Team: the members of a tenant
//   - Agents: the SQL Cockpit Agents paired with your tenants
//   - Instances: SQL Cockpit Server instances
//   - Support: your support tickets
//
// Printing a Client, or logging it with log/slog, shows the portal URL and settings only. The
// API key is never included.
type Client struct {
	Account   *AccountService
	APIKeys   *APIKeysService
	Tenants   *TenantsService
	Team      *TeamService
	Agents    *AgentsService
	Instances *InstancesService
	Support   *SupportService

	cfg *config
}

// config is kept behind a pointer so that printing a Client value shows an address, not the key.
type config struct {
	apiKey     string
	portalURL  string
	baseURL    string
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int
	userAgent  string
}

// Option configures a [Client]. Pass options to [NewClient].
type Option func(*settings)

type settings struct {
	apiKey     *string
	portalURL  *string
	httpClient *http.Client
	timeout    *time.Duration
	maxRetries *int
	userAgent  *string
}

// WithAPIKey sets the portal API key. Without it, NewClient reads SQL_COCKPIT_API_KEY.
// Create a key in the portal under Settings > Developer access.
func WithAPIKey(key string) Option {
	return func(s *settings) { s.apiKey = &key }
}

// WithPortalURL sets the portal's address. Without it, NewClient reads SQL_COCKPIT_PORTAL_URL,
// then uses https://portal.sql-cockpit.com. The URL must use https; http is allowed only for
// localhost, 127.0.0.1 and [::1]. It must not contain credentials, a query or a fragment. The API
// base URL (ending in /api/portal/v1) is accepted too.
func WithPortalURL(portalURL string) Option {
	return func(s *settings) { s.portalURL = &portalURL }
}

// WithHTTPClient sets the http.Client used to send requests, for example to use a proxy or a
// custom transport. The client is copied, and the copy never follows redirects. Its Timeout is
// kept, but WithTimeout is the usual way to bound a call.
func WithHTTPClient(httpClient *http.Client) Option {
	return func(s *settings) { s.httpClient = httpClient }
}

// WithTimeout sets how long one attempt may take. The default is 20 seconds; it must be more
// than 0 and at most 10 minutes. The context passed to each method can end a call sooner.
func WithTimeout(timeout time.Duration) Option {
	return func(s *settings) { s.timeout = &timeout }
}

// WithMaxRetries sets how many times a failed GET (429 or 5xx) is retried: 0, 1 or 2. The
// default is 2. Writes (POST, PATCH, DELETE) are never retried.
func WithMaxRetries(maxRetries int) Option {
	return func(s *settings) { s.maxRetries = &maxRetries }
}

// WithUserAgent adds your own product token in front of the client's, so the header becomes
// "<userAgent> sqlcockpit-go/<version>". It must be printable ASCII, at most 200 characters.
func WithUserAgent(userAgent string) Option {
	return func(s *settings) { s.userAgent = &userAgent }
}

// NewClient creates a client. With no options it reads the API key from SQL_COCKPIT_API_KEY and
// the portal URL from SQL_COCKPIT_PORTAL_URL (default https://portal.sql-cockpit.com).
//
// It returns an [*Error] with code CodeInvalidConfig when a setting is missing or not allowed.
func NewClient(opts ...Option) (*Client, error) {
	var s settings
	for _, opt := range opts {
		if opt != nil {
			opt(&s)
		}
	}

	rawURL := os.Getenv(EnvPortalURL)
	if s.portalURL != nil {
		rawURL = *s.portalURL
	}
	portalURL, err := normalizePortalURL(rawURL)
	if err != nil {
		return nil, err
	}

	rawKey := os.Getenv(EnvAPIKey)
	if s.apiKey != nil {
		rawKey = *s.apiKey
	}
	apiKey, err := normalizeAPIKey(rawKey)
	if err != nil {
		return nil, err
	}

	timeout := DefaultTimeout
	if s.timeout != nil {
		timeout = *s.timeout
	}
	if timeout <= 0 || timeout > maxTimeout {
		return nil, configError("The timeout must be more than 0 and at most 10 minutes.")
	}

	maxRetries := DefaultMaxRetries
	if s.maxRetries != nil {
		maxRetries = *s.maxRetries
	}
	if maxRetries < 0 || maxRetries > 2 {
		return nil, configError("Max retries must be 0, 1 or 2.")
	}

	userAgent := "sqlcockpit-go/" + Version
	if s.userAgent != nil {
		prefix := strings.TrimSpace(*s.userAgent)
		if len(prefix) > 200 || !isPrintableASCII(prefix, true) {
			return nil, configError("The user agent must be printable ASCII, at most 200 characters.")
		}
		if prefix != "" {
			userAgent = prefix + " " + userAgent
		}
	}

	httpClient := &http.Client{}
	if s.httpClient != nil {
		clone := *s.httpClient
		httpClient = &clone
	}
	// Never follow a redirect: the Authorization header must not travel to another address.
	httpClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	c := &Client{cfg: &config{
		apiKey:     apiKey,
		portalURL:  portalURL,
		baseURL:    portalURL + apiBasePath,
		httpClient: httpClient,
		timeout:    timeout,
		maxRetries: maxRetries,
		userAgent:  userAgent,
	}}
	c.Account = &AccountService{client: c}
	c.APIKeys = &APIKeysService{client: c}
	c.Tenants = &TenantsService{client: c}
	c.Team = &TeamService{client: c}
	c.Agents = &AgentsService{client: c}
	c.Instances = &InstancesService{client: c}
	c.Support = &SupportService{client: c}
	return c, nil
}

// PortalURL returns the portal's address, without the /api/portal/v1 path.
func (c Client) PortalURL() string {
	if c.cfg == nil {
		return ""
	}
	return c.cfg.portalURL
}

// Timeout returns how long one attempt may take.
func (c Client) Timeout() time.Duration {
	if c.cfg == nil {
		return 0
	}
	return c.cfg.timeout
}

// MaxRetries returns how many times a failed GET is retried.
func (c Client) MaxRetries() int {
	if c.cfg == nil {
		return 0
	}
	return c.cfg.maxRetries
}

// String describes the client without its API key.
func (c Client) String() string {
	return fmt.Sprintf("sqlcockpit.Client{PortalURL: %q, Timeout: %s, MaxRetries: %d}", c.PortalURL(), c.Timeout(), c.MaxRetries())
}

// GoString is used by the %#v verb. It never includes the API key.
func (c Client) GoString() string {
	return c.String()
}

// LogValue is used by log/slog. It never includes the API key.
func (c Client) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("portal_url", c.PortalURL()),
		slog.Duration("timeout", c.Timeout()),
		slog.Int("max_retries", c.MaxRetries()),
	)
}

// MarshalJSON describes the client without its API key.
func (c Client) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		PortalURL  string `json:"portal_url"`
		Timeout    string `json:"timeout"`
		MaxRetries int    `json:"max_retries"`
	}{c.PortalURL(), c.Timeout().String(), c.MaxRetries()})
}

func normalizePortalURL(value string) (string, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		raw = DefaultPortalURL
	}
	// Error messages never repeat the URL: it may hold credentials.
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Opaque != "" {
		return "", configError("The portal URL is not a valid URL.")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || strings.Contains(raw, "#") {
		return "", configError("The portal URL must not contain credentials, a query string or a fragment.")
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !isLoopbackHost(u.Hostname()) {
			return "", configError("The portal URL must use https:// (http:// is only allowed for localhost).")
		}
	default:
		return "", configError("The portal URL must use https:// (http:// is only allowed for localhost).")
	}
	normalized := strings.TrimRight(u.String(), "/")
	if strings.HasSuffix(strings.ToLower(normalized), apiBasePath) {
		normalized = strings.TrimRight(normalized[:len(normalized)-len(apiBasePath)], "/")
	}
	return normalized, nil
}

func isLoopbackHost(host string) bool {
	return strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1"
}

func normalizeAPIKey(value string) (string, error) {
	key := strings.TrimSpace(value)
	if key == "" {
		return "", configError("An API key is required. Pass WithAPIKey or set SQL_COCKPIT_API_KEY. Create a key in the portal under Settings > Developer access.")
	}
	if len(key) < 20 || len(key) > 200 || !isPrintableASCII(key, false) {
		return "", configError("The API key does not look like a SQL Cockpit portal API key.")
	}
	return key, nil
}

func isPrintableASCII(value string, allowSpace bool) bool {
	for i := 0; i < len(value); i++ {
		b := value[i]
		if b < 0x20 || b >= 0x7f || (b == ' ' && !allowSpace) {
			return false
		}
	}
	return true
}
