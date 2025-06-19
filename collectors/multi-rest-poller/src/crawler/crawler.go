package crawler

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/itchyny/gojq"
	"gopkg.in/yaml.v3"
)

type StepProfilerData struct {
	Name    string
	Config  Step
	Data    any
	Context Context
	Extra   map[string]any
}

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warning(msg string, args ...any)
	Error(msg string, args ...any)
}

type stdLogger struct {
	logger *log.Logger
}

func NewDefaultLogger() Logger {
	return &stdLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *stdLogger) Info(msg string, args ...any) {
	l.logger.Println("[INFO]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Debug(msg string, args ...any) {
	l.logger.Println("[DEBUG]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Warning(msg string, args ...any) {
	l.logger.Println("[WARN]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Error(msg string, args ...any) {
	l.logger.Println("[ERROR]", fmt.Sprintf(msg, args...)+"\n")
}

const RES_KEY = "$res"

type Config struct {
	Steps          []Step               `yaml:"steps" json:"steps"`
	RootContext    interface{}          `yaml:"rootContext" json:"rootContext"`
	Authentication *AuthenticatorConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
	Headers        map[string]string    `yaml:"headers,omitempty" json:"headers,omitempty"`
	Stream         bool                 `yaml:"stream,omitempty" json:"stream,omitempty"`
}

type Step struct {
	Type              string                `yaml:"type" json:"type"`
	Name              string                `yaml:"name,omitempty" json:"name,omitempty"`
	Path              string                `yaml:"path,omitempty" json:"path,omitempty"`
	As                string                `yaml:"as,omitempty" json:"as,omitempty"`
	Values            []interface{}         `yaml:"values,omitempty" json:"values,omitempty"`
	Steps             []Step                `yaml:"steps,omitempty" json:"steps,omitempty"`
	Request           *RequestConfig        `yaml:"request,omitempty" json:"request,omitempty"`
	ResultTransformer string                `yaml:"resultTransformer,omitempty" json:"resultTransformer,omitempty"`
	MergeWithParentOn string                `yaml:"mergeWithParentOn,omitempty" json:"mergeWithParentOn,omitempty"`
	MergeOn           string                `yaml:"mergeOn,omitempty" json:"mergeOn,omitempty"`
	MergeWithContext  *MergeWithContextRule `yaml:"mergeWithContext,omitempty" json:"mergeWithContext,omitempty"`
}

type RequestConfig struct {
	URL            string               `yaml:"url" json:"url"`
	Method         string               `yaml:"method" json:"method"`
	Headers        map[string]string    `yaml:"headers,omitempty" json:"headers,omitempty"`
	Body           string               `yaml:"body,omitempty" json:"body,omitempty"`
	Pagination     Pagination           `yaml:"pagination,omitempty" json:"pagination,omitempty"`
	Authentication *AuthenticatorConfig `yaml:"auth,omitempty" json:"auth,omitempty"`
}

type MergeWithContextRule struct {
	Name string `yaml:"name"`
	Rule string `yaml:"rule"`
}

type Context struct {
	Data          interface{}
	ParentContext string
	key           string
	depth         int
}

type stepExecution struct {
	step              Step
	currentContextKey string
	currentContext    *Context
	contextMap        map[string]*Context
}

type ApiCrawler struct {
	clientRoundtripper  http.RoundTripper
	Config              Config
	ContextMap          map[string]*Context
	globalAuthenticator Authenticator
	DataStream          chan any
	logger              Logger
	profiler            chan StepProfilerData
	enableProfilation   bool
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

	if nil == cfg.RootContext {
		panic("globals.rootContext must be either [] or {}")
	}

	if _, ok := cfg.RootContext.([]interface{}); cfg.Stream && !ok {
		panic("globals.stream can be used only on array globals.rootContext")
	}

	c := &ApiCrawler{
		clientRoundtripper: http.DefaultTransport,
		Config:             cfg,
		ContextMap:         map[string]*Context{},
		logger:             NewDefaultLogger(),
		profiler:           nil,
	}

	// handle stream channel
	if cfg.Stream {
		c.DataStream = make(chan any)
	}

	// instantiate global authenticator
	if cfg.Authentication != nil {
		c.globalAuthenticator = NewAuthenticator(*cfg.Authentication)
	} else {
		c.globalAuthenticator = NoopAuthenticator{}
	}
	return c
}

func (a *ApiCrawler) GetDataStream() chan interface{} {
	return a.DataStream
}

func (a *ApiCrawler) GetData() interface{} {
	return a.ContextMap["root"].Data
}

func (a *ApiCrawler) SetLogger(logger Logger) {
	a.logger = logger
}

func (a *ApiCrawler) SetClientRoundTripper(rt http.RoundTripper) {
	a.clientRoundtripper = rt
}

func (a *ApiCrawler) EnableProfiler() chan StepProfilerData {
	a.enableProfilation = true
	a.profiler = make(chan StepProfilerData)
	return a.profiler
}

func deepCopy[T any](src T) (T, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	dec := gob.NewDecoder(&buf)

	if err := enc.Encode(src); err != nil {
		var zero T
		return zero, err
	}

	var dst T
	if err := dec.Decode(&dst); err != nil {
		return dst, err
	}

	return dst, nil
}

func (a *ApiCrawler) pushProfilerData(name string, exec *stepExecution, Data any, extra ...any) {
	if a.profiler == nil {
		return
	}

	// Defensive copy of step, with Steps cleared
	cleanConfig, _ := deepCopy(exec.step)
	cleanConfig.Steps = make([]Step, 0)

	// Convert variadic args into map[string]any
	extraMap := make(map[string]any)
	for i := 0; i+1 < len(extra); i += 2 {
		key, ok := extra[i].(string)
		if !ok {
			continue // skip invalid key
		}
		extraMap[key] = extra[i+1]
	}

	d := StepProfilerData{
		Name:    name,
		Context: *exec.currentContext,
		Data:    Data,
		Config:  cleanConfig,
		Extra:   extraMap,
	}

	a.profiler <- d
}

func newStepExecution(step Step, currentContextKey string, contextMap map[string]*Context) *stepExecution {
	return &stepExecution{
		step:              step,
		currentContextKey: currentContextKey,
		contextMap:        contextMap,
		currentContext:    contextMap[currentContextKey],
	}
}

func (c *ApiCrawler) Run(ctx context.Context) error {
	rootCtx := &Context{
		Data:          c.Config.RootContext,
		ParentContext: "",
		depth:         0,
		key:           "root",
	}

	c.ContextMap["root"] = rootCtx
	currentContext := "root"

	for _, step := range c.Config.Steps {
		ecxec := newStepExecution(step, currentContext, c.ContextMap)
		if err := c.ExecuteStep(ctx, ecxec); err != nil {
			return err
		}
	}
	return nil
}

func (c *ApiCrawler) ExecuteStep(ctx context.Context, exec *stepExecution) error {
	switch exec.step.Type {
	case "request":
		return c.handleRequest(ctx, exec)
	case "forEach":
		return c.handleForEach(ctx, exec)
	default:
		return fmt.Errorf("unknown step type: %s", exec.step.Type)
	}
}

func (c *ApiCrawler) handleRequest(ctx context.Context, exec *stepExecution) error {
	c.logger.Info("[Request] Preparing %s", exec.step.Name)

	// 1. Expand URL using Go template
	tmpl, err := template.New("url").Parse(exec.step.Request.URL)
	if err != nil {
		return fmt.Errorf("error parsing URL template: %w", err)
	}
	var urlBuf bytes.Buffer
	templateCtx := contextMapToTemplate(exec.contextMap)
	if err := tmpl.Execute(&urlBuf, templateCtx); err != nil {
		return fmt.Errorf("error executing URL template: %w", err)
	}
	_url := urlBuf.String()

	// instantiate authenticator
	authenticator := c.globalAuthenticator
	if exec.step.Request.Authentication != nil {
		authenticator = NewAuthenticator(*exec.step.Request.Authentication)
	}

	// instantiate paginator
	paginator, err := NewPaginator(ConfigP{exec.step.Request.Pagination})
	if err != nil {
		return fmt.Errorf("error creating request paginator: %w", err)
	}
	stop := false
	next := paginator.NextFromCtx()

	for !stop {
		// context cancelation handling
		select {
		case <-ctx.Done():
			return ctx.Err() // Context cancelled
		default:
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
			req, err := http.NewRequest(exec.step.Request.Method, urlObj.String(), reqBody)
			if err != nil {
				return fmt.Errorf("error creating HTTP request: %w", err)
			}
			// Apply headers from both config and paginator
			// priority is (ascending order)
			// 1. Global
			// 2. Request
			// 3. Pagination
			for k, v := range c.Config.Headers {
				req.Header.Set(k, v)
			}
			for k, v := range exec.step.Request.Headers {
				req.Header.Set(k, v)
			}
			for k, v := range next.Headers {
				req.Header.Set(k, v)
			}

			// apply authentication
			authenticator.PrepareRequest(req)

			client := &http.Client{Transport: c.clientRoundtripper}

			c.logger.Info("[Request] %s", urlObj.String())

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

			profileStepName := fmt.Sprintf("Request '%s'", exec.step.Name)
			c.pushProfilerData(profileStepName, exec, raw, "url", urlObj.String())

			// 4. Apply JQ transformer
			transformed := raw
			c.logger.Debug("[Request] Got response: status %s", resp.Status)

			if exec.step.ResultTransformer != "" {
				c.logger.Debug("[Request] transforming with expression: %s", exec.step.ResultTransformer)

				query, err := gojq.Parse(exec.step.ResultTransformer)
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

			profileStepName = fmt.Sprintf("Request Transfomerd '%s'", exec.step.Name)
			c.pushProfilerData(profileStepName, exec, transformed, "url", urlObj.String())

			// 1. Explicit merge rule (advanced use)
			if exec.step.MergeOn != "" {
				c.logger.Debug("[Request] merging-on with expression: %s", exec.step.MergeOn)

				// Simple jq merge on current context
				updated, err := applyMergeRule(exec.currentContext.Data, exec.step.MergeOn, transformed)
				if err != nil {
					return fmt.Errorf("mergeOn failed: %w", err)
				}
				exec.currentContext.Data = updated
			} else if exec.step.MergeWithParentOn != "" {
				c.logger.Debug("[Request] merging-with-parent with expression: %s", exec.step.MergeWithParentOn)

				parentCtx := exec.contextMap[exec.currentContext.ParentContext]
				// Simple jq merge on current context
				updated, err := applyMergeRule(parentCtx.Data, exec.step.MergeWithParentOn, transformed)
				if err != nil {
					return fmt.Errorf("mergeWithParentOn failed: %w", err)
				}
				parentCtx.Data = updated
			} else if exec.step.MergeWithContext != nil {
				c.logger.Debug("[Request] merging-with-context with expression: %s:%s",
					exec.step.MergeWithContext.Name, exec.step.MergeWithContext.Rule)

				// 2. Named context merge (cross-scope update)
				targetCtx, ok := exec.contextMap[exec.step.MergeWithContext.Name]
				if !ok {
					return fmt.Errorf("context '%s' not found", exec.step.MergeWithContext.Name)
				}
				updated, err := applyMergeRule(targetCtx.Data, exec.step.MergeWithContext.Rule, transformed)
				if err != nil {
					return fmt.Errorf("mergeWithContext failed: %w", err)
				}
				targetCtx.Data = updated
			} else {
				c.logger.Debug("[Request] default merge")

				// 3. Simple assignment (shallow)
				switch data := exec.currentContext.Data.(type) {
				case []interface{}:
					exec.currentContext.Data = append(data, transformed.([]interface{})...) // Reassigns to field of original struct
				case map[string]interface{}:
					if transformedMap, ok := transformed.(map[string]interface{}); ok {
						for k, v := range transformedMap {
							data[k] = v // Modifies in-place
						}
					}
				default:
					exec.currentContext.Data = transformed
				}
			}

			profileStepName = fmt.Sprintf("Request Merged '%s'", exec.step.Name)
			c.pushProfilerData(profileStepName, exec, exec.currentContext.Data, "url", urlObj.String())

			for _, step := range exec.step.Steps {
				newExec := newStepExecution(step, exec.currentContextKey, exec.contextMap)
				// newExec := newStepExecution(step, exec.currentContextKey, c.ContextMap)
				if err := c.ExecuteStep(ctx, newExec); err != nil {
					return err
				}
			}

			// at this point all inner steps have been executed for all entries in this call
			// the tree has been completely retrieved and we can check the stream
			if exec.currentContext.depth == 0 && c.Config.Stream {
				// No need to check conversion since rootContext is enforced to be an array
				array_data := exec.currentContext.Data.([]interface{})
				for _, d := range array_data {
					c.DataStream <- d
				}

				// reset data
				exec.currentContext.Data = []interface{}{}
			}
		}
	}

	return nil
}

func (c *ApiCrawler) handleForEach(ctx context.Context, exec *stepExecution) error {
	c.logger.Info("[Foreach] Preparing %s", exec.step.Name)

	results := []interface{}{}

	if len(exec.step.Path) != 0 && exec.step.Values == nil {
		c.logger.Debug("[Foreach] Extracting from parent context with rule: %s", exec.step.Path)

		query, err := gojq.Parse(exec.step.Path)
		if err != nil {
			return fmt.Errorf("invalid jq path '%s': %w", exec.step.Path, err)
		}

		iter := query.Run(exec.currentContext.Data)
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
	} else if exec.step.Values != nil {
		c.logger.Debug("[Foreach] using values over path: %s, values %+v", exec.step.Path, exec.step.Values)

		for _, v := range exec.step.Values {
			results = append(results, map[string]interface{}{"value": v})
		}
	}

	profileStepName := fmt.Sprintf("Foreach Extract '%s'", exec.step.Name)
	c.pushProfilerData(profileStepName, exec, results)

	executionResults := make([]interface{}, 0)
	for i, item := range results {
		// context cancelation handling
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			c.logger.Info("[ForEach] Iteration %d as '%s'", i, exec.step.As, "item", item)

			childContextMap := childMapWith(exec.contextMap, exec.currentContext, exec.step.As, item)

			profileStepName := fmt.Sprintf("Foreach [%d] '%s'", i, exec.step.Name)
			c.pushProfilerData(profileStepName, exec, item)

			for _, nested := range exec.step.Steps {
				newExec := newStepExecution(nested, exec.step.As, childContextMap)
				if err := c.ExecuteStep(ctx, newExec); err != nil {
					return err
				}
			}

			profileStepName = fmt.Sprintf("Foreach [%d] Result '%s'", i, exec.step.Name)
			c.pushProfilerData(profileStepName, exec, childContextMap[exec.step.As].Data)

			executionResults = append(executionResults, childContextMap[exec.step.As].Data)
		}
	}

	// We need to path the context with the result of the nested data.
	// This has to be done only if we are using path selector, foreach with hadcoded values already merge with some othe context
	query, err := gojq.Parse(exec.step.Path + " = $new")
	if err != nil {
		return fmt.Errorf("invalid merge rule JQ: %w", err)
	}
	code, err := gojq.Compile(query, gojq.WithVariables([]string{"$new"}))
	if err != nil {
		return fmt.Errorf("failed to compile merge rule: %w", err)
	}

	// Run the query against contextData, passing $new as a variable
	iter := code.Run(exec.currentContext.Data, executionResults)

	v, ok := iter.Next()
	if !ok {
		return fmt.Errorf("patch yielded nothing")
	}
	if err, isErr := v.(error); isErr {
		return err
	}

	// Assign new patched data
	exec.currentContext.Data = v

	profileStepName = fmt.Sprintf("Foreach Merged '%s'", exec.step.Name)
	c.pushProfilerData(profileStepName, exec, exec.currentContext.Data)

	// at this point all inner steps have been executed for all entries in this call
	// the tree has been completely retrieved and we can check the stream
	if exec.currentContext.depth == 0 && c.Config.Stream {
		// No need to check conversion since rootContext is enforced to be an array
		array_data := exec.currentContext.Data.([]interface{})
		for _, d := range array_data {
			c.DataStream <- d
		}

		// reset data
		exec.currentContext.Data = []interface{}{}
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

func childMapWith(base map[string]*Context, currentCotnext *Context, key string, value interface{}) map[string]*Context {
	newMap := make(map[string]*Context, len(base)+1)
	for k, v := range base {
		newMap[k] = v
	}
	newMap[key] = &Context{
		Data:          value,
		ParentContext: currentCotnext.key,
		key:           key,
		depth:         currentCotnext.depth + 1,
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
