package mnml

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// The SDK against the API's own OpenAPI document (spec/openapi.json): every field of every body
// and answer is a struct field here, no more and no fewer. A field the API adds fails here until
// the SDK has it. scripts/update-spec.sh refreshes the document.

type schema struct {
	Properties map[string]schema `json:"properties"`
	Items      *schema           `json:"items"`
	AnyOf      []schema          `json:"anyOf"`
	Enum       []string          `json:"enum"`
	Deprecated bool              `json:"deprecated"`
}

type operation struct {
	RequestBody *struct {
		Content map[string]struct {
			Schema schema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]struct {
			Schema schema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}

func loadSpec(t *testing.T) map[string]map[string]operation {
	raw, err := os.ReadFile("spec/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]operation `json:"paths"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Paths
}

func keys(s schema) []string {
	var out []string
	for k := range s.Properties {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func prop(s schema, name string) schema {
	p := s.Properties[name]
	if p.Items != nil {
		return *p.Items
	}
	for _, a := range p.AnyOf {
		if a.Properties != nil {
			return a
		}
	}
	return p
}

func fields(v any) []string {
	var out []string
	t := reflect.TypeOf(v)
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

func same(t *testing.T, what string, spec, sdk []string) {
	t.Helper()
	if !slices.Equal(spec, sdk) {
		t.Errorf("%s:\n spec %v\n sdk  %v", what, spec, sdk)
	}
}

func TestEveryRequestAndAnswerFieldIsAStructField(t *testing.T) {
	paths := loadSpec(t)
	body := func(p string) schema { return paths[p]["post"].RequestBody.Content["application/json"].Schema }
	data := func(p, m string) schema {
		for code, r := range paths[p][m].Responses {
			if strings.HasPrefix(code, "2") {
				return r.Content["application/json"].Schema.Properties["data"]
			}
		}
		t.Fatalf("%s %s has no 2xx answer", m, p)
		return schema{}
	}

	same(t, "renders body", keys(body("/v2/renders")), fields(RenderRequest{}))
	same(t, "edits body", keys(body("/v2/edits")), fields(EditRequest{}))
	same(t, "enhancements body", keys(body("/v2/enhancements")), fields(EnhancementRequest{}))
	same(t, "videos body", keys(body("/v2/videos")), fields(VideoRequest{}))

	same(t, "render answer", keys(data("/v2/renders", "post")), fields(RenderStarted{}))
	same(t, "edit answer", keys(data("/v2/edits", "post")), fields(JobStarted{}))
	job := data("/v2/jobs/{id}", "get")
	same(t, "job", keys(job), fields(Job{}))
	same(t, "job output", keys(prop(job, "outputs")), fields(JobOutput{}))
	same(t, "cancel", keys(data("/v2/jobs/{id}/cancel", "post")), fields(JobCanceled{}))

	account := data("/v2/account", "get")
	same(t, "account", keys(account), fields(Account{}))
	same(t, "account credits", keys(prop(account, "credits")), fields(AccountCredits{}))
	same(t, "account key", keys(prop(account, "key")), fields(AccountKey{}))
	same(t, "account limits", keys(prop(account, "limits")), fields(AccountLimits{}))

	engines := data("/v2/engines", "get")
	same(t, "engines", keys(engines), fields(EngineCatalog{}))
	engine := prop(engines, "engines")
	same(t, "engine", keys(engine), fields(Engine{}))
	same(t, "engine prices", keys(prop(engine, "prices")), fields(EnginePrices{}))
	same(t, "engine capabilities", keys(prop(engine, "capabilities")), fields(EngineCapabilities{}))
	video := prop(engines, "video_models")
	same(t, "video model", keys(video), fields(VideoModel{}))
	same(t, "video price", keys(prop(video, "prices")), fields(VideoPrice{}))

	enum := func(s schema) []string { e := slices.Clone(s.Enum); slices.Sort(e); return e }
	sorted := func(v ...string) []string { slices.Sort(v); return v }
	same(t, "modes", enum(body("/v2/renders").Properties["mode"]),
		sorted(ModeExterior, ModeInterior, ModeMasterplan, ModePlan, ModeLandscape, ModeProduct, ModeTextToRender))
	same(t, "aspect ratios", enum(body("/v2/renders").Properties["aspect_ratio"]),
		sorted(AspectAuto, Aspect1x1, Aspect3x2, Aspect4x3, Aspect5x4, Aspect16x9, Aspect21x9, Aspect2x3, Aspect3x4, Aspect4x5, Aspect9x16))
	same(t, "enhancement kinds", enum(body("/v2/enhancements").Properties["kind"]),
		sorted(EnhanceUpscale, EnhanceEnhance, EnhanceBGRemove, EnhanceOutpaint))
	same(t, "job statuses", enum(job.Properties["status"]),
		sorted(StatusQueued, StatusProcessing, StatusSucceeded, StatusFailed, StatusCanceled))
}

// Every answer field the spec deprecates, as "operation path: field path", and the struct field
// that carries it here. Each one's doc has a "Deprecated:" paragraph naming its dollar field.
var deprecatedFields = map[string]string{
	"POST /v2/jobs/{id}/cancel: credits_refunded":      "JobCanceled.credits_refunded",
	"POST /v2/edits: credits_charged":                  "JobStarted.credits_charged",
	"POST /v2/enhancements: credits_charged":           "JobStarted.credits_charged",
	"POST /v2/videos: credits_charged":                 "JobStarted.credits_charged",
	"POST /v2/renders: credits_charged":                "RenderStarted.credits_charged",
	"POST /v2/renders: jobs[].credits_charged":         "Job.credits_charged",
	"POST /v2/renders: jobs[].credits_refunded":        "Job.credits_refunded",
	"GET /v2/jobs/{id}: credits_charged":               "Job.credits_charged",
	"GET /v2/jobs/{id}: credits_refunded":              "Job.credits_refunded",
	"GET /v2/account: credits":                         "Account.credits",
	"GET /v2/account: credits.spendable":               "AccountCredits.spendable",
	"GET /v2/account: key.daily_credit_limit":          "AccountKey.daily_credit_limit",
	"GET /v2/account: limits.daily_credits":            "AccountLimits.daily_credits",
	"GET /v2/engines: engines[].prices.render":         "EnginePrices.render",
	"GET /v2/engines: engines[].prices.edit":           "EnginePrices.edit",
	"GET /v2/engines: video_models[].prices[].credits": "VideoPrice.credits",
}

// The credits.low webhook is not in the spec; its credit fields are deprecated all the same.
var deprecatedWebhookFields = []string{"CreditsLow.balance", "CreditsLow.threshold"}

func TestTheSDKDeprecatesWhatTheSpecDeprecates(t *testing.T) {
	paths := loadSpec(t)
	var found []string
	var walk func(op string, s schema, path string)
	walk = func(op string, s schema, path string) {
		at := path
		if s.Items != nil {
			s, at = *s.Items, path+"[]"
		}
		for _, a := range s.AnyOf {
			if a.Properties != nil {
				s = a
			}
		}
		for k, v := range s.Properties {
			p := k
			if at != "" {
				p = at + "." + k
			}
			if v.Deprecated {
				found = append(found, op+": "+p)
			}
			walk(op, v, p)
		}
	}
	for p, methods := range paths {
		for m, o := range methods {
			for code, r := range o.Responses {
				if strings.HasPrefix(code, "2") {
					if c, ok := r.Content["application/json"]; ok {
						walk(strings.ToUpper(m)+" "+p, c.Schema.Properties["data"], "")
					}
				}
			}
		}
	}
	var want []string
	for k := range deprecatedFields {
		want = append(want, k)
	}
	slices.Sort(found)
	found = slices.Compact(found) // a render answers 200 (with a wait) or 202, the same fields
	slices.Sort(want)
	same(t, "deprecated in the spec", found, want)

	// The struct fields whose doc carries a "Deprecated:" paragraph, as "Type.json_name".
	var marked []string
	fset := token.NewFileSet()
	for _, file := range []string{"types.go", "webhooks.go"} {
		f, err := parser.ParseFile(fset, file, nil, parser.ParseComments)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok || strings.HasSuffix(ts.Name.Name, "Request") {
				return false // request fields (ImageURL) are not answers
			}
			for _, fd := range st.Fields.List {
				if fd.Doc == nil || fd.Tag == nil || !strings.Contains(fd.Doc.Text(), "\nDeprecated: ") {
					continue
				}
				tag := reflect.StructTag(strings.Trim(fd.Tag.Value, "`")).Get("json")
				name, _, _ := strings.Cut(tag, ",")
				marked = append(marked, ts.Name.Name+"."+name)
			}
			return false
		})
	}
	wantMarked := slices.Clone(deprecatedWebhookFields)
	for _, v := range deprecatedFields {
		if !slices.Contains(wantMarked, v) {
			wantMarked = append(wantMarked, v)
		}
	}
	slices.Sort(marked)
	slices.Sort(wantMarked)
	same(t, "deprecated in the SDK", wantMarked, marked)
}
