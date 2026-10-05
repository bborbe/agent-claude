// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"errors"
	"time"

	"github.com/bborbe/k8s"
	k8smocks "github.com/bborbe/k8s/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

var _ = Describe("Publisher", func() {
	var (
		ctx             context.Context
		deployer        *k8smocks.K8sConfigMapDeployer
		currentDateTime libtime.CurrentDateTime
		recorder        heartbeat.ActivityRecorder
		publisher       heartbeat.Publisher
	)

	BeforeEach(func() {
		ctx = context.Background()
		deployer = &k8smocks.K8sConfigMapDeployer{}
		deployer.GetReturns(nil, errors.New("not found"))
		currentDateTime = libtime.NewCurrentDateTime()
		currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
		recorder = heartbeat.NewActivityRecorder(currentDateTime)
		publisher = heartbeat.NewPublisher(
			recorder,
			heartbeat.NewConfigMapWriter(
				deployer,
				k8s.Namespace("default"),
				heartbeat.ConfigMapName,
				currentDateTime,
			),
			10*time.Millisecond,
		)
	})

	// start runs the publisher's real loop in the background and returns the
	// cancel func plus the channel Run's result arrives on.
	start := func() (context.CancelFunc, chan error) {
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- publisher.Run(runCtx)
		}()
		return cancel, done
	}

	It("refreshes every active session on each tick and returns nil on cancel", func() {
		recorder.Record(ctx, "abc")
		recorder.Record(ctx, "def")
		cancel, done := start()
		Eventually(deployer.DeployCallCount).Should(BeNumerically(">=", 2))
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("writes a notified session at once, without waiting for a tick", func() {
		// An hour-long interval means no tick can fire during the spec, so a
		// write observed here can only have come from the notification path.
		publisher = heartbeat.NewPublisher(
			recorder,
			heartbeat.NewConfigMapWriter(
				deployer,
				k8s.Namespace("default"),
				heartbeat.ConfigMapName,
				currentDateTime,
			),
			time.Hour,
		)
		cancel, done := start()

		recorder.Record(ctx, "abc")

		Eventually(deployer.DeployCallCount).Should(Equal(1))
		_, deployed := deployer.DeployArgsForCall(0)
		Expect(deployed.Data).To(HaveKey("abc"))

		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("writes nothing when no session has been served", func() {
		cancel, done := start()
		Consistently(
			deployer.DeployCallCount,
			100*time.Millisecond,
			10*time.Millisecond,
		).Should(Equal(0))
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("does not refresh a session that has been idle past the cutoff", func() {
		recorder.Record(ctx, "abc")
		currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 2, 0, 0, time.UTC)))
		cancel, done := start()
		Consistently(
			deployer.DeployCallCount,
			100*time.Millisecond,
			10*time.Millisecond,
		).Should(Equal(0))
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})

	It("keeps running when a write fails", func() {
		deployer.DeployReturns(errors.New("configmaps is forbidden"))
		recorder.Record(ctx, "abc")
		cancel, done := start()
		Eventually(deployer.DeployCallCount).Should(BeNumerically(">=", 2))
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	})
})
