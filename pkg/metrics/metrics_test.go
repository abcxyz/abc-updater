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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		appID:      testAppID,
		appVersion: testVersion,
		installID:  testInstallID,
		httpClient: &http.Client{Timeout: 1 * time.Second},
		optOut:     false,
		config: &metricsConfig{
			ServerURL: testServerURL,
			NoMetrics: false,
		},
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	t.Run("happy_path", func(t *testing.T) {
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
					c.httpClient = &http.Client{Timeout: 2}
					return c
				}(),
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx := context.Background()

				installPath := t.TempDir() + "/" + installIDFileName
				if tc.installID != "" {
					if err := storeInstallID(testAppID, installPath, &InstallIDData{tc.installID}); err != nil {
						t.Fatalf("test setup failed: %s", err.Error())
					}
				}
				envVars := map[string]string{
					"METRICS_URL": testServerURL,
				}
				lookuper := envconfig.MapLookuper(envVars)
				opts := []Option{
					WithLookuper(lookuper),
					WithInstallIDFileOverride(installPath),
				}
				if tc.client != nil {
					opts = append(opts, WithHTTPClient(tc.client))
				}

				i, err := New(ctx, testAppID, testVersion, opts...)
				if err != nil {
					t.Errorf("unexpected error: %s", err.Error())
				}
				got, ok := i.(*client)
				if !ok {
					t.Fatal("Expected New to return client, but cast failed.")
				}

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
					tc.want.installID = got.installID
				}

				if diff := cmp.Diff(got.installID, storedID.InstallID); diff != "" {
					t.Errorf("install id in client does not match stored. Diff (-client +stored): %s", diff)
				}

				if diff := cmp.Diff(got, tc.want); diff != "" {
					t.Errorf("unexpected client fields. Diff (-got +want): %s", diff)
				}
			})
		}
	})

	// Not all failure cases can be easily tested, will test subset that is easy
	// to reproduce.
	t.Run("unhappy_path", func(t *testing.T) {
		t.Parallel()

		cases := []struct { //nolint:forcetypeassert
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
					gotV, ok := c.(*client)
					if !ok {
						t.Fatal("Expected New to return client, but cast failed.")
					}
					if diff := cmp.Diff(gotV, tc.want); diff != "" {
						t.Errorf("unexpected metricWriter value. Diff (-got +want): %s", diff)
					}
				}
				if diff := testutil.DiffErrString(err, tc.wantError); diff != "" {
					t.Errorf("unexpected error: %s", diff)
				}
			})
		}
	})
}

func TestWriteMetric(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		client      *client
		responder   http.HandlerFunc
		wantRequest *SendMetricRequest
		wantErr     string
	}{
		{
			name:   "metric_success",
			client: defaultClient(),
			wantRequest: &SendMetricRequest{
				AppID:      testAppID,
				AppVersion: testVersion,
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  testInstallID,
			},
		},
		{
			name: "metric_opt_out_noop",
			client: func() *client {
				c := defaultClient()
				c.optOut = true
				return c
			}(),
			wantRequest: nil,
		},
		{
			name:   "metric_4xx_returns_error",
			client: defaultClient(),
			responder: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				fmt.Fprintf(w, "bad request")
			},
			wantRequest: &SendMetricRequest{
				AppID:      testAppID,
				AppVersion: testVersion,
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  testInstallID,
			},
			wantErr: "received 400 response",
		},
		{
			name:   "metric_5xx_returns_error",
			client: defaultClient(),
			responder: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "internal error")
			},
			wantRequest: &SendMetricRequest{
				AppID:      testAppID,
				AppVersion: testVersion,
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  testInstallID,
			},
			wantErr: "received 500 response",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()

			var gotRequest *SendMetricRequest
			ts := httptest.NewServer(func() http.Handler {
				mux := http.NewServeMux()
				mux.HandleFunc("POST /sendMetrics", func(w http.ResponseWriter, r *http.Request) {
					if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
						t.Errorf("error reading request to test server: %s", err.Error())
					}

					if tc.responder != nil {
						tc.responder(w, r)
						return
					}

					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "ok")
				})

				return mux
			}())
			t.Cleanup(ts.Close)

			tc.client.config.ServerURL = ts.URL

			err := tc.client.WriteMetric(ctx, "foo", 1)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(tc.wantRequest, gotRequest); diff != "" {
				t.Errorf("unexpected request diff (-got +want): %s", diff)
			}
		})
	}
}

func TestWriteMetricAsync(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		client      *client
		timeout     time.Duration
		wantRequest *SendMetricRequest
		wantErr     string
	}{
		{
			name:   "metric_success",
			client: defaultClient(),
			wantRequest: &SendMetricRequest{
				AppID:      testAppID,
				AppVersion: testVersion,
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  testInstallID,
			},
		},
		{
			name:    "metric_success_timeout_set",
			client:  defaultClient(),
			timeout: 3 * time.Second,
			wantRequest: &SendMetricRequest{
				AppID:      testAppID,
				AppVersion: testVersion,
				Metrics:    map[string]int64{"foo": 1},
				InstallID:  testInstallID,
			},
		},
		{
			name: "metric_opt_out_noop",
			client: func() *client {
				c := defaultClient()
				c.optOut = true
				return c
			}(),
			wantRequest: nil,
		},
		{
			name:        "metric_failure_timeout",
			client:      defaultClient(),
			timeout:     1 * time.Nanosecond,
			wantRequest: nil,
			wantErr:     "context deadline exceeded",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var gotRequest *SendMetricRequest
			ts := httptest.NewServer(func() http.Handler {
				mux := http.NewServeMux()
				mux.HandleFunc("POST /sendMetrics", func(w http.ResponseWriter, r *http.Request) {
					// Add artificial latency to ensure our timeouts hit
					time.Sleep(50 * time.Nanosecond)

					if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
						t.Errorf("error reading request to test server: %s", err.Error())
					}

					w.WriteHeader(http.StatusOK)
					fmt.Fprintln(w, "ok")
				})
				return mux
			}())
			t.Cleanup(ts.Close)

			tc.client.config.ServerURL = ts.URL

			ctx := context.Background()
			if tc.timeout > 0 {
				var done func()
				ctx, done = context.WithTimeout(ctx, tc.timeout)
				defer done()
			}

			err := tc.client.WriteMetricAsync(ctx, "foo", 1)()
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(tc.wantRequest, gotRequest); diff != "" {
				t.Errorf("unexpected request diff (-got +want): %s", diff)
			}
		})
	}
}

func TestContext(t *testing.T) {
	t.Parallel()

	client1 := defaultClient()
	client2 := defaultClient()
	client2.installID = "somethingDifferent"

	checkFromContext(context.Background(), t, NoopWriter())

	ctx := WithClient(context.Background(), client1)
	checkFromContext(ctx, t, client1)

	ctx = WithClient(ctx, client2)
	checkFromContext(ctx, t, client2)
}

func checkFromContext(ctx context.Context, tb testing.TB, want MetricWriter) {
	tb.Helper()

	if diff := cmp.Diff(want, FromContext(ctx)); diff != "" {
		tb.Errorf("unexpected metrics client in context diff (-got +want): %s", diff)
	}
}
