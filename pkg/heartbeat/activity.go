// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat

import (
	"context"
	"slices"
	"sync"
	"time"

	libtime "github.com/bborbe/time"
)

// ActivityRecorder remembers which sessions this service has recently served.
type ActivityRecorder interface {
	// Record marks sessionID as active as of now. It is called when a prompt
	// on that session returns.
	Record(ctx context.Context, sessionID string)

	// Active returns the ids of every session that recorded activity within
	// cutoff of now, sorted so a caller's output is deterministic.
	Active(ctx context.Context, cutoff time.Duration) []string
}

// NewActivityRecorder returns an ActivityRecorder that timestamps activity
// with the supplied clock. The clock is injected so a test can pin it.
func NewActivityRecorder(currentDateTime libtime.CurrentDateTimeGetter) ActivityRecorder {
	return &activityRecorder{
		currentDateTime: currentDateTime,
		lastActivity:    map[string]libtime.DateTime{},
	}
}

type activityRecorder struct {
	currentDateTime libtime.CurrentDateTimeGetter
	mutex           sync.Mutex
	lastActivity    map[string]libtime.DateTime
}

func (a *activityRecorder) Record(ctx context.Context, sessionID string) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	a.lastActivity[sessionID] = a.currentDateTime.Now()
}

func (a *activityRecorder) Active(ctx context.Context, cutoff time.Duration) []string {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	now := a.currentDateTime.Now().Time()
	result := make([]string, 0, len(a.lastActivity))
	for sessionID, last := range a.lastActivity {
		if now.Sub(last.Time()) < cutoff {
			result = append(result, sessionID)
		}
	}
	slices.Sort(result)
	return result
}
