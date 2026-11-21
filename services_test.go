package sqlcockpit

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAccountGet(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{
		"name":"Ada Lovelace","email":"ada@example.com","email_verified":true,"billing_account":true,"team_member":false,
		"plan":{"name":"Team","description":null},
		"subscription":{"status":"active","cycle":"month","started_at":"2026-01-15T09:30:00+00:00","trial_ends_at":null,"ends_at":null},
		"connected_via":{"type":"api_key","name":"ci-pipeline","scopes":["portal:read","team:write","support:write"],"expires_at":"2026-12-01T00:00:00+00:00"},
		"api_key_name":"ci-pipeline","portal_url":"https://portal.sql-cockpit.com","billing_note":"Billing stays in the portal."}}`))

	account, err := p.client(t).Account.Get(bg)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/account", "", "")

	if account.Name != "Ada Lovelace" || account.Email != "ada@example.com" || !account.EmailVerified || !account.BillingAccount {
		t.Errorf("account = %+v", account)
	}
	if account.Plan == nil || account.Plan.Name != "Team" || account.Plan.Description != nil {
		t.Errorf("plan = %+v", account.Plan)
	}
	if account.Subscription == nil || account.Subscription.Cycle != "month" {
		t.Fatalf("subscription = %+v", account.Subscription)
	}
	want := time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)
	if s := account.Subscription; s.StartedAt == nil || !s.StartedAt.Equal(want) || s.TrialEndsAt != nil || s.EndsAt != nil {
		t.Errorf("subscription times = %+v", s)
	}
	if v := account.ConnectedVia; v.Type != "api_key" || len(v.Scopes) != 3 || v.ExpiresAt == nil {
		t.Errorf("connected_via = %+v", v)
	}
	if account.APIKeyName == nil || *account.APIKeyName != "ci-pipeline" {
		t.Errorf("api_key_name = %v", account.APIKeyName)
	}
}

func TestAccountGetWithoutPlan(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"name":"Sam","email":"sam@example.com","plan":null,"subscription":null,"api_key_name":null}}`))
	account, err := p.client(t).Account.Get(bg)
	if err != nil {
		t.Fatal(err)
	}
	if account.Plan != nil || account.Subscription != nil || account.APIKeyName != nil {
		t.Errorf("account = %+v", account)
	}
}

func TestAccountUsage(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"billing_account":true,"plan":"Team",
		"period":{"start":"2026-11-01T00:00:00+00:00","end":"2026-12-01T00:00:00+00:00"},
		"allowances":[
			{"key":"agents","label":"Agents","metered":false,"used":3,"limit":10,"unlimited":false,"included":true,"remaining":7,"at_limit":false},
			{"key":"seats","label":"Seats","metered":false,"used":2,"limit":null,"unlimited":true,"included":true,"remaining":null,"at_limit":false}
		]}}`))

	usage, err := p.client(t).Account.Usage(bg)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/usage", "", "")

	if !usage.BillingAccount || usage.Plan == nil || *usage.Plan != "Team" || usage.Period == nil {
		t.Fatalf("usage = %+v", usage)
	}
	if !usage.Period.End.Equal(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("period end = %v", usage.Period.End)
	}
	if len(usage.Allowances) != 2 {
		t.Fatalf("got %d allowances, want 2", len(usage.Allowances))
	}
	if a := usage.Allowances[0]; a.Key != "agents" || a.Used != 3 || a.Limit == nil || *a.Limit != 10 || a.Remaining == nil || *a.Remaining != 7 {
		t.Errorf("allowance 0 = %+v", a)
	}
	if a := usage.Allowances[1]; !a.Unlimited || a.Limit != nil || a.Remaining != nil {
		t.Errorf("allowance 1 = %+v", a)
	}
}

func TestAccountUsageTeamMemberOnly(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"billing_account":false,"team_member_only":true,"message":"Ask the owner.","plan":null,"period":null,"allowances":[]}}`))
	usage, err := p.client(t).Account.Usage(bg)
	if err != nil {
		t.Fatal(err)
	}
	if usage.BillingAccount || !usage.TeamMemberOnly || usage.Message != "Ask the owner." || usage.Period != nil || len(usage.Allowances) != 0 {
		t.Errorf("usage = %+v", usage)
	}
}

