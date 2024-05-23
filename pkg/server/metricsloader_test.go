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

package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abcxyz/pkg/renderer"
)

const testAppID = "testApp"

func setupTestServer(tb testing.TB, allowed map[string]AllowedMetricsResponse, returnError *int) *httptest.Server {
	tb.Helper()
	ren, err := renderer.New(r.Context(), nil, renderer.WithOnError(func(err error) {
		tb.Fatalf("error rendering json in test server: %s", err.Error())
	}))
	if err != nil {
		tb.Fatalf("error creating renderer for test server: %w", err.Error())
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if returnError != nil {
			ren.RenderJSON(w, *returnError, fmt.Errorf("something went wrong for testing purposes"))
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/manifest.json":
			appList := make([]string, 0, len(allowed))
			for k, _ := range allowed {
				appList = append(appList, k)
			}
			response := ManifestResponse{appList}
			ren.RenderJSON(w, http.StatusOK, &response)
			return

		case r.Method == http.MethodGet && r.URL.Path == "/metrics.json":
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) >= 2 {
				if appID := parts[len(parts)-2]; appID != "" {
					if v, ok := allowed[appID]; ok {
						ren.RenderJSON(w, http.StatusOK, &v)
						return
					}
				}
			}
			// Technically this is xml with current implementation, but we don't care about parsing error bodies.
			ren.RenderJSON(w, http.StatusNotFound, fmt.Errorf("noSuchKey"))
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return

		default:
			// Technically this is xml with current implementation, but we don't care about parsing error bodies.
			ren.RenderJSON(w, http.StatusNotFound, fmt.Errorf("noSuchKey"))
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}
	}))

	tb.Cleanup(func() {
		ts.Close()
	})
	return ts
}

func TestMetricsDB_Update(t *testing.T) {

}
