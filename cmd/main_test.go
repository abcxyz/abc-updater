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

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/thejerf/slogassert"

	"github.com/abcxyz/abc-updater/pkg/metrics"
	"github.com/abcxyz/abc-updater/pkg/server"
	"github.com/abcxyz/pkg/logging"
	"github.com/abcxyz/pkg/renderer"
)

// Assert testMetricsDB satisfies pkg.MetricsLookuper.
var _ server.MetricsLookuper = (*testMetricsDB)(nil)

type testMetricsDB struct {
	apps map[string]*server.AppMetrics
}

// Update is a Noop.
func (db *testMetricsDB) Update(ctx context.Context, params *server.MetricsLoadParams) error {
	return nil
}

func (db *testMetricsDB) GetAllowedMetrics(appID string) (*server.AppMetrics, error) {
	if db.apps == nil {
		// TODO: this should probably log an error and bubble up as a 5xx
		return nil, fmt.Errorf("no metric definition found for app %s", appID)
	}
	v, ok := db.apps[appID]
	// TODO: this should bubble up as a 404
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
		db         server.MetricsLookuper
		body       io.Reader
		wantStatus int
		wantLogs   map[*slogassert.LogMessageMatch]int
	}{
		// TODO: more test cases if by some miracle this ugly thing doesn't get ousted in code review
		{
			name: "happy_single_metric",
			db: &testMetricsDB{apps: map[string]*server.AppMetrics{"test": {
				AppID: "test",
				Allowed: map[string]interface{}{
					"foo": struct{}{},
					"bar": struct{}{},
				},
			}}},
			body: marshalRequest(t, &metrics.SendMetricRequest{
				AppID:      "test",
				AppVersion: "1.0",
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  "asdf",
			}),
			wantStatus: 202,
			wantLogs: map[*slogassert.LogMessageMatch]int{{
				Message: "metric received",
				Level:   slog.LevelInfo,
				Attrs: map[string]any{
					"metric.app_id":      "test",
					"metric.app_version": "1.0",
					"metric.name":        "foo",
					"metric.count":       1,
					"metric.install_id":  "asdf",
				},
				AllAttrsMatch: false,
			}: 1},
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
			handleMetric(h, tc.db).ServeHTTP(w, req)
			response := w.Result()
			defer response.Body.Close()

			if got, want := response.StatusCode, tc.wantStatus; got != want {
				t.Errorf("unexpected response code. got %d want %d", got, want)
			}

			// Normally we wouldn't test log messages, but as that is the way metrics
			// are being exported, it seems important to do so here.
			for k, want := range tc.wantLogs {
				// TODO: I don't like that this panics if there are no matches, would rather handle error myself
				// I have https://github.com/thejerf/slogassert/pull/5 to try and fix it.
				if got := logHandler.AssertSomePrecise(*k); got != want {
					t.Errorf("Unexpected number of logs containing [%v]. Got [%d], want [%d]", k, got, want)
				}
			}
		})
	}
}
