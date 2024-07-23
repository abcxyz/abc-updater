// Copyright 2024 The Authors (see AUTHORS file)
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/thejerf/slogassert"

	"github.com/abcxyz/abc-updater/pkg/metrics"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
)

// Assert testMetricsDB satisfies pkg.MetricsLookuper.
var _ MetricsLookuper = (*testMetricsDB)(nil)

var testInstallTime = mustMarshal(time.Date(2024, 7, 3, 2, 8, 0, 0, time.UTC))

func mustMarshal(in time.Time) string {
	buf, err := in.MarshalText()
	if err != nil {
		panic(fmt.Errorf("couldn't marshal time: %w", err))
	}
	return string(buf)
}

type testMetricsDB struct {
	apps map[string]*AppMetrics
}

// Update is a Noop.
func (db *testMetricsDB) Update(ctx context.Context, params *MetricsLoadParams) error {
	return nil
}

func (db *testMetricsDB) GetAllowedMetrics(appID string) (*AppMetrics, error) {
	if db.apps == nil {
		return nil, fmt.Errorf("no metric definition found for app %s", appID)
	}
	v, ok := db.apps[appID]
	if !ok {
		return nil, fmt.Errorf("no metric definition found for app %s", appID)
	}
	return v, nil
}

func marshalRequest(tb testing.TB, req *metrics.SendMetricRequest) io.Reader {
	tb.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		tb.Fatalf("could not marshal json: %s", err.Error())
	}
	return bytes.NewReader(b)
}

func TestHandleMetric(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		db         MetricsLookuper
		body       io.Reader
		wantStatus int
		wantLogs   map[*slogassert.LogMessageMatch]int
	}{
		{
			name: "happy_single_metric",
			db: &testMetricsDB{apps: map[string]*AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body: marshalRequest(t, &metrics.SendMetricRequest{
				AppID:       "test",
				AppVersion:  "1.0",
				Metrics:     map[string]int64{"foo": 1},
				InstallTime: testInstallTime,
			}),
			wantStatus: 202,
			wantLogs: map[*slogassert.LogMessageMatch]int{{
				Message: "metric received",
				Level:   slog.LevelInfo,
				Attrs: map[string]any{
					"metric.app_id":       "test",
					"metric.app_version":  "1.0",
					"metric.name":         "foo",
					"metric.count":        1,
					"metric.install_time": testInstallTime,
				},
				AllAttrsMatch: false,
			}: 1},
		},
		{
			name: "happy_multi_metric",
			db: &testMetricsDB{apps: map[string]*AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body: marshalRequest(t, &metrics.SendMetricRequest{
				AppID:      "test",
				AppVersion: "1.0",
				Metrics: map[string]int64{
					"foo": 1,
					"bar": 2,
				},
				InstallTime: testInstallTime,
			}),
			wantStatus: 202,
			wantLogs: map[*slogassert.LogMessageMatch]int{
				{
					Message: "metric received",
					Level:   slog.LevelInfo,
					Attrs: map[string]any{
						"metric.app_id":       "test",
						"metric.app_version":  "1.0",
						"metric.name":         "foo",
						"metric.count":        1,
						"metric.install_time": testInstallTime,
					},
					AllAttrsMatch: false,
				}: 1,
				{
					Message: "metric received",
					Level:   slog.LevelInfo,
					Attrs: map[string]any{
						"metric.app_id":       "test",
						"metric.app_version":  "1.0",
						"metric.name":         "bar",
						"metric.count":        2,
						"metric.install_time": testInstallTime,
					},
					AllAttrsMatch: false,
				}: 1,
			},
		},
		{
			name: "happy_unknown_metric",
			db: &testMetricsDB{apps: map[string]*AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body: marshalRequest(t, &metrics.SendMetricRequest{
				AppID:      "test",
				AppVersion: "1.0",
				Metrics: map[string]int64{
					"foo":     1,
					"unknown": 2,
				},
				InstallTime: testInstallTime,
			}),
			wantStatus: 202,
			wantLogs: map[*slogassert.LogMessageMatch]int{{
				Message: "metric received",
				Level:   slog.LevelInfo,
				Attrs: map[string]any{
					"metric.app_id":       "test",
					"metric.app_version":  "1.0",
					"metric.name":         "foo",
					"metric.count":        1,
					"metric.install_time": testInstallTime,
				},
				AllAttrsMatch: false,
			}: 1},
		},
		{
			name: "unknown_app_returns_404",
			db: &testMetricsDB{apps: map[string]*AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body: marshalRequest(t, &metrics.SendMetricRequest{
				AppID:      "unknown",
				AppVersion: "1.0",
				Metrics: map[string]int64{
					"foo":     1,
					"unknown": 2,
				},
				InstallTime: testInstallTime,
			}),
			wantStatus: 404,
		},
		{
			name: "malformed_request_returns_400",
			db: &testMetricsDB{apps: map[string]*AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body:       strings.NewReader("40t9u2rgo2gh09joqijgo0194u0{{{{}}}}{+{}{}"),
			wantStatus: 400,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			h, err := renderer.New(ctx, nil,
				renderer.WithOnError(func(err error) {
					t.Fatalf("failed to render: %s", err.Error())
				}))
			if err != nil {
				t.Fatalf("failed to setup test: %s", err.Error())
			}
			req := httptest.NewRequest(http.MethodPost, "/sendMetrics", tc.body)
			req.Header.Set("User-Agent", "github.com/abcxyz/abc-updater")
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")
			logHandler := slogassert.New(t, slog.LevelInfo, nil)
			req = req.WithContext(logging.WithLogger(req.Context(), slog.New(logHandler)))

			w := httptest.NewRecorder()
			HandleMetric(h, tc.db).ServeHTTP(w, req)
			response := w.Result()
			defer response.Body.Close()

			if got, want := response.StatusCode, tc.wantStatus; got != want {
				t.Errorf("unexpected response code. got %d want %d", got, want)
			}

			// Normally we wouldn't test log messages, but as that is the way metrics
			// are being exported, it seems important to do so here.
			for k, want := range tc.wantLogs {
				// TODO: Switch to logHandler.Assert() if https://github.com/thejerf/slogassert/pull/5 is merged.
				// This violates https://google.github.io/styleguide/go/decisions#assertion-libraries
				// in it's current state, if pr is merged I will be able to avoid that
				// path.
				if got := logHandler.AssertSomePrecise(*k); got != want {
					t.Errorf("Unexpected number of logs containing [%v]. Got [%d], want [%d]", k, got, want)
				}
			}
		})
	}
}
