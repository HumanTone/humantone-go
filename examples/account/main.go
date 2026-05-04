// account is a small example program that fetches the account info: plan,
// remaining credits, and subscription expiry. Run it with HUMANTONE_API_KEY
// set in the environment.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/humantone/humantone-go"
)

func main() {
	client, err := humantone.NewClient(humantone.Config{
		APIKey: os.Getenv("HUMANTONE_API_KEY"),
	})
	if err != nil {
		log.Fatal(err)
	}

	info, err := client.Account.Get(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Plan: %s\n", info.Plan.Name)
	fmt.Printf("Credits: %d\n", info.Credits.Total)
	fmt.Printf("Word limit: %d\n", info.Plan.MaxWords)

	if info.Subscription.ExpiresAt != nil {
		fmt.Printf("Expires: %s\n", info.Subscription.ExpiresAt.Format("2006-01-02"))
	}
}
