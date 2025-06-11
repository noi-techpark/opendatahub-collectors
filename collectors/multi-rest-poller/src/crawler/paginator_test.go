package crawler

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type PaginatorMockHTTPResult struct {
	Body   string            `yaml:"body"`
	Header map[string]string `yaml:"header"`
}

type PaginatorTestFile struct {
	Configuration    ConfigP                   `yaml:"configuration"`
	HTTPResults      []PaginatorMockHTTPResult `yaml:"httpResults"`
	PaginationStates []RequestParts            `yaml:"paginationState"`
	InitialState     map[string]interface{}    `yaml:"initialState"`
	NowMock          string                    `yaml:"nowMock,omitempty"`
}

// LoadPaginatorTestFile loads a YAML file and returns the paginator config and mocked HTTP responses
func LoadPaginatorTestFile(path string) (*Paginator, []*http.Response, PaginatorTestFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, PaginatorTestFile{}, fmt.Errorf("failed to read file: %w", err)
	}

	var testFile PaginatorTestFile
	if err := yaml.Unmarshal(data, &testFile); err != nil {
		return nil, nil, PaginatorTestFile{}, fmt.Errorf("failed to parse yaml: %w", err)
	}

	nowFunc = func() time.Time {
		if len(testFile.NowMock) != 0 {
			ti, err := time.Parse(time.RFC3339, testFile.NowMock)
			if err != nil {
				panic(err)
			}
			return ti
		}
		return time.Now()
	}

	paginator, err := NewPaginator(testFile.Configuration)
	if err != nil {
		return nil, nil, PaginatorTestFile{}, err
	}

	var responses []*http.Response
	for _, r := range testFile.HTTPResults {
		body := io.NopCloser(strings.NewReader(r.Body))

		headers := http.Header{}
		for k, v := range r.Header {
			headers.Set(k, v)
		}

		responses = append(responses, &http.Response{
			StatusCode: 200,
			Body:       body,
			Header:     headers,
		})
	}

	return paginator, responses, testFile, nil
}

func runPaginatorTest(t *testing.T, path string, expectedSteps int) {
	p, responses, test, err := LoadPaginatorTestFile(path)
	require.NoError(t, err)
	require.Equalf(t, len(responses)-1, len(test.PaginationStates), "state <-> request length missmatch")

	defer func() { nowFunc = time.Now }()

	nowFunc = func() time.Time {
		if len(test.NowMock) != 0 {
			ti, err := time.Parse(time.RFC3339, test.NowMock)
			require.Nilf(t, err, "now mocker is not in RFC3339: %s", test.NowMock)
			return ti
		}
		return time.Now()
	}

	// Validate internal context at initialization (ctx)
	for key, expectedVal := range test.InitialState {
		actual, exists := p.Ctx()[key]
		require.Truef(t, exists, "Missing ctx key: %s", key)
		require.Equalf(t, expectedVal, actual, "Mismatch at initialization for key %s", key)
	}

	var step int
	stop := false
	for !stop {
		resp := responses[step]
		reqParts, done, err := p.Next(resp)
		require.NoError(t, err)
		if done {
			break
		}

		require.Less(t, step, len(test.PaginationStates), "did not stop")

		normalizeRequestParts := func(r *RequestParts) *RequestParts {
			if r.BodyParams == nil {
				r.BodyParams = map[string]interface{}{}
			}
			if r.Headers == nil {
				r.Headers = map[string]string{}
			}
			if r.QueryParams == nil {
				r.QueryParams = map[string]string{}
			}
			return r
		}

		// Validate internal context (ctx)
		require.EqualValuesf(t, normalizeRequestParts(&test.PaginationStates[step]), normalizeRequestParts(reqParts), "Mismatch at step %d", step)

		step++
	}
	require.Equalf(t, expectedSteps, step+1, "steps count missmatch")
}

func TestNowMocking(t *testing.T) {
	defer func() { nowFunc = time.Now }() // Restore original after test

	nowFunc = func() time.Time {
		return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	}

	dt, err := toTime("now +1d", time.RFC3339)
	require.NoError(t, err)
	assert.Equal(t, "2025-01-02T00:00:00Z", dt.Format(time.RFC3339))
}

func TestIntIncrement(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test1_int_increment.yaml", 3)
}

func TestDatetime(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test2_datetime.yaml", 3)
}

func TestNextToken(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test3_next_token.yaml", 3)
}

func TestEmpty(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test4_empty.yaml", 1)
}

func TestEmptyArray(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test5_empty_array.yaml", 1)
}

func TestDatetimeNow(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test6_now_datetime.yaml", 3)
}

func TestDatetimeNowMultiStop(t *testing.T) {
	runPaginatorTest(t, "testdata/paginator/test7_now_datetime_multistop.yaml", 2)
}
