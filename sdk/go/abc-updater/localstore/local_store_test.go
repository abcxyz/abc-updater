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
	"bytes"
	"encoding/json"
	"github.com/abcxyz/pkg/testutil"
	"github.com/google/go-cmp/cmp"
	"os"
	"path/filepath"
	"testing"
)

type testObj struct {
	Foo string   `json:"foo,omitempty"`
	Bar int64    `json:"bar,omitempty"`
	Baz *testObj `json:"baz,omitempty"`
}

func TestLoadJSONFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		path      string
		fs        map[string]string
		want      testObj
		wantError string
	}{
		{
			name: "happy_path",
			path: "data.json",
			fs: map[string]string{"data.json": toJSON(testObj{
				Foo: "foo",
				Bar: 15,
				Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
			}, t),
			},
			want: testObj{
				Foo: "foo",
				Bar: 15,
				Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
			},
		},
		{
			// TODO: this SHOULD NOT pass. Something is wrong with test
			name: "minimal_json",
			path: "data.json",
			fs:   map[string]string{"data.json": "{}"},
			want: testObj{
				Foo: "foo",
				Bar: 15,
				Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
			},
		}, {
			name:      "file_missing",
			path:      "data.json",
			fs:        map[string]string{},
			wantError: "failed to open json file",
		}, {
			name:      "invalid_json",
			path:      "data.json",
			fs:        map[string]string{"data.json": "i'm not valid json!!!!"},
			wantError: "failed to load json file",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			path := filepath.Join(base, tc.path)
			populateFiles(base, tc.fs, t)
			var got testObj

			if diff := testutil.DiffErrString(LoadJSONFile(path, &got), tc.wantError); diff != "" {
				t.Errorf("unexpected err: %s", diff)
			}

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("got unexpected response:\n%s", diff)
			}
		})
	}
}

//
//func TestStoreJSONFile(t *testing.T) {
//	t.Parallel()
//	type args[T any] struct {
//		path string
//		data T
//	}
//	type testCase[T any] struct {
//		name    string
//		args    args[T]
//		wantErr bool
//	}
//	tests := []testCase[ /* TODO: Insert concrete types here */ ]{
//		// TODO: Add test cases.
//	}
//	for _, tc := range tests {
//		t.Run(tc.name, func(t *testing.T) {
//			t.Parallel()
//			if err := StoreJSONFile(tc.args.path, tc.args.data); (err != nil) != tc.wantErr {
//				t.Errorf("StoreJSONFile() error = %v, wantErr %v", err, tc.wantErr)
//			}
//		})
//	}
//}

func populateFiles(base string, nameContents map[string]string, t *testing.T) {
	t.Helper()
	for name, contents := range nameContents {
		if err := os.WriteFile(filepath.Join(base, name), []byte(contents), 0777); err != nil {
			t.Fatalf("Could not write file %v: %v", name, err)
		}
	}
}

func toJSON(data any, t *testing.T) string {
	t.Helper()
	buf := bytes.Buffer{}
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(data); err != nil {
		t.Fatalf("could not encode json: %v", err)
	}
	return buf.String()
}
