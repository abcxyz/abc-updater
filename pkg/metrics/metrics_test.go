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

package metrics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

const (
	testAppID     = "asdf"
	testVersion   = "1.0.0"
	testInstallID = "yv66vt6tvu8="
	testServerURL = "https://example.com"
)

func defaultClient() *client {
	return &client{
		AppID:      testAppID,
		Version:    testVersion,
		InstallID:  testInstallID,
		HTTPClient: &http.Client{Timeout: 1 * time.Second},
		OptOut:     false,
		Config: &metricsConfig{
			ServerURL: testServerURL,
			NoMetrics: false,
		},
	}
}

// Not all failure cases can be easily tested, will test subset that is easy
// to reproduce.
func Test_New_unhappy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		appID     string
		env       map[string]string
		want      *client
		wantError string
	}{
		{
			name:      "empty_app_id_fails",
			appID:     "",
			wantError: "appID cannot be empty",
		},
		{
			name:      "opt_out_env_noop_no_err",
			appID:     testAppID,
			env:       map[string]string{"NO_METRICS": "TRUE"},
			want:      NoopWriter().(*client),
			wantError: "",
		},
		{
			name:      "bad_url_noop",
			appID:     testAppID,
			env:       map[string]string{"METRICS_URL": "htttpq://%foo*(*fg.com4/\\"},
			wantError: "failed to parse server URL",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			c, err := New(ctx, tc.appID, "1", WithLookuper(envconfig.MapLookuper(tc.env)))
			if c == nil && tc.want != nil {
				t.Errorf("got nil MetricWriter but expected non-nil")
			}
			if c != nil {
				gotV := c.(*client)
				if diff := cmp.Diff(gotV, tc.want); diff != "" {
					t.Errorf("unexpected metricWriter value. Diff (-got +want): %s", diff)
				}
			}
			if diff := testutil.DiffErrString(err, tc.wantError); len(diff) != 0 {
				t.Errorf("unexpected error: %s", diff)
			}
		})
	}
}

func Test_New_Happy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		client    *http.Client
		installID string
		want      *client
	}{
		{
			name: "happy_path_no_install_id",
			want: defaultClient(),
		},
		{
			name:      "happy_path_with_install_id",
			installID: testInstallID,
			want:      defaultClient(),
		},
		{
			name:      "happy_path_with_custom_http_client",
			installID: testInstallID,
			client:    &http.Client{Timeout: 2},
			want: func() *client {
				c := defaultClient()
				c.HTTPClient = &http.Client{Timeout: 2}
				return c
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			
			ctx := context.Background()

			installPath := t.TempDir() + "/" + installIDFileName
			if len(tc.installID) > 0 {
				if err := storeInstallID(testAppID, installPath, &InstallIDData{tc.installID}); err != nil {
					t.Fatalf("test setup failed: %s", err.Error())
				}
			}
			envVars := map[string]string{
				"METRICS_URL": testServerURL,
			}
			lookupper := envconfig.MapLookuper(envVars)
			opts := make([]Option, 0, 2)
			opts = append(opts, WithLookuper(lookupper))
			opts = append(opts, WithInstallIDFileOverride(installPath))
			if tc.client != nil {
				opts = append(opts, WithHTTPClient(tc.client))
			}

			i, err := New(ctx, testAppID, testVersion, opts...)
			got := i.(*client)

			storedID, err := loadInstallID(testAppID, installPath)
			if err != nil {
				t.Fatalf("could not load install ID for checking side effects")
			}
			if len(tc.installID) > 0 {
				if diff := cmp.Diff(storedID.InstallID, tc.installID); diff != "" {
					t.Errorf("install id changed. Diff (-got +want): %s", diff)
				}
			} else if storedID.InstallID == "" {
				t.Errorf("install id not saved")
			} else {
				// We cannot know ahead of time if generated, so copy from got to want.
				tc.want.InstallID = got.InstallID
			}

			if diff := cmp.Diff(got.InstallID, storedID.InstallID); diff != "" {
				t.Errorf("install id in client does not match stored. Diff (-client +stored): %s", diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("unexpected client fields. Diff (-got +want): %s", diff)
			}
		})
	}
}

