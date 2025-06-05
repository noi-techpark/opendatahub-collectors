package crawler_testing

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
)

type MockRoundTripper struct {
	MockMap map[string]string // normalized URL => filepath
}

func NewMockRoundTripper(config map[string]string) *MockRoundTripper {
	return &MockRoundTripper{MockMap: normalizeMapKeys(config)}
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	normalized := normalizeURL(req.URL)

	filePath, ok := m.MockMap[normalized]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "mock not found"}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       io.NopCloser(bytes.NewBufferString(`{"error": "failed to read mock"}`)),
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Request:    req,
		}, nil
	}

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBuffer(data)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

func normalizeMapKeys(input map[string]string) map[string]string {
	output := make(map[string]string)
	for raw, path := range input {
		parsed, err := url.Parse(raw)
		if err != nil {
			continue // or panic if strict
		}
		normalized := normalizeURL(parsed)
		output[normalized] = path
	}
	return output
}

// Normalize URL by sorting query params and stripping trailing slash
func normalizeURL(u *url.URL) string {
	base := u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/")
	params := u.Query()

	var sorted []string
	for k, vs := range params {
		for _, v := range vs {
			sorted = append(sorted, url.QueryEscape(k)+"="+url.QueryEscape(v))
		}
	}
	sort.Strings(sorted)

	if len(sorted) > 0 {
		return base + "?" + strings.Join(sorted, "&")
	}
	return base
}
