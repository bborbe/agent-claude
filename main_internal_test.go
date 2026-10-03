// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is package main (not main_test) because the application struct is
// unexported and an external test cannot reach it. Ginkgo registers specs in one
// global suite per test binary, so the RunSpecs already in main_test.go runs them.
package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/bborbe/argument/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("application argument parsing", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		// argument.Parse registers flags on the global flag.CommandLine, so a
		// second Parse in the same process panics with "flag redefined" unless
		// the flag set is reset first. Same reset the argument package's own
		// argument_parse_test.go performs.
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		os.Args = []string{"agent-claude"}
	})

	Context("with a service-agent environment", func() {
		BeforeEach(func() {
			Expect(os.Setenv("AGENT_TYPE", "service")).To(Succeed())
			DeferCleanup(func() { _ = os.Unsetenv("AGENT_TYPE") })
			Expect(os.Unsetenv("TASK_CONTENT")).To(Succeed())
			Expect(os.Unsetenv("TASK_ID")).To(Succeed())
		})

		It(
			"parses with no error, with the service agent type and the default listen address",
			func() {
				app := &application{}
				err := argument.Parse(ctx, app)
				Expect(err).NotTo(HaveOccurred())
				Expect(app.AgentType).To(Equal("service"))
				Expect(app.Listen).To(Equal(":9090"))
			},
		)
	})

	Context("with a task-agent environment", func() {
		BeforeEach(func() {
			Expect(os.Setenv("TASK_CONTENT", "# a task")).To(Succeed())
			DeferCleanup(func() { _ = os.Unsetenv("TASK_CONTENT") })
			Expect(os.Unsetenv("AGENT_TYPE")).To(Succeed())
		})

		It("parses with no error and leaves the agent type empty", func() {
			app := &application{}
			err := argument.Parse(ctx, app)
			Expect(err).NotTo(HaveOccurred())
			Expect(app.AgentType).To(Equal(""))
		})
	})

	It("binds INTERACTIVE_AUTH_TOKEN into the struct field", func() {
		Expect(os.Setenv("INTERACTIVE_AUTH_TOKEN", "test-token")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("INTERACTIVE_AUTH_TOKEN") })
		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.InteractiveAuthToken).To(Equal("test-token"))
	})
})

var _ = Describe("application.buildClaudeEnv", func() {
	It("returns an empty map when no env source is configured", func() {
		app := &application{}
		Expect(app.buildClaudeEnv()).To(BeEmpty())
	})

	It("parses the ad-hoc CLAUDE_ENV pairs", func() {
		app := &application{ClaudeEnvRaw: "GH_TOKEN=abc,FOO=bar"}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"GH_TOKEN": "abc",
			"FOO":      "bar",
		}))
	})

	It("lets the dedicated Anthropic fields override the same keys from CLAUDE_ENV", func() {
		app := &application{
			ClaudeEnvRaw:       "ANTHROPIC_MODEL=from-claude-env,FOO=bar",
			AnthropicBaseURL:   "https://api.minimax.io/anthropic",
			AnthropicAuthToken: "secret",
			AnthropicModel:     "sonnet",
		}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"ANTHROPIC_BASE_URL":   "https://api.minimax.io/anthropic",
			"ANTHROPIC_AUTH_TOKEN": "secret",
			"ANTHROPIC_MODEL":      "sonnet",
			"FOO":                  "bar",
		}))
	})
})

var _ = Describe("application.runService", func() {
	// The service agent's whole point is that it outlives a task and answers when
	// addressed. Run blocks until the context is cancelled and then returns nil,
	// which is what makes returning it directly from Run correct. Listen is port 0
	// so the spec never contends for the :9090 the executor's probe targets.
	It("serves until the context is cancelled, then returns nil", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := &application{Listen: "127.0.0.1:0", InteractiveAuthToken: "test-token"}
		done := make(chan error, 1)
		go func() {
			done <- app.runService(ctx, prometheus.NewRegistry(), map[string]string{})
		}()

		Consistently(done, 100*time.Millisecond).ShouldNot(Receive())
		cancel()
		Eventually(done, 10*time.Second).Should(Receive(BeNil()))
	})

	// Regression guard for the shape-scoped enforcement: the check lives in runService
	// (where the agent shape is known) rather than in a required:"true" struct tag,
	// because the tag is evaluated for every agent shape and would also reject
	// task-routed jobs. This is the boundary the new code crosses — a struct-equality or
	// constant-value assertion would not exercise it.
	It("fails to start when no interactive auth token is configured", func() {
		app := &application{Listen: "127.0.0.1:0"}
		err := app.runService(context.Background(), prometheus.NewRegistry(), map[string]string{})
		Expect(err).To(HaveOccurred())
	})
})
