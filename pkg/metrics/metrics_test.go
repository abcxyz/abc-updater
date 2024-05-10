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

package metrics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCreateMetrics(t *testing.T) {

}

func TestSendMetricsSync(t *testing.T) {
	t.Parallel()

	// Record calls made to test server. Separate per test using a per-test
	// unique id in URL.
	reqMap := sync.Map{}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, prevExist := reqMap.Swap(r.RequestURI, r)
		if prevExist {
			t.Fatalf("multiple requests to same url: %s", r.RequestURI)
		}
		if !strings.HasSuffix(r.RequestURI, "/sendMetrics") {
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, http.StatusText(http.StatusNotFound))
			return
		}

		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))

	t.Cleanup(func() {
		ts.Close()
	})

	cases := []struct {
		name    string
		appID   string
		version string
		metrics map[string]int
		env     map[string]string
		wantRequest map[string]any
		wantErr string
		installID  *InstallIDData
	}{
		{
			name: "one_metric_success"
		},
	}

}
