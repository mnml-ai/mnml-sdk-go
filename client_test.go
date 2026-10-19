package mnml

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const jobJSON = `{"id":"42","status":"succeeded","kind":"render","engine":"v4.6-ultra",` +
	`"outputs":[{"url":"https://api.test/v2/files/42?exp=1&sig=x","media":"image","expires_at":"2026-10-18T10:00:00Z"}],` +
	`"credits_charged":30,"credits_refunded":0,"error":null,"created_at":"2026-10-18T09:00:00Z","completed_at":"2026-10-18T09:00:40Z"}`

func jobWith(status string) string {
	return strings.Replace(jobJSON, `"succeeded"`, `"`+status+`"`, 1)
}

type seen struct {
	method, url string
	header      http.Header
	body        string
}

// fakeAPI answers from a queue and records every request it saw.
type fakeAPI struct {
	mu       sync.Mutex
	answers  []func(http.ResponseWriter)
	requests []seen
	delays   []time.Duration
	srv      *httptest.Server
}

func newFake(t *testing.T) *fakeAPI {
	f := &fakeAPI{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.requests = append(f.requests, seen{r.Method, r.URL.String(), r.Header.Clone(), string(body)})
		if len(f.answers) == 0 {
			f.mu.Unlock()
			t.Errorf("no answer queued for %s %s", r.Method, r.URL)
			w.WriteHeader(599)
			return
		}
		next := f.answers[0]
		f.answers = f.answers[1:]
		f.mu.Unlock()
		w.Header().Set("X-Request-Id", "req_test")
		next(w)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeAPI) answer(status int, body string, headers ...string) *fakeAPI {
	f.answers = append(f.answers, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		for i := 0; i+1 < len(headers); i += 2 {
			w.Header().Set(headers[i], headers[i+1])
		}
		w.WriteHeader(status)
		io.WriteString(w, body)
	})
	return f
}

func (f *fakeAPI) ok(data string) *fakeAPI {
	return f.answer(200, `{"success":true,"data":`+data+`}`)
}

func (f *fakeAPI) fail(status int, code, message string, headers ...string) *fakeAPI {
	return f.answer(status, `{"success":false,"error":{"code":"`+code+`","message":"`+message+`"}}`, headers...)
}

