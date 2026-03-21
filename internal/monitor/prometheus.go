package monitor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PrometheusClient queries a Prometheus server.
type PrometheusClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPrometheusClient creates a new Prometheus client.
func NewPrometheusClient(baseURL string) *PrometheusClient {
	return &PrometheusClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// promResponse is the raw Prometheus API response.
type promResponse struct {
	Status string   `json:"status"`
	Data   promData `json:"data"`
	Error  string   `json:"error,omitempty"`
}

type promData struct {
	ResultType string           `json:"resultType"`
	Result     []promMetricItem `json:"result"`
}

type promMetricItem struct {
	Metric map[string]string `json:"metric"`
	Value  []interface{}     `json:"value,omitempty"`  // instant query
	Values [][]interface{}   `json:"values,omitempty"` // range query
}

// Query executes an instant query against Prometheus.
func (c *PrometheusClient) Query(query string, t time.Time) (KSMetricData, error) {
	params := url.Values{}
	params.Set("query", query)
	if !t.IsZero() {
		params.Set("time", strconv.FormatInt(t.Unix(), 10))
	}

	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/query?" + params.Encode())
	if err != nil {
		return KSMetricData{}, fmt.Errorf("prometheus query failed: %w", err)
	}
	defer resp.Body.Close()

	return c.parseResponse(resp)
}

// QueryRange executes a range query against Prometheus.
func (c *PrometheusClient) QueryRange(query string, start, end time.Time, step time.Duration) (KSMetricData, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("start", strconv.FormatInt(start.Unix(), 10))
	params.Set("end", strconv.FormatInt(end.Unix(), 10))
	params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', 0, 64))

	resp, err := c.httpClient.Get(c.baseURL + "/api/v1/query_range?" + params.Encode())
	if err != nil {
		return KSMetricData{}, fmt.Errorf("prometheus range query failed: %w", err)
	}
	defer resp.Body.Close()

	return c.parseResponse(resp)
}

func (c *PrometheusClient) parseResponse(resp *http.Response) (KSMetricData, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return KSMetricData{}, fmt.Errorf("failed to read response: %w", err)
	}

	var pr promResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return KSMetricData{}, fmt.Errorf("failed to parse response: %w", err)
	}

	if pr.Status != "success" {
		return KSMetricData{}, fmt.Errorf("prometheus error: %s", pr.Error)
	}

	return convertPromData(pr.Data), nil
}

// convertPromData converts raw Prometheus response data to KubeSphere format.
func convertPromData(pd promData) KSMetricData {
	md := KSMetricData{
		ResultType: pd.ResultType,
	}

	for _, item := range pd.Result {
		mv := KSMetricValue{
			Metadata: item.Metric,
		}

		if item.Value != nil && len(item.Value) == 2 {
			ts, val := parsePromValue(item.Value)
			point := KSPoint{ts, val}
			mv.Sample = &point
		}

		if item.Values != nil {
			for _, v := range item.Values {
				if len(v) == 2 {
					ts, val := parsePromValue(v)
					mv.Series = append(mv.Series, KSPoint{ts, val})
				}
			}
		}

		md.MetricValues = append(md.MetricValues, mv)
	}

	return md
}

func parsePromValue(v []interface{}) (float64, float64) {
	var ts, val float64

	switch t := v[0].(type) {
	case float64:
		ts = t
	case json.Number:
		ts, _ = t.Float64()
	}

	switch vv := v[1].(type) {
	case string:
		val, _ = strconv.ParseFloat(vv, 64)
	case float64:
		val = vv
	case json.Number:
		val, _ = vv.Float64()
	}

	return ts, val
}
