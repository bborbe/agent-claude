// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"errors"
	"time"

	agentlib "github.com/bborbe/agent"
	agentmocks "github.com/bborbe/agent/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

var _ = Describe("ObservingSessionFactory", func() {
	var (
		ctx             context.Context
		delegate        *agentmocks.SessionFactory
		session         *agentmocks.Session
		currentDateTime libtime.CurrentDateTime
		recorder        heartbeat.ActivityRecorder
		sessions        agentlib.SessionFactory
	)

	BeforeEach(func() {
		ctx = context.Background()
		currentDateTime = libtime.NewCurrentDateTime()
		currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
		recorder = heartbeat.NewActivityRecorder(currentDateTime)
		delegate = &agentmocks.SessionFactory{}
		session = &agentmocks.Session{}
		delegate.CreateReturns(session)
		sessions = heartbeat.NewObservingSessionFactory(delegate, recorder)
	})

	It("is empty before any prompt", func() {
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(BeEmpty())
	})

	It("records the session id after a prompt returns", func() {
		_, err := sessions.Create("abc").Prompt(ctx, "hello")
		Expect(err).To(BeNil())
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(Equal([]string{"abc"}))
	})

	It("returns the delegate's result unchanged", func() {
		session.PromptReturns("the answer", nil)
		result, err := sessions.Create("abc").Prompt(ctx, "hello")
		Expect(err).To(BeNil())
		Expect(result).To(Equal("the answer"))
	})

	It("records the id even when the prompt fails", func() {
		session.PromptReturns("", errors.New("boom"))
		_, err := sessions.Create("abc").Prompt(ctx, "hello")
		Expect(err).NotTo(BeNil())
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(Equal([]string{"abc"}))
	})

	It("delegates Close to the wrapped session", func() {
		Expect(sessions.Create("abc").Close(ctx)).To(Succeed())
		Expect(session.CloseCallCount()).To(Equal(1))
	})
})
