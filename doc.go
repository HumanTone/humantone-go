// Package humantone is the official Go client for the HumanTone API
// (https://humantone.io). It rewrites AI-generated text into natural-sounding
// human prose and provides an AI Likelihood Indicator that scores how AI-like
// any text reads.
//
// The package exposes three endpoints via *Client: Humanize, Detect, and
// Account.Get. All methods accept a context.Context for cancellation and
// deadlines, and return either a typed result or an error wrapping a sentinel
// (such as ErrAuthentication or ErrInsufficientCredits) inside a *Error
// struct that carries the HTTP status, request ID, and other context.
//
// # Quickstart
//
//	client, err := humantone.NewClient(humantone.Config{
//	    APIKey: os.Getenv("HUMANTONE_API_KEY"),
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	result, err := client.Humanize(context.Background(), humantone.HumanizeRequest{
//	    Text: "Your AI-generated draft goes here. At least 30 words.",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	fmt.Println(result.Text)
//
// The package has no third-party dependencies — only the Go standard library.
// See https://humantone.io/docs/api/ for the underlying REST API documentation.
package humantone
