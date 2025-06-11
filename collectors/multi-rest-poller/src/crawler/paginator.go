package crawler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/itchyny/gojq"
	"gopkg.in/yaml.v3"
)

type Param struct {
	Name      string `yaml:"name"`
	Location  string `yaml:"location"` // "query", "body", "header"
	Type      string `yaml:"type"`     // "int", "float", "datetime", "dynamic"`
	Format    string `yaml:"format,omitempty"`
	Default   string `yaml:"default"`
	Increment string `yaml:"increment,omitempty"`
	Source    string `yaml:"source,omitempty"` // "body:selector" or "header:selector"
}

type StopCondition struct {
	Type       string `yaml:"type"`       // "responseBody", "requestParam"
	Expression string `yaml:"expression"` // used by jq

	Param   string `yaml:"param,omitempty"`   // for requestParam
	Compare string `yaml:"compare,omitempty"` // "lt", "lte", "eq", "gt", "gte"
	Value   any    `yaml:"value,omitempty"`   // value to compare against
}

type Pagination struct {
	Params []Param         `yaml:"params"`
	StopOn []StopCondition `yaml:"stopOn"`
}

type ConfigP struct {
	Pagination Pagination `yaml:"pagination"`
}

type PaginationContext map[string]interface{}

type Paginator struct {
	config  ConfigP
	ctx     PaginationContext
	stopped bool
}

type RequestParts struct {
	QueryParams map[string]string      `yaml:"queryParams"`
	BodyParams  map[string]interface{} `yaml:"bodyParams"`
	Headers     map[string]string      `yaml:"headers"`
}

// NewPaginator creates a new paginator from YAML config
func NewPaginator(cfg ConfigP) (*Paginator, error) {
	p := &Paginator{
		config:  cfg,
		ctx:     make(PaginationContext),
		stopped: false,
	}

	// initialize context
	return p, p.initializeContext()
}

// NewPaginatorFromFile creates a new paginator from YAML config
func NewPaginatorFromFile(yamlData []byte) (*Paginator, error) {
	var cfg ConfigP
	if err := yaml.Unmarshal(yamlData, &cfg); err != nil {
		return nil, err
	}
	return NewPaginator(cfg)
}
func (p *Paginator) Ctx() PaginationContext {
	return p.ctx
}

func evalJQ(expr string, input interface{}) (interface{}, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}
	iter := query.Run(input)

	v, ok := iter.Next()
	if !ok {
		return nil, fmt.Errorf("no result from jq expression")
	}
	if err, isErr := v.(error); isErr {
		return nil, fmt.Errorf("jq error: %w", err)
	}
	return v, nil
}

var nowFunc = func() time.Time {
	return time.Now().UTC()
}

func (p *Paginator) initializeContext() error {
	for _, param := range p.config.Pagination.Params {
		if param.Type == "dynamic" {
			continue
		}

		// Parse default into actual typed value
		var parsed any
		var err error

		switch param.Type {
		case "int":
			parsed, err = strconv.Atoi(param.Default)
		case "float":
			parsed, err = strconv.ParseFloat(param.Default, 64)
		case "datetime":
			parsed, err = toTime(param.Default, param.Format)
		default:
			parsed = param.Default
		}
		if err != nil {
			return fmt.Errorf("invalid default value for param '%s': %w", param.Name, err)
		}
		p.ctx[param.Name] = parsed
	}
	return nil
}

func (p *Paginator) applyIncrements() error {
	for _, param := range p.config.Pagination.Params {
		if param.Type == "dynamic" {
			continue
		}

		// initializeContext creates all needed params, therefore no need to check existence
		val := p.ctx[param.Name]
		if param.Increment != "" {
			// Apply increment if defined
			switch param.Type {
			case "datetime":
				tval, err := toTime(val, param.Format)
				if err != nil {
					return fmt.Errorf("failed to parse datetime param '%s': %w", param.Name, err)
				}
				// dur, err := str2duration.ParseDuration(param.Increment)
				newTime, err := addSmartDuration(tval, param.Increment)
				if err != nil {
					return fmt.Errorf("failed to parse datetime increment '%s': %w", param.Increment, err)
				}
				// newTime := tval.Add(dur)
				p.ctx[param.Name] = newTime.Format(param.Format)

			case "int", "float":
				// Increment using jq math expression (e.g. `. + 10`)
				res, err := evalJQ(param.Increment, val)
				if err != nil {
					return fmt.Errorf("jq eval error on increment for '%s': %w", param.Name, err)
				}
				p.ctx[param.Name] = res

			default:
				return fmt.Errorf("unknown increment type: %s", param.Type)
			}
		}
	}
	return nil
}

