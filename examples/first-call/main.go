// Command first-call prints the name, email and plan of the account that owns your API key.
//
// Run it with:
//
//	SQL_COCKPIT_API_KEY=your-key go run ./examples/first-call
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	sqlcockpit "github.com/sql-cockpit/sql-cockpit-go"
)

func main() {
	client, err := sqlcockpit.NewClient() // reads SQL_COCKPIT_API_KEY
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	account, err := client.Account.Get(ctx)
	if err != nil {
		log.Fatal(err)
	}

	plan := "no plan"
	if account.Plan != nil {
		plan = account.Plan.Name
	}
	fmt.Printf("Name:  %s\n", account.Name)
	fmt.Printf("Email: %s\n", account.Email)
	fmt.Printf("Plan:  %s\n", plan)
}
