package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/itchyny/gojq"
	"gopkg.in/yaml.v3"
)

const RES_KEY = "$res"

type Config struct {
	Steps   []Step  `yaml:"steps"`
	Globals Globals `yaml:"globals"`
}

type Globals struct {
	RootoContext interface{} `yaml:"rootContext"`
}

type Step struct {
	Type              string                `yaml:"type"`
	Name              string                `yaml:"name,omitempty"`
	Path              string                `yaml:"path,omitempty"`
	As                string                `yaml:"as,omitempty"`
	Steps             []Step                `yaml:"steps,omitempty"`
	Request           *RequestConfig        `yaml:"request,omitempty"`
	ResultTransformer string                `yaml:"resultTransformer,omitempty"`
	MergeWithParentOn string                `yaml:"mergeWithParentOn,omitempty"`
	MergeOn           string                `yaml:"mergeOn,omitempty"`
	MergeWithContext  *MergeWithContextRule `yaml:"mergeWithContext,omitempty"`
}

type RequestConfig struct {
	URL        string            `yaml:"url"`
	Method     string            `yaml:"method"`
	Headers    map[string]string `yaml:"headers,omitempty"`
	Body       string            `yaml:"body,omitempty"`
	Pagination Pagination        `yaml:"pagination,omitempty"`
}

type MergeWithContextRule struct {
	Name string `yaml:"name"`
	Rule string `yaml:"rule"`
}

type Context struct {
	Data          interface{}
	ParentContext string
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
		Data:          c.Config.Globals.RootoContext,
		ParentContext: "",
	}

	c.ContextMap["root"] = rootCtx
	currentContext := "root"

	for _, step := range c.Config.Steps {
		if err := c.ExecuteStep(step, currentContext, c.ContextMap); err != nil {
			return err
		}
	}
	return nil
}

func (c *ApiCrawler) ExecuteStep(step Step, currentContext string, contextMap map[string]*Context) error {
	switch step.Type {
	case "request":
		return c.handleRequest(step, currentContext, contextMap)
	case "forEach":
		return c.handleForEach(step, currentContext, contextMap)
	default:
		return fmt.Errorf("unknown step type: %s", step.Type)
	}
}

