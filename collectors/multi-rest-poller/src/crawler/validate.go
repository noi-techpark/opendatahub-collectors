package crawler

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Message  string
	Location string // optional, e.g. "steps[0].request.url"
}

func (e ValidationError) Error() string {
	if e.Location != "" {
		return fmt.Sprintf("%s: %s", e.Location, e.Message)
	}
	return e.Message
}

func ValidateConfig(cfg Config) []ValidationError {
	var errs []ValidationError

	// rootContext required, must be [] or map
	if cfg.RootContext == nil {
		errs = append(errs, ValidationError{"rootContext is required", "rootContext"})
	} else {
		switch cfg.RootContext.(type) {
		case []interface{}:
		case map[string]interface{}:
		default:
			errs = append(errs, ValidationError{"rootContext must be [] or {}", "rootContext"})
		}
	}

	// stream requires rootContext to be []interface{}
	if cfg.Stream {
		if _, ok := cfg.RootContext.([]interface{}); !ok {
			errs = append(errs, ValidationError{"stream=true requires rootContext to be an array", "stream"})
		}
	}

	// validate Authentication if present
	if cfg.Authentication != nil {
		errs = append(errs, validateAuth(*cfg.Authentication, "auth")...)
	}

	// headers optional, but if present must be map[string]string (assumed unmarshalled correctly)

	// steps required and non-empty
	if len(cfg.Steps) == 0 {
		errs = append(errs, ValidationError{"steps must be a non-empty array", "steps"})
	} else {
		for i, step := range cfg.Steps {
			errs = append(errs, validateStep(step, fmt.Sprintf("steps[%d]", i))...)
		}
	}

	return errs
}

func validateAuth(auth AuthenticatorConfig, location string) []ValidationError {
	var errs []ValidationError

	t := strings.ToLower(auth.Type)
	if t != "basic" && t != "bearer" && t != "oauth" {
		errs = append(errs, ValidationError{fmt.Sprintf("auth.type must be one of [basic, bearer, oauth], got '%s'", auth.Type), location + ".type"})
	}

	if t == "bearer" && auth.Token == "" {
		errs = append(errs, ValidationError{"auth.token is required when type is bearer", location + ".token"})
	}

	if t == "oauth" {
		if auth.Method == "" {
			errs = append(errs, ValidationError{"auth.method is required when type is oauth", location + ".method"})
		} else if auth.Method != "password" && auth.Method != "client_credentials" {
			errs = append(errs, ValidationError{"auth.method must be password or client_credentials", location + ".method"})
		}
		if auth.TokenURL == "" {
			errs = append(errs, ValidationError{"auth.tokenUrl is required when type is oauth", location + ".tokenUrl"})
		}

		if auth.Method == "client_credentials" {
			if auth.ClientID == "" {
				errs = append(errs, ValidationError{"auth.clientId is required when method is client_credentials", location + ".clientId"})
			}
			if auth.ClientSecret == "" {
				errs = append(errs, ValidationError{"auth.clientSecret is required when method is client_credentials", location + ".clientSecret"})
			}
		}

		if auth.Method == "password" {
			if auth.Username == "" {
				errs = append(errs, ValidationError{"auth.username is required when method is password", location + ".username"})
			}
			if auth.Password == "" {
				errs = append(errs, ValidationError{"auth.password is required when method is password", location + ".password"})
			}
		}
	}

	if t == "basic" {
		if auth.Username == "" {
			errs = append(errs, ValidationError{"auth.username is required when type is basic", location + ".username"})
		}
		if auth.Password == "" {
			errs = append(errs, ValidationError{"auth.password is required when type is basic", location + ".password"})
		}
	}

	return errs
}

func validateStep(step Step, location string) []ValidationError {
	var errs []ValidationError

	t := strings.ToLower(step.Type)
	if t != "foreach" && t != "request" {
		errs = append(errs, ValidationError{fmt.Sprintf("step.type must be 'foreach' or 'request', got '%s'", step.Type), location + ".type"})
		return errs
	}

	if t == "foreach" {
		// foreach rules
		if step.Path == "" {
			errs = append(errs, ValidationError{"foreach step requires path", location + ".path"})
		}
		if step.As == "" {
			errs = append(errs, ValidationError{"foreach step requires as", location + ".as"})
		}
		// if len(step.Steps) == 0 {
		// 	errs = append(errs, ValidationError{"foreach step requires nested steps", location + ".steps"})
		// }
		// Validate nested steps
		for i, nested := range step.Steps {
			errs = append(errs, validateStep(nested, fmt.Sprintf("%s.steps[%d]", location, i))...)
		}

		// MergeWithContext if present
		if step.MergeWithContext != nil {
			if step.MergeWithContext.Name == "" {
				errs = append(errs, ValidationError{"mergeWithContext.name is required", location + ".mergeWithContext.name"})
			}
			if step.MergeWithContext.Rule == "" {
				errs = append(errs, ValidationError{"mergeWithContext.rule is required", location + ".mergeWithContext.rule"})
			}
		}

	} else if t == "request" {
		// request step rules
		if step.Request == nil {
			errs = append(errs, ValidationError{"request step requires a request field", location + ".request"})
			return errs
		}
		errs = append(errs, validateRequest(*step.Request, location+".request")...)

		// Validate nested steps if any
		for i, nested := range step.Steps {
			errs = append(errs, validateStep(nested, fmt.Sprintf("%s.steps[%d]", location, i))...)
		}
	}

	// Validate mergeOn and mergeWithParentOn if present (just presence + syntax of jq could be checked elsewhere)
	if step.MergeOn != "" {
		// could validate jq here with gojq.Parse(step.MergeOn)
	}
	if step.MergeWithParentOn != "" {
		// could validate jq here with gojq.Parse(step.MergeWithParentOn)
	}

	return errs
}

