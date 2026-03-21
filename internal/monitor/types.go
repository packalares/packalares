package monitor

import (
	"fmt"
	"strconv"
)

// KSMetricsResponse matches the KubeSphere monitoring API response format.
type KSMetricsResponse struct {
	Results []KSMetric `json:"results"`
}

// KSMetric matches the KubeSphere monitoring Metric type.
type KSMetric struct {
	MetricName string       `json:"metric_name,omitempty"`
	MetricData KSMetricData `json:"data,omitempty"`
	Error      string       `json:"error,omitempty"`
}

// KSMetricData matches the KubeSphere MetricData type.
type KSMetricData struct {
	ResultType   string          `json:"resultType,omitempty"`
	MetricValues []KSMetricValue `json:"result,omitempty"`
}

// KSMetricValue matches the KubeSphere MetricValue type.
type KSMetricValue struct {
	Metadata map[string]string `json:"metric,omitempty"`
	Sample   *KSPoint          `json:"value,omitempty"`
	Series   []KSPoint         `json:"values,omitempty"`
}

// KSPoint is a [timestamp, value] pair matching KubeSphere format.
// The first element is unix timestamp, second is the metric value.
type KSPoint [2]float64

func (p KSPoint) Timestamp() float64 {
	return p[0]
}

func (p KSPoint) Value() float64 {
	return p[1]
}

// MarshalJSON encodes the point as [timestamp, "value_string"] to match KubeSphere format.
func (p KSPoint) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%v,%q]", p[0], strconv.FormatFloat(p[1], 'f', -1, 64))), nil
}
