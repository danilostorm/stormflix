package httpapi

import (
	"net/http"
	"time"
)

type homeClientTelemetryInput struct {
	FirstContentMS float64 `json:"first_content_ms"`
	ResponseMS     float64 `json:"response_ms"`
	RenderMS       float64 `json:"render_ms"`
}

func (s *server) homeClientTelemetry(w http.ResponseWriter, r *http.Request) {
	var in homeClientTelemetryInput
	if decodeJSON(w, r, &in) != nil {
		return
	}
	in.FirstContentMS = boundedMetric(in.FirstContentMS, 300000)
	in.ResponseMS = boundedMetric(in.ResponseMS, 300000)
	in.RenderMS = boundedMetric(in.RenderMS, 300000)
	s.homeMetrics.ObserveClient(millisecondsDuration(in.FirstContentMS), millisecondsDuration(in.ResponseMS), millisecondsDuration(in.RenderMS))
	w.WriteHeader(http.StatusNoContent)
}

func millisecondsDuration(value float64) time.Duration {
	return time.Duration(value * float64(time.Millisecond))
}
