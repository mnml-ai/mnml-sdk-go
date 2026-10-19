package mnml

import (
	"encoding/json"
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
