// Package sqlcockpit is the official Go client for the SQL Cockpit portal API.
//
// Use it to read your account, plan usage, API keys, tenants, team, Agents, instances and
// support tickets, and to manage team members and open tickets from your own code.
//
// Create a client with [NewClient]. It reads the API key from SQL_COCKPIT_API_KEY unless you
// pass [WithAPIKey]:
//
//	client, err := sqlcockpit.NewClient()
//	if err != nil {
//		log.Fatal(err)
//	}
//	account, err := client.Account.Get(ctx)
//
// Every method takes a [context.Context] first. Cancel it, or give it a deadline, to stop a
// call and any retry wait.
//
// Every error the client returns for a failed call is an [*Error], except when the context is
// canceled or its deadline passes, in which case the method returns ctx.Err(). Use
// [errors.Is] with [ErrNotFound], [ErrValidation] and the other sentinels, or the helpers such
// as [IsNotFound], to handle one case.
//
// The client only talks to the portal over https (http is allowed for localhost), never follows
// redirects, refuses responses larger than 2 MiB, and keeps the API key out of errors and
// formatted output.
package sqlcockpit
