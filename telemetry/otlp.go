package telemetry

import (
	"encoding/json"
	"strconv"
)

const (
	metricActiveTime = "claude_code.active_time.total"
	metricCostUsage  = "claude_code.cost.usage"

	temporalityDelta = 1

	sessionIdAttribute = "session.id"
)

type exportMetricsRequest struct {
	ResourceMetrics []resourceMetrics `json:"resourceMetrics"`
}

type resourceMetrics struct {
	ScopeMetrics []scopeMetrics `json:"scopeMetrics"`
}

type scopeMetrics struct {
	Metrics []metric `json:"metrics"`
}

type metric struct {
	Name string `json:"name"`
	Sum  *sum   `json:"sum"`
}

type sum struct {
	AggregationTemporality int         `json:"aggregationTemporality"`
	DataPoints             []dataPoint `json:"dataPoints"`
}

type dataPoint struct {
	Attributes []keyValue `json:"attributes"`
	AsDouble   *float64   `json:"asDouble"`
	AsInt      string     `json:"asInt"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

type anyValue struct {
	StringValue string `json:"stringValue"`
}

func (s *Store) IngestMetrics(body []byte) error {
	var request exportMetricsRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return err
	}

	for _, resource := range request.ResourceMetrics {
		for _, scope := range resource.ScopeMetrics {
			for index := range scope.Metrics {
				s.ingestMetric(&scope.Metrics[index])
			}
		}
	}
	return nil
}

func (s *Store) ingestMetric(m *metric) {
	if m.Sum == nil {
		return
	}
	if m.Name != metricActiveTime && m.Name != metricCostUsage {
		return
	}

	isDelta := m.Sum.AggregationTemporality == temporalityDelta
	for index := range m.Sum.DataPoints {
		point := &m.Sum.DataPoints[index]
		sessionId := attributeValue(point.Attributes, sessionIdAttribute)
		if sessionId == "" {
			continue
		}
		s.fold(sessionId, m.Name, pointValue(point), isDelta)
	}
}

func attributeValue(attributes []keyValue, key string) string {
	for _, attribute := range attributes {
		if attribute.Key == key {
			return attribute.Value.StringValue
		}
	}
	return ""
}

// pointValue prefers asDouble; OTLP JSON encodes int64 values as strings.
func pointValue(point *dataPoint) float64 {
	if point.AsDouble != nil {
		return *point.AsDouble
	}

	value, err := strconv.ParseInt(point.AsInt, 10, 64)
	if err != nil {
		return 0
	}
	return float64(value)
}
