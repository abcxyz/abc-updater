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
	"testing"
)

func Test_generateInstallID(t *testing.T) {
	t.Parallel()
	got, err := generateInstallID()
	if err != nil {
		t.Fatalf("generating install ID should never return an err: %s", err.Error())
	}
	if got, want := len(got), 12; got != want {
		t.Errorf("unexpected id length got=%d want=%d", got, want)
	}
}
