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

package updater

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// Note: These tests rely on timing and could be flaky if breakpoints are used.
func Test_asyncFunctionCall(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		input      func() (string, error)
		want       string
		wantWrites int64
	}{
		{
			name:       "happy_path",
			input:      func() (string, error) { return "done", nil },
			want:       "done\n",
			wantWrites: 1,
		},
		{
			name:       "happy_path_no_update",
			input:      func() (string, error) { return "", nil },
			want:       "",
			wantWrites: 0,
		},
		{
			name:       "error_path_still_returns",
			input:      func() (string, error) { return "", fmt.Errorf("failed") },
			want:       "",
			wantWrites: 0,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			outBuf := testWriter{}
			resultFunc := asyncFunctionCall(context.Background(), tc.input,
				func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

			resultFunc()
			if got := outBuf.Buf.String(); got != tc.want {
				t.Errorf("incorrect output got=%s, want=%s", got, tc.want)
			}
			if got := outBuf.Writes; got != tc.wantWrites {
				t.Errorf("incorrect number of interactions got=%d, want=%d", got, tc.wantWrites)
			}
		})
	}
}

// This test is timing dependent, but would require to be paused for more than
// an hour to cause flakiness.
func Test_asyncFunctionCallContextCanceled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	outBuf := testWriter{}
	inputFunc := func() (string, error) { time.Sleep(1 * time.Hour); return "should_not_execute", nil }
	resultFunc := asyncFunctionCall(ctx, inputFunc, func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

	// Context canceled before timeouts.
	cancel()

	// Should return immediately since context was canceled.
	resultFunc()
	if got := outBuf.Buf.String(); got != "" {
		t.Errorf("incorrect output got=%s, want=%s", got, "")
	}
	if got := outBuf.Writes; got != 0 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 0)
	}
}

func Test_asyncFunctionCallWaitForResultToWrite(t *testing.T) {
	t.Parallel()
	outBuf := testWriter{}
	inputFunc := func() (string, error) {
		return "foobar", nil
	}
	resultFunc := asyncFunctionCall(context.Background(), inputFunc, func(s string) { fmt.Fprintf(&outBuf, "%s\n", s) })

	// Give goroutine a reasonable time to finish (in theory this test could
	// give a false negative, in the unhappy case there is a race)
	time.Sleep(50 * time.Millisecond)

	// resultFunc() hasn't been run, no output should appear
	if got := outBuf.Buf.String(); got != "" {
		t.Errorf("incorrect output got=%s, want=%s", got, "")
	}
	if got := outBuf.Writes; got != 0 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 0)
	}

	resultFunc()

	// Now buffer should include output.
	if got := outBuf.Buf.String(); got != "foobar\n" {
		t.Errorf("incorrect output got=%s, want=%s", got, "foobar\n")
	}
	if got := outBuf.Writes; got != 1 {
		t.Errorf("incorrect number of interactions got=%d, want=%d", got, 1)
	}
}
