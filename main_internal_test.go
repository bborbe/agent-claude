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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	interactive "github.com/bborbe/agent/interactive"
	agentmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/argument/v2"
	"github.com/bborbe/k8s"
	k8smocks "github.com/bborbe/k8s/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
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

	It("binds A2A_PUBLIC_URL into the struct field", func() {
		Expect(os.Setenv("A2A_PUBLIC_URL", "https://agent.example.test/a2a")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("A2A_PUBLIC_URL") })
		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.A2APublicURL).To(Equal("https://agent.example.test/a2a"))
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
	//
	// With the heartbeat wiring this also exercises the degrade path: the test
	// process has no in-cluster service-account mount, so k8s.CreateClientset("")
	// fails at the call site and buildHeartbeatPublisher is never reached.
	// runService logs and serves anyway, and this spec passing is the evidence
	// that a broken liveness path does not stop prompt serving.
	It("serves until the context is cancelled, then returns nil", func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		app := &application{
			Listen:               "127.0.0.1:0",
			InteractiveAuthToken: "test-token",
			A2APublicURL:         "https://agent.example.test/a2a",
		}
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
		Expect(err).To(MatchError(ContainSubstring("INTERACTIVE_AUTH_TOKEN")))
	})

	// The guard is exercised in a goroutine on purpose: if the check were missing, the
	// synchronous call would start serving and block until the suite timeout instead of
	// failing fast, so the failure mode would be a hang rather than a clear assertion.
	It("fails to start when no A2A public URL is configured", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		app := &application{Listen: "127.0.0.1:0", InteractiveAuthToken: "test-token"}
		done := make(chan error, 1)
		go func() {
			done <- app.runService(ctx, prometheus.NewRegistry(), map[string]string{})
		}()

		Eventually(
			done,
			5*time.Second,
		).Should(Receive(MatchError(ContainSubstring("A2A_PUBLIC_URL"))))
	})
})

// The three address settings passed to the interactive service — Listen, ProviderBaseURL
// and A2APublicURL — are all strings, so a misordered constructor call compiles and passes
// any struct-equality check. This spec serves the real Agent Card route and asserts the
// advertised URL, which is the only check that tells the two orders apart.
var _ = Describe("application.newInteractiveService", func() {
	It("advertises the configured A2A public URL in the Agent Card", func() {
		app := &application{
			Listen:               "127.0.0.1:0",
			ProviderBaseURL:      "http://provider.example.test",
			InteractiveAuthToken: "test-token",
			A2APublicURL:         "https://agent.example.test/a2a",
		}
		service := app.newInteractiveService(
			&agentmocks.SessionFactory{},
			prometheus.NewRegistry(),
			interactive.NewPermissionRegistry(),
		)

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil)
		service.Handler().ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		Expect(recorder.Body.String()).To(ContainSubstring("https://agent.example.test/a2a"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("provider.example.test"))
		Expect(recorder.Body.String()).NotTo(ContainSubstring("127.0.0.1"))
	})
})

var _ = Describe("inClusterNamespace", func() {
	It("reads the namespace and trims the trailing newline", func() {
		path := filepath.Join(GinkgoT().TempDir(), "namespace")
		Expect(os.WriteFile(path, []byte("dev\n"), 0600)).To(Succeed())

		namespace, err := inClusterNamespace(context.Background(), path)

		Expect(err).NotTo(HaveOccurred())
		Expect(namespace).To(Equal(k8s.Namespace("dev")))
	})

	It("returns an error when the file is absent", func() {
		path := filepath.Join(GinkgoT().TempDir(), "missing")

		_, err := inClusterNamespace(context.Background(), path)

		Expect(err).To(HaveOccurred())
	})

	It("returns an error when the file holds only whitespace", func() {
		path := filepath.Join(GinkgoT().TempDir(), "namespace")
		Expect(os.WriteFile(path, []byte("  \n"), 0600)).To(Succeed())

		_, err := inClusterNamespace(context.Background(), path)

		Expect(err).To(HaveOccurred())
	})
})

// buildHeartbeatPublisher takes the clientset as a parameter so these specs can
// exercise both of its paths. The only runService spec takes the degrade branch
// (no in-cluster mount), so without these the helper would be untested.
var _ = Describe("buildHeartbeatPublisher", func() {
	// inClusterNamespacePath is a package-level seam: the helper reads the constant
	// path, so each spec points it at a temp file for the duration of the spec and
	// restores the original afterwards.
	It("builds a publisher when the pod namespace resolves", func() {
		path := filepath.Join(GinkgoT().TempDir(), "namespace")
		Expect(os.WriteFile(path, []byte("dev\n"), 0600)).To(Succeed())
		original := inClusterNamespacePath
		inClusterNamespacePath = path
		DeferCleanup(func() { inClusterNamespacePath = original })

		publisher, err := buildHeartbeatPublisher(
			context.Background(),
			&k8smocks.K8sInterface{},
			heartbeat.NewActivityRecorder(libtime.NewCurrentDateTime()),
			libtime.NewCurrentDateTime(),
		)

		Expect(err).NotTo(HaveOccurred())
		Expect(publisher).NotTo(BeNil())
	})

	It("returns an error when the pod namespace cannot be resolved", func() {
		original := inClusterNamespacePath
		inClusterNamespacePath = filepath.Join(GinkgoT().TempDir(), "missing")
		DeferCleanup(func() { inClusterNamespacePath = original })

		publisher, err := buildHeartbeatPublisher(
			context.Background(),
			&k8smocks.K8sInterface{},
			heartbeat.NewActivityRecorder(libtime.NewCurrentDateTime()),
			libtime.NewCurrentDateTime(),
		)

		Expect(err).To(HaveOccurred())
		Expect(publisher).To(BeNil())
	})
})
