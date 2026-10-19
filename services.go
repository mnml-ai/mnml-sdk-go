package mnml

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"iter"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// WaitOptions say how long Wait keeps reading a job. The zero value is the defaults.
type WaitOptions struct {
	// Interval is the least time between two reads. Default 3 s. Each read is held by the API until
	// the job settles (up to 90 s), so this only spaces reads the API answered early.
	Interval time.Duration
	// Timeout is when to give up. Default 10 minutes; the job keeps running.
	Timeout time.Duration
}

func (o *WaitOptions) or(interval, timeout time.Duration) WaitOptions {
	w := WaitOptions{Interval: interval, Timeout: timeout}
	if o != nil {
		if o.Interval > 0 {
			w.Interval = o.Interval
		}
		if o.Timeout > 0 {
			w.Timeout = o.Timeout
		}
	}
	return w
}

// RendersService makes renders from a source image or a prompt alone.
type RendersService struct{ c *Client }

// Create starts a render. With WithWait the API holds the answer while the render runs, and
// RenderStarted.Jobs carries the outputs, with no polling.
func (s *RendersService) Create(ctx context.Context, req *RenderRequest, opts ...RequestOption) (*RenderStarted, error) {
	o := collect(opts)
	path, err := waitQuery("/v2/renders", o.wait)
	if err != nil {
		return nil, err
	}
	var out RenderStarted
	if err := s.c.request(ctx, http.MethodPost, path, req, o.idempotencyKey, s.c.heldTimeout(o.wait), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// CreateAndWait starts the render and waits for every job it started (one per Count). The API
// holds the first answer while it runs, so most renders come back without a single poll.
func (s *RendersService) CreateAndWait(ctx context.Context, req *RenderRequest, wait *WaitOptions, opts ...RequestOption) ([]Job, error) {
	started, err := s.Create(ctx, req, append(opts, WithWait(MaxWaitSeconds))...)
	if err != nil {
		return nil, err
	}
	if len(started.Jobs) == len(started.IDs) {
		done := true
		for i := range started.Jobs {
			done = done && started.Jobs[i].Settled()
		}
		if done {
			return started.Jobs, nil
		}
	}
	return s.c.Jobs.WaitAll(ctx, started.IDs, wait)
}

// EditsService makes prompt edits and erases, over the whole image or a region.
type EditsService struct{ c *Client }

// Create starts an edit.
func (s *EditsService) Create(ctx context.Context, req *EditRequest, opts ...RequestOption) (*JobStarted, error) {
	return create(ctx, s.c, "/v2/edits", req, opts)
}

// CreateAndWait starts an edit and waits for it.
func (s *EditsService) CreateAndWait(ctx context.Context, req *EditRequest, wait *WaitOptions, opts ...RequestOption) (*Job, error) {
	return createAndWait(ctx, s.c, "/v2/edits", req, wait, opts)
}

// EnhancementsService upscales, enhances, removes backgrounds and outpaints.
type EnhancementsService struct{ c *Client }

// Create starts an enhancement.
func (s *EnhancementsService) Create(ctx context.Context, req *EnhancementRequest, opts ...RequestOption) (*JobStarted, error) {
	return create(ctx, s.c, "/v2/enhancements", req, opts)
}

// CreateAndWait starts an enhancement and waits for it.
func (s *EnhancementsService) CreateAndWait(ctx context.Context, req *EnhancementRequest, wait *WaitOptions, opts ...RequestOption) (*Job, error) {
	return createAndWait(ctx, s.c, "/v2/enhancements", req, wait, opts)
}

// VideosService makes video from a still image. A clip takes minutes.
type VideosService struct{ c *Client }

// Create starts a video.
func (s *VideosService) Create(ctx context.Context, req *VideoRequest, opts ...RequestOption) (*JobStarted, error) {
	return create(ctx, s.c, "/v2/videos", req, opts)
}

// CreateAndWait starts a video and waits for it: reads every 10 s, for up to 20 minutes, unless wait says otherwise.
func (s *VideosService) CreateAndWait(ctx context.Context, req *VideoRequest, wait *WaitOptions, opts ...RequestOption) (*Job, error) {
	w := wait.or(10*time.Second, 20*time.Minute)
	return createAndWait(ctx, s.c, "/v2/videos", req, &w, opts)
}

func create(ctx context.Context, c *Client, path string, req any, opts []RequestOption) (*JobStarted, error) {
	o := collect(opts)
	var out JobStarted
	if err := c.request(ctx, http.MethodPost, path, req, o.idempotencyKey, c.timeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func createAndWait(ctx context.Context, c *Client, path string, req any, wait *WaitOptions, opts []RequestOption) (*Job, error) {
	started, err := create(ctx, c, path, req, opts)
	if err != nil {
		return nil, err
	}
	return c.Jobs.Wait(ctx, started.ID, wait)
}

// JobsService reads, waits for, follows and cancels jobs.
type JobsService struct{ c *Client }

// Get reads a job. With WithWait the API holds the read and answers as soon as the job settles.
func (s *JobsService) Get(ctx context.Context, id string, opts ...RequestOption) (*Job, error) {
	o := collect(opts)
	path, err := waitQuery("/v2/jobs/"+url.PathEscape(id), o.wait)
	if err != nil {
		return nil, err
	}
	var out Job
	if err := s.c.request(ctx, http.MethodGet, path, nil, "", s.c.heldTimeout(o.wait), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Cancel cancels a job; one cancelled before it produced anything is refunded.
func (s *JobsService) Cancel(ctx context.Context, id string, opts ...RequestOption) (*JobCanceled, error) {
	o := collect(opts)
	var out JobCanceled
	if err := s.c.request(ctx, http.MethodPost, "/v2/jobs/"+url.PathEscape(id)+"/cancel", nil, o.idempotencyKey, s.c.timeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Wait reads the job until it succeeds, fails or is cancelled, and returns it. Each read asks the
// API to hold it until the job settles, so a render usually takes one read. When the time is up
// it returns a *TimeoutError; the job keeps running.
func (s *JobsService) Wait(ctx context.Context, id string, wait *WaitOptions) (*Job, error) {
	w := wait.or(3*time.Second, 10*time.Minute)
	deadline := time.Now().Add(w.Timeout)
	for {
		left := int(time.Until(deadline) / time.Second)
		began := time.Now()
		var opts []RequestOption
		if left >= 1 {
			opts = append(opts, WithWait(min(MaxWaitSeconds, left)))
		}
		job, err := s.Get(ctx, id, opts...)
		if err != nil {
			return nil, err
		}
		if job.Settled() {
			return job, nil
		}
		if time.Now().Add(w.Interval).After(deadline) {
			return nil, &TimeoutError{JobID: id}
		}
		// Answered early, still running: space the next read.
		if pause := w.Interval - time.Since(began); pause > 0 {
			if err := s.c.sleep(ctx, pause); err != nil {
				return nil, err
			}
		}
	}
}

// WaitAll is Wait for several jobs at once, such as every id a render with Count started. It
// returns the jobs in the order of ids, or the first error.
func (s *JobsService) WaitAll(ctx context.Context, ids []string, wait *WaitOptions) ([]Job, error) {
	jobs := make([]Job, len(ids))
	errs := make([]error, len(ids))
	var wg sync.WaitGroup
	for i, id := range ids {
		wg.Add(1)
		go func() {
			defer wg.Done()
			job, err := s.Wait(ctx, id, wait)
			if err != nil {
				errs[i] = err
				return
			}
			jobs[i] = *job
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

// Stream follows a job as it runs, over one connection (Server-Sent Events): each change of the
// job, then one last event ("done", "timeout" or "error"), after which the sequence ends.
// Breaking out of the loop, or cancelling ctx, hangs up; the job keeps running.
//
//	for ev, err := range client.Jobs.Stream(ctx, id) {
//		if err != nil { return err }
//		if ev.Type == "done" { fmt.Println(ev.Job.Outputs[0].URL) }
//	}
func (s *JobsService) Stream(ctx context.Context, id string) iter.Seq2[*JobStreamEvent, error] {
	return func(yield func(*JobStreamEvent, error) bool) {
		path := "/v2/jobs/" + url.PathEscape(id) + "/events"
		build := func(ctx context.Context) (*http.Request, error) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.c.baseURL+path, nil)
			if err != nil {
				return nil, err
			}
			s.c.headers(req, true, "text/event-stream")
			return req, nil
		}
		res, cancel, err := s.c.send(ctx, build, s.c.timeout, true)
		if err != nil {
			yield(nil, err)
			return
		}
		defer cancel()
		defer res.Body.Close()
		if res.StatusCode >= 300 {
			yield(nil, readError(res))
			return
		}
		scanner := bufio.NewScanner(res.Body)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		var block []string
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if line != "" {
				block = append(block, line)
				continue
			}
			ev, err := parseEvent(block)
			block = block[:0]
			if err != nil {
				yield(nil, err)
				return
			}
			if ev == nil {
				continue
			}
			if !yield(ev, nil) || ev.Final() {
				return
			}
		}
		if ctx.Err() != nil {
			yield(nil, ctx.Err())
			return
		}
		yield(nil, &Error{
			Code:      "STREAM_ENDED",
			Message:   "The job stream ended before the job settled. Read the job, or open the stream again.",
			Status:    res.StatusCode,
			RequestID: res.Header.Get("X-Request-Id"),
		})
	}
}

// parseEvent reads one Server-Sent Events block; nil for a comment (": ping") or an unknown event.
func parseEvent(lines []string) (*JobStreamEvent, error) {
	typ := "message"
	var data []string
	for _, line := range lines {
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			typ = value
		case "data":
			data = append(data, value)
		}
	}
	if len(data) == 0 {
		return nil, nil
	}
	payload := []byte(strings.Join(data, "\n"))
	switch typ {
	case "job", "done", "timeout":
		var job Job
		if err := json.Unmarshal(payload, &job); err != nil {
			return nil, err
		}
		return &JobStreamEvent{Type: typ, Job: &job}, nil
	case "error":
		var e JobError
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &JobStreamEvent{Type: typ, Error: &e}, nil
	}
	return nil, nil
}

// FilesService downloads a job's outputs. Their links are signed and expire, so download what you keep.
type FilesService struct{ c *Client }

// DownloadedFile is a job output's bytes.
type DownloadedFile struct {
	Data []byte
	// ContentType is image/jpeg, image/png, image/webp or video/mp4.
	ContentType string
}

// Download fetches the file behind an output's link. The signature in the link is the credential,
// so your key is not sent with it. An expired link is an *Error with NOT_FOUND: read the job again
// for a fresh one.
func (s *FilesService) Download(ctx context.Context, link string) (*DownloadedFile, error) {
	build := func(ctx context.Context) (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, link, nil)
		if err != nil {
			return nil, err
		}
		s.c.headers(req, false, "*/*")
		return req, nil
	}
	res, cancel, err := s.c.send(ctx, build, s.c.timeout, false)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return nil, readError(res)
	}
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return &DownloadedFile{Data: data, ContentType: res.Header.Get("Content-Type")}, nil
}

// AccountService reads the key's account.
type AccountService struct{ c *Client }

// Get reads the account: balance, tier and limits.
func (s *AccountService) Get(ctx context.Context) (*Account, error) {
	var out Account
	if err := s.c.request(ctx, http.MethodGet, "/v2/account", nil, "", s.c.timeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// EnginesService lists the engines and video models.
type EnginesService struct{ c *Client }

// List lists them, with their prices and capabilities.
func (s *EnginesService) List(ctx context.Context) (*EngineCatalog, error) {
	var out EngineCatalog
	if err := s.c.request(ctx, http.MethodGet, "/v2/engines", nil, "", s.c.timeout, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
