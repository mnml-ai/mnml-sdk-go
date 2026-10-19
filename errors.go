package mnml

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Error is an answer that was not a success: the API's own code, message and request id. The
// codes are listed at https://developers.mnml.ai/docs/errors.
//
//	var apiErr *mnml.Error
//	if errors.As(err, &apiErr) && apiErr.Code == "INSUFFICIENT_CREDITS" { … }
type Error struct {
	// Code is the API's error code, such as INSUFFICIENT_CREDITS or RATE_LIMITED.
	Code    string
	Message string
	// Status is the HTTP status.
	Status int
	// RequestID is the X-Request-Id to quote to support.
	RequestID string
	// Details says more about the refusal, when the API sent any (details.reason on a 429, say).
	Details json.RawMessage
	// Issues are, on VALIDATION_FAILED, each field that was refused.
	Issues []Issue
}

func (e *Error) Error() string { return fmt.Sprintf("mnml: %s (%d): %s", e.Code, e.Status, e.Message) }

// TimeoutError is returned by Jobs.Wait when its time is up before the job settled; the job keeps running.
type TimeoutError struct{ JobID string }

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("mnml: job %s did not finish in time; it is still running, read it again later", e.JobID)
}

func errorFrom(res *http.Response, env *envelope) *Error {
	e := &Error{
		Code:      "HTTP_ERROR",
		Message:   fmt.Sprintf("The API answered %d.", res.StatusCode),
		Status:    res.StatusCode,
		RequestID: res.Header.Get("X-Request-Id"),
	}
	if env != nil && !env.Success && env.Error != nil {
		if env.Error.Code != "" {
			e.Code = env.Error.Code
		}
		if env.Error.Message != "" {
			e.Message = env.Error.Message
		}
		if len(env.Error.Details) > 0 && string(env.Error.Details) != "null" {
			e.Details = env.Error.Details
		}
		e.Issues = env.Error.Issues
	}
	return e
}

func readError(res *http.Response) *Error {
	raw, _ := io.ReadAll(res.Body)
	var env envelope
	if json.Unmarshal(raw, &env) != nil {
		return errorFrom(res, nil)
	}
	return errorFrom(res, &env)
}
