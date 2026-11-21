package sqlcockpit

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// uuidArg checks that value is a UUID and returns it in lower case. Nothing is sent otherwise.
func uuidArg(value, name string) (string, error) {
	if !uuidPattern.MatchString(value) {
		return "", notUUIDError(name)
	}
	return strings.ToLower(value), nil
}

func memberPath(tenantID, membershipID string) (string, error) {
	tenant, err := uuidArg(tenantID, "tenantID")
	if err != nil {
		return "", err
	}
	membership, err := uuidArg(membershipID, "membershipID")
	if err != nil {
		return "", err
	}
	return "/tenants/" + tenant + "/members/" + membership, nil
}

// AccountService reads your account and plan usage. Use it as client.Account.
type AccountService struct {
	client *Client
}

// Get returns your name, email, plan and subscription, and the credential that made the call.
//
// GET /account
func (s *AccountService) Get(ctx context.Context) (*Account, error) {
	var out Account
	if err := s.client.do(ctx, http.MethodGet, "/account", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Usage returns what your plan includes this billing period and how much of it is used.
//
// GET /usage
func (s *AccountService) Usage(ctx context.Context) (*Usage, error) {
	var out Usage
	if err := s.client.do(ctx, http.MethodGet, "/usage", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// APIKeysService lists your portal API keys. Use it as client.APIKeys.
//
// Creating and revoking keys need the keys:write scope, which an API key never holds, so this
// client cannot do either. Manage keys in the portal under Settings > Developer access.
type APIKeysService struct {
	client *Client
}

// List returns your portal API keys, newest first, with their status and last four characters.
// Never their values.
//
// GET /api-keys
func (s *APIKeysService) List(ctx context.Context) ([]APIKey, error) {
	var out struct {
		Keys []APIKey `json:"keys"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/api-keys", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Keys, nil
}

// TenantsService lists your tenants. Use it as client.Tenants.
type TenantsService struct {
	client *Client
}

// List returns every tenant you own or are a member of.
//
// GET /tenants
func (s *TenantsService) List(ctx context.Context) ([]Tenant, error) {
	var out struct {
		Tenants []Tenant `json:"tenants"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/tenants", nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Tenants, nil
}

// TeamService manages the members of a tenant. Use it as client.Team.
//
// Every id must be a UUID. Anything else is refused with ErrValidation before a request is sent.
type TeamService struct {
	client *Client
}

// List returns the whole team of a tenant: the owner, the members, and, when you can manage
// the team, the roles you may assign and the seats your plan includes.
//
// GET /tenants/{tenantId}/members
func (s *TeamService) List(ctx context.Context, tenantID string) (*Team, error) {
	tenant, err := uuidArg(tenantID, "tenantID")
	if err != nil {
		return nil, err
	}
	var out Team
	if err := s.client.do(ctx, http.MethodGet, "/tenants/"+tenant+"/members", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Add adds someone to a tenant. A person without a SQL Cockpit account receives an email to set
// a password; until then their status is MemberStatusPendingPassword. It is never retried.
//
// POST /tenants/{tenantId}/members
func (s *TeamService) Add(ctx context.Context, tenantID string, params AddMemberParams) (*MemberResult, error) {
	tenant, err := uuidArg(tenantID, "tenantID")
	if err != nil {
		return nil, err
	}
	body := addMemberRequest{Name: params.Name, Email: params.Email, Role: params.Role}
	var out MemberResult
	if err := s.client.do(ctx, http.MethodPost, "/tenants/"+tenant+"/members", nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ChangeRole gives a member another role. The owner's role cannot change. It sends
// confirm: true for you, so only call it when you mean it.
//
// PATCH /tenants/{tenantId}/members/{membershipId}
func (s *TeamService) ChangeRole(ctx context.Context, tenantID, membershipID string, role Role) (*MemberResult, error) {
	path, err := memberPath(tenantID, membershipID)
	if err != nil {
		return nil, err
	}
	body := changeRoleRequest{Role: role, Confirm: true}
	var out MemberResult
	if err := s.client.do(ctx, http.MethodPatch, path, nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Remove removes a member from a tenant. Their SQL Cockpit account stays. It sends
// confirm: true for you, so only call it when you mean it.
//
// DELETE /tenants/{tenantId}/members/{membershipId}
func (s *TeamService) Remove(ctx context.Context, tenantID, membershipID string) (*RemovedResult, error) {
	path, err := memberPath(tenantID, membershipID)
	if err != nil {
		return nil, err
	}
	var out RemovedResult
	if err := s.client.do(ctx, http.MethodDelete, path, nil, confirmRequest{Confirm: true}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ResendInvite sends the set-password email again to a member whose status is
// MemberStatusPendingPassword.
//
// POST /tenants/{tenantId}/members/{membershipId}/resend
func (s *TeamService) ResendInvite(ctx context.Context, tenantID, membershipID string) (*SentResult, error) {
	path, err := memberPath(tenantID, membershipID)
	if err != nil {
		return nil, err
	}
	var out SentResult
	if err := s.client.do(ctx, http.MethodPost, path+"/resend", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AgentsService lists your SQL Cockpit Agents. Use it as client.Agents.
type AgentsService struct {
	client *Client
}

// List returns the Agents paired with the tenants you can reach. params may be nil.
//
// GET /agents
func (s *AgentsService) List(ctx context.Context, params *ListAgentsParams) ([]Agent, error) {
	query := url.Values{}
	if params != nil {
		if params.TenantID != "" {
			tenant, err := uuidArg(params.TenantID, "tenantID")
			if err != nil {
				return nil, err
			}
			query.Set("tenant_id", tenant)
		}
		if params.Status != "" {
			query.Set("status", string(params.Status))
		}
	}
	var out struct {
		Agents []Agent `json:"agents"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/agents", query, nil, &out); err != nil {
		return nil, err
	}
	return out.Agents, nil
}

// InstancesService lists your SQL Cockpit Server instances. Use it as client.Instances.
type InstancesService struct {
	client *Client
}

// List returns the instances of the tenants you can reach. params may be nil.
//
// GET /instances
func (s *InstancesService) List(ctx context.Context, params *ListInstancesParams) ([]Instance, error) {
	query := url.Values{}
	if params != nil && params.TenantID != "" {
		tenant, err := uuidArg(params.TenantID, "tenantID")
		if err != nil {
			return nil, err
		}
		query.Set("tenant_id", tenant)
	}
	var out struct {
		Instances []Instance `json:"instances"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/instances", query, nil, &out); err != nil {
		return nil, err
	}
	return out.Instances, nil
}

// SupportService reads and opens support tickets. Use it as client.Support.
type SupportService struct {
	client *Client
}

// List returns the tickets you have opened, newest first. params may be nil.
//
// GET /support-tickets
func (s *SupportService) List(ctx context.Context, params *ListTicketsParams) ([]TicketSummary, error) {
	query := url.Values{}
	if params != nil && params.Status != "" {
		query.Set("status", string(params.Status))
	}
	var out struct {
		Tickets []TicketSummary `json:"tickets"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/support-tickets", query, nil, &out); err != nil {
		return nil, err
	}
	return out.Tickets, nil
}

// Get returns a ticket with its message, the Team's response and every reply.
//
// GET /support-tickets/{ticketId}
func (s *SupportService) Get(ctx context.Context, ticketID string) (*Ticket, error) {
	ticket, err := uuidArg(ticketID, "ticketID")
	if err != nil {
		return nil, err
	}
	var out struct {
		Ticket Ticket `json:"ticket"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/support-tickets/"+ticket, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out.Ticket, nil
}

// Create opens a support ticket with the SQL Cockpit Team. A ticket that names an Agent is
// raised as high priority. It is never retried, so a ticket is never sent twice.
//
// POST /support-tickets
func (s *SupportService) Create(ctx context.Context, params CreateTicketParams) (*Ticket, error) {
	body := createTicketRequest{Subject: params.Subject, Message: params.Message}
	if params.TenantID != "" {
		tenant, err := uuidArg(params.TenantID, "tenantID")
		if err != nil {
			return nil, err
		}
		body.TenantID = tenant
	}
	if params.AgentID != "" {
		agent, err := uuidArg(params.AgentID, "agentID")
		if err != nil {
			return nil, err
		}
		body.AgentID = agent
	}
	var out struct {
		Ticket Ticket `json:"ticket"`
	}
	if err := s.client.do(ctx, http.MethodPost, "/support-tickets", nil, body, &out); err != nil {
		return nil, err
	}
	return &out.Ticket, nil
}
