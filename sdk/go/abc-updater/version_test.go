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
	"strings"
	"testing"

	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

func TestCheckAppVersion(t *testing.T) {
	t.Parallel()

	sampleAppResponse, err := json.Marshal(AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		GitHubURL:      "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	})
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
	}{
		{
			name:    "outdated_version",
			appID:   "sample_app_1",
			version: "v0.0.1",
			env: map[string]string{
				"ABC_UPDATER_URL": ts.URL,
			},
			want: fmt.Sprintf(outputFormat,
				"Sample App 1",
				"0.0.1",
				"1.0.0",
				"https://github.com/abcxyz/sample_app_1"),
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
			want: fmt.Sprintf(outputFormat,
				"Sample App 1",
				"0.0.1",
				"1.0.0",
				"https://github.com/abcxyz/sample_app_1"),
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var b bytes.Buffer
			params := &CheckVersionParams{
				AppID:    tc.appID,
				Version:  tc.version,
				Writer:   &b,
				Lookuper: envconfig.MapLookuper(tc.env),
			}

			err := CheckAppVersion(context.Background(), params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if got, want := b.String(), tc.want; got != want {
				t.Errorf("incorrect output got=%s, want=%s", got, want)
			}
		})
	}
}
