package mnml

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A throwaway secret made fresh for every run: no secret, real or sample, lives in this repo.
var key = func() []byte {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}()

var secret = "whsec_" + base64.StdEncoding.EncodeToString(key)

var now = time.Unix(1_760_000_000, 0)

var eventBody = []byte(`{"type":"job.succeeded","timestamp":"2026-10-18T09:00:40Z","data":` + jobJSON + `}`)

func webhookHeaders(sig string, ts int64) http.Header {
	h := http.Header{}
	h.Set("Webhook-Id", "msg_1")
	h.Set("Webhook-Timestamp", strconv.FormatInt(ts, 10))
	h.Set("Webhook-Signature", sig)
	return h
}

func TestASignedDeliveryVerifiesAndReadsAsAJob(t *testing.T) {
	sig, _ := SignWebhook(secret, "msg_1", now.Unix(), eventBody)
	ev, err := VerifyWebhook(eventBody, webhookHeaders(sig, now.Unix()), secret, now)
	if err != nil {
		t.Fatal(err)
	}
	job, err := ev.Job()
	if err != nil || job.ID != "42" || ev.Type != "job.succeeded" {
		t.Fatalf("ev %+v job %+v err %v", ev, job, err)
	}
	if _, err := ev.CreditsLow(); err == nil {
		t.Fatal("a job event read as credits.low")
	}
}

func TestAnyOfSeveralSignaturesMayMatch(t *testing.T) {
	sig, _ := SignWebhook(secret, "msg_1", now.Unix(), eventBody)
	if _, err := VerifyWebhook(eventBody, webhookHeaders("v1,c29tZXRoaW5n "+sig, now.Unix()), secret, now); err != nil {
		t.Fatal(err)
	}
}

func TestAChangedBodyAnOldDeliveryAndMissingHeadersAreRefused(t *testing.T) {
	sig, _ := SignWebhook(secret, "msg_1", now.Unix(), eventBody)
	changed := []byte(strings.Replace(string(eventBody), "42", "43", 1))
	if _, err := VerifyWebhook(changed, webhookHeaders(sig, now.Unix()), secret, now); !errors.Is(err, ErrWebhookVerification) {
		t.Fatalf("changed body: %v", err)
	}
	old := now.Unix() - 600
	oldSig, _ := SignWebhook(secret, "msg_1", old, eventBody)
	if _, err := VerifyWebhook(eventBody, webhookHeaders(oldSig, old), secret, now); !errors.Is(err, ErrWebhookVerification) {
		t.Fatalf("old delivery: %v", err)
	}
	if _, err := VerifyWebhook(eventBody, http.Header{}, secret, now); !errors.Is(err, ErrWebhookVerification) {
		t.Fatalf("missing headers: %v", err)
	}
}

func TestTheSignatureIsTheStandardWebhooksHMAC(t *testing.T) {
	// HMAC-SHA256 of id.timestamp.body under the decoded secret; the whsec_ prefix is optional.
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(`msg_1.1614265330.{"test":1}`))
	want := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, s := range []string{secret, base64.StdEncoding.EncodeToString(key)} {
		sig, err := SignWebhook(s, "msg_1", 1614265330, []byte(`{"test":1}`))
		if err != nil || sig != want {
			t.Fatalf("sig %s err %v, want %s", sig, err, want)
		}
	}
}
