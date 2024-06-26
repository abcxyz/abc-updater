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

package updater

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

	"github.com/google/go-cmp/cmp"
	"github.com/sethvargo/go-envconfig"

	"github.com/abcxyz/pkg/testutil"
)

// Instrumented io.Writer.
type testWriter struct {
	Buf    bytes.Buffer
	Writes int64
}

func (w *testWriter) Write(p []byte) (n int, err error) {
	w.Writes++
	return w.Buf.Write(p)
}

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
		fmt.Fprintf(w, "%s\n", string(sampleAppResponse))
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
				"UPDATER_URL": ts.URL,
			},
			want: `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
		},
		{
			name:    "current_version",
			appID:   "sample_app_1",
			version: "v1.0.0",
			env: map[string]string{
				"UPDATER_URL": ts.URL,
			},
			want: "",
		},
		{
			name:    "outdated_version_but_cached_check",
			appID:   "sample_app_1",
			version: "0.0.1",
			env: map[string]string{
				"UPDATER_URL": ts.URL,
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
				"UPDATER_URL": ts.URL,
			},
			want: `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
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
				"UPDATER_URL": ts.URL,
			},
			want:    "",
			wantErr: http.StatusText(http.StatusNotFound),
		},
		{
			name:    "invalid_version",
			appID:   "sample_app_1",
			version: "vab1.0.0.12.2",
			env: map[string]string{
				"UPDATER_URL": ts.URL,
			},
			want:    "",
			wantErr: "failed to parse check version \"vab1.0.0.12.2\"",
		},
		{
			name:    "opt_out_ignore_all",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				"UPDATER_URL":        ts.URL,
				ignoreVersionsEnvVar: "all",
			},
			want: "",
		},
		{
			name:    "opt_out_ignore_match",
			appID:   "sample_app_1",
			version: "v0.1.0",
			env: map[string]string{
				"UPDATER_URL":        ts.URL,
				ignoreVersionsEnvVar: "1.0.0",
			},
			want: "",
		},
		{
			name:    "opt_out_no_match_not_ignored",
			appID:   "sample_app_1",
			version: "v0.0.1",
			env: map[string]string{
				"UPDATER_URL":        ts.URL,
				ignoreVersionsEnvVar: "0.0.2",
			},
			want: `Sample App 1 version 1.0.0 is available at [https://github.com/abcxyz/sample_app_1]. Use SAMPLE_APP_1_IGNORE_VERSIONS="1.0.0" (or "all") to ignore.`,
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
				if err := setLocalCachedData(params, tc.cached); err != nil {
					t.Errorf("unexpected error setting up test cache file: %v", err)
				}
			}

			output, err := CheckAppVersionSync(context.Background(), params)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(output, tc.want); diff != "" {
				t.Errorf("output was not as expected (-got,+want): %s", diff)
			}
		})
	}
}

// Note: These tests rely on timing and could be flaky if breakpoints are used.
func Test_asyncFunctionCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		input      func() (string, error)
		want       string
		wantWrites int64
	}{
		{
			name:       "happy_path",
			input:      func() (string, error) { return "done", nil },
			want:       "done\n",
			wantWrites: 1,
		},
		{
			name:       "happy_path_no_update",
			input:      func() (string, error) { return "", nil },
			want:       "",
			wantWrites: 0,
		},
		{
			name:       "error_path_still_returns",
			input:      func() (string, error) { return "", fmt.Errorf("failed") },
			want:       "",
			wantWrites: 0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outBuf := testWriter{}
			resultFunc := asyncFunctionCall(context.Background(), tc.input,
				func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

			resultFunc()
			if got := outBuf.Buf.String(); got != tc.want {
				t.Errorf("incorrect output got=%s, want=%s", got, tc.want)
			}
			if got := outBuf.Writes; got != tc.wantWrites {
				t.Errorf("incorrect number of interactions got=%d, want=%d", got, tc.wantWrites)
			}
		})
	}
}

