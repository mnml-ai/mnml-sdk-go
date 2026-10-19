package mnml

import (
	"encoding/json"
	"time"
)

// Render modes.
const (
	ModeExterior     = "exterior"
	ModeInterior     = "interior"
	ModeMasterplan   = "masterplan"
	ModePlan         = "plan"
	ModeLandscape    = "landscape"
	ModeProduct      = "product"
	ModeTextToRender = "text-to-render"
)

// Aspect ratios.
const (
	AspectAuto = "auto"
	Aspect1x1  = "1:1"
	Aspect3x2  = "3:2"
	Aspect4x3  = "4:3"
	Aspect5x4  = "5:4"
	Aspect16x9 = "16:9"
	Aspect21x9 = "21:9"
	Aspect2x3  = "2:3"
	Aspect3x4  = "3:4"
	Aspect4x5  = "4:5"
	Aspect9x16 = "9:16"
)

// What a reference image lends the render.
const (
	ReferenceAuto       = "auto"
	ReferenceStyle      = "style"
	ReferenceMaterial   = "material"
	ReferenceAtmosphere = "atmosphere"
	ReferenceColor      = "color"
	ReferenceGeometry   = "geometry"
)

// Enhancement kinds.
const (
	EnhanceUpscale  = "upscale"
	EnhanceEnhance  = "enhance"
	EnhanceBGRemove = "bg-remove"
	EnhanceOutpaint = "outpaint"
)

// Job statuses. Succeeded, failed and canceled are final.
const (
	StatusQueued     = "queued"
	StatusProcessing = "processing"
	StatusSucceeded  = "succeeded"
	StatusFailed     = "failed"
	StatusCanceled   = "canceled"
)

// Int returns a pointer to v, for the optional numeric fields of a request.
func Int(v int) *int { return &v }

// Int64 returns a pointer to v.
func Int64(v int64) *int64 { return &v }

// Bool returns a pointer to v.
func Bool(v bool) *bool { return &v }

// Reference is a reference image and what to take from it.
type Reference struct {
	// Image is the reference itself, or leave it empty and set JobID.
	Image Image `json:"image,omitempty"`
	// JobID is a finished job of yours, whose output is the reference.
	JobID string `json:"job_id,omitempty"`
	// Mode is one of the Reference* constants. Default "auto".
	Mode string `json:"mode,omitempty"`
}

// Source is where an image comes from: the image itself, or a finished job of yours.
type Source struct {
	Image Image  `json:"image,omitempty"`
	JobID string `json:"job_id,omitempty"`
}

// Region is an area of the image, in fractions (0–1) from its top-left corner: a Box or a Polygon.
type Region struct {
	Box *Box `json:"box,omitempty"`
	// Polygon is an outline of 3 to 64 [x, y] points.
	Polygon [][2]float64 `json:"polygon,omitempty"`
}

