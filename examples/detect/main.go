// detect is a small example program that scores a sample text on the AI
// Likelihood Indicator. Run it with HUMANTONE_API_KEY set in the environment.
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

	result, err := client.Detect(context.Background(),
		"Some text whose AI likelihood you would like to score for analysis purposes today.")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("AI likelihood: %d/100\n", result.AIScore)
}