func TestAPIKeysList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"keys":[
		{"id":42,"name":"ci-pipeline","status":"active","ends_with":"mnop","created_at":"2026-11-01T10:00:00+00:00","last_used_at":null,"expires_at":"2026-11-08T10:00:00+00:00","revoked_at":null},
		{"id":7,"name":"old","status":"revoked","ends_with":"wxyz","created_at":"2026-10-01T10:00:00+00:00","last_used_at":"2026-10-02T10:00:00+00:00","expires_at":"2026-10-02T11:00:00+00:00","revoked_at":"2026-10-02T11:00:00+00:00"}
	]}}`))

	keys, err := p.client(t).APIKeys.List(bg)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/api-keys", "", "")

	if len(keys) != 2 {
		t.Fatalf("got %d keys, want 2", len(keys))
	}
	if k := keys[0]; k.ID != 42 || k.Name != "ci-pipeline" || k.Status != "active" || k.EndsWith != "mnop" || k.LastUsedAt != nil || k.RevokedAt != nil || k.CreatedAt == nil {
		t.Errorf("key 0 = %+v", k)
	}
	if k := keys[1]; k.ID != 7 || k.RevokedAt == nil || k.LastUsedAt == nil {
		t.Errorf("key 1 = %+v", k)
	}
}

func TestTenantsList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"tenants":[
		{"id":"`+testTenantID+`","name":"Acme","slug":"acme","subdomain":"acme","domain":null,"status":"active","your_role":"owner",
		 "is_owner":true,"abilities":["team.manage","agents.view"],"agents_count":4,"created_at":"2026-02-03T04:05:06+00:00"}
	]}}`))

	tenants, err := p.client(t).Tenants.List(bg)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/tenants", "", "")

	if len(tenants) != 1 {
		t.Fatalf("got %d tenants, want 1", len(tenants))
	}
	tenant := tenants[0]
	if tenant.ID != testTenantID || tenant.YourRole != RoleOwner || !tenant.IsOwner || tenant.AgentsCount != 4 || len(tenant.Abilities) != 2 {
		t.Errorf("tenant = %+v", tenant)
	}
	if tenant.Subdomain == nil || *tenant.Subdomain != "acme" || tenant.Domain != nil {
		t.Errorf("subdomain/domain = %v/%v", tenant.Subdomain, tenant.Domain)
	}
	if !tenant.CreatedAt.Equal(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)) {
		t.Errorf("created_at = %v", tenant.CreatedAt)
	}
}

const teamJSON = `{"ok":true,"data":{
	"tenant":{"id":"` + testTenantID + `","name":"Acme"},
	"owner":{"name":"Ada Lovelace","email":"ada@example.com"},
	"members":[
		{"id":"` + testMembershipID + `","name":"Sam Lee","email":"sam@example.com","role":"developer","role_label":"Developer",
		 "status":"pending_password","joined_at":null,"added_at":"2026-11-19T08:00:00+00:00"}
	],
	"you_can_manage":true,
	"assignable_roles":["admin","developer","operator","read-only"],
	"role_summaries":{"admin":"Manages the team."},
	"seats":{"used":2,"limit":5}}}`

func TestTeamList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, teamJSON))

	// An upper-case id is sent in lower case.
	team, err := p.client(t).Team.List(bg, strings.ToUpper(testTenantID))
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/tenants/"+testTenantID+"/members", "", "")

	if team.Tenant.ID != testTenantID || team.Tenant.Name != "Acme" || team.Owner.Email != "ada@example.com" || !team.YouCanManage {
		t.Errorf("team = %+v", team)
	}
	if len(team.Members) != 1 {
		t.Fatalf("got %d members, want 1", len(team.Members))
	}
	if m := team.Members[0]; m.ID != testMembershipID || m.Role != RoleDeveloper || m.Status != MemberStatusPendingPassword || m.JoinedAt != nil || m.AddedAt.IsZero() {
		t.Errorf("member = %+v", m)
	}
	if len(team.AssignableRoles) != 4 || team.AssignableRoles[3] != RoleReadOnly {
		t.Errorf("assignable_roles = %v", team.AssignableRoles)
	}
	if team.RoleSummaries["admin"] != "Manages the team." {
		t.Errorf("role_summaries = %v", team.RoleSummaries)
	}
	if team.Seats == nil || team.Seats.Used != 2 || team.Seats.Limit == nil || *team.Seats.Limit != 5 {
		t.Errorf("seats = %+v", team.Seats)
	}
}

