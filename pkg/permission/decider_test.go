// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission_test

import (
	"context"
	stderrors "errors"

	claudelib "github.com/bborbe/agent/claude"
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
