// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat

import (
	"context"

	agentlib "github.com/bborbe/agent"
)

// NewObservingSessionFactory wraps delegate so that every session it builds
// records its id with recorder when a prompt returns. The library calls
// SessionFactory.Create only on first use and never again, so the activity
// signal has to be captured on the Session it returns, not on Create.
func NewObservingSessionFactory(
	delegate agentlib.SessionFactory,
	recorder ActivityRecorder,
) agentlib.SessionFactory {
	return &observingSessionFactory{
		delegate: delegate,
		recorder: recorder,
	}
}

type observingSessionFactory struct {
	delegate agentlib.SessionFactory
	recorder ActivityRecorder
}

func (f *observingSessionFactory) Create(id string) agentlib.Session {
	return &observingSession{
		delegate: f.delegate.Create(id),
		recorder: f.recorder,
		id:       id,
	}
}

type observingSession struct {
	delegate agentlib.Session
	recorder ActivityRecorder
	id       string
}

func (s *observingSession) Prompt(ctx context.Context, prompt string) (string, error) {
	result, err := s.delegate.Prompt(ctx, prompt)
	// Record after the delegate returns: the idle cutoff is measured from the
	// last completed prompt. Record unconditionally — a turn that failed still
	// means the worker was addressed and is running.
	s.recorder.Record(ctx, s.id)
	return result, err
}

func (s *observingSession) Close(ctx context.Context) error {
	return s.delegate.Close(ctx)
}
