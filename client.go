// Package mnml is the official Go client for the mnml API (https://developers.mnml.ai):
// architecture renders, edits, enhancements and video.
//
//	client, err := mnml.NewClient() // reads MNML_API_KEY
//	img, _ := mnml.ImageFromFile("massing.png")
//	jobs, err := client.Renders.CreateAndWait(ctx, &mnml.RenderRequest{
//		Mode:   mnml.ModeExterior,
//		Image:  img,
//		Prompt: "Timber facade, late afternoon light",
//	}, nil)
//	fmt.Println(jobs[0].Outputs[0].URL)
//
// Every call returns the answer's data; a refusal is an *Error with the API's code.
package mnml

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Version is this SDK's version, sent in the User-Agent.
const Version = "0.2.0"

// MaxWaitSeconds is the longest the API holds an answer (wait), under Cloudflare's 100-second limit.
const MaxWaitSeconds = 90

// maxRetryAfter: a longer Retry-After (a daily limit runs to midnight UTC) is the caller's to handle.
const maxRetryAfter = 60 * time.Second

// Client is the mnml API client. Make one and reuse it; it is safe for concurrent use.
type Client struct {
	apiKey     string
	baseURL    string
	maxRetries int
	timeout    time.Duration
	http       *http.Client
	sleep      func(ctx context.Context, d time.Duration) error

	// Renders makes renders from a source image or a prompt alone.
	Renders *RendersService
	// Edits makes prompt edits and erases, over the whole image or a region.
	Edits *EditsService
	// Enhancements upscales, enhances, removes backgrounds and outpaints.
	Enhancements *EnhancementsService
	// Videos makes video from a still image.
	Videos *VideosService
	// Jobs reads, waits for, follows and cancels jobs.
	Jobs *JobsService
	// Files downloads a job's outputs.
	Files *FilesService
	// Account reads the key's account: balance, tier and limits.
	Account *AccountService
	// Engines lists the engines and video models, with their prices and capabilities.
	Engines *EnginesService
}

// Option configures a Client.
type Option func(*Client)

// WithAPIKey sets the API key (mk_live_…). The default is the MNML_API_KEY environment variable.
func WithAPIKey(key string) Option { return func(c *Client) { c.apiKey = key } }

// WithBaseURL sets the API's address. The default is https://api.mnml.ai.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(url, "/") }
}

// WithMaxRetries sets the retries for a 429, a 5xx or a dropped connection. The default is 2.
func WithMaxRetries(n int) Option { return func(c *Client) { c.maxRetries = max(0, n) } }

// WithTimeout sets the time per request. The default is 60 seconds; a held read (wait) adds its own time.
func WithTimeout(d time.Duration) Option { return func(c *Client) { c.timeout = d } }

// WithHTTPClient sets the *http.Client to send with (a proxy, a test transport). Its own Timeout
// should be zero: the client times each attempt itself, and a held read can last 150 seconds.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// NewClient makes a client. It fails only when there is no API key.
func NewClient(opts ...Option) (*Client, error) {
	c := &Client{
		apiKey:     os.Getenv("MNML_API_KEY"),
		baseURL:    "https://api.mnml.ai",
		maxRetries: 2,
		timeout:    60 * time.Second,
		http:       &http.Client{},
		sleep:      sleepCtx,
	}
	for _, o := range opts {
		o(c)
	}
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, errors.New("mnml: pass WithAPIKey, or set the MNML_API_KEY environment variable")
	}
	c.Renders = &RendersService{c}
	c.Edits = &EditsService{c}
	c.Enhancements = &EnhancementsService{c}
	c.Videos = &VideosService{c}
	c.Jobs = &JobsService{c}
	c.Files = &FilesService{c}
	c.Account = &AccountService{c}
	c.Engines = &EnginesService{c}
	return c, nil
}

// RequestOption sets something on one create call.
type RequestOption func(*requestOptions)

type requestOptions struct {
	idempotencyKey string
	wait           int
}

// WithIdempotencyKey makes a retry of a spending call return the first answer instead of charging
// again. The client makes one per call when you pass none, and keeps it across its own retries.
func WithIdempotencyKey(key string) RequestOption {
	return func(o *requestOptions) { o.idempotencyKey = key }
}

