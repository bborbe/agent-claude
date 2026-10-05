// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package heartbeat publishes a liveness entry per actively-served session
// into the claude-worker-heartbeats ConfigMap, in the shape the cluster
// liveness reader (scripts/cluster-heartbeat.py in bborbe/claude-supervisor)
// consumes. The reader is the source of truth and is not modified: it maps
// each session id to a JSON object carrying a refreshedAt field in RFC3339
// form and treats a stamp older than 60 seconds as dead.
package heartbeat

import (
	"time"

	"github.com/bborbe/k8s"
)

// ConfigMapName is the ConfigMap the reader reads and this package writes.
const ConfigMapName k8s.Name = "claude-worker-heartbeats"

// RefreshInterval is how often an active session's entry is re-stamped. It is
// deliberately shorter than the reader's 60-second TTL — three refreshes fit
// in one TTL window, so a single missed tick cannot make a live worker read as
// stale.
const RefreshInterval = 20 * time.Second

// IdleCutoff is how long a session may go without completing a prompt before
// its entry stops being refreshed. The entry then ages out of the reader's
// view on its own, so an idle conversation does not read as a live worker.
const IdleCutoff = 90 * time.Second
