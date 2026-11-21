package sqlcockpit_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	sqlcockpit "github.com/sql-cockpit/sql-cockpit-go"
)

func ExampleNewClient() {
	// A stand-in for the portal so this example runs offline. In your own code leave
	// WithPortalURL out, and leave WithAPIKey out to read SQL_COCKPIT_API_KEY.
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"ok":true,"data":{"name":"Ada Lovelace","email":"ada@example.com","plan":{"name":"Team","description":null}}}`)
	}))
	defer portal.Close()

	client, err := sqlcockpit.NewClient(
		sqlcockpit.WithAPIKey("sco_example_0123456789abcdef"),
		sqlcockpit.WithPortalURL(portal.URL),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	account, err := client.Account.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Name:  %s\n", account.Name)
	fmt.Printf("Email: %s\n", account.Email)
	fmt.Printf("Plan:  %s\n", account.Plan.Name)
	// Output:
	// Name:  Ada Lovelace
	// Email: ada@example.com
	// Plan:  Team
}

func ExampleIsNotFound() {
	portal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"ok":false,"error":{"code":"not_found","message":"Tenant not found, or not one your account can reach."}}`)
	}))
	defer portal.Close()

	client, err := sqlcockpit.NewClient(
		sqlcockpit.WithAPIKey("sco_example_0123456789abcdef"),
		sqlcockpit.WithPortalURL(portal.URL),
	)
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.Team.List(context.Background(), "3f2504e0-4f89-41d3-9a0c-0305e82c3301")
	var apiErr *sqlcockpit.Error
	switch {
	case sqlcockpit.IsNotFound(err):
		fmt.Println("No such tenant.")
	case errors.As(err, &apiErr):
		fmt.Println(apiErr.Code, apiErr.Message)
	case err != nil:
		log.Fatal(err)
	}
	// Output:
	// No such tenant.
}
