# mnml Go SDK

The official Go client for the [mnml API](https://developers.mnml.ai): architecture renders,
edits, enhancements and video, from your own service.

- Every endpoint of API v2: renders, edits, enhancements, video, jobs, account, engines
- Typed requests and answers, checked against the API's OpenAPI document
- Retries, timeouts and idempotency keys handled for you
- `CreateAndWait` and `Jobs.Wait` to wait for a job, `Jobs.Stream` to follow one in a `range`
  loop, `Files.Download` to keep its output
- Webhook signature verification
- Standard library only. Go 1.23 or newer

```bash
go get github.com/mnml-ai/mnml-sdk-go
```

## Quick start

Create an API key in the [console](https://developers.mnml.ai/console/keys), then:

```go
package main

import (
	"context"
	"fmt"
	"log"

	mnml "github.com/mnml-ai/mnml-sdk-go"
)

func main() {
	client, err := mnml.NewClient() // reads MNML_API_KEY
	if err != nil {
		log.Fatal(err)
	}
	img, err := mnml.ImageFromFile("massing.png") // or mnml.Image("https://…")
	if err != nil {
		log.Fatal(err)
	}
	jobs, err := client.Renders.CreateAndWait(context.Background(), &mnml.RenderRequest{
		Mode:   mnml.ModeExterior,
		Image:  img,
		Prompt: "Timber facade, late afternoon light, olive trees",
	}, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(jobs[0].Status, jobs[0].Outputs[0].URL)
}
```

Every call is paid from your prepaid API balance, in US dollars. Top it up on
[Billing](https://developers.mnml.ai/console/billing).

### Amounts are in dollars

Every amount the API answers with has a US-dollar field, a `float64` to four decimals (`0.2475`
is 24.75 cents):

| Where                                    | Dollar fields                                                        |
| ---------------------------------------- | -------------------------------------------------------------------- |
| a create (`RenderStarted`, `JobStarted`) | `ChargedUSD`                                                         |
| `Jobs.Get` (`Job`)                       | `ChargedUSD`, `RefundedUSD`                                          |
| `Jobs.Cancel` (`JobCanceled`)            | `RefundedUSD`                                                        |
| `Account.Get` (`Account`)                | `BalanceUSD`, `Key.DailyLimitUSD`, `Limits.DailyUSD` (nil: no limit) |
| `Engines.List` (`EngineCatalog`)         | `Prices.RenderUSD`, `Prices.EditUSD`, video `Prices[i].USD`          |
| the `credits.low` webhook (`CreditsLow`) | `BalanceUSD`, `ThresholdUSD`                                         |

```go
account, err := client.Account.Get(ctx)
fmt.Printf("$%.4f left\n", account.BalanceUSD)

catalog, err := client.Engines.List(ctx)
for _, e := range catalog.Engines {
	fmt.Printf("%s: $%.4f a render\n", e.ID, e.Prices.RenderUSD)
}
```

The credit fields beside them (`CreditsCharged`, `CreditsRefunded`, `Account.Credits`,
`DailyCreditLimit`, `DailyCredits`, `Prices.Render` and `Edit`, `VideoPrice.Credits`, and the
webhook's `Balance` and `Threshold`) are deprecated: `staticcheck` and gopls flag them, and a
later release removes them. Read the `USD` fields instead.

## Configuration

```go
client, err := mnml.NewClient(
	mnml.WithAPIKey(os.Getenv("MNML_API_KEY")), // the default
	mnml.WithMaxRetries(2),                     // retries for a 429, a 5xx or a dropped connection
	mnml.WithTimeout(60*time.Second),           // per request
	mnml.WithHTTPClient(&http.Client{}),        // your own transport, proxy or test server
)
```

A `*Client` is safe for concurrent use: make one and share it.

## Methods

| Method                                 | Endpoint                    | What it does                                        |
| -------------------------------------- | --------------------------- | --------------------------------------------------- |
| `Renders.Create(ctx, req)`             | `POST /v2/renders`          | Render from an image, or from a prompt alone        |
| `Edits.Create(ctx, req)`               | `POST /v2/edits`            | Edit or erase, over the whole image or a region     |
| `Enhancements.Create(ctx, req)`        | `POST /v2/enhancements`     | Upscale, enhance, remove the background, outpaint   |
| `Videos.Create(ctx, req)`              | `POST /v2/videos`           | Video from a still                                  |
| `Jobs.Get(ctx, id)`                    | `GET /v2/jobs/{id}`         | Read a job                                          |
| `Jobs.Cancel(ctx, id)`                 | `POST /v2/jobs/{id}/cancel` | Cancel a job (refunded if it had not produced yet)  |
| `Account.Get(ctx)`                     | `GET /v2/account`           | Balance, tier and limits for this key               |
| `Engines.List(ctx)`                    | `GET /v2/engines`           | Engines, video models, prices and capabilities      |
| `Files.Download(ctx, url)`             | `GET /v2/files/{id}`        | A job output's bytes, from its signed link          |
| `Jobs.Wait(ctx, id, wait)`             | `GET /v2/jobs/{id}`         | Read a job until it succeeds, fails or is cancelled |
| `Jobs.WaitAll(ctx, ids, wait)`         | `GET /v2/jobs/{id}`         | `Wait` for several jobs at once                     |
| `Jobs.Stream(ctx, id)`                 | `GET /v2/jobs/{id}/events`  | Follow a job as it changes, over one connection     |
| `<service>.CreateAndWait(ctx, req, w)` | the create, then the job    | Start a job and wait for it (a render: every job)   |

Creates take `mnml.WithIdempotencyKey(key)` and `mnml.WithWait(seconds)`. The client makes an
idempotency key per call when you pass none and keeps it across its own retries, so a retried
call is never charged twice. Optional numbers are pointers: `Count: mnml.Int(2)`.

### Edits and regions

```go
job, err := client.Edits.CreateAndWait(ctx, &mnml.EditRequest{
	JobID:  render.ID,
	Prompt: "Replace the lawn with a reflecting pool",
	Region: &mnml.Region{Box: &mnml.Box{X: 0, Y: 0.6, Width: 1, Height: 0.4}},
}, nil)
```

### Following a job

```go
for ev, err := range client.Jobs.Stream(ctx, started.ID) {
	if err != nil {
		return err
	}
	if ev.Type == "done" {
		fmt.Println(ev.Job.Outputs[0].URL)
	}
}
```

### Downloading outputs

Output links are signed and expire within hours. Keep what you need:

```go
file, err := client.Files.Download(ctx, job.Outputs[0].URL)
os.WriteFile("render.jpg", file.Data, 0o644)
```

## Errors

A refusal is a `*mnml.Error` with the API's `Code` (`INSUFFICIENT_CREDITS`, `VALIDATION_FAILED`,
`RATE_LIMITED` …), the HTTP `Status`, the `RequestID` to quote to support, and, on
`VALIDATION_FAILED`, each refused field in `Issues`. `Jobs.Wait` returns a `*mnml.TimeoutError`
when its time is up; the job keeps running. The codes:
[developers.mnml.ai/docs/errors](https://developers.mnml.ai/docs/errors).

```go
var apiErr *mnml.Error
if errors.As(err, &apiErr) && apiErr.Code == "INSUFFICIENT_CREDITS" {
	// top up
}
```

## Webhooks

Verify each delivery against your endpoint's `whsec_` secret, with the raw body:

```go
http.HandleFunc("/mnml", func(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	ev, err := mnml.VerifyWebhook(body, r.Header, secret, time.Now())
	if err != nil {
		http.Error(w, "bad signature", http.StatusBadRequest)
		return
	}
	switch ev.Type {
	case "job.succeeded":
		job, _ := ev.Job()
		fmt.Println(job.Outputs[0].URL)
	case "credits.low":
		low, _ := ev.CreditsLow()
		fmt.Printf("balance down to $%.4f: top up\n", low.BalanceUSD)
	}
})
```

## Versioning

This module follows [Semantic Versioning](https://semver.org/). The API is versioned in its path
(`/v2`); a release never changes which API version it calls without a major version. See
[CHANGELOG.md](./CHANGELOG.md).

## License

MIT
