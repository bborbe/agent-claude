// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package envbag provides a string type for a comma-separated KEY=VALUE bag
// whose log rendering shows only the key names, never the values.
package envbag

import (
	"sort"
	"strings"

	"github.com/bborbe/agent/envparse"
)

// KeyValueList is a comma-separated KEY=VALUE bag, the shape supplied through
// the CLAUDE_ENV and ENV_CONTEXT settings. Its underlying type is string, so
// the argument parser fills it from a flag or an env var unchanged.
//
// KeyValueList implements fmt.Stringer because the startup configuration log
// (github.com/bborbe/argument/v2) formats every untagged field with %v, which
// calls String on a value that implements it. The rendering deliberately omits
// every value: either bag can carry a credential — ANTHROPIC_AUTH_TOKEN is read
// from CLAUDE_ENV when its dedicated field is empty — so a value must never
// reach the log.
type KeyValueList string

// String returns the key names of the parsed pairs, sorted and joined with
// ",", so the startup log still shows which variables a pod received without
// exposing any value. It is the log rendering only: consumers read the values
// through Pairs. An empty bag renders as the empty string, as does a bag whose
// entries all lack '='.
func (k KeyValueList) String() string {
	pairs := k.Pairs()
	if len(pairs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

// Pairs returns the KEY=VALUE pairs the bag carries, parsed with
// envparse.KeyValuePairs. Every runtime consumer must go through this method so
// it receives exactly the pairs the raw string holds; String is the log
// rendering and is not a substitute for it. An empty bag yields nil.
func (k KeyValueList) Pairs() map[string]string {
	return envparse.KeyValuePairs(string(k))
}
