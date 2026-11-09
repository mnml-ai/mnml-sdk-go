package mnml

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// WebhookTolerance is how far a delivery's timestamp may be from now.
const WebhookTolerance = 5 * time.Minute

// ErrWebhookVerification is wrapped by every error VerifyWebhook returns for a delivery that does not hold.
var ErrWebhookVerification = errors.New("mnml: webhook verification failed")

// WebhookEvent is a webhook message. Type is "job.succeeded", "job.failed", "job.canceled",
// "credits.low" or "webhook.test"; new types may be added, so ignore one you do not handle.
type WebhookEvent struct {
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Data      json.RawMessage `json:"data"`
}

// Job is, for a job.* event, the job as Jobs.Get returns it.
func (e *WebhookEvent) Job() (*Job, error) {
	if !strings.HasPrefix(e.Type, "job.") {
		return nil, errors.New("mnml: not a job event: " + e.Type)
	}
	var j Job
	return &j, json.Unmarshal(e.Data, &j)
}

// CreditsLow is, for credits.low, the balance and the threshold it fell under.
func (e *WebhookEvent) CreditsLow() (*CreditsLow, error) {
	if e.Type != "credits.low" {
		return nil, errors.New("mnml: not a credits.low event: " + e.Type)
	}
	var c CreditsLow
	return &c, json.Unmarshal(e.Data, &c)
}

// CreditsLow is sent when the balance fell under the threshold (at most once a day).
type CreditsLow struct {
	// BalanceUSD is the balance now, in US dollars (four decimals).
	BalanceUSD float64 `json:"balance_usd"`
	// ThresholdUSD is the line it fell under, in US dollars.
	ThresholdUSD float64 `json:"threshold_usd"`
	// Balance is the balance now, in API credits.
	//
	// Deprecated: read BalanceUSD. The same amount in API credits ($0.0099 each); a later release removes it.
	Balance int64 `json:"balance"`
	// Threshold is the line it fell under, in API credits.
	//
	// Deprecated: read ThresholdUSD. The same amount in API credits ($0.0099 each); a later release removes it.
	Threshold int64 `json:"threshold"`
}

// VerifyWebhook checks a delivery: the Standard Webhooks scheme the API signs with (webhook-id,
// webhook-timestamp, webhook-signature, HMAC-SHA256 of id.timestamp.body under your whsec_
// secret). Pass the RAW body, the exact bytes received, or the check fails. now is the time to
// check the timestamp against; pass time.Now().
//
//	body, _ := io.ReadAll(r.Body)
//	ev, err := mnml.VerifyWebhook(body, r.Header, secret, time.Now())
func VerifyWebhook(body []byte, header http.Header, secret string, now time.Time) (*WebhookEvent, error) {
	id := header.Get("webhook-id")
	ts := header.Get("webhook-timestamp")
	sig := header.Get("webhook-signature")
	if id == "" || ts == "" || sig == "" {
		return nil, wrap("missing webhook headers")
	}
	timestamp, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || abs(now.Unix()-timestamp) > int64(WebhookTolerance/time.Second) {
		return nil, wrap("webhook timestamp is too old or in the future")
	}
	expected, err := SignWebhook(secret, id, timestamp, body)
	if err != nil {
		return nil, err
	}
	ok := false
	// The header may carry several space-separated signatures (during a rotation).
	for _, candidate := range strings.Split(sig, " ") {
		ok = hmac.Equal([]byte(candidate), []byte(expected)) || ok
	}
	if !ok {
		return nil, wrap("webhook signature does not match")
	}
	var ev WebhookEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// SignWebhook is the webhook-signature value for a delivery: "v1,<base64>".
func SignWebhook(secret, id string, timestamp int64, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil {
		return "", errors.New("mnml: the webhook secret is not base64 after whsec_")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(id + "." + strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func wrap(msg string) error { return errors.Join(ErrWebhookVerification, errors.New(msg)) }

func abs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
