package telemetry

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	envEnableTelemetry = "CLAUDE_CODE_ENABLE_TELEMETRY"
	envMetricsExporter = "OTEL_METRICS_EXPORTER"
	envOtlpEndpoint    = "OTEL_EXPORTER_OTLP_ENDPOINT"
	envOtlpProtocol    = "OTEL_EXPORTER_OTLP_PROTOCOL"

	requiredProtocol = "http/json"
)

type ExportState string

const (
	ExportConfigured    ExportState = "configured"
	ExportMisconfigured ExportState = "misconfigured"
	ExportNotConfigured ExportState = "not_configured"
	ExportReceiving     ExportState = "receiving"
)

type ExportStatus struct {
	Detail string      `json:"detail,omitempty"`
	State  ExportState `json:"status"`
}

type claudeSettings struct {
	Env map[string]any `json:"env"`
}

type Detector struct {
	boundPort    int
	settingsPath string
}

func NewDetector(boundPort int, settingsPath string) *Detector {
	return &Detector{boundPort: boundPort, settingsPath: settingsPath}
}

func (d *Detector) Status() ExportStatus {
	settingsData, err := os.ReadFile(d.settingsPath)
	if err != nil {
		return notConfiguredStatus("no telemetry env in " + d.settingsPath + " (may still be enabled via shell env)")
	}

	var settings claudeSettings
	if err := json.Unmarshal(settingsData, &settings); err != nil {
		return notConfiguredStatus("cannot parse " + d.settingsPath)
	}

	enabled := envString(settings.Env, envEnableTelemetry)
	if enabled == "" {
		return notConfiguredStatus("no telemetry env in " + d.settingsPath + " (may still be enabled via shell env)")
	}
	isEnabled := enabled == "1" || strings.EqualFold(enabled, "true")
	if !isEnabled {
		return notConfiguredStatus(envEnableTelemetry + "=" + enabled)
	}

	problems := make([]string, 0)
	protocol := envString(settings.Env, envOtlpProtocol)
	if protocol != requiredProtocol {
		problems = append(problems, fmt.Sprintf("protocol %q (want %s)", protocol, requiredProtocol))
	}

	exporter := envString(settings.Env, envMetricsExporter)
	if !strings.Contains(exporter, "otlp") {
		problems = append(problems, fmt.Sprintf("metrics exporter %q (want otlp)", exporter))
	}

	if problem := d.endpointProblem(envString(settings.Env, envOtlpEndpoint)); problem != "" {
		problems = append(problems, problem)
	}

	if len(problems) > 0 {
		return ExportStatus{Detail: strings.Join(problems, "; "), State: ExportMisconfigured}
	}
	return ExportStatus{State: ExportConfigured}
}

func (d *Detector) endpointProblem(endpoint string) string {
	expected := fmt.Sprintf("http://127.0.0.1:%d/otlp", d.boundPort)
	if endpoint == "" {
		return "no " + envOtlpEndpoint + " (want " + expected + ")"
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Sprintf("endpoint %q is not a valid URL (want %s)", endpoint, expected)
	}

	mismatch := fmt.Sprintf("endpoint %q (want %s)", endpoint, expected)
	if parsed.Scheme != "http" {
		return mismatch
	}
	hostname := parsed.Hostname()
	isLoopback := hostname == "127.0.0.1" || hostname == "localhost"
	if !isLoopback {
		return mismatch
	}
	if parsed.Port() != strconv.Itoa(d.boundPort) {
		return mismatch
	}
	if strings.TrimSuffix(parsed.Path, "/") != "/otlp" {
		return mismatch
	}
	return ""
}

func notConfiguredStatus(detail string) ExportStatus {
	return ExportStatus{Detail: detail, State: ExportNotConfigured}
}

func envString(env map[string]any, key string) string {
	value, ok := env[key]
	if !ok {
		return ""
	}

	text, ok := value.(string)
	if ok {
		return text
	}
	return fmt.Sprint(value)
}