func TestTeamListWhenYouCannotManage(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"tenant":{"id":"`+testTenantID+`","name":"Acme"},
		"owner":{"name":"Ada","email":"ada@example.com"},"members":[],"you_can_manage":false,
		"assignable_roles":null,"role_summaries":{},"seats":null}}`))

	team, err := p.client(t).Team.List(bg, testTenantID)
	if err != nil {
		t.Fatal(err)
	}
	if team.YouCanManage || team.AssignableRoles != nil || team.Seats != nil {
		t.Errorf("team = %+v", team)
	}
}

const memberJSON = `{"ok":true,"data":{"member":{"id":"` + testMembershipID + `","name":"Sam Lee","email":"sam@example.com",
	"role":"admin","role_label":"Admin","status":"active","joined_at":"2026-11-19T09:00:00+00:00","added_at":"2026-11-19T08:00:00+00:00"},
	"message":"Done."}}`

func TestTeamAdd(t *testing.T) {
	p := newPortal(t, respond(http.StatusCreated, memberJSON))

	result, err := p.client(t).Team.Add(bg, testTenantID, AddMemberParams{Name: "Sam Lee", Email: "sam@example.com", Role: RoleDeveloper})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "POST", "/tenants/"+testTenantID+"/members", "", `{"name":"Sam Lee","email":"sam@example.com","role":"developer"}`)

	if result.Member.ID != testMembershipID || result.Member.JoinedAt == nil || result.Message != "Done." {
		t.Errorf("result = %+v", result)
	}
}

func TestTeamChangeRole(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, memberJSON))

	result, err := p.client(t).Team.ChangeRole(bg, testTenantID, testMembershipID, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "PATCH", "/tenants/"+testTenantID+"/members/"+testMembershipID, "", `{"role":"admin","confirm":true}`)

	if result.Member.Role != RoleAdmin {
		t.Errorf("role = %q, want admin", result.Member.Role)
	}
}

func TestTeamRemove(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"removed":true,"email":"sam@example.com","tenant_id":"`+testTenantID+`"}}`))

	result, err := p.client(t).Team.Remove(bg, testTenantID, testMembershipID)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "DELETE", "/tenants/"+testTenantID+"/members/"+testMembershipID, "", `{"confirm":true}`)

	if !result.Removed || result.Email != "sam@example.com" || result.TenantID != testTenantID {
		t.Errorf("result = %+v", result)
	}
}

func TestTeamResendInvite(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"sent":true,"email":"sam@example.com"}}`))

	result, err := p.client(t).Team.ResendInvite(bg, testTenantID, testMembershipID)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "POST", "/tenants/"+testTenantID+"/members/"+testMembershipID+"/resend", "", "")

	if !result.Sent || result.Email != "sam@example.com" {
		t.Errorf("result = %+v", result)
	}
}

const agentsJSON = `{"ok":true,"data":{"agents":[
	{"id":"` + testAgentID + `","tenant_id":"` + testTenantID + `","tenant_name":"Acme","name":"sql-01","hostname":"sql-01.acme.local",
	 "status":"active","connection":"online","update_channel":"stable","desired_role":null,
	 "last_seen_at":"2026-11-20T08:00:00+00:00","registered_at":"2026-01-01T00:00:00+00:00","revoked_at":null,
	 "latest_command":{"type":"collect","status":"succeeded","created_at":"2026-11-20T07:59:00+00:00"}}
]}}`

func TestAgentsList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, agentsJSON))

	agents, err := p.client(t).Agents.List(bg, &ListAgentsParams{TenantID: testTenantID, Status: AgentStatusActive})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/agents", "status=active&tenant_id="+testTenantID, "")

	if len(agents) != 1 {
		t.Fatalf("got %d agents, want 1", len(agents))
	}
	a := agents[0]
	if a.ID != testAgentID || a.Status != AgentStatusActive || a.Hostname == nil || a.DesiredRole != nil || a.RevokedAt != nil || a.LastSeenAt == nil {
		t.Errorf("agent = %+v", a)
	}
	if a.LatestCommand == nil || a.LatestCommand.Type != "collect" || a.LatestCommand.CreatedAt.IsZero() {
		t.Errorf("latest_command = %+v", a.LatestCommand)
	}
}

func TestAgentsListWithoutFilters(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"agents":[]}}`))

	agents, err := p.client(t).Agents.List(bg, nil)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/agents", "", "")
	if len(agents) != 0 {
		t.Errorf("got %d agents, want 0", len(agents))
	}
}

func TestInstancesList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"instances":[
		{"id":"`+testAgentID+`","tenant_id":"`+testTenantID+`","tenant_name":null,"hostname":"cockpit.acme.com","public_url":"https://cockpit.acme.com",
		 "hosting_mode":"hosted","status":"running","revision":12,"applied_revision":null,"last_seen_at":null,"created_at":"2026-03-04T05:06:07+00:00"}
	]}}`))

	instances, err := p.client(t).Instances.List(bg, &ListInstancesParams{TenantID: testTenantID})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/instances", "tenant_id="+testTenantID, "")

	if len(instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(instances))
	}
	i := instances[0]
	if i.HostingMode != "hosted" || i.Revision != 12 || i.AppliedRevision != nil || i.TenantName != nil || i.PublicURL == nil || i.CreatedAt.IsZero() {
		t.Errorf("instance = %+v", i)
	}
}

func TestInstancesListWithoutFilters(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"instances":[]}}`))
	if _, err := p.client(t).Instances.List(bg, nil); err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/instances", "", "")
}