func (p *Paginator) extractDynamicParams(body interface{}, headers map[string][]string) error {
	for _, param := range p.config.Pagination.Params {
		if param.Type != "dynamic" {
			continue
		}

		sourceParts := strings.SplitN(param.Source, ":", 2)
		sourceType := sourceParts[0]
		sourcePath := ""
		if len(sourceParts) > 1 {
			sourcePath = sourceParts[1]
		}

		switch sourceType {
		case "body":
			if sourcePath == "" {
				return fmt.Errorf("missing jq expression for param '%s'", param.Name)
			}
			val, err := evalJQ(sourcePath, body)
			if err != nil {
				return fmt.Errorf("jq error for %s: %w", param.Name, err)
			}
			p.ctx[param.Name] = val

		case "header":
			if sourcePath == "" {
				return fmt.Errorf("missing header key for param '%s'", param.Name)
			}
			if val, ok := headers[sourcePath]; ok && len(val) > 0 {
				p.ctx[param.Name] = val[0]
			}

		default:
			return fmt.Errorf("unsupported source type '%s' for param '%s'", sourceType, param.Name)
		}
	}
	return nil
}

func compareValues(param Param, a, b any, op string) (bool, error) {
	switch param.Type {
	case "int":
		af, err := toFloat64(a)
		if err != nil {
			return false, err
		}
		bf, err := toFloat64(b)
		if err != nil {
			return false, err
		}
		return floatCompare(af, bf, op)

	case "float":
		af, err := toFloat64(a)
		if err != nil {
			return false, err
		}
		bf, err := toFloat64(b)
		if err != nil {
			return false, err
		}
		return floatCompare(af, bf, op)

	case "datetime":
		ta, err := toTime(a, param.Format)
		if err != nil {
			return false, err
		}
		var tb time.Time
		switch vb := b.(type) {
		case string:
			tb, err = toTime(vb, param.Format)
			if err != nil {
				return false, fmt.Errorf("invalid stop condition datetime: %w", err)
			}
		case time.Time:
			tb = vb
		default:
			return false, fmt.Errorf("unsupported datetime compare value: %v", b)
		}
		return timeCompare(ta, tb, op)

	default:
		// string compare fallback
		as := fmt.Sprintf("%v", a)
		bs := fmt.Sprintf("%v", b)
		switch op {
		case "eq":
			return as == bs, nil
		case "lt":
			return as < bs, nil
		case "lte":
			return as <= bs, nil
		case "gt":
			return as > bs, nil
		case "gte":
			return as >= bs, nil
		default:
			return false, fmt.Errorf("unsupported compare operator: %s", op)
		}
	}
}

func floatCompare(a, b float64, op string) (bool, error) {
	switch op {
	case "lt":
		return a < b, nil
	case "lte":
		return a <= b, nil
	case "gt":
		return a > b, nil
	case "gte":
		return a >= b, nil
	case "eq":
		return a == b, nil
	default:
		return false, fmt.Errorf("unsupported float compare operator: %s", op)
	}
}

func timeCompare(a, b time.Time, op string) (bool, error) {
	switch op {
	case "lt":
		return a.Before(b), nil
	case "lte":
		return a.Before(b) || a.Equal(b), nil
	case "gt":
		return a.After(b), nil
	case "gte":
		return a.After(b) || a.Equal(b), nil
	case "eq":
		return a.Equal(b), nil
	default:
		return false, fmt.Errorf("unsupported time compare operator: %s", op)
	}
}

func parseParamPath(path string) (location, name string, err error) {
	if !strings.HasPrefix(path, ".") {
		return "", "", fmt.Errorf("invalid param path: %s", path)
	}
	parts := strings.Split(strings.TrimPrefix(path, "."), ".")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid param path: %s", path)
	}
	return parts[0], parts[1], nil
}

func toFloat64(v any) (float64, error) {
	switch t := v.(type) {
	case int:
		return float64(t), nil
	case int64:
		return float64(t), nil
	case float64:
		return t, nil
	case string:
		return strconv.ParseFloat(t, 64)
	default:
		return 0, fmt.Errorf("cannot convert to float: %v", v)
	}
}

