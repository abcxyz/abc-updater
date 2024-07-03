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
	"github.com/abcxyz/pkg/logging"
)

// asyncFunctionCall handles the async part of SendMetricRequest, but accepts
// a function other than SendMetricRequestSync for testing.
func asyncFunctionCall(ctx context.Context, funcToCall func() error) func() {
	doneCh := make(chan string, 1)

	go func() {
		defer close(doneCh)
		err := funcToCall()
		if err != nil {
			logging.FromContext(ctx).DebugContext(ctx, "failed to log metrics",
				"error", err)
			return
		}
	}()

	return func() {
		select {
		case <-ctx.Done():
			// Context was cancelled
		case _, _ = <-doneCh:
			// Program returned
		}
	}
}