func (c *ApiCrawler) handleRequest(step Step, currentContext string, contextMap map[string]*Context) error {
	_ctx := contextMap[currentContext]
	// 1. Expand URL using Go template
	tmpl, err := template.New("url").Parse(step.Request.URL)
	if err != nil {
		return fmt.Errorf("error parsing URL template: %w", err)
	}
	var urlBuf bytes.Buffer
	templateCtx := contextMapToTemplate(contextMap)
	if err := tmpl.Execute(&urlBuf, templateCtx); err != nil {
		return fmt.Errorf("error executing URL template: %w", err)
	}
	_url := urlBuf.String()
	fmt.Printf("[Request] Fetching: %s\n", _url)

	paginator, err := NewPaginator(ConfigP{step.Request.Pagination})
	if err != nil {
		return fmt.Errorf("error creating request paginator: %w", err)
	}
	stop := false
	next := paginator.NextFromCtx()

	for !stop {
		// 1. Inject query params
		urlObj, err := url.Parse(_url)
		if err != nil {
			return fmt.Errorf("invalid URL: %w", err)
		}
		query := urlObj.Query()
		for k, v := range next.QueryParams {
			query.Set(k, v)
		}
		urlObj.RawQuery = query.Encode()

		// 2. Encode body if needed
		var reqBody io.Reader
		if len(next.BodyParams) > 0 {
			bodyJSON, err := json.Marshal(next.BodyParams)
			if err != nil {
				return fmt.Errorf("error encoding body params: %w", err)
			}
			reqBody = bytes.NewReader(bodyJSON)
		}

		// 2. Create and send HTTP request
		req, err := http.NewRequest(step.Request.Method, urlObj.String(), reqBody)
		if err != nil {
			return fmt.Errorf("error creating HTTP request: %w", err)
		}
		// Apply headers from both config and paginator
		for k, v := range step.Request.Headers {
			req.Header.Set(k, v)
		}
		for k, v := range next.Headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Transport: c.clientRoundtripper}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error performing HTTP request: %w", err)
		}
		defer resp.Body.Close()

		// run next
		next, stop, err = paginator.Next(resp)
		if err != nil {
			return fmt.Errorf("paginator update error: %w", err)
		}

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
			var singleResult interface{}
			count := 0

			for {
				v, ok := iter.Next()
				if !ok {
					break
				}
				if err, isErr := v.(error); isErr {
					return fmt.Errorf("jq error: %w", err)
				}

				count++
				if count > 1 {
					return fmt.Errorf("resultTransformer yielded more than one value")
				}

				singleResult = v
			}
			transformed = singleResult
		}

		fmt.Printf("[Request] Got response (transformed): %+v\n", transformed)

		// 1. Explicit merge rule (advanced use)
		if step.MergeOn != "" {
			// Simple jq merge on current context
			updated, err := applyMergeRule(_ctx.Data, step.MergeOn, transformed)
			if err != nil {
				return fmt.Errorf("mergeOn failed: %w", err)
			}
			_ctx.Data = updated
		} else if step.MergeWithParentOn != "" {
			parentCtx := contextMap[_ctx.ParentContext]
			// Simple jq merge on current context
			updated, err := applyMergeRule(parentCtx.Data, step.MergeWithParentOn, transformed)
			if err != nil {
				return fmt.Errorf("mergeWithParentOn failed: %w", err)
			}
			parentCtx.Data = updated
		} else if step.MergeWithContext != nil {
			// 2. Named context merge (cross-scope update)
			targetCtx, ok := contextMap[step.MergeWithContext.Name]
			if !ok {
				return fmt.Errorf("context '%s' not found", step.MergeWithContext.Name)
			}
			updated, err := applyMergeRule(targetCtx.Data, step.MergeWithContext.Rule, transformed)
			if err != nil {
				return fmt.Errorf("mergeWithContext failed: %w", err)
			}
			targetCtx.Data = updated
		} else {
			// 3. Simple assignment (shallow)
			switch data := _ctx.Data.(type) {
			case []interface{}:
				_ctx.Data = append(data, transformed.([]interface{})...) // Reassigns to field of original struct
			case map[string]interface{}:
				if transformedMap, ok := transformed.(map[string]interface{}); ok {
					for k, v := range transformedMap {
						data[k] = v // Modifies in-place
					}
				}
			default:
				_ctx.Data = transformed
			}
		}

		for _, step := range step.Steps {
			if err := c.ExecuteStep(step, currentContext, c.ContextMap); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *ApiCrawler) handleForEach(step Step, currentContext string, contextMap map[string]*Context) error {
	_ctx := contextMap[currentContext]
	query, err := gojq.Parse(step.Path)
	if err != nil {
		return fmt.Errorf("invalid jq path '%s': %w", step.Path, err)
	}

	iter := query.Run(_ctx.Data)
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

	executionResults := make([]interface{}, 0)
	for i, item := range results {
		fmt.Printf("[ForEach] Iteration %d as '%s': %v\n", i, step.As, item)

		childContextMap := childMapWith(contextMap, currentContext, step.As, item)
		for _, nested := range step.Steps {
			if err := c.ExecuteStep(nested, step.As, childContextMap); err != nil {
				return err
			}
		}
		executionResults = append(executionResults, childContextMap[step.As].Data)
	}
	// return nil

	{
		query, err := gojq.Parse(step.Path + " = $new")
		if err != nil {
			return fmt.Errorf("invalid merge rule JQ: %w", err)
		}
		code, err := gojq.Compile(query, gojq.WithVariables([]string{"$new"}))
		if err != nil {
			return fmt.Errorf("failed to compile merge rule: %w", err)
		}

		// Run the query against contextData, passing $res as a variable
		iter := code.Run(_ctx.Data, executionResults)

		v, ok := iter.Next()
		if !ok {
			return fmt.Errorf("patch yielded nothing")
		}
		if err, isErr := v.(error); isErr {
			return err
		}

		// Assign new patched data
		_ctx.Data = v
	}

	return nil
}

func applyMergeRule(contextData interface{}, rule string, result interface{}) (interface{}, error) {
	// Parse the JQ expression
	query, err := gojq.Parse(rule)
	if err != nil {
		return nil, fmt.Errorf("invalid merge rule JQ: %w", err)
	}

	// Create the evaluation context with $res variable bound
	code, err := gojq.Compile(query, gojq.WithVariables([]string{"$res"}))
	if err != nil {
		return nil, fmt.Errorf("failed to compile merge rule: %w", err)
	}

	// Run the query against contextData, passing $res as a variable
	iter := code.Run(contextData, result)

	// Collect the results, expecting exactly one
	var values []interface{}
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if errVal, isErr := v.(error); isErr {
			return nil, fmt.Errorf("error running JQ: %w", errVal)
		}
		values = append(values, v)
	}

	// Enforce exactly one result
	if len(values) != 1 {
		return nil, fmt.Errorf("merge rule must produce exactly one result, got %d", len(values))
	}

	return values[0], nil
}

func childMapWith(base map[string]*Context, currentCotnext, key string, value interface{}) map[string]*Context {
	newMap := make(map[string]*Context, len(base)+1)
	for k, v := range base {
		newMap[k] = v
	}
	newMap[key] = &Context{
		Data:          value,
		ParentContext: currentCotnext,
	}
	return newMap
}

func contextMapToTemplate(base map[string]*Context) map[string]interface{} {
	result := make(map[string]interface{})
	// root special case
	if rootMap, ok := base["root"].Data.(map[string]interface{}); ok {
		for k, v := range rootMap {
			result[k] = v
		}
	}

	for k, c := range base {
		if k == "root" {
			continue
		}
		result[k] = c.Data
	}
	return result
}
