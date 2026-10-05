// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat

import (
	"context"
	"slices"
	"time"

	"github.com/golang/glog"
)

// Publisher re-stamps the entry for every recently-active session on a fixed
// interval until the context is cancelled.
type Publisher interface {
	// Run refreshes active sessions every interval and returns nil when the
	// context is cancelled. A failing write is logged and skipped: a broken
	// liveness path must not take prompt serving down.
	Run(ctx context.Context) error
}

// NewPublisher returns a Publisher that re-stamps every session active within
// IdleCutoff once per interval.
func NewPublisher(recorder ActivityRecorder, writer Writer, interval time.Duration) Publisher {
	return &publisher{
		recorder: recorder,
		writer:   writer,
		interval: interval,
	}
}

type publisher struct {
	recorder ActivityRecorder
	writer   Writer
	interval time.Duration
}

func (p *publisher) Run(ctx context.Context) error {
	glog.V(2).Infof("cluster heartbeat publisher started interval=%s", p.interval)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case sessionID := <-p.recorder.Notifications():
			p.refreshOne(ctx, sessionID)
		case <-ticker.C:
			p.refresh(ctx)
		}
	}
}

// refresh re-stamps every session active within IdleCutoff. A failing write is
// logged and the loop continues, so one session's cluster error never stops the
// others from being refreshed.
func (p *publisher) refresh(ctx context.Context) {
	for _, sessionID := range p.recorder.Active(ctx, IdleCutoff) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		p.writeOne(ctx, sessionID)
	}
}

// refreshOne stamps one notified session, but only while it is still active.
//
// The cutoff is checked here and not only in refresh because a notification
// can be drained late — the loop may have been inside a slow write — and a
// session that has since gone idle must not be stamped back to life.
func (p *publisher) refreshOne(ctx context.Context, sessionID string) {
	if !slices.Contains(p.recorder.Active(ctx, IdleCutoff), sessionID) {
		return
	}
	p.writeOne(ctx, sessionID)
}

// writeOne stamps one session. A failing write is logged and swallowed: a
// broken liveness path must not take prompt serving down.
func (p *publisher) writeOne(ctx context.Context, sessionID string) {
	if err := p.writer.Write(ctx, sessionID); err != nil {
		glog.Warningf("cluster heartbeat write failed session=%s: %v", sessionID, err)
	}
}
