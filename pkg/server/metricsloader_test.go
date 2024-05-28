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
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/renderer"
	"github.com/abcxyz/pkg/testutil"
)

func setupTestServer(tb testing.TB, allowed map[string]*AllowedMetricsResponse, returnErrorCode int) *httptest.Server {
	tb.Helper()
	ren, err := renderer.New(context.Background(), nil, renderer.WithOnError(func(err error) {
		tb.Fatalf("error rendering json in test server: %s", err.Error())
	}))
	if err != nil {
		tb.Fatalf("error creating renderer for test server: %s", err.Error())
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if returnErrorCode != 0 {
			ren.RenderJSON(w, returnErrorCode, fmt.Errorf("something went wrong for testing purposes"))
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/manifest.json":
			appList := make([]string, 0, len(allowed))
			for k := range allowed {
				appList = append(appList, k)
			}
			response := ManifestResponse{appList}
			ren.RenderJSON(w, http.StatusOK, &response)
			return

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/metrics.json"):
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) >= 2 {
				if appID := parts[len(parts)-2]; appID != "" {
					if v, ok := allowed[appID]; ok && v != nil {
						ren.RenderJSON(w, http.StatusOK, &v)
						return
					}
				}
			}
			// Technically this is xml with current implementation, but we don't care about parsing error bodies.
			ren.RenderJSON(w, http.StatusNotFound, fmt.Errorf("noSuchKey"))
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return

		default:
			// Technically this is xml with current implementation, but we don't care about parsing error bodies.
			ren.RenderJSON(w, http.StatusNotFound, fmt.Errorf("noSuchKey"))
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}
	}))

	tb.Cleanup(func() {
		ts.Close()
	})
	return ts
}

func TestMetricsDB_GetAllowedMetrics(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		state     map[string]*AppMetrics
		appID     string
		want      *AppMetrics
		wantError string
	}{
		{
			name: "happy",
			state: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
			},
			appID: "foo",
			want: &AppMetrics{
				AppID: "foo",
				Allowed: map[string]interface{}{
					"metric1": struct{}{},
					"metric2": struct{}{},
				},
			},
		},
		{
			name:      "unhappy_nil_map_returns_error",
			appID:     "foo",
			want:      nil,
			wantError: "no metric definition found for app",
		},
		{
			name: "unhappy_missing_app_returns error",
			state: map[string]*AppMetrics{
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
			},
			appID:     "foo",
			want:      nil,
			wantError: "no metric definition found for app",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			db := MetricsDB{apps: tc.state}
			got, err := db.GetAllowedMetrics(tc.appID)
			if diff := testutil.DiffErrString(err, tc.wantError); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("unexpected output. Diff (-got +want): %s", diff)
			}
		})
	}
}

func TestMetricsDB_Update(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name            string
		before          map[string]*AppMetrics
		serverMap       map[string]*AllowedMetricsResponse
		returnErrorCode int
		want            map[string]*AppMetrics
		wantError       string
	}{
		{
			name: "happy_first_update",
			serverMap: map[string]*AllowedMetricsResponse{
				"foo": {Metrics: []string{"metric1", "metric2"}},
				"bar": {Metrics: []string{"metric1"}},
			},
			want: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
		},
		{
			name: "happy_successive_update",
			before: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
			serverMap: map[string]*AllowedMetricsResponse{
				"foo": {Metrics: []string{"metric1", "metric3"}},
				"baz": {Metrics: []string{"metric1"}},
			},
			want: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric3": struct{}{},
					},
				},
				"baz": {
					AppID: "baz",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
		},
		{
			name: "unhappy_app_fetch_uses_config_value",
			before: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
			serverMap: map[string]*AllowedMetricsResponse{
				"foo": {Metrics: []string{"metric1", "metric3"}},
				"bar": nil,
				"baz": {Metrics: []string{"metric1"}},
			},
			want: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric3": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
				"baz": {
					AppID: "baz",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
		},
		{
			name: "unhappy_cannot_load_manifest_noop_returns_error",
			before: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
			returnErrorCode: http.StatusInternalServerError,
			wantError:       "could not load manifest",
			want: map[string]*AppMetrics{
				"foo": {
					AppID: "foo",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
						"metric2": struct{}{},
					},
				},
				"bar": {
					AppID: "bar",
					Allowed: map[string]interface{}{
						"metric1": struct{}{},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			ts := setupTestServer(t, tc.serverMap, tc.returnErrorCode)

			params := MetricsLoadParams{
				ServerURL: ts.URL,
				Client:    http.DefaultClient,
			}

			db := &MetricsDB{
				apps: tc.before,
			}

			if diff := testutil.DiffErrString(db.Update(ctx, &params), tc.wantError); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(db.apps, tc.want); diff != "" {
				t.Errorf("unexpected end state. Diff: (-got +want): %s", diff)
			}
		})
	}
}