func TestSupportList(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"tickets":[
		{"id":"`+testTicketID+`","subject":"Agent offline","status":"open","status_label":"Open","priority":"high",
		 "tenant":{"id":"`+testTenantID+`","name":"Acme"},"created_at":"2026-11-20T08:00:00+00:00","updated_at":"2026-11-20T08:05:00+00:00",
		 "portal_url":"https://portal.sql-cockpit.com/support/`+testTicketID+`"}
	]}}`))

	tickets, err := p.client(t).Support.List(bg, &ListTicketsParams{Status: TicketStatusOpen})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/support-tickets", "status=open", "")

	if len(tickets) != 1 {
		t.Fatalf("got %d tickets, want 1", len(tickets))
	}
	if tk := tickets[0]; tk.ID != testTicketID || tk.Status != TicketStatusOpen || tk.Priority != "high" || tk.Tenant == nil || tk.Tenant.Name != "Acme" {
		t.Errorf("ticket = %+v", tk)
	}
}

func TestSupportListWithoutFilters(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"tickets":[]}}`))
	if _, err := p.client(t).Support.List(bg, nil); err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/support-tickets", "", "")
}

func TestSupportGet(t *testing.T) {
	p := newPortal(t, respond(http.StatusOK, `{"ok":true,"data":{"ticket":{
		"id":"`+testTicketID+`","subject":"Agent offline","status":"waiting_on_customer","status_label":"Waiting on you","priority":"high",
		"tenant":null,"created_at":"2026-11-20T08:00:00+00:00","updated_at":"2026-11-20T09:00:00+00:00","portal_url":"https://portal.sql-cockpit.com/support/1",
		"message":"The Agent on sql-01 stopped reporting.","agent":{"id":"`+testAgentID+`","name":"sql-01","hostname":null},
		"team_response":"Please restart it.","responded_at":"2026-11-20T08:30:00+00:00","resolved_at":null,"closed_at":null,
		"replies":[{"from":"you","body":"Restarted.","created_at":"2026-11-20T09:00:00+00:00"},{"from":"sql_cockpit_team","body":"Thanks.","created_at":"2026-11-20T09:01:00+00:00"}]}}}`))

	ticket, err := p.client(t).Support.Get(bg, testTicketID)
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "GET", "/support-tickets/"+testTicketID, "", "")

	if ticket.ID != testTicketID || ticket.Status != TicketStatusWaitingOnCustomer || ticket.Tenant != nil {
		t.Errorf("summary fields = %+v", ticket.TicketSummary)
	}
	if ticket.Message != "The Agent on sql-01 stopped reporting." || ticket.TeamResponse == nil || ticket.RespondedAt == nil || ticket.ResolvedAt != nil {
		t.Errorf("ticket = %+v", ticket)
	}
	if ticket.Agent == nil || ticket.Agent.ID != testAgentID || ticket.Agent.Hostname != nil {
		t.Errorf("agent = %+v", ticket.Agent)
	}
	if len(ticket.Replies) != 2 || ticket.Replies[1].From != "sql_cockpit_team" || ticket.Replies[0].CreatedAt.IsZero() {
		t.Errorf("replies = %+v", ticket.Replies)
	}
}

const createdTicketJSON = `{"ok":true,"data":{"ticket":{"id":"` + testTicketID + `","subject":"Agent offline","status":"sent",
	"status_label":"Sent","priority":"high","tenant":null,"created_at":"2026-11-20T08:00:00+00:00","updated_at":"2026-11-20T08:00:00+00:00",
	"portal_url":"https://portal.sql-cockpit.com/support/1","message":"Your ticket was sent."}}}`

func TestSupportCreate(t *testing.T) {
	p := newPortal(t, respond(http.StatusCreated, createdTicketJSON))

	ticket, err := p.client(t).Support.Create(bg, CreateTicketParams{
		Subject:  "Agent offline",
		Message:  "The Agent on sql-01 stopped reporting.",
		TenantID: testTenantID,
		AgentID:  strings.ToUpper(testAgentID),
	})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "POST", "/support-tickets", "",
		`{"subject":"Agent offline","message":"The Agent on sql-01 stopped reporting.","tenant_id":"`+testTenantID+`","agent_id":"`+testAgentID+`"}`)

	if ticket.ID != testTicketID || ticket.Status != TicketStatusSent || ticket.Message != "Your ticket was sent." {
		t.Errorf("ticket = %+v", ticket)
	}
}

func TestSupportCreateWithoutTenant(t *testing.T) {
	p := newPortal(t, respond(http.StatusCreated, createdTicketJSON))

	_, err := p.client(t).Support.Create(bg, CreateTicketParams{Subject: "A question", Message: "How do I add a seat to my plan?"})
	if err != nil {
		t.Fatal(err)
	}
	p.expect(t, "POST", "/support-tickets", "", `{"subject":"A question","message":"How do I add a seat to my plan?"}`)
}
