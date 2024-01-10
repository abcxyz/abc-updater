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

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/testutil"
)

func TestInitLocalStoreWithDir(t *testing.T) {
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
			name:    "empty_dir",
			dir:     "",
			wantErr: "must supply non empty directory",
		},
	}

	for _, tc := range cases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			localStore, err := initLocalStoreWithDir(tc.dir)
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
		name    string
		data    localData
		want    string
		wantErr string
	}{
		{
			name: "time_0_encodes",
			data: localData{LastVersionCheckUTCEpochSec: 0},
			want: "{\"last_version_check_utc_epoch_sec\":0}\n",
		},
		{
			name: "generic_time",
			data: localData{LastVersionCheckUTCEpochSec: 1704825396},
			want: "{\"last_version_check_utc_epoch_sec\":1704825396}\n",
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
			data: "{\"last_version_check_utc_epoch_sec\":123}\n",
			want: &localData{LastVersionCheckUTCEpochSec: 123},
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