func addSmartDuration(t time.Time, expr string) (time.Time, error) {
	re := regexp.MustCompile(`(?i)([+-]?\d+)([yMwdhms])`)
	matches := re.FindAllStringSubmatch(expr, -1)
	if matches == nil {
		return t, fmt.Errorf("invalid duration: %s", expr)
	}
	for _, m := range matches {
		num, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "y":
			t = t.AddDate(num, 0, 0)
		case "M":
			t = t.AddDate(0, num, 0)
		case "w":
			t = t.AddDate(0, 0, 7*num)
		case "d":
			t = t.AddDate(0, 0, num)
		case "h":
			t = t.Add(time.Duration(num) * time.Hour)
		case "m":
			t = t.Add(time.Duration(num) * time.Minute)
		case "s":
			t = t.Add(time.Duration(num) * time.Second)
		}
	}
	return t, nil
}

func toTime(value any, format string) (time.Time, error) {
	switch t := value.(type) {
	case time.Time:
		return t, nil
	case string:
		value = strings.TrimSpace(t)
		if strings.HasPrefix(t, "now") {
			offset := strings.TrimSpace(strings.TrimPrefix(t, "now"))
			now := nowFunc()

			if offset == "" {
				return now, nil
			}

			// Matches things like "+1d", "- 2h", etc.
			re := regexp.MustCompile(`^([+-])\s*(\d+[a-zA-Z]*)$`)
			matches := re.FindStringSubmatch(offset)
			if len(matches) != 3 {
				return time.Time{}, fmt.Errorf("invalid now offset: %s", offset)
			}

			// sign := matches[1]
			// durStr := matches[2]
			// dur, err := str2duration.ParseDuration(durStr)
			now, err := addSmartDuration(now, offset)
			if err != nil {
				return time.Time{}, fmt.Errorf("invalid duration: %w", err)
			}

			// if sign == "-" {
			// 	now = now.Add(-dur)
			// } else {
			// 	now = now.Add(dur)
			// }
			return now, nil
		}

		// Regular timestamp
		return time.Parse(format, t)
	default:
		return time.Time{}, fmt.Errorf("cannot convert to time: %v", value)
	}
}

func (p *Paginator) shouldStop(body interface{}) (bool, error) {
	for _, cond := range p.config.Pagination.StopOn {
		switch cond.Type {
		case "responseBody":
			res, err := evalJQ(cond.Expression, body)
			if err != nil {
				return false, err
			}
			if b, ok := res.(bool); ok && b {
				return true, nil
			}

		case "requestParam":
			paramLoc, paramName, err := parseParamPath(cond.Param)
			if err != nil {
				return false, err
			}
			// Lookup param definition for correct type/format
			var paramDef *Param
			for _, pdef := range p.config.Pagination.Params {
				if pdef.Location == paramLoc && pdef.Name == paramName {
					paramDef = &pdef
					break
				}
			}
			if paramDef == nil {
				return false, fmt.Errorf("param definition not found for %s", cond.Param)
			}

			val := p.ctx[paramName]
			if val == nil {
				continue
			}

			ok, err := compareValues(*paramDef, val, cond.Value, cond.Compare)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// Next advances the paginator and returns query/body/header params for the next request
func (p *Paginator) Next(resp *http.Response) (*RequestParts, bool, error) {
	if p.stopped {
		return nil, true, nil
	}

	// Step 1: Read body into buffer
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, false, fmt.Errorf("failed to read body: %w", err)
	}

	// Step 2: Restore body for future use
	resp.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))

	// Step 3: Decode into JSON
	var bodyJSON interface{}
	if err := json.Unmarshal(buf.Bytes(), &bodyJSON); err != nil {
		return nil, false, fmt.Errorf("failed to decode body: %w", err)
	}

	headers := map[string][]string(resp.Header)

	if err := p.extractDynamicParams(bodyJSON, headers); err != nil {
		return nil, false, err
	}

	if err := p.applyIncrements(); err != nil {
		return nil, false, err
	}

	stop, err := p.shouldStop(bodyJSON)
	if err != nil {
		return nil, false, err
	}
	if stop {
		p.stopped = true
		return nil, true, nil
	}

	q := make(map[string]string)
	h := make(map[string]string)
	b := make(map[string]interface{})

	for _, param := range p.config.Pagination.Params {
		val := p.ctx[param.Name]
		switch param.Location {
		case "query":
			q[param.Name] = fmt.Sprintf("%v", val)
		case "header":
			h[param.Name] = fmt.Sprintf("%v", val)
		case "body":
			b[param.Name] = val
		}
	}

	return &RequestParts{
		QueryParams: q,
		BodyParams:  b,
		Headers:     h,
	}, false, nil
}
