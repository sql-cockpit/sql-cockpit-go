package sqlcockpit

import "time"

// Role is a role a tenant member can hold.
type Role string

// The roles. RoleOwner cannot be assigned through the API.
const (
	RoleOwner     Role = "owner"
	RoleAdmin     Role = "admin"
	RoleDeveloper Role = "developer"
	RoleOperator  Role = "operator"
	RoleReadOnly  Role = "read-only"
)

// AgentStatus is the status of a SQL Cockpit Agent.
type AgentStatus string

// The Agent statuses.
const (
	AgentStatusPending   AgentStatus = "pending"
	AgentStatusActive    AgentStatus = "active"
	AgentStatusRevoked   AgentStatus = "revoked"
	AgentStatusSuspended AgentStatus = "suspended"
)

// TicketStatus is the status of a support ticket.
type TicketStatus string

// The ticket statuses.
const (
	TicketStatusOpen              TicketStatus = "open"
	TicketStatusPending           TicketStatus = "pending"
	TicketStatusSent              TicketStatus = "sent"
	TicketStatusWaitingOnCustomer TicketStatus = "waiting_on_customer"
	TicketStatusResolved          TicketStatus = "resolved"
	TicketStatusClosed            TicketStatus = "closed"
)

// MemberStatus is the status of a team member.
type MemberStatus string

// The member statuses. A member who has not yet set a password is MemberStatusPendingPassword.
const (
	MemberStatusActive          MemberStatus = "active"
	MemberStatusPendingPassword MemberStatus = "pending_password"
)

// Account is your profile, plan and subscription, and the credential that made the call.
// Plan and Subscription are nil when you have none.
type Account struct {
	Name           string        `json:"name"`
	Email          string        `json:"email"`
	EmailVerified  bool          `json:"email_verified"`
	BillingAccount bool          `json:"billing_account"`
	TeamMember     bool          `json:"team_member"`
	Plan           *Plan         `json:"plan"`
	Subscription   *Subscription `json:"subscription"`
	ConnectedVia   ConnectedVia  `json:"connected_via"`
	APIKeyName     *string       `json:"api_key_name"`
	PortalURL      string        `json:"portal_url"`
	BillingNote    string        `json:"billing_note"`
}

// Plan is the plan on an [Account].
type Plan struct {
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

// Subscription is the subscription on an [Account]. Cycle is "month" or "year".
type Subscription struct {
	Status      string     `json:"status"`
	Cycle       string     `json:"cycle"`
	StartedAt   *time.Time `json:"started_at"`
	TrialEndsAt *time.Time `json:"trial_ends_at"`
	EndsAt      *time.Time `json:"ends_at"`
}

// ConnectedVia is the credential that made the call. Type is "api_key" or "oauth".
type ConnectedVia struct {
	Type      string     `json:"type"`
	Name      string     `json:"name"`
	Scopes    []string   `json:"scopes"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Usage is what your plan includes this billing period and how much of it is used. A team
// member who is not a billing account gets TeamMemberOnly, a Message and no allowances.
type Usage struct {
	BillingAccount bool         `json:"billing_account"`
	TeamMemberOnly bool         `json:"team_member_only,omitempty"`
	Message        string       `json:"message,omitempty"`
	Plan           *string      `json:"plan"`
	Period         *UsagePeriod `json:"period"`
	Allowances     []Allowance  `json:"allowances"`
}

// UsagePeriod is the billing period of a [Usage].
type UsagePeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Allowance is one thing your plan includes. Limit and Remaining are nil when unlimited.
type Allowance struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Metered   bool   `json:"metered"`
	Used      int    `json:"used"`
	Limit     *int   `json:"limit"`
	Unlimited bool   `json:"unlimited"`
	Included  bool   `json:"included"`
	Remaining *int   `json:"remaining"`
	AtLimit   bool   `json:"at_limit"`
}

// APIKey is one of your portal API keys. The key's value is never returned; EndsWith holds its
// last four characters to tell keys apart. Status is "active", "expired" or "revoked".
type APIKey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Status     string     `json:"status"`
	EndsWith   string     `json:"ends_with"`
	CreatedAt  *time.Time `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
}

// Tenant is a tenant you own or are a member of, with your role and what it lets you do.
type Tenant struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Subdomain   *string   `json:"subdomain"`
	Domain      *string   `json:"domain"`
	Status      string    `json:"status"`
	YourRole    Role      `json:"your_role"`
	IsOwner     bool      `json:"is_owner"`
	Abilities   []string  `json:"abilities"`
	AgentsCount int       `json:"agents_count"`
	CreatedAt   time.Time `json:"created_at"`
}

// Member is a member of a tenant. JoinedAt is nil until they have set a password.
type Member struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Email     string       `json:"email"`
	Role      Role         `json:"role"`
	RoleLabel string       `json:"role_label"`
	Status    MemberStatus `json:"status"`
	JoinedAt  *time.Time   `json:"joined_at"`
	AddedAt   time.Time    `json:"added_at"`
}

// Team is the owner and members of a tenant. AssignableRoles and Seats are only set when you
// can manage the team (YouCanManage); otherwise they are nil.
type Team struct {
	Tenant          TeamTenant        `json:"tenant"`
	Owner           TeamOwner         `json:"owner"`
	Members         []Member          `json:"members"`
	YouCanManage    bool              `json:"you_can_manage"`
	AssignableRoles []Role            `json:"assignable_roles"`
	RoleSummaries   map[string]string `json:"role_summaries"`
	Seats           *Seats            `json:"seats"`
}

