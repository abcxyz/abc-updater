// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package server

import (
	"net/http"

	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
)

// TODO: figure out how to make modules so this doesn't get re-defined multiple places
type SendMetricRequest struct {
	// The ID of the application to check.
	AppID string `json:"appId"`

	// The version of the app to check for updates.
	// Should be of form vMAJOR[.MINOR[.PATCH[-PRERELEASE][+BUILD]]] (e.g., v1.0.1)
	AppVersion string `json:"appVersion"`

	// TODO: this is a bit different from design doc, is it ok?
	Metrics map[string]int64 `json:"metrics"`

	// InstallID. Expected to be a hex 8-4-4-4-12 formatted v4 UUID.
	InstallID string `json:"installId"`
}

// HandleMetric returns a http.Handler for processing POST requests for sending
// metrics.
func HandleMetric(h *renderer.Renderer, db MetricsLookuper) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := logging.FromContext(r.Context())
		metricLogger := logger.WithGroup("metric")
		logger.InfoContext(r.Context(), "handling request")

		metrics, err := DecodeRequest[SendMetricRequest](r.Context(), w, r, h)
		if err != nil {
			// Error response already handled by pkg.DecodeRequest.
			return
		}

		allowedMetrics, err := db.GetAllowedMetrics(metrics.AppID)
		if err != nil {
			h.RenderJSON(w, http.StatusNotFound, err)
			logger.WarnContext(r.Context(), "received metric request for unknown app")
			return
		}

		// Currently we only expose an API for a single metric on the client,
		// but I suspect multiple metrics will be added later on, and effort is
		// about the same to support both.
		for name, count := range metrics.Metrics {
			if allowedMetrics.MetricAllowed(name) {
				// TODO: does this leak sensitive information? Is default logger preferred.
				metricLogger.InfoContext(r.Context(), "metric received", "appID", metrics.AppID, "appVersion", metrics.AppVersion, "installID", metrics.InstallID, "name", name, "count", count)
			} else {
				// TODO: do we want to return a warning to client or fail silently?
				logger.WarnContext(r.Context(), "received unknown metric for app", "appID", metrics.AppID)
			}
		}

		// Client does not currently read body, future changes are acceptable.
		h.RenderJSON(w, http.StatusAccepted, map[string]string{"message": "ok"})
	})
}