// WithWait asks the API to hold the answer up to this many seconds (1–90) while the job runs.
func WithWait(seconds int) RequestOption { return func(o *requestOptions) { o.wait = seconds } }

func collect(opts []RequestOption) requestOptions {
	var o requestOptions
	for _, f := range opts {
		f(&o)
	}
	return o
}

func waitQuery(path string, wait int) (string, error) {
	if wait == 0 {
		return path, nil
	}
	if wait < 1 || wait > MaxWaitSeconds {
		return "", fmt.Errorf("mnml: wait is 1 to %d seconds, not %d", MaxWaitSeconds, wait)
	}
	return path + "?wait=" + strconv.Itoa(wait), nil
}

func (c *Client) heldTimeout(wait int) time.Duration {
	return c.timeout + time.Duration(wait)*time.Second
}

// request is a JSON call, answered with the envelope's data decoded into out, or an *Error.
func (c *Client) request(ctx context.Context, method, path string, body any, idempotencyKey string, timeout time.Duration, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	key := ""
	if method == http.MethodPost {
		// One key per call, kept across the client's own retries, so a retry never charges twice.
		key = idempotencyKey
		if key == "" {
			key = newUUID()
		}
	}
	build := func(ctx context.Context) (*http.Request, error) {
		var r io.Reader
		if payload != nil {
			r = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, r)
		if err != nil {
			return nil, err
		}
		c.headers(req, true, "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		return req, nil
	}
	res, cancel, err := c.send(ctx, build, timeout, false)
	if err != nil {
		return err
	}
	defer cancel()
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	var env envelope
	if json.Unmarshal(raw, &env) == nil && env.Success && res.StatusCode < 300 && len(env.Data) > 0 {
		return json.Unmarshal(env.Data, out)
	}
	return errorFrom(res, &env)
}

func (c *Client) headers(req *http.Request, authorized bool, accept string) {
	req.Header.Set("User-Agent", "mnml-sdk-go/"+Version)
	req.Header.Set("Accept", accept)
	if authorized {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// send is one call with the client's retries: a 429, a 5xx or a dropped connection is sent again
// (the same headers, so the same idempotency key) after Retry-After or a backoff. The last answer
// is returned, whatever it is, with the cancel of its attempt's context: call it once the body is
// read. A stream's attempt has no time limit once its headers arrive.
func (c *Client) send(ctx context.Context, build func(context.Context) (*http.Request, error), timeout time.Duration, stream bool) (*http.Response, context.CancelFunc, error) {
	for attempt := 0; ; attempt++ {
		actx, cancel := context.WithCancel(ctx)
		timer := time.AfterFunc(timeout, cancel)
		req, err := build(actx)
		if err != nil {
			timer.Stop()
			cancel()
			return nil, nil, err
		}
		res, err := c.http.Do(req)
		timedOut := !timer.Stop()
		if err != nil {
			cancel()
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if attempt >= c.maxRetries {
				if timedOut {
					return nil, nil, fmt.Errorf("mnml: the request did not answer within %s: %w", timeout, err)
				}
				return nil, nil, err
			}
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return nil, nil, err
			}
			continue
		}
		if retryable(res.StatusCode) && attempt < c.maxRetries {
			wait, ok := retryAfter(res)
			if !ok {
				wait = backoff(attempt)
			}
			if wait <= maxRetryAfter {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
				cancel()
				if err := c.sleep(ctx, wait); err != nil {
					return nil, nil, err
				}
				continue
			}
		}
		if !stream {
			// The body is read under the same time limit as the headers.
			timer.Reset(timeout)
			stop := cancel
			cancel = func() { timer.Stop(); stop() }
		}
		return res, cancel, nil
	}
}

func retryable(status int) bool {
	return status == 429 || status == 500 || status == 502 || status == 503 || status == 504
}

func retryAfter(res *http.Response) (time.Duration, bool) {
	h := res.Header.Get("Retry-After")
	if h == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(h); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(h); err == nil {
		if d := time.Until(at); d > 0 {
			return d, true
		}
	}
	return 0, false
}

// backoff is 0.5 s, 1 s, 2 s … with a little jitter.
func backoff(attempt int) time.Duration {
	ms := 500*math.Pow(2, float64(attempt)) + float64(mrand.IntN(250))
	return time.Duration(ms) * time.Millisecond
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
