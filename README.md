# SQL Cockpit Go client

The official Go client for the SQL Cockpit portal API. Use it to read your account, plan usage, API keys, tenants, team, Agents, instances and support tickets, and to manage team members and open tickets from your own code.

It uses only the standard library and needs Go 1.22 or later.

## Install

```sh
go get github.com/sql-cockpit/sql-cockpit-go
```

The package name is `sqlcockpit`.

## Your first program

Create an API key in the portal under **Settings > Developer access**, then put it in the environment:

```sh
export SQL_COCKPIT_API_KEY=your-key
```

```go
package main

import (
	"context"
	"fmt"
	"log"

	sqlcockpit "github.com/sql-cockpit/sql-cockpit-go"
)

func main() {
	client, err := sqlcockpit.NewClient() // reads SQL_COCKPIT_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	account, err := client.Account.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(account.Name, account.Email)
	if account.Plan != nil {
		fmt.Println("Plan:", account.Plan.Name)
	}
}
```

The same program is in [examples/first-call](examples/first-call/main.go): `go run ./examples/first-call`.

## Methods

Every method takes a `context.Context` first and returns the response's `data`, already unwrapped. List methods return slices. Struct fields use Go names (`CreatedAt`, `TenantID`); the JSON tags keep the API's names. Fields the API can send as `null` are pointers.

| Method | HTTP operation | Returns |
| --- | --- | --- |
| `client.Account.Get(ctx)` | `GET /account` | `*Account` |
| `client.Account.Usage(ctx)` | `GET /usage` | `*Usage` |
| `client.APIKeys.List(ctx)` | `GET /api-keys` | `[]APIKey` |
| `client.Tenants.List(ctx)` | `GET /tenants` | `[]Tenant` |
| `client.Team.List(ctx, tenantID)` | `GET /tenants/{tenantId}/members` | `*Team` (owner, members, roles, seats) |
| `client.Team.Add(ctx, tenantID, sqlcockpit.AddMemberParams{Name, Email, Role})` | `POST /tenants/{tenantId}/members` | `*MemberResult` |
| `client.Team.ChangeRole(ctx, tenantID, membershipID, role)` | `PATCH /tenants/{tenantId}/members/{membershipId}` | `*MemberResult` |
| `client.Team.Remove(ctx, tenantID, membershipID)` | `DELETE /tenants/{tenantId}/members/{membershipId}` | `*RemovedResult` |
| `client.Team.ResendInvite(ctx, tenantID, membershipID)` | `POST /tenants/{tenantId}/members/{membershipId}/resend` | `*SentResult` |
| `client.Agents.List(ctx, &sqlcockpit.ListAgentsParams{TenantID, Status})` | `GET /agents` | `[]Agent` |
| `client.Instances.List(ctx, &sqlcockpit.ListInstancesParams{TenantID})` | `GET /instances` | `[]Instance` |
| `client.Support.List(ctx, &sqlcockpit.ListTicketsParams{Status})` | `GET /support-tickets` | `[]TicketSummary` |
| `client.Support.Get(ctx, ticketID)` | `GET /support-tickets/{ticketId}` | `*Ticket` |
| `client.Support.Create(ctx, sqlcockpit.CreateTicketParams{Subject, Message, TenantID, AgentID})` | `POST /support-tickets` | `*Ticket` |

The list filters are optional: pass `nil`, or leave a field empty.

`Team.ChangeRole` and `Team.Remove` send `confirm: true` for you. The API asks for it so a change is never made by accident, so only call them when you mean it.

Ids in a path (`tenantID`, `membershipID`, `ticketID`) and the `TenantID` and `AgentID` fields must be UUIDs. Anything else is refused with `ErrValidation` before a request is sent.

### API keys cannot create keys

Creating and revoking API keys needs the `keys:write` scope. An API key never holds it: only a person signing in through OAuth can grant it. So this client can list your keys but not create or revoke them. Do that in the portal under **Settings > Developer access**.

Plans, billing and deleting anything other than a team membership are not part of the API either. Do those in the portal.

## Errors

Every error from a failed call is a `*sqlcockpit.Error` with:

