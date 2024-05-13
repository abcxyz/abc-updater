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
	"fmt"
	"github.com/sethvargo/go-envconfig"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const (
	testAppID = "testapp"
	testVersion = "1.0.0"
	testInstallID = "yv66vt6tvu8="
	testServerURL = "https://example.com"
)

var testClient http.Client

func defaultClient() Client {
	return Client{
		appID:      testAppID,
		version:    testVersion,
		installID:  testInstallID,
		httpClient: &testClient,
		optOut:     false,
		config:     &metricsConfig{
			ServerURL: testServerURL,
			NoMetrics: false,
		},
	}
}
func TestNew(t *testing.T) {
	t.Parallel()

	cases := []struct{
		name string
		opts []Option
		serverURL string
        optOut bool
		installID string
		want Client
		wantErr string
	} {
		{
			name: "happy_path_no_opts_no_install_id_succeeds",
			want: defaultClient(),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()

			// Set up env vars
			envVars := map[string]string{
				"ABC_METRICS_URL": testServerURL,
			}
			if tc.optOut {
				envVars["NO_METRICS"] = "TRUE"
			}
			lookupper := envconfig.MapLookuper(envVars)

			got, err := New(ctx, testAppID, testVersion, WithLookuper(lookupper)
			// TODO: I don't like where this is going, maybe just make separate tests rather than table driven?)
		})
	}

}

func TestWriteMetric(t *testing.T) {
	t.Parallel()

	// Record calls made to test server. Separate per test using a per-test
	// unique id in URL.
	reqMap := sync.Map{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, prevExist := reqMap.Swap(r.RequestURI, r)
		if prevExist {
			t.Fatalf("multiple requests to same url: %s", r.RequestURI)
		}
		if !strings.HasSuffix(r.RequestURI, "/sendMetrics") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name    string
		appID   string
		version string
		metrics map[string]int
		env     map[string]string
		wantRequest map[string]any
		wantErr string
		installID  *InstallIDData
	}{
		{
			name: "one_metric_success"
		},
	}

}
