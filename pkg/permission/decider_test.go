// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission_test

import (
	"context"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"time"

	claudelib "github.com/bborbe/agent/claude"
	interactive "github.com/bborbe/agent/interactive"
	agentmocks "github.com/bborbe/agent/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/mocks"
	"github.com/bborbe/agent-claude/pkg/permission"
)

var _ = Describe("Decider", func() {
	var (
		ctx      context.Context
		registry *agentmocks.ClaudePermissionDecider
		gate     *mocks.PermissionGate
		decider  claudelib.PermissionDecider
	)

	BeforeEach(func() {
		ctx = context.Background()
		registry = &agentmocks.ClaudePermissionDecider{}
		gate = &mocks.PermissionGate{}
		decider = permission.NewDecider(registry, gate)
	})

	It("returns the registry's decision for a /prompt request and never calls the gate", func() {
		registry.DecidePermissionReturns(claudelib.PermissionDecision{Allow: true}, nil)

		decision, err := decider.DecidePermission(ctx, claudelib.PermissionRequest{
			ToolName: "Bash",
			Source:   claudelib.PromptPermissionSource,
		})

		Expect(err).To(BeNil())
		Expect(decision.Allow).To(BeTrue())
		Expect(registry.DecidePermissionCallCount()).To(Equal(1))
		Expect(gate.DecideCallCount()).To(Equal(0))
	})

	It("treats a request with an empty Source as the compatibility path", func() {
		registry.DecidePermissionReturns(
			claudelib.PermissionDecision{Allow: false, Message: "no"},
			nil,
		)

		decision, err := decider.DecidePermission(ctx, claudelib.PermissionRequest{
			ToolName: "Bash",
		})

		Expect(err).To(BeNil())
		Expect(decision.Allow).To(BeFalse())
		Expect(decision.Message).To(Equal("no"))
		Expect(registry.DecidePermissionCallCount()).To(Equal(1))
		Expect(gate.DecideCallCount()).To(Equal(0))
	})

	It("returns the gate's decision for an A2A request and never calls the registry", func() {
		gate.DecideReturns(claudelib.PermissionDecision{Allow: true})

		decision, err := decider.DecidePermission(ctx, claudelib.PermissionRequest{
			ToolName: "Bash",
			Source:   claudelib.A2APermissionSource,
		})

		Expect(err).To(BeNil())
		Expect(decision.Allow).To(BeTrue())
		Expect(gate.DecideCallCount()).To(Equal(1))
		Expect(registry.DecidePermissionCallCount()).To(Equal(0))
	})

	It("hands the gate the request it was given, Source included", func() {
		request := claudelib.PermissionRequest{
			ToolName:     "Bash",
			Description:  "run a command",
			InputPreview: "rm -rf /",
			Source:       claudelib.A2APermissionSource,
		}

		_, err := decider.DecidePermission(ctx, request)

		Expect(err).To(BeNil())
		_, got := gate.DecideArgsForCall(0)
		Expect(got).To(Equal(request))
		Expect(got.Source).To(Equal(claudelib.A2APermissionSource))
	})

	It("returns a registry error on a /prompt request unwrapped and unchanged", func() {
		sentinel := stderrors.New("registry failure")
		registry.DecidePermissionReturns(claudelib.PermissionDecision{}, sentinel)

		_, err := decider.DecidePermission(ctx, claudelib.PermissionRequest{
			ToolName: "Bash",
			Source:   claudelib.PromptPermissionSource,
		})

		Expect(err).To(BeIdenticalTo(sentinel))
		Expect(gate.DecideCallCount()).To(Equal(0))
	})
})

// The negative evidence for the interactive path, driven through the production
// routing rather than by calling the registry directly. The whole risk of the A2A
// gate is that the decider sitting in front of the registry for every session
// diverts a /prompt gate to the store; the counter-mock spec above proves the call
// count, and these specs prove the consequence — no poster process, no store item.
//
// The assertion is deliberately a pair. A request reaching the registry alone is
// satisfiable by a decider that also posts to the store, and a poster that never ran
// alone is satisfiable by a decider that dropped the request on the floor. Both must
// hold at once, so a broken implementation cannot pass by satisfying one of them.
var _ = Describe("Decider interactive path", func() {
	var (
		dir        string
		markerPath string
		registry   interactive.PermissionRegistry
		decider    claudelib.PermissionDecider
	)

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
		markerPath = filepath.Join(dir, "poster-ran")
		// A poster that, were it ever invoked, would leave a marker inside the temp
		// dir and print an allow verdict. Either one would be a failure: this request
		// must produce no store traffic at all. The marker lives in the temp dir so
		// Ginkgo removes it with the rest of the spec's files.
		posterPath := writePoster(dir, fmt.Sprintf(
			"#!/bin/sh\n: > %q\necho 'DECISION: allow -- should never run'\nexit 0\n",
			markerPath,
		))

		registry = interactive.NewPermissionRegistry()
		gate := permission.NewGate(posterPath, "test-producer")
		decider = permission.NewDecider(registry, gate)
	})

	// markerAbsent polls the marker file over a window long enough to have caught a
	// synchronous poster invocation. A value actual would be checked once and prove
	// nothing about the time the decider had to shell out; the function actual makes
	// Gomega poll for the whole window.
	markerAbsent := func() {
		Consistently(func() string {
			return markerPath
		}, 100*time.Millisecond).ShouldNot(BeAnExistingFile())
	}

	It("routes a /prompt request to the registry and never invokes the poster", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		// DecidePermission blocks until a verdict or cancellation, so the turn runs in
		// a goroutine and the spec reads the registry while it is parked there.
		go func() {
			_, _ = decider.DecidePermission(ctx, claudelib.PermissionRequest{
				ToolName: "Bash",
				Source:   claudelib.PromptPermissionSource,
			})
		}()

		Eventually(registry.List).Should(HaveLen(1))
		Expect(registry.List()[0].ToolName).To(Equal("Bash"))
		markerAbsent()

		cancel()
		Expect(markerPath).NotTo(BeAnExistingFile())
	})

	It("routes an empty-Source request the same way, with no store traffic", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		go func() {
			_, _ = decider.DecidePermission(ctx, claudelib.PermissionRequest{
				ToolName: "Read",
			})
		}()

		Eventually(registry.List).Should(HaveLen(1))
		Expect(registry.List()[0].ToolName).To(Equal("Read"))
		markerAbsent()

		cancel()
		Expect(markerPath).NotTo(BeAnExistingFile())
	})
})