- `Code`: the API's error code, such as `not_found` or `validation_failed`, or one of the client's own (`invalid_config`, `invalid_argument`, `unexpected_redirect`, `invalid_response`, `response_too_large`, `connection_error`, `timeout`)
- `Status`: the HTTP status, or 0 when no response arrived
- `Message`: what went wrong, in plain words
- `Fields`: for validation errors, the messages for each field
- `RetryAfter`: for 429, the wait the portal asked for

Check one case with `errors.Is` or a helper:

| Sentinel | Helper | When |
| --- | --- | --- |
| `ErrUnauthenticated` | `IsUnauthenticated` | 401. The key is missing, wrong, revoked or expired. |
| `ErrForbidden` | `IsForbidden` | 403. The key lacks the scope, your plan has no room, or your role cannot do this. |
| `ErrNotFound` | `IsNotFound` | 404. Not found, or not something your account can reach. |
| `ErrValidation` | `IsValidation` | 400 or 422, or a bad id caught before sending. See `Fields`. |
| `ErrRateLimited` | `IsRateLimited` | 429. See `RetryAfter`. |
| `ErrServer` | `IsServerError` | 5xx. |

```go
_, err := client.Team.Add(ctx, tenantID, sqlcockpit.AddMemberParams{
	Name:  "Sam Lee",
	Email: "sam@example.com",
	Role:  sqlcockpit.RoleDeveloper,
})
var apiErr *sqlcockpit.Error
switch {
case sqlcockpit.IsValidation(err) && errors.As(err, &apiErr):
	fmt.Println(apiErr.Fields)
case sqlcockpit.IsNotFound(err):
	fmt.Println("No such tenant.")
case err != nil:
	log.Fatal(err)
}
```

GET requests are retried on 429 and 5xx, up to 2 times by default. The client waits for Retry-After when the portal sends it, but never more than 30 seconds. Writes (POST, PATCH, DELETE) are never retried, so a ticket or invitation is never sent twice.

## Cancellation

Every method takes a context. Cancel it, or give it a deadline, to stop the call and any retry wait:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

tickets, err := client.Support.List(ctx, &sqlcockpit.ListTicketsParams{Status: sqlcockpit.TicketStatusOpen})
```

When the context ends first, the method returns `ctx.Err()`, so `errors.Is(err, context.DeadlineExceeded)` or `errors.Is(err, context.Canceled)` is true.

## Configuration

```go
client, err := sqlcockpit.NewClient(
	sqlcockpit.WithAPIKey(os.Getenv("SQL_COCKPIT_API_KEY")),
	sqlcockpit.WithPortalURL("https://portal.sql-cockpit.com"),
	sqlcockpit.WithTimeout(20*time.Second),
	sqlcockpit.WithMaxRetries(2),
	sqlcockpit.WithUserAgent("acme-sync/1.0"),
	sqlcockpit.WithHTTPClient(&http.Client{Transport: myTransport}),
)
```

| Option | Default | Notes |
| --- | --- | --- |
| `WithAPIKey` | `SQL_COCKPIT_API_KEY` | Required. |
| `WithPortalURL` | `SQL_COCKPIT_PORTAL_URL`, then `https://portal.sql-cockpit.com` | Must use https. http is allowed for `localhost`, `127.0.0.1` and `[::1]`. |
| `WithTimeout` | 20 seconds | Per attempt. At most 10 minutes. |
| `WithMaxRetries` | 2 | 0 to 2. GET requests only. |
| `WithUserAgent` | none | Put in front of `sqlcockpit-go/<version>`. |
| `WithHTTPClient` | a new `http.Client` | For proxies or custom transports. It is copied, and the copy never follows redirects. |

A `Client` is safe for concurrent use. Create one and share it.

## Safety

- `fmt.Print(client)`, `%#v`, `json.Marshal(client)` and `log/slog` show only the portal URL and settings. The API key is never included, and it is removed from any error message that echoes it.
- Redirects are refused, not followed, so the key is never sent to another address.
- Responses over 2 MiB are refused.
- Each request sends `User-Agent: sqlcockpit-go/<version>`.

## Development

```sh
gofmt -l .
go vet ./...
go test -race ./...
```

## License

MIT
