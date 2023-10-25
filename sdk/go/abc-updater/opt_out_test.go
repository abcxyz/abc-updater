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
	"testing"

	"github.com/google/go-cmp/cmp"
)

//nolint:paralleltest // can't set env vars in parallel tests
func TestInitOptOutSettings(t *testing.T) {
	cases := []struct {
		name    string
		appID   string
		envVars map[string]string
		want    *optOutSettings
	}{
		{
			name:    "no_env_vars_set",
			appID:   "sample_app_1",
			envVars: map[string]string{},
			want: &optOutSettings{
				MuteAllVersionUpdates: false,
				MuteVersion:           "",
			},
		},
		{
			name:  "set_mute_all",
			appID: "sample_app_1",
			envVars: map[string]string{
				updatesMuteAllEnvVar("sample_app_1"): "1",
			},
			want: &optOutSettings{
				MuteAllVersionUpdates: true,
				MuteVersion:           "",
			},
		},
		{
			name:  "set_mute_version",
			appID: "sample_app_1",
			envVars: map[string]string{
				updatesMuteVersionEnvVar("sample_app_1"): "1.0.0",
			},
			want: &optOutSettings{
				MuteAllVersionUpdates: false,
				MuteVersion:           "1.0.0",
			},
		},
		{
			name:  "set_mute_version_and_all",
			appID: "sample_app_1",
			envVars: map[string]string{
				updatesMuteVersionEnvVar("sample_app_1"): "1.0.0",
				updatesMuteAllEnvVar("sample_app_1"):     "1",
			},
			want: &optOutSettings{
				MuteAllVersionUpdates: true,
				MuteVersion:           "1.0.0",
			},
		},
	}

	//nolint:paralleltest // can't set env vars in parallel tests
	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			for name, value := range tc.envVars {
				t.Setenv(name, value)
			}

			config := initOptOutSettings(tc.appID)
			if diff := cmp.Diff(tc.want, config); diff != "" {
				t.Errorf("Config unexpected diff (-want,+got):\n%s", diff)
			}
		})
	}
}
