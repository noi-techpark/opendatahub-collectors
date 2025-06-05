package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"os"

	"github.com/itchyny/gojq"
	"gopkg.in/yaml.v3"
)

const RES_KEY = "$res"

type Config struct {
	Steps []Step `yaml:"steps"`
}

type Step struct {
	Type              string                `yaml:"type"`
	Name              string                `yaml:"name,omitempty"`
	Path              string                `yaml:"path,omitempty"`
	As                string                `yaml:"as,omitempty"`
	Steps             []Step                `yaml:"steps,omitempty"`
	Request           *RequestConfig        `yaml:"request,omitempty"`
	ResultTransformer string                `yaml:"resultTransformer,omitempty"`
	ResultName        string                `yaml:"resultName,omitempty"`
	MergeWithParentOn string                `yaml:"mergeWithParentOn,omitempty"`
	MergeWithContext  *MergeWithContextRule `yaml:"mergeWithContext,omitempty"`
}

type RequestConfig struct {
	URL     string            `yaml:"url"`
	Method  string            `yaml:"method"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    string            `yaml:"body,omitempty"`
}

type MergeWithContextRule struct {
	Name string `yaml:"name"`
	Rule string `yaml:"rule"`
}

type Context struct {
	Data map[string]interface{}
}

type ApiCrawler struct {
	clientRoundtripper http.RoundTripper
	Config             Config
	ContextMap         map[string]*Context
}

func NewApiCrawler(configPath string) *ApiCrawler {
	data, err := os.ReadFile(configPath)
	if err != nil {
		panic(err)
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		panic(err)
	}

	return &ApiCrawler{
		clientRoundtripper: http.DefaultTransport,
		Config:             cfg,
		ContextMap:         map[string]*Context{},
	}
}

func (a *ApiCrawler) SetClientRoundTripper(rt http.RoundTripper) {
	a.clientRoundtripper = rt
}

func (c *ApiCrawler) Run() error {
	rootCtx := &Context{
		Data: map[string]interface{}{},
	}
	for _, step := range c.Config.Steps {
		if err := c.ExecuteStep(step, rootCtx); err != nil {
			return err
		}
	}
	return nil
}

func (c *ApiCrawler) ExecuteStep(step Step, ctx *Context) error {
	switch step.Type {
	case "request":
		return c.handleRequest(step, ctx)
	case "forEach":
		return c.handleForEach(step, ctx)
	default:
		return fmt.Errorf("unknown step type: %s", step.Type)
	}
}

func (c *ApiCrawler) handleRequest(step Step, ctx *Context) error {
	// 1. Expand URL using Go template
	tmpl, err := template.New("url").Parse(step.Request.URL)
	if err != nil {
		return fmt.Errorf("error parsing URL template: %w", err)
	}
	var urlBuf bytes.Buffer
	if err := tmpl.Execute(&urlBuf, ctx.Data); err != nil {
		return fmt.Errorf("error executing URL template: %w", err)
	}
	url := urlBuf.String()
	fmt.Printf("[Request] Fetching: %s\n", url)

	// 2. Create and send HTTP request
	req, err := http.NewRequest(step.Request.Method, url, nil) // body support optional later
	if err != nil {
		return fmt.Errorf("error creating HTTP request: %w", err)
	}
	for k, v := range step.Request.Headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Transport: c.clientRoundtripper}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error performing HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// 3. Decode JSON response into interface{}
	var raw interface{}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return fmt.Errorf("error decoding JSON: %w", err)
	}

	// 4. Apply JQ transformer
	transformed := raw
	if step.ResultTransformer != "" {
		query, err := gojq.Parse(step.ResultTransformer)
		if err != nil {
			return fmt.Errorf("invalid resultTransformer JQ: %w", err)
		}
		iter := query.Run(raw)
		var results []interface{}
		for {
			v, ok := iter.Next()
			if !ok {
				break
			}
			if err, isErr := v.(error); isErr {
				return fmt.Errorf("jq error: %w", err)
			}
			results = append(results, v)
		}
		transformed = results
	}

	fmt.Printf("[Request] Got response (transformed): %+v\n", transformed)

	// 5. Apply ResultName (store result as named key)
	// if step.ResultName != "" {
	// 	ctx.Data[step.ResultName] = transformed
	// }

	// 1. Explicit merge rule (advanced use)
	if step.MergeWithParentOn != "" {
		// Simple jq merge on current context
		updated, err := applyMergeRule(ctx.Data, step.MergeWithParentOn, transformed)
		if err != nil {
			return fmt.Errorf("mergeWithParentOn failed: %w", err)
		}
		ctx.Data = updated
	} else if step.MergeWithContext != nil {
		// 2. Named context merge (cross-scope update)
		targetCtx, ok := c.ContextMap[step.MergeWithContext.Name]
		if !ok {
			return fmt.Errorf("context '%s' not found", step.MergeWithContext.Name)
		}
		updated, err := applyMergeRule(targetCtx.Data, step.MergeWithContext.Rule, transformed)
		if err != nil {
			return fmt.Errorf("mergeWithContext failed: %w", err)
		}
		targetCtx.Data = updated
	} else if step.ResultName != "" {
		// 3. Simple assignment (shallow)
		ctx.Data[step.ResultName] = transformed
	} else if transformedMap, ok := transformed.(map[string]interface{}); ok {
		// 4. If `transformed` is a map and nothing else is specified → deep merge
		for k, v := range transformedMap {
			ctx.Data[k] = v
		}
	} /*else {
		// 5. Else fallback (e.g. assign under special key)
		ctx.Data[RES_KEY] = transformed
	}*/

	return nil
}

func (c *ApiCrawler) handleForEach(step Step, ctx *Context) error {
	query, err := gojq.Parse(step.Path)
	if err != nil {
		return fmt.Errorf("invalid jq path '%s': %w", step.Path, err)
	}

	iter := query.Run(ctx.Data)
	results := []interface{}{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if err, isErr := v.(error); isErr {
			return fmt.Errorf("jq error: %w", err)
		}
		results = append(results, v)
	}

	// Make sure the result is an array (jq might emit one-by-one items)
	if len(results) == 1 {
		if arr, ok := results[0].([]interface{}); ok {
			results = arr
		}
	}

	for i, item := range results {
		fmt.Printf("[ForEach] Iteration %d as '%s': %v\n", i, step.As, item)

		childCtx := &Context{
			Data: copyMapWith(ctx.Data, step.As, item),
		}

		for _, nested := range step.Steps {
			if err := c.ExecuteStep(nested, childCtx); err != nil {
				return err
			}
		}
	}

	return nil
}

func applyMergeRule(contextData map[string]interface{}, rule string, result interface{}) (map[string]interface{}, error) {
	ctx := map[string]interface{}{
		RES_KEY: result,
	}
	for k, v := range contextData {
		ctx[k] = v
	}
	query, err := gojq.Parse(rule)
	if err != nil {
		return nil, err
	}
	iter := query.Run(ctx)
	value, ok := iter.Next()
	if !ok {
		return contextData, nil
	}
	if err, isErr := value.(error); isErr {
		return nil, err
	}
	// NOTE: This assumes that the rule mutates a top-level object
	// If your DSL implies mutation like `.foo =| $res`, you'll need to parse the assignment target and write to it.
	// Here we assume `rule` returns a new map
	newMap, ok := value.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("merge rule did not return a valid map")
	}
	return newMap, nil
}

func copyMapWith(base map[string]interface{}, key string, value interface{}) map[string]interface{} {
	newMap := make(map[string]interface{}, len(base)+1)
	for k, v := range base {
		newMap[k] = v
	}
	newMap[key] = value
	return newMap
}
