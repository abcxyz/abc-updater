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

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

//nolint:paralleltest // can't set env vars in parallel tests
func TestCheckAppVersion(t *testing.T) {
	sampleAppResponse, err := json.Marshal(AppResponse{
		AppID:          "sample_app_1",
		AppName:        "Sample App 1",
		GithubURL:      "https://github.com/abcxyz/sample_app_1",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Errorf("failed to encode json %v", err)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.RequestURI, "sample_app_1/data.json") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, string(sampleAppResponse))
	}))

	t.Setenv("ABC_UPDATER_URL", ts.URL)

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name    string
		appID   string
		version string
		want    string
		wantErr string
	}{
		{
			name:    "outdated_version",
			appID:   "sample_app_1",
			version: "v0.0.1",
			want: fmt.Sprintf("A new version of %s is available! Your current version is %s. Version %s is available at %s.\n",
				"Sample App 1",
				"v0.0.1",
				"1.0.0",
				"https://github.com/abcxyz/sample_app_1"),
		},
		{
			name:    "current_version",
			appID:   "sample_app_1",
			version: "v1.0.0",
			want:    "",
		},
		{
			name:    "invalid_app_id",
			appID:   "bad_app",
			version: "v1.0.0",
			wantErr: "unable to retrieve data for requested app",
		},
		{
			name:    "invalid_version",
			appID:   "sample_app_1",
			version: "1.0.0.12.2",
			wantErr: "version is not a valid semantic version string",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var b bytes.Buffer
			err := CheckAppVersion(context.Background(), tc.appID, tc.version, &b)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			got := b.String()
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("incorrect output written, diff (-got +want):\n%s", diff)
			}
		})
	}
}