func TestWriteMetric(t *testing.T) {
	t.Parallel()

	// Record calls made to test server. Separate per test using a per-test
	// unique id in URL.
	var reqMap sync.Map

	// Request body is intentionally leaked to allow for inspection in test cases.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saveReq := r.Clone(context.Background())
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("error copying body: %s", err.Error())
		}
		saveReq.Body = io.NopCloser(bytes.NewBuffer(body))
		r.Body = io.NopCloser(bytes.NewBuffer(body))
		_, prevExist := reqMap.Swap(r.RequestURI, saveReq)
		if prevExist {
			t.Fatalf("multiple requests to same url: %s", r.RequestURI)
		}
		if !strings.HasSuffix(r.RequestURI, "/sendMetrics") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		if strings.HasSuffix(r.RequestURI, "400/sendMetrics") {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintln(w, "bad request")
			return
		}

		if strings.HasSuffix(r.RequestURI, "500/sendMetrics") {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprintln(w, "internal error")
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name                 string
		metric               string
		count                int
		client               *client
		responseCodeOverride int
		wantRequest          *SendMetricRequest
		wantErr              string
	}{
		{
			name:   "metric_success",
			metric: "foo",
			count:  1,
			client: defaultClient(),
			wantRequest: &SendMetricRequest{
				AppID:     testAppID,
				Version:   testVersion,
				Metrics:   map[string]int{"foo": 1},
				InstallID: testInstallID,
			},
		},
		{
			name:   "metric_opt_out_noop",
			metric: "foo",
			count:  1,
			client: func() *client {
				c := defaultClient()
				c.OptOut = true
				return c
			}(),
			wantRequest: nil,
		},
		{
			name:                 "metric_4xx_returns_error",
			metric:               "foo",
			count:                1,
			client:               defaultClient(),
			responseCodeOverride: http.StatusBadRequest,
			wantRequest: &SendMetricRequest{
				AppID:     testAppID,
				Version:   testVersion,
				Metrics:   map[string]int{"foo": 1},
				InstallID: testInstallID,
			},
			wantErr: "received 400 response",
		},
		{
			name:                 "metric_5xx_returns_error",
			metric:               "foo",
			count:                1,
			client:               defaultClient(),
			responseCodeOverride: http.StatusInternalServerError,
			wantRequest: &SendMetricRequest{
				AppID:     testAppID,
				Version:   testVersion,
				Metrics:   map[string]int{"foo": 1},
				InstallID: testInstallID,
			},
			wantErr: "received 500 response",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			relativePath := fmt.Sprintf("/%d", rand.Uint64())
			if tc.responseCodeOverride != 0 {
				relativePath = fmt.Sprintf("%s/%d", relativePath, tc.responseCodeOverride)
			}
			tc.client.Config.ServerURL = fmt.Sprintf("%s%s", ts.URL, relativePath)

			err := tc.client.WriteMetric(ctx, tc.metric, tc.count)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}
			val, ok := reqMap.Load(relativePath + "/sendMetrics")
			if tc.wantRequest != nil {
				if !ok {
					t.Errorf("no http request received, expected body of: %v", *tc.wantRequest)
				}
				request := val.(*http.Request)
				defer request.Body.Close()

				var got SendMetricRequest
				if err := json.NewDecoder(request.Body).Decode(&got); err != nil {
					t.Errorf("error reading request to test server: %s", err.Error())
				}
				if diff := cmp.Diff(&got, tc.wantRequest); diff != "" {
					t.Errorf("unexpected request body. Diff (-got +want): %s", diff)
				}
			} else {
				if ok {
					t.Errorf("did not expect a request but got one")
				}
			}
		})
	}
}
