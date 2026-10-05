// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"time"

	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

var _ = Describe("ActivityRecorder", func() {
	var (
		ctx             context.Context
		currentDateTime libtime.CurrentDateTime
		recorder        heartbeat.ActivityRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		currentDateTime = libtime.NewCurrentDateTime()
		currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
		recorder = heartbeat.NewActivityRecorder(currentDateTime)
	})

	It("returns nothing before anything is recorded", func() {
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(BeEmpty())
	})

	It("returns a recorded id while it is within the cutoff", func() {
		recorder.Record(ctx, "abc")
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(Equal([]string{"abc"}))
	})

	It("drops an id once the cutoff has elapsed since its last record", func() {
		recorder.Record(ctx, "abc")
		currentDateTime.SetNow(
			libtime.DateTime(time.Date(2026, 10, 5, 12, 2, 0, 0, time.UTC)),
		)
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(BeEmpty())
	})

	It("keeps an id that records again after the cutoff would have elapsed", func() {
		recorder.Record(ctx, "abc")
		currentDateTime.SetNow(
			libtime.DateTime(time.Date(2026, 10, 5, 12, 2, 0, 0, time.UTC)),
		)
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(BeEmpty())
		recorder.Record(ctx, "abc")
		Expect(recorder.Active(ctx, heartbeat.IdleCutoff)).To(Equal([]string{"abc"}))
	})

	It("returns ids sorted", func() {
		recorder.Record(ctx, "zeta")
		recorder.Record(ctx, "alpha")
		recorder.Record(ctx, "mu")
		Expect(
			recorder.Active(ctx, heartbeat.IdleCutoff),
		).To(Equal([]string{"alpha", "mu", "zeta"}))
	})

	It("pins the spec's cadence numbers", func() {
		Expect(heartbeat.RefreshInterval).To(Equal(20 * time.Second))
		Expect(heartbeat.IdleCutoff).To(Equal(90 * time.Second))
		Expect(heartbeat.RefreshInterval).To(BeNumerically("<", 60*time.Second))
	})
})
