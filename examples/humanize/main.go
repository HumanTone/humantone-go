// humanize is a small example program that takes the text in HUMANTONE_API_KEY
// account credits and rewrites a fixed sample using the standard humanization
// level. Run it with HUMANTONE_API_KEY set in the environment.
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

	result, err := client.Humanize(context.Background(), humantone.HumanizeRequest{
		Text: "Artificial intelligence has transformed how content teams work. " +
			"Writers now use AI tools to draft, edit, and polish their content. " +
			"This shift continues to reshape professional writing workflows " +
			"across many industries.",
		Level: humantone.LevelStandard,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Text)
	fmt.Printf("Credits used: %d\n", result.CreditsUsed)
}
