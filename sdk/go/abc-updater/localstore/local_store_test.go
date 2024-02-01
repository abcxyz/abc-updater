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

package localstore

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/testutil"
)

func TestInitWithDir(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	cases := []struct {
		name    string
		dir     string
		want    *localStore
		wantErr string
	}{
		{
			name: "supply_dir",
			dir:  filepath.Join(tempDir, "hello_1"),
			want: &localStore{directory: filepath.Join(tempDir, "hello_1")},
		},
		{
			name:    "empty_string_dir",
			dir:     "",
			wantErr: "directory cannot be empty string",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			localStore, err := InitWithDir(tc.dir)
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
		})
	}
}

func TestUpdateLocalData(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		existingData *localData
		data         *localData
		want         string
		wantErr      string
	}{
		{
			name: "time_0_encodes",
			data: &localData{LastVersionCheck: 0},
			want: "{\"lastVersionCheck\":0}\n",
		},
		{
			name: "generic_time",
			data: &localData{LastVersionCheck: 1704825396},
			want: "{\"lastVersionCheck\":1704825396}\n",
		},
		{
			name:         "update_when_exists",
			existingData: &localData{LastVersionCheck: 1604825396},
			data:         &localData{LastVersionCheck: 1704825396},
			want:         "{\"lastVersionCheck\":1704825396}\n",
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
			if tc.existingData != nil {
				err := localStore.UpdateLocalData(tc.existingData)
				if err != nil {
					t.Errorf("failed to load existing data: %v", err)
				}
			}

			err := localStore.UpdateLocalData(tc.data)

			if diff := testutil.DiffErrString(err, tc.wantErr); diff != "" {
				t.Error(diff)
			}

			f, err := os.Open(localStore.localDataPath())
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
			data: "{\"lastVersionCheck\":123}\n",
			want: &localData{LastVersionCheck: 123},
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

			f, err := os.Create(localStore.localDataPath())
			if err != nil {
				t.Errorf("failed to create file:%v", err)
			}
			_, err = f.WriteString(tc.data)
			if err != nil {
				t.Errorf("failed to write to file: %v", err)
			}

			// run
			got, err := localStore.LoadLocalData()

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