// TeamTenant names the tenant of a [Team].
type TeamTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TeamOwner is the owner of a tenant.
type TeamOwner struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Seats is how many seats a team uses and how many the plan includes. Limit is nil when
// unlimited.
type Seats struct {
	Used  int  `json:"used"`
	Limit *int `json:"limit"`
}

// AddMemberParams is what [TeamService.Add] sends. Name is 2 to 120 characters and Email up
// to 254. Role is any role except RoleOwner.
type AddMemberParams struct {
	Name  string
	Email string
	Role  Role
}

// MemberResult is what [TeamService.Add] and [TeamService.ChangeRole] return.
type MemberResult struct {
	Member  Member `json:"member"`
	Message string `json:"message"`
}

// RemovedResult is what [TeamService.Remove] returns.
type RemovedResult struct {
	Removed  bool   `json:"removed"`
	Email    string `json:"email"`
	TenantID string `json:"tenant_id"`
}

// SentResult is what [TeamService.ResendInvite] returns.
type SentResult struct {
	Sent  bool   `json:"sent"`
	Email string `json:"email"`
}

// Agent is a SQL Cockpit Agent paired with one of your tenants.
type Agent struct {
	ID            string        `json:"id"`
	TenantID      string        `json:"tenant_id"`
	TenantName    *string       `json:"tenant_name"`
	Name          string        `json:"name"`
	Hostname      *string       `json:"hostname"`
	Status        AgentStatus   `json:"status"`
	Connection    string        `json:"connection"`
	UpdateChannel *string       `json:"update_channel"`
	DesiredRole   *string       `json:"desired_role"`
	LastSeenAt    *time.Time    `json:"last_seen_at"`
	RegisteredAt  *time.Time    `json:"registered_at"`
	RevokedAt     *time.Time    `json:"revoked_at"`
	LatestCommand *AgentCommand `json:"latest_command"`
}

// AgentCommand is the latest command an [Agent] ran.
type AgentCommand struct {
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// ListAgentsParams filters [AgentsService.List]. Leave a field empty to not filter by it.
// TenantID must be a UUID.
type ListAgentsParams struct {
	TenantID string
	Status   AgentStatus
}

// Instance is a hosted or self-hosted SQL Cockpit Server instance.
type Instance struct {
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	TenantName      *string    `json:"tenant_name"`
	Hostname        *string    `json:"hostname"`
	PublicURL       *string    `json:"public_url"`
	HostingMode     string     `json:"hosting_mode"`
	Status          string     `json:"status"`
	Revision        int        `json:"revision"`
	AppliedRevision *int       `json:"applied_revision"`
	LastSeenAt      *time.Time `json:"last_seen_at"`
	CreatedAt       time.Time  `json:"created_at"`
}

// ListInstancesParams filters [InstancesService.List]. TenantID, when set, must be a UUID.
type ListInstancesParams struct {
	TenantID string
}

// TicketSummary is a support ticket as listed. Priority is "normal" or "high". Tenant is nil
// when the ticket is not about a tenant.
type TicketSummary struct {
	ID          string        `json:"id"`
	Subject     string        `json:"subject"`
	Status      TicketStatus  `json:"status"`
	StatusLabel string        `json:"status_label"`
	Priority    string        `json:"priority"`
	Tenant      *TicketTenant `json:"tenant"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	PortalURL   string        `json:"portal_url"`
}

// TicketTenant names the tenant a ticket is about.
type TicketTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Ticket is a support ticket with its message, the Team's response and every reply.
//
// [SupportService.Get] fills every field. [SupportService.Create] returns the summary fields
// plus Message, which then holds the portal's confirmation rather than the ticket text.
type Ticket struct {
	TicketSummary

	Message      string        `json:"message"`
	Agent        *TicketAgent  `json:"agent"`
	TeamResponse *string       `json:"team_response"`
	RespondedAt  *time.Time    `json:"responded_at"`
	ResolvedAt   *time.Time    `json:"resolved_at"`
	ClosedAt     *time.Time    `json:"closed_at"`
	Replies      []TicketReply `json:"replies"`
}

// TicketAgent names the Agent a ticket is about.
type TicketAgent struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Hostname *string `json:"hostname"`
}

// TicketReply is one reply on a ticket. From is "you" or "sql_cockpit_team".
type TicketReply struct {
	From      string    `json:"from"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// ListTicketsParams filters [SupportService.List]. Leave Status empty for every ticket.
type ListTicketsParams struct {
	Status TicketStatus
}

// CreateTicketParams is what [SupportService.Create] sends. Subject is 4 to 160 characters and
// Message 10 to 5000. TenantID and AgentID are optional UUIDs; AgentID must belong to TenantID,
// and naming an Agent raises the ticket as high priority.
type CreateTicketParams struct {
	Subject  string
	Message  string
	TenantID string
	AgentID  string
}

// Request bodies.

type addMemberRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  Role   `json:"role"`
}

type changeRoleRequest struct {
	Role    Role `json:"role"`
	Confirm bool `json:"confirm"`
}

type confirmRequest struct {
	Confirm bool `json:"confirm"`
}

type createTicketRequest struct {
	Subject  string `json:"subject"`
	Message  string `json:"message"`
	TenantID string `json:"tenant_id,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
}