func (f *fakeAPI) client(t *testing.T, opts ...Option) *Client {
	c, err := NewClient(append([]Option{WithAPIKey("test-key"), WithBaseURL(f.srv.URL)}, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	c.sleep = func(_ context.Context, d time.Duration) error {
		f.delays = append(f.delays, d)
		return nil
	}
	return c
}

var png = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3}

func TestRenderPostsItsBodyWithKeyIdempotencyKeyAndDataURI(t *testing.T) {
	f := newFake(t).ok(`{"id":"42","ids":["42"],"status":"queued","credits_charged":30,"replayed":false,"notes":[]}`)
	c := f.client(t)

	started, err := c.Renders.Create(context.Background(), &RenderRequest{
		Prompt:     "Timber facade, dusk",
		Mode:       ModeExterior,
		Image:      ImageFromBytes(png),
		References: []Reference{{Image: "https://x.test/ref.jpg", Mode: ReferenceMaterial}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if started.ID != "42" || started.Status != StatusQueued {
		t.Fatalf("started = %+v", started)
	}
	r := f.requests[0]
	if r.method != "POST" || r.url != "/v2/renders" {
		t.Fatalf("%s %s", r.method, r.url)
	}
	if r.header.Get("Authorization") != "Bearer test-key" || !strings.HasPrefix(r.header.Get("User-Agent"), "mnml-sdk-go/") {
		t.Fatalf("headers %v", r.header)
	}
	if len(r.header.Get("Idempotency-Key")) != 36 {
		t.Fatalf("idempotency key %q", r.header.Get("Idempotency-Key"))
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(r.body), &body)
	if !strings.HasPrefix(body["image"].(string), "data:image/png;base64,") {
		t.Fatalf("image %v", body["image"])
	}
	if _, sent := body["engine"]; sent {
		t.Fatal("an unset field was sent")
	}
	if _, sent := body["seed"]; sent {
		t.Fatal("an unset seed was sent")
	}
}

func TestRetryAfter503SendsTheSameIdempotencyKey(t *testing.T) {
	f := newFake(t).
		fail(503, "SERVICE_UNAVAILABLE", "Busy.", "Retry-After", "5").
		ok(`{"id":"7","status":"queued","credits_charged":2,"replayed":false,"notes":[]}`)
	c := f.client(t)

	if _, err := c.Edits.Create(context.Background(), &EditRequest{Prompt: "Add trees", JobID: "41"}); err != nil {
		t.Fatal(err)
	}
	if len(f.requests) != 2 || f.requests[0].header.Get("Idempotency-Key") != f.requests[1].header.Get("Idempotency-Key") {
		t.Fatal("the retry did not carry the same key")
	}
	if len(f.delays) != 1 || f.delays[0] != 5*time.Second {
		t.Fatalf("delays %v", f.delays)
	}
}

func TestALongRetryAfterIsTheCallersToHandle(t *testing.T) {
	f := newFake(t).fail(429, "RATE_LIMITED", "Daily limit reached.", "Retry-After", "3600")
	c := f.client(t)

	_, err := c.Account.Get(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "RATE_LIMITED" || apiErr.Status != 429 || apiErr.RequestID != "req_test" {
		t.Fatalf("err = %v", err)
	}
	if len(f.requests) != 1 || len(f.delays) != 0 {
		t.Fatal("a 3600 s Retry-After was retried")
	}
}

func TestARefusalCarriesCodeMessageAndIssues(t *testing.T) {
	f := newFake(t).answer(400, `{"success":false,"error":{"code":"VALIDATION_FAILED","message":"Check the fields.",`+
		`"issues":[{"path":"prompt","message":"Required."}],"details":{"reason":"x"}}}`)
	c := f.client(t)

	_, err := c.Renders.Create(context.Background(), &RenderRequest{})
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "VALIDATION_FAILED" || apiErr.Issues[0].Path != "prompt" {
		t.Fatalf("err = %v", err)
	}
	if string(apiErr.Details) != `{"reason":"x"}` {
		t.Fatalf("details %s", apiErr.Details)
	}
}

func TestAnAnswerThatIsNotTheEnvelopeIsAnHTTPError(t *testing.T) {
	f := newFake(t).answer(502, "<html>Bad gateway</html>")
	c := f.client(t, WithMaxRetries(0))

	_, err := c.Engines.List(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) || apiErr.Code != "HTTP_ERROR" || apiErr.Status != 502 {
		t.Fatalf("err = %v", err)
	}
}

func TestWaitHoldsEachReadAndReturnsTheSettledJob(t *testing.T) {
	f := newFake(t).ok(jobWith("processing")).ok(jobJSON)
	c := f.client(t)

	job, err := c.Jobs.Wait(context.Background(), "42", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !job.Settled() || job.Outputs[0].URL != "https://api.test/v2/files/42?exp=1&sig=x" {
		t.Fatalf("job %+v", job)
	}
	for _, r := range f.requests {
		if !strings.HasPrefix(r.url, "/v2/jobs/42?wait=") {
			t.Fatalf("read %s", r.url)
		}
	}
}

func TestWaitGivesUpAfterItsTimeout(t *testing.T) {
	f := newFake(t).ok(jobWith("processing"))
	c := f.client(t)

	_, err := c.Jobs.Wait(context.Background(), "42", &WaitOptions{Timeout: 10 * time.Millisecond})
	var te *TimeoutError
	if !errors.As(err, &te) || te.JobID != "42" {
		t.Fatalf("err = %v", err)
	}
}

func TestARenderThatSettledInTheHeldAnswerNeedsNoPoll(t *testing.T) {
	f := newFake(t).ok(`{"id":"42","ids":["42"],"status":"succeeded","credits_charged":30,"replayed":false,"notes":[],"jobs":[` + jobJSON + `]}`)
	c := f.client(t)

	jobs, err := c.Renders.CreateAndWait(context.Background(), &RenderRequest{Prompt: "A", Mode: ModeTextToRender}, nil)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("jobs %v err %v", jobs, err)
	}
	if len(f.requests) != 1 || f.requests[0].url != "/v2/renders?wait=90" {
		t.Fatalf("requests %+v", f.requests)
	}
}

func TestAWaitOutside1To90IsRefusedBeforeAnyCall(t *testing.T) {
	f := newFake(t)
	c := f.client(t)
	if _, err := c.Jobs.Get(context.Background(), "42", WithWait(91)); err == nil {
		t.Fatal("wait 91 was accepted")
	}
	if len(f.requests) != 0 {
		t.Fatal("a call was made")
	}
}

func TestADownloadSendsNoKey(t *testing.T) {
	f := newFake(t)
	f.answers = append(f.answers, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "image/jpeg")
		io.WriteString(w, "JPEGBYTES")
	})
	c := f.client(t)

	file, err := c.Files.Download(context.Background(), f.srv.URL+"/v2/files/42?exp=1&sig=x")
	if err != nil || string(file.Data) != "JPEGBYTES" || file.ContentType != "image/jpeg" {
		t.Fatalf("file %+v err %v", file, err)
	}
	if f.requests[0].header.Get("Authorization") != "" {
		t.Fatal("the key was sent with a download")
	}
}

func TestAStreamYieldsEachChangeThenTheLastEvent(t *testing.T) {
	f := newFake(t)
	f.answers = append(f.answers, func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, ": ping\n\nevent: job\ndata: "+jobWith("processing")+"\n\nevent: done\ndata: "+jobJSON+"\n\nevent: job\ndata: {\"never\":1}\n\n")
	})
	c := f.client(t)

	var types []string
	for ev, err := range c.Jobs.Stream(context.Background(), "42") {
		if err != nil {
			t.Fatal(err)
		}
		types = append(types, ev.Type)
	}
	if strings.Join(types, ",") != "job,done" {
		t.Fatalf("types %v", types)
	}
	if f.requests[0].header.Get("Accept") != "text/event-stream" {
		t.Fatal("not asked as an event stream")
	}
}

func TestAStreamThatEndsEarlySaysSo(t *testing.T) {
	f := newFake(t)
	f.answers = append(f.answers, func(w http.ResponseWriter) {
		io.WriteString(w, "event: job\ndata: "+jobWith("processing")+"\n\n")
	})
	c := f.client(t)

	var last error
	for _, err := range c.Jobs.Stream(context.Background(), "42") {
		last = err
	}
	var apiErr *Error
	if !errors.As(last, &apiErr) || apiErr.Code != "STREAM_ENDED" {
		t.Fatalf("last %v", last)
	}
}

func TestImagesAreLabelledByTheirFirstBytes(t *testing.T) {
	if !strings.HasPrefix(string(ImageFromBytes(png)), "data:image/png;base64,") {
		t.Fatal("png")
	}
	if !strings.HasPrefix(string(ImageFromBytes([]byte{0xFF, 0xD8, 0xFF, 0})), "data:image/jpeg;base64,") {
		t.Fatal("jpeg")
	}
}

func TestARegionIsSentAsTheAPIReadsIt(t *testing.T) {
	b, _ := json.Marshal(Region{Polygon: [][2]float64{{0.1, 0.2}, {0.5, 0.2}, {0.3, 0.6}}})
	if string(b) != `{"polygon":[[0.1,0.2],[0.5,0.2],[0.3,0.6]]}` {
		t.Fatal(string(b))
	}
	b, _ = json.Marshal(Region{Box: &Box{X: 0, Y: 0.5, Width: 1, Height: 0.5}})
	if string(b) != `{"box":{"x":0,"y":0.5,"width":1,"height":0.5}}` {
		t.Fatal(string(b))
	}
}

func TestAClientNeedsAKey(t *testing.T) {
	t.Setenv("MNML_API_KEY", "")
	if _, err := NewClient(); err == nil {
		t.Fatal("no key was accepted")
	}
}