func validateRequest(req RequestConfig, location string) []ValidationError {
	var errs []ValidationError

	if req.URL == "" {
		errs = append(errs, ValidationError{"request.url is required", location + ".url"})
	}
	if req.Method == "" {
		errs = append(errs, ValidationError{"request.method is required", location + ".method"})
	} else {
		m := strings.ToUpper(req.Method)
		if m != "GET" && m != "POST" {
			errs = append(errs, ValidationError{"request.method must be GET or POST", location + ".method"})
		}
	}

	if req.Authentication != nil {
		errs = append(errs, validateAuth(*req.Authentication, location+".auth")...)
	}

	if len(req.Pagination.Params) > 0 || len(req.Pagination.StopOn) > 0 {
		errs = append(errs, validatePagination(req.Pagination, location+".pagination")...)
	}

	// headers and body can be left as is for now

	return errs
}

func validatePagination(p Pagination, location string) []ValidationError {
	var errs []ValidationError

	if len(p.Params) == 0 {
		errs = append(errs, ValidationError{"pagination.params must be a non-empty array", location + ".params"})
	}
	for i, param := range p.Params {
		errs = append(errs, validatePaginationParam(param, fmt.Sprintf("%s.params[%d]", location, i))...)
	}
	if len(p.StopOn) == 0 {
		errs = append(errs, ValidationError{"pagination.stopOn must be a non-empty array", location + ".stopOn"})
	}
	for i, stop := range p.StopOn {
		errs = append(errs, validatePaginationStop(stop, fmt.Sprintf("%s.stopOn[%d]", location, i))...)
	}
	return errs
}

func validatePaginationParam(param Param, location string) []ValidationError {
	var errs []ValidationError

	if param.Name == "" {
		errs = append(errs, ValidationError{"pagination param name is required", location + ".name"})
	}
	if param.Location != "query" && param.Location != "body" && param.Location != "header" {
		errs = append(errs, ValidationError{"pagination param location must be one of [query, body, header]", location + ".location"})
	}
	typ := strings.ToLower(param.Type)
	if typ != "int" && typ != "float" && typ != "datetime" && typ != "dynamic" {
		errs = append(errs, ValidationError{"pagination param type must be one of [int, float, datetime, dynamic]", location + ".type"})
	}
	if typ == "datetime" && param.Format == "" {
		errs = append(errs, ValidationError{"pagination param format is required when type is datetime", location + ".format"})
	}
	if typ == "dynamic" && param.Source == "" {
		errs = append(errs, ValidationError{"pagination param source is required when type is dynamic", location + ".source"})
	}
	// Default can be anything, skipping type check here

	return errs
}

func validatePaginationStop(stop StopCondition, location string) []ValidationError {
	var errs []ValidationError

	t := strings.ToLower(stop.Type)
	if t != "responsebody" && t != "requestparam" {
		errs = append(errs, ValidationError{"pagination stop type must be one of [responseBody, requestParam]", location + ".type"})
	}

	if t == "responsebody" && stop.Expression == "" {
		errs = append(errs, ValidationError{"pagination stop expression is required when type is responseBody", location + ".expression"})
	}
	if t == "requestparam" {
		if stop.Param == "" {
			errs = append(errs, ValidationError{"pagination stop param is required when type is requestParam", location + ".param"})
		}
		if stop.Compare == "" {
			errs = append(errs, ValidationError{"pagination stop compare is required when type is requestParam", location + ".compare"})
		} else {
			cmp := strings.ToLower(stop.Compare)
			validCmp := map[string]bool{"lt": true, "lte": true, "eq": true, "gt": true, "gte": true}
			if !validCmp[cmp] {
				errs = append(errs, ValidationError{"pagination stop compare must be one of [lt, lte, eq, gt, gte]", location + ".compare"})
			}
		}
		if stop.Value == nil {
			errs = append(errs, ValidationError{"pagination stop value is required when type is requestParam", location + ".value"})
		}
	}

	return errs
}
