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

package metrics

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// Note: These tests rely on timing and could be flaky if breakpoints are used.
func Test_asyncFunctionCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input func() error
	}{
		{
			name:  "happy_path_returns",
			input: func() error { return nil },
		},
		{
			name:  "error_path_still_returns",
			input: func() error { return fmt.Errorf("I had an issue") },
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resultFunc := asyncFunctionCall(context.Background(), tc.input)
			resultFunc()
		})
	}
}

// This test is timing dependent, but would require to be paused for more than
// an hour to cause flakiness.
func Test_asyncFunctionCallContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	inputFunc := func() error {
		t.Helper()
		time.Sleep(1 * time.Hour)
		t.Fatalf("this should not execute")
		return fmt.Errorf("should not execute")
	}
	resultFunc := asyncFunctionCall(ctx, inputFunc)

	// Context canceled before timeouts.
	cancel()

	// Should return immediately since context was canceled.
	resultFunc()
}

func Test_asyncFunctionCallDispatches(t *testing.T) {
	t.Parallel()
	runs := int64(0)

	inputFunc := func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	}
	resultFunc := asyncFunctionCall(context.Background(), inputFunc)

	resultFunc()

	if got, want := atomic.LoadInt64(&runs), int64(1); got != want {
		t.Errorf("function ran unexpected number of times. got: %v want: %v", got, want)
	}
}