// This test is timing dependent, but would require to be paused for more than
// an hour to cause flakiness.
func Test_asyncFunctionCallContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	outBuf := testWriter{}
	inputFunc := func() (string, error) { time.Sleep(1 * time.Hour); return "should_not_execute", nil }
	resultFunc := asyncFunctionCall(ctx, inputFunc, func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

	// Context canceled before timeouts.
	cancel()

	// Should return immediately since context was canceled.
	resultFunc()
	if got := outBuf.Buf.String(); got != "" {
		t.Errorf("incorrect output got=%s, want=%s", got, "")
	}
	if got := outBuf.Writes; got != 0 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 0)
	}
}

func Test_asyncFunctionCallWaitForResultToWrite(t *testing.T) {
	t.Parallel()
	outBuf := testWriter{}
	inputFunc := func() (string, error) {
		return "foobar", nil
	}
	resultFunc := asyncFunctionCall(context.Background(), inputFunc, func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

	// Give goroutine a reasonable time to finish (in theory this test could
	// give a false negative, in the unhappy case there is a race)
	time.Sleep(50 * time.Millisecond)

	// resultFunc() hasn't been run, no output should appear
	if got := outBuf.Buf.String(); got != "" {
		t.Errorf("incorrect output got=%s, want=%s", got, "")
	}
	if got := outBuf.Writes; got != 0 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 0)
	}

	resultFunc()

	// Now buffer should include output.
	if got := outBuf.Buf.String(); got != "foobar\n" {
		t.Errorf("incorrect output got=%s, want=%s", got, "foobar\n")
	}
	if got := outBuf.Writes; got != 1 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 1)
	}
}

func TestIsIgnored(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		version string
		config  *versionConfig
		want    bool
		wantErr string
	}{
		{
			name:    "nothing_ignored",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: nil,
			},
			want: false,
		},
		{
			name:    "all_ignored",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"all"},
			},
			want: true,
		},
		{
			name:    "all_ignored_other_info",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", "all", ">1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_no_match",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", ">1.0.0"},
			},
			want: false,
		},
		{
			name:    "version_match_last",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.1", "<1.0.0", ">1.0.0", "1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_exact_match",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lt",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"<1.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gt",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{">0.0.1"},
			},
			want: true,
		},
		{
			name:    "version_constraint_lte",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"<=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_constraint_gte",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{">=1.0.0"},
			},
			want: true,
		},
		{
			name:    "version_prerelease",
			version: "1.1.0-alpha",
			config: &versionConfig{
				IgnoreVersions: []string{"1.1.0-alpha"},
			},
			want: true,
		},
		{
			name:    "version_broken",
			version: "abcd",
			config: &versionConfig{
				IgnoreVersions: []string{"1.1.0-alpha"},
			},
			want:    false,
			wantErr: "failed to parse version",
		},
		{
			name:    "constraint_broken",
			version: "1.0.0",
			config: &versionConfig{
				IgnoreVersions: []string{"alsdkfas"},
			},
			want:    false,
			wantErr: "Malformed constraint",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.config.isIgnored(tc.version)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if want := tc.want; got != want {
				t.Errorf("incorrect IsIgnored got=%t, want=%t", got, want)
			}
		})
	}
}

func TestIgnoreAll(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		config *versionConfig
		want   bool
	}{
		{
			name: "no_versions_not_ignored",
			config: &versionConfig{
				IgnoreVersions: nil,
			},
			want: false,
		},
		{
			name: "only_all_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"all"},
			},
			want: true,
		},
		{
			name: "concrete_list_not_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0", "3.0.2"},
			},
			want: false,
		},
		{
			name: "concrete_list_with_all_ignored",
			config: &versionConfig{
				IgnoreVersions: []string{"1.0.0", "3.0.2", "all"},
			},
			want: true,
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.config.ignoreAll()

			if want := tc.want; got != want {
				t.Errorf("incorrect allVersionUpdatesIgnored got=%t, want=%t", got, want)
			}
		})
	}
}