// Box is a rectangle, in fractions (0–1) of the image.
type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// RenderRequest renders from a source image, or from a prompt alone (Mode "text-to-render").
type RenderRequest struct {
	// Prompt is what to render, up to 4,000 characters. Required.
	Prompt string `json:"prompt"`
	// Engine is an engine id or name ("v4.6-ultra", "v4.6"); Engines.List lists them.
	Engine string `json:"engine,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Image  Image  `json:"image,omitempty"`
	// Deprecated: use Image.
	ImageURL string `json:"image_url,omitempty"`
	JobID    string `json:"job_id,omitempty"`
	// References are up to 15 reference images; how many an engine takes is its MaxReferences.
	References []Reference `json:"references,omitempty"`
	// Settings are Studio's settings for this engine and mode, name to value. Omit for Auto.
	Settings    map[string]string `json:"settings,omitempty"`
	AspectRatio string            `json:"aspect_ratio,omitempty"`
	// Count is 1–4 variations of this brief, each its own job and charge.
	Count *int   `json:"count,omitempty"`
	Seed  *int64 `json:"seed,omitempty"`
	// WebhookURL is a public https:// URL to send this job's settle event to.
	WebhookURL string `json:"webhook_url,omitempty"`
}

// EditRequest is a prompt edit or an erase, over the whole image or a region.
type EditRequest struct {
	// Kind is "edit" (the default), which changes what the prompt says, or "erase", which removes what the region covers.
	Kind   string `json:"kind,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	Image  Image  `json:"image,omitempty"`
	// Deprecated: use Image.
	ImageURL   string      `json:"image_url,omitempty"`
	JobID      string      `json:"job_id,omitempty"`
	Engine     string      `json:"engine,omitempty"`
	Mode       string      `json:"mode,omitempty"`
	References []Reference `json:"references,omitempty"`
	Region     *Region     `json:"region,omitempty"`
	// Mask is a PNG the size of the image, white where to change, black where to keep.
	Mask       Image  `json:"mask,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// EnhancementRequest is an upscale, enhance, background removal or outpaint.
type EnhancementRequest struct {
	// Kind is one of the Enhance* constants. Required.
	Kind  string `json:"kind"`
	Image Image  `json:"image,omitempty"`
	// Deprecated: use Image.
	ImageURL string `json:"image_url,omitempty"`
	JobID    string `json:"job_id,omitempty"`
	// Creativity is 0–100, for "enhance".
	Creativity *int   `json:"creativity,omitempty"`
	Prompt     string `json:"prompt,omitempty"`
	// AspectRatio is the frame to extend to, for "outpaint".
	AspectRatio string `json:"aspect_ratio,omitempty"`
	WebhookURL  string `json:"webhook_url,omitempty"`
}

// VideoRequest makes a video from a still image.
type VideoRequest struct {
	Image Image `json:"image,omitempty"`
	// Deprecated: use Image.
	ImageURL string `json:"image_url,omitempty"`
	JobID    string `json:"job_id,omitempty"`
	// Model is a video model id or name. Default "v2.0-flash".
	Model string `json:"model,omitempty"`
	// DurationSeconds is 10 or 15 on the v2.0 models (default 10); v1.1 has one fixed length.
	DurationSeconds *int `json:"duration_seconds,omitempty"`
	// CameraMovement is "static" (the default), "auto", or one or two moves joined by a comma ("dolly-in,tilt-up").
	CameraMovement string `json:"camera_movement,omitempty"`
	// Motion is "subtle" (the default), "balanced", "dynamic" or "auto".
	Motion string `json:"motion,omitempty"`
	Prompt string `json:"prompt,omitempty"`
	// EndFrame is a last frame to move towards.
	EndFrame *Source `json:"end_frame,omitempty"`
	// Cinematic is v1.1 only: the Cinematic variant, which needs EndFrame.
	Cinematic  *bool  `json:"cinematic,omitempty"`
	WebhookURL string `json:"webhook_url,omitempty"`
}

// JobStarted is what a create call answers: the job it started.
type JobStarted struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	CreditsCharged int64  `json:"credits_charged"`
	// Replayed is true when this answers an earlier identical request: nothing new was charged.
	Replayed bool `json:"replayed"`
	// Notes say what the API changed or dropped from the request.
	Notes []string `json:"notes"`
}

// RenderStarted is what a render answers: every job it started (one per Count).
type RenderStarted struct {
	ID             string   `json:"id"`
	IDs            []string `json:"ids"`
	Status         string   `json:"status"`
	CreditsCharged int64    `json:"credits_charged"`
	Replayed       bool     `json:"replayed"`
	Notes          []string `json:"notes"`
	// Jobs is set only with a wait: every job as it stood when the wait ended, outputs included.
	Jobs []Job `json:"jobs,omitempty"`
}

// Job is a job: its status and, once it succeeded, its outputs.
type Job struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	// Kind is what the job does: "render", "edit", "upscale", "video" and so on.
	Kind   string  `json:"kind"`
	Engine *string `json:"engine"`
	// Outputs are signed links that expire: download what you keep.
	Outputs         []JobOutput `json:"outputs"`
	CreditsCharged  int64       `json:"credits_charged"`
	CreditsRefunded int64       `json:"credits_refunded"`
	Error           *JobError   `json:"error"`
	CreatedAt       time.Time   `json:"created_at"`
	CompletedAt     *time.Time  `json:"completed_at"`
}

// Settled reports whether the job is done: succeeded, failed or canceled.
func (j *Job) Settled() bool { return isSettled(j.Status) }

func isSettled(status string) bool {
	return status == StatusSucceeded || status == StatusFailed || status == StatusCanceled
}

// JobOutput is one result of a job.
type JobOutput struct {
	// URL is a signed link, served by the API. Download it before ExpiresAt.
	URL string `json:"url"`
	// Media is "image" or "video".
	Media     string    `json:"media"`
	ExpiresAt time.Time `json:"expires_at"`
}

// JobError says why a job failed.
type JobError struct {
	// Code is UNSAFE_CONTENT, NO_CHANGE, CANCELED or RENDER_FAILED.
	Code    string `json:"code"`
	Message string `json:"message"`
}

// JobCanceled is what a cancel answers.
type JobCanceled struct {
	ID string `json:"id"`
	// Outcome is "refunded", "requested", "too-late" or "already-settled".
	Outcome         string `json:"outcome"`
	CreditsRefunded int64  `json:"credits_refunded"`
}

// Account is the key's account: balance, tier and limits.
type Account struct {
	ID    string  `json:"id"`
	Email string  `json:"email"`
	Name  *string `json:"name"`
	// Tier is "paid" once the account has topped up its API wallet; it sets the limits.
	Tier    string         `json:"tier"`
	Credits AccountCredits `json:"credits"`
	Key     AccountKey     `json:"key"`
	Limits  AccountLimits  `json:"limits"`
}

// AccountCredits is the balance.
type AccountCredits struct {
	// Spendable is what the next call can spend.
	Spendable int64 `json:"spendable"`
}

// AccountKey is the key a call used.
type AccountKey struct {
	ID             string   `json:"id"`
	AllowedOrigins []string `json:"allowed_origins"`
	// DailyCreditLimit is your own daily limit for this key, or nil for none.
	DailyCreditLimit *int64 `json:"daily_credit_limit"`
}

// AccountLimits are the limits a key works under.
type AccountLimits struct {
	// Scope is "account" (shared by every key, free) or "key" (this key's own, paid).
	Scope             string `json:"scope"`
	RequestsPerMinute int    `json:"requests_per_minute"`
	ReadsPerMinute    int    `json:"reads_per_minute"`
	ConcurrentJobs    int    `json:"concurrent_jobs"`
	// DailyCredits is what this key may still spend per UTC day; nil is no limit.
	DailyCredits *int64 `json:"daily_credits"`
}

// EngineCatalog is the engines and video models, with their prices and capabilities.
type EngineCatalog struct {
	Engines     []Engine     `json:"engines"`
	VideoModels []VideoModel `json:"video_models"`
}

// Engine is a render engine.
type Engine struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Tier     string `json:"tier"`
	Default  bool   `json:"default"`
	PaidOnly bool   `json:"paid_only"`
	// Available reports whether this account may use it now (a paid-only engine needs a paid account).
	Available    bool               `json:"available"`
	Prices       EnginePrices       `json:"prices"`
	Capabilities EngineCapabilities `json:"capabilities"`
}

// EnginePrices are an engine's prices, in credits.
type EnginePrices struct {
	Render int64 `json:"render"`
	Edit   int64 `json:"edit"`
}

// EngineCapabilities say what an engine can do.
type EngineCapabilities struct {
	TextToRender   bool `json:"text_to_render"`
	Masks          bool `json:"masks"`
	EditReferences bool `json:"edit_references"`
	MaxReferences  int  `json:"max_references"`
}

// VideoModel is a video model.
type VideoModel struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Default   bool         `json:"default"`
	Available bool         `json:"available"`
	Prices    []VideoPrice `json:"prices"`
}

// VideoPrice is a video price, per length.
type VideoPrice struct {
	// DurationSeconds is the clip length, or nil for the model's one fixed length.
	DurationSeconds *int  `json:"duration_seconds"`
	Credits         int64 `json:"credits"`
}

// JobStreamEvent is one event of Jobs.Stream: the job as it stands ("job"), then exactly one last
// event: "done" (the job settled), "timeout" (still running after ten minutes: open the stream
// again) or "error" (the API could not read the job; it still runs).
type JobStreamEvent struct {
	Type  string
	Job   *Job
	Error *JobError
}

// Final reports whether this is the stream's last event.
func (e *JobStreamEvent) Final() bool { return e.Type != "job" }

// Issue is one field the API refused, on a VALIDATION_FAILED answer.
type Issue struct {
	// Path is the field, as a dotted path ("references.0.mode").
	Path    string `json:"path"`
	Message string `json:"message"`
}

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
		Issues  []Issue         `json:"issues"`
	} `json:"error"`
}
