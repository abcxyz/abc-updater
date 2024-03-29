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

package abcupdater

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

func TestCheckAppVersionSync(t *testing.T) {
	t.Parallel()

	testAppResponse := AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		AppRepoURL:     "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	}

	sampleAppResponse, err := json.Marshal(testAppResponse)
	if err != nil {
		t.Errorf("failed to encode json %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.RequestURI, "sample_app_1/data.json") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(sampleAppResponse))
	}))

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name    string
		appID   string
		version string
		env     map[string]string
		want    string
		wantErr string
		cached  *LocalVersionData
	}{
		{
			name:    "outdated_version",
			appID:   "sample_app_1",
			version: "v0.0.1",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want: `A new version of Sample App 1 is available! Your current version is 0.0.1. Version 1.0.0 is available at https://github.com/abcxyz/sample_app_1.

To disable notifications for this new version, set SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0". To disable all version notifications, set SAMPLE_APP_1_IGNORE_VERSIONS="all".
`,
		},
		{
			name:    "current_version",
			appID:   "sample_app_1",
			version: "v1.0.0",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want: "",
		},
		{
			name:    "outdated_version_but_cached_check",
			appID:   "sample_app_1",
			version: "0.0.1",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want: "",
			cached: &LocalVersionData{
				LastCheckTimestamp: time.Now().Unix(),
				AppResponse:        testAppResponse,
			},
		},
		{
			name:    "outdated_version_cached_check_expired",
			appID:   "sample_app_1",
			version: "0.0.1",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want: `A new version of Sample App 1 is available! Your current version is 0.0.1. Version 1.0.0 is available at https://github.com/abcxyz/sample_app_1.

To disable notifications for this new version, set SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0". To disable all version notifications, set SAMPLE_APP_1_IGNORE_VERSIONS="all".
`,
			cached: &LocalVersionData{
				LastCheckTimestamp: time.Now().Add(-25 * time.Hour).Unix(),
				AppResponse:        testAppResponse,
			},
		},
		{
			name:    "invalid_app_id",
			appID:   "bad_app",
			version: "v1.0.0",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want:    "",
			wantErr: http.StatusText(http.StatusNotFound),
		},
		{
			name:    "invalid_version",
			appID:   "sample_app_1",
			version: "vab1.0.0.12.2",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want:    "",
			wantErr: "failed to parse check version \"vab1.0.0.12.2\"",
		},
		{
			name:    "opt_out_ignore_all",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				"ABC_UPDATER_URL":                    ts.URL,
				ignoreVersionsEnvVar("sample_app_1"): "all",
			},
			want: "",
		},
		{
			name:    "opt_out_ignore_match",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				"ABC_UPDATER_URL":                    ts.URL,
				ignoreVersionsEnvVar("sample_app_1"): "1.0.0",
			},
			want: "",
		},
		{
			name:    "opt_out_no_match_not_ignored",
			appID:   "sample_app_1",
			version: "v0.0.1",
			env: map[string]string{
				"ABC_UPDATER_URL":                    ts.URL,
				ignoreVersionsEnvVar("sample_app_1"): "0.0.2",
			},
			want: `A new version of Sample App 1 is available! Your current version is 0.0.1. Version 1.0.0 is available at https://github.com/abcxyz/sample_app_1.

To disable notifications for this new version, set SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0". To disable all version notifications, set SAMPLE_APP_1_IGNORE_VERSIONS="all".
`,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cacheFile := filepath.Join(t.TempDir(), "data.json")

			params := &CheckVersionParams{
				AppID:             tc.appID,
				Version:           tc.version,
				Lookuper:          envconfig.MapLookuper(tc.env),
				CacheFileOverride: cacheFile,
			}

			if tc.cached != nil {
				if err := params.setLocalCachedData(tc.cached); err != nil {
					t.Errorf("unexpected error setting up test cache file: %v", err)
				}
			}

			output, err := CheckAppVersionSync(context.Background(), params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if got, want := output, tc.want; got != want {
				t.Errorf("incorrect output got=%s, want=%s", got, want)
			}
		})
	}
}

// Note: These tests rely on timing and could be flaky if breakpoints are used.
func Test_asyncFunctionCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input func() (string, error)
		want  string
	}{
		{
			name:  "happy_path",
			input: func() (string, error) { return "done", nil },
			want:  "done\n",
		},
		{
			name:  "error_path_still_returns",
			input: func() (string, error) { return "", fmt.Errorf("failed") },
			want:  "",
		},
		{
			// This test will take a long time due to forcing timeout.
			name:  "timeout_gets_applied",
			input: func() (string, error) { time.Sleep(9999 * time.Hour); return "should_not_execute", nil },
			want:  "",
		},
		// TODO: would like a clean way to test the canceled context case, not sure
		// how to make it not leak into rest of test cases
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outBuf := bytes.Buffer{}
			resultFunc := asyncFunctionCall(context.Background(), tc.input, &outBuf)
			resultFunc()
			if got := outBuf.String(); got != tc.want {
				t.Errorf("incorrect output got=%s, want=%s", got, tc.want)
			}
		})
	}
}
