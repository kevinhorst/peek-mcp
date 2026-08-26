package telemetry

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	metricActiveTime = "claude_code.active_time.total"
	metricCostUsage  = "claude_code.cost.usage"

	temporalityDelta = 1

	sessionIdAttribute = "session.id"

	eventNamePrefix   = "claude_code."
	eventToolDecision = "claude_code.tool_decision"
	eventToolResult   = "claude_code.tool_result"

	attrEventName      = "event.name"
	attrToolName       = "tool_name"
	attrToolUseId      = "tool_use_id"
	attrDecision       = "decision"
	attrDecisionSource = "source"
	attrToolInput      = "tool_input"

	sourceConfig        = "config"
	sourceHook          = "hook"
	sourceUserTemporary = "user_temporary"
	sourceUserPermanent = "user_permanent"
	sourceUserReject    = "user_reject"
	sourceUserAbort     = "user_abort"
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

type exportLogsRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type scopeLogs struct {
	LogRecords []logRecord `json:"logRecords"`
}

type logRecord struct {
	Attributes   []keyValue `json:"attributes"`
	Body         anyValue   `json:"body"`
	TimeUnixNano string     `json:"timeUnixNano"`
}

func (s *Store) IngestLogs(body []byte) error {
	var request exportLogsRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return err
	}

	for _, resource := range request.ResourceLogs {
		for _, scope := range resource.ScopeLogs {
			for index := range scope.LogRecords {
				s.ingestLogRecord(&scope.LogRecords[index])
			}
		}
	}
	return nil
}

func (s *Store) ingestLogRecord(record *logRecord) {
	sessionId := attributeValue(record.Attributes, sessionIdAttribute)
	if sessionId == "" {
		return
	}

	switch logEventName(record) {
	case eventToolDecision:
		s.foldDecision(sessionId, PermissionDecision{
			Decision:  attributeValue(record.Attributes, attrDecision),
			Source:    attributeValue(record.Attributes, attrDecisionSource),
			Timestamp: timeFromUnixNano(record.TimeUnixNano),
			Tool:      attributeValue(record.Attributes, attrToolName),
			ToolUseId: attributeValue(record.Attributes, attrToolUseId),
		})
	case eventToolResult:
		command := commandFromToolInput(attributeValue(record.Attributes, attrToolInput))
		s.enrichCommand(sessionId, attributeValue(record.Attributes, attrToolUseId), command)
	}
}

// logEventName returns the prefixed event name; Claude Code emits it in the
// record body and repeats it unprefixed in the event.name attribute.
func logEventName(record *logRecord) string {
	if name := record.Body.StringValue; name != "" {
		return name
	}
	if name := attributeValue(record.Attributes, attrEventName); name != "" {
		return eventNamePrefix + name
	}
	return ""
}

// toolInput mirrors the fields peek extracts from denied tool inputs on the
// transcript side (claude.deniedToolInput).
type toolInput struct {
	Command      string `json:"command"`
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
}

func commandFromToolInput(encoded string) string {
	if encoded == "" {
		return ""
	}

	var input toolInput
	if err := json.Unmarshal([]byte(encoded), &input); err != nil {
		return ""
	}
	if input.Command != "" {
		return input.Command
	}
	if input.FilePath != "" {
		return input.FilePath
	}
	return input.NotebookPath
}

func timeFromUnixNano(value string) time.Time {
	nanos, err := strconv.ParseInt(value, 10, 64)
	if err != nil || nanos <= 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
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
