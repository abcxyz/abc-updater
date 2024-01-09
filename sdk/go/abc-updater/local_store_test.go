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

package abcupdater

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
)

func TestInitLocalStore(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	cases := []struct {
		name     string
		settings localStoreSettings
		want     *localStore
		wantErr  string
	}{
		{
			name: "use_app_id",
			settings: localStoreSettings{
				AppID: "hello_1",
				testLocalStoreDirFn: func(s string) (string, error) {
					return filepath.Join(tempDir, "hello_1"), nil
				},
			},
			want: &localStore{directory: filepath.Join(tempDir, "hello_1")},
		},
		{
			name: "use_directory",
			settings: localStoreSettings{
				Directory: filepath.Join(tempDir, "hello_2"),
			},
			want: &localStore{directory: filepath.Join(tempDir, "hello_2")},
		},
		{
			name:     "fails_with_no_dir_or_app_id",
			settings: localStoreSettings{},
			wantErr:  "must supply either appID or directory in settings",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			localStore, err := initLocalStore(&tc.settings)
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if tc.want == nil {
				if localStore != nil {
					t.Errorf("expect nil localStore but got %v", localStore)
				}

				return
			}

			if got, want := localStore.directory, tc.want.directory; got != want {
				t.Errorf("incorrect directory got=%s, want=%s", got, want)
			}

			// check that directory exists
			if _, err := os.Stat(localStore.directory); os.IsNotExist(err) {
				t.Errorf("directory was not created: %s", localStore.directory)
			}
		})
	}
}

func TestUpdateLocalData(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		data    localData
		want    string
		wantErr string
	}{
		{
			name: "time_0_encodes",
			data: localData{LastVersionCheckTimestamp: 0},
			want: "{\"last_version_check_timestamp\":0}\n",
		},
		{
			name: "generic_time",
			data: localData{LastVersionCheckTimestamp: 1704825396},
			want: "{\"last_version_check_timestamp\":1704825396}\n",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			localStore := localStore{
				directory: tempDir,
			}

			err := localStore.updateLocalData(&tc.data)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			f, err := os.Open(localStore.localDataFilename())
			if err != nil {
				t.Errorf("failed to open data file: %v", err)
			}
			defer f.Close()

			bytes, err := io.ReadAll(f)
			if err != nil {
				t.Errorf("failed to read data file: %v", err)
			}

			if got, want := string(bytes), tc.want; got != want {
				t.Errorf("incorrect encoding got=%s, want=%s", got, want)
			}
		})
	}
}

func TestLoadLocalData(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		data    string
		want    *localData
		wantErr string
	}{
		{
			name:    "empty_local_data",
			data:    "",
			want:    nil,
			wantErr: "failed to decode",
		},
		{
			name:    "bad_json",
			data:    "hello",
			want:    nil,
			wantErr: "failed to decode",
		},
		{
			name: "decodes_valid_json",
			data: "{\"last_version_check_timestamp\":123}\n",
			want: &localData{LastVersionCheckTimestamp: 123},
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// set up
			tempDir := t.TempDir()
			localStore := localStore{
				directory: tempDir,
			}

			f, err := os.Create(localStore.localDataFilename())
			if err != nil {
				t.Errorf("failed to create file:%v", err)
			}
			_, err = f.WriteString(tc.data)
			if err != nil {
				t.Errorf("failed to write to file: %v", err)
			}

			// run
			got, err := localStore.loadLocalData()

			// validate
			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("localData unexpected diff (-want,+got):\n%s", diff)
			}
		})
	}
}
