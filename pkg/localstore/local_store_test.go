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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/abcxyz/pkg/testutil"
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
			fs: map[string]string{
				"data.json": testToJSON(t, testObj{
					Foo: "foo",
					Bar: 15,
					Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
				}),
			},
			want: testObj{
				Foo: "foo",
				Bar: 15,
				Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
			},
		},
		{
			name: "minimal_json",
			path: "data.json",
			fs:   map[string]string{"data.json": "{}"},
			want: testObj{},
		},
		{
			name: "null_json",
			path: "data.json",
			fs:   map[string]string{"data.json": "null"},
			want: testObj{},
		},
		{
			name:      "file_missing",
			path:      "data.json",
			fs:        map[string]string{},
			wantError: "failed to open json file",
		},
		{
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
			testPopulateFiles(t, base, tc.fs)
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

func TestStoreJSONFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		path      string
		data      testObj
		fs        map[string]string
		wantFS    map[string]string
		wantError string
	}{
		{
			name: "happy_path_overwrite",
			path: "data.json",
			data: testObj{
				Foo: "bar",
				Bar: 1,
				Baz: nil,
			},
			fs: map[string]string{
				"data.json": testToJSON(t, testObj{
					Foo: "foo",
					Bar: 15,
					Baz: &testObj{Foo: "nestfoo", Bar: 16, Baz: nil},
				}),
			},
			wantFS: map[string]string{
				"data.json": testToJSON(t, testObj{
					Foo: "bar",
					Bar: 1,
					Baz: nil,
				}),
			},
		},
		{
			name: "happy_path_create_tree",
			path: "foo/bar/data.json",
			data: testObj{
				Foo: "bar",
				Bar: 1,
				Baz: nil,
			},
			fs: map[string]string{},
			wantFS: map[string]string{
				"foo/bar/data.json": testToJSON(t, testObj{
					Foo: "bar",
					Bar: 1,
					Baz: nil,
				}),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			path := filepath.Join(base, tc.path)
			testPopulateFiles(t, base, tc.fs)

			if diff := testutil.DiffErrString(StoreJSONFile(path, &tc.data), tc.wantError); diff != "" {
				t.Errorf("unexpected err: %s", diff)
			}

			if diff := cmp.Diff(loadDirContents(t, base), tc.wantFS); diff != "" {
				t.Errorf("got unexpected file system state:\n%s", diff)
			}
		})
	}
}

func testPopulateFiles(t *testing.T, base string, nameContents map[string]string) {
	t.Helper()
	for name, contents := range nameContents {
		//nolint:gosec // if you look at os.Create() 666 is the default. User's umask may further restrict permissions.
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(name)), []byte(contents), 0o666); err != nil {
			t.Fatalf("Could not write file %v: %v", name, err)
		}
	}
}

func testToJSON(t *testing.T, data any) string {
	t.Helper()
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(data); err != nil {
		t.Fatalf("could not encode json: %v", err)
	}
	return buf.String()
}

// loadDirContents reads all the files recursively under "dir", returning their contents as a
// map[filename]->string. Returns nil if dir doesn't exist. Keys use slash separators, not
// native.
func loadDirContents(t *testing.T, dir string) map[string]string {
	t.Helper()

	if _, err := os.Stat(dir); err != nil {
		t.Fatal(err)
	}

	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("ReadFile(): %w", err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("Rel(): %w", err)
		}
		out[filepath.ToSlash(rel)] = string(contents)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir(): %v", err)
	}
	return out
}
