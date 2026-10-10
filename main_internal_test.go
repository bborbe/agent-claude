// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is package main (not main_test) because the application struct is
// unexported and an external test cannot reach it. Ginkgo registers specs in one
// global suite per test binary, so the RunSpecs already in main_test.go runs them.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
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

	It("binds A2A_AGENT_NAME into the struct field", func() {
		Expect(os.Setenv("A2A_AGENT_NAME", "test-agent-name")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("A2A_AGENT_NAME") })
		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.A2AAgentName).To(Equal("test-agent-name"))
	})

	// The whole key-names-only fix rests on the argument parser filling a named
	// string type from the env var unchanged. A struct-field assertion would not
	// prove that, so this parses the real env var and reads the pairs back.
	It("binds CLAUDE_ENV into the struct field as parsed pairs", func() {
		Expect(os.Setenv("CLAUDE_ENV", "A=1,B=2")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("CLAUDE_ENV") })
		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.ClaudeEnvRaw.Pairs()).To(Equal(map[string]string{"A": "1", "B": "2"}))
	})

	It("binds ENV_CONTEXT into the struct field as parsed pairs", func() {
		Expect(os.Setenv("ENV_CONTEXT", "A=1,B=2")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("ENV_CONTEXT") })
		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.EnvContextRaw.Pairs()).To(Equal(map[string]string{"A": "1", "B": "2"}))
	})
})

// The startup configuration log is written by a library
// (github.com/bborbe/argument/v2), which honours only display:"hidden" and
// display:"length" — every other display value falls through to the branch that
// logs the field verbatim. A struct-tag assertion would prove only that a string
// was typed, not that the library redacts it, so this spec captures what the
// printer actually writes.
var _ = Describe("application startup argument log", func() {
	It("reports the Anthropic auth token as a length, never its value", func() {
		const sentinel = "sentinel-anthropic-auth-token-8f3a"

		var buffer bytes.Buffer
		log.SetOutput(&buffer)
		DeferCleanup(func() { log.SetOutput(os.Stderr) })

		app := &application{AnthropicAuthToken: sentinel}
		Expect(argument.Print(context.Background(), app)).To(Succeed())

		output := buffer.String()
		Expect(output).To(ContainSubstring("AnthropicAuthToken"))
		Expect(output).To(ContainSubstring(
			fmt.Sprintf("AnthropicAuthToken length %d", len(sentinel)),
		))
		Expect(output).NotTo(ContainSubstring(sentinel))
	})

	It("reports the attention token as a length, never its value", func() {
		const sentinel = "sentinel-pod-attention-token-4d19"

		var buffer bytes.Buffer
		log.SetOutput(&buffer)
		DeferCleanup(func() { log.SetOutput(os.Stderr) })

		app := &application{AttentionToken: sentinel}
		Expect(argument.Print(context.Background(), app)).To(Succeed())

		output := buffer.String()
		Expect(output).To(ContainSubstring("AttentionToken"))
		Expect(output).To(ContainSubstring(
			fmt.Sprintf("AttentionToken length %d", len(sentinel)),
		))
		Expect(output).NotTo(ContainSubstring(sentinel))
	})

	It("renders GatewaySecret as a length, never its value", func() {
		const sentinel = "sentinel-gateway-secret-4f18"

		var buffer bytes.Buffer
		log.SetOutput(&buffer)
		DeferCleanup(func() { log.SetOutput(os.Stderr) })

		app := &application{GatewaySecret: sentinel}
		Expect(argument.Print(context.Background(), app)).To(Succeed())

		output := buffer.String()
		Expect(output).To(ContainSubstring("GatewaySecret"))
		Expect(output).To(ContainSubstring(
			fmt.Sprintf("GatewaySecret length %d", len(sentinel)),
		))
		Expect(output).NotTo(ContainSubstring(sentinel))
	})

	// Either bag can carry a credential: buildClaudeEnv sources
	// ANTHROPIC_AUTH_TOKEN from CLAUDE_ENV when its dedicated field is empty.
	// The second bag deliberately uses a key name no marker heuristic would
	// flag, so this spec distinguishes key-names-only from a
	// marker-based redaction that would leave an innocuously named secret in
	// the log.
	It("renders the CLAUDE_ENV and ENV_CONTEXT bags as key names only, never their values", func() {
		const claudeEnvSentinel = "sentinel-claude-env-2c71"
		const envContextSentinel = "sentinel-env-context-9b04"

		var buffer bytes.Buffer
		log.SetOutput(&buffer)
		DeferCleanup(func() { log.SetOutput(os.Stderr) })

		app := &application{
			ClaudeEnvRaw:  "ANTHROPIC_AUTH_TOKEN=" + claudeEnvSentinel + ",FOO=bar",
			EnvContextRaw: "INNOCUOUS_NAME=" + envContextSentinel,
		}
		Expect(argument.Print(context.Background(), app)).To(Succeed())

		output := buffer.String()
		Expect(output).To(ContainSubstring("Argument: ClaudeEnvRaw 'ANTHROPIC_AUTH_TOKEN,FOO'"))
		Expect(output).To(ContainSubstring("Argument: EnvContextRaw 'INNOCUOUS_NAME'"))
		Expect(output).NotTo(ContainSubstring(claudeEnvSentinel))
		Expect(output).NotTo(ContainSubstring(envContextSentinel))
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

	// The library replaces the child environment with a fixed allowlist, so a pod
	// whose own env carries the attention store still leaves its Claude child
	// unable to reach it — the poster falls back to localhost:18080, where nothing
	// listens. These two names ride the map that becomes the library's final env
	// layer, which is what closes that gap.
	It("forwards the attention-store vars to the Claude child", func() {
		app := &application{
			AttentionStoreURL: "http://attention-controller:18081",
			AttentionToken:    "pod-attention-token",
		}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"POD_ATTENTION_STORE_URL": "http://attention-controller:18081",
			"POD_ATTENTION_TOKEN":     "pod-attention-token",
		}))
	})

	It("omits the attention-store vars when unset, leaving a laptop run unchanged", func() {
		app := &application{ClaudeEnvRaw: "FOO=bar"}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{"FOO": "bar"}))
	})

	// The same allowlist gap as the attention-store pair above, with a sharper
	// symptom: the vault service refuses every /api/v1/* request without
	// X-Gateway-Secret, so a pod whose own env carries the secret still leaves its
	// Claude child unable to read a vault file — and the refusal is a 401, which
	// reads like a wrong secret rather than a missing forwarding.
	It("forwards GATEWAY_SECRET to the Claude child", func() {
		app := &application{GatewaySecret: "vault-gateway-secret"}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"GATEWAY_SECRET": "vault-gateway-secret",
		}))
	})

	It("lets the dedicated GatewaySecret field override the same key from CLAUDE_ENV", func() {
		app := &application{
			ClaudeEnvRaw:  "GATEWAY_SECRET=from-claude-env,FOO=bar",
			GatewaySecret: "from-field",
		}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"GATEWAY_SECRET": "from-field",
			"FOO":            "bar",
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
			A2AAgentName:         "claude-interactive",
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

		app := &application{
			Listen:               "127.0.0.1:0",
			InteractiveAuthToken: "test-token",
			A2AAgentName:         "claude-interactive",
		}
		done := make(chan error, 1)
		go func() {
			done <- app.runService(ctx, prometheus.NewRegistry(), map[string]string{})
		}()

		Eventually(
			done,
			5*time.Second,
		).Should(Receive(MatchError(ContainSubstring("A2A_PUBLIC_URL"))))
	})

	// Same goroutine shape as the missing-URL spec, for the same reason: without the
	// guard the synchronous call would serve and block until the suite timeout.
	It("fails to start when no A2A agent name is configured", func() {
		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)

		app := &application{
			Listen:               "127.0.0.1:0",
			InteractiveAuthToken: "test-token",
			A2APublicURL:         "https://agent.example.test/a2a",
		}
		done := make(chan error, 1)
		go func() {
			done <- app.runService(ctx, prometheus.NewRegistry(), map[string]string{})
		}()

		Eventually(
			done,
			5*time.Second,
		).Should(Receive(MatchError(ContainSubstring("A2A_AGENT_NAME"))))
	})
})

// The public URL now travels inside interactive.CardConfig rather than as a positional
// string argument, so this spec guards which application values fill the card's Name and
// PublicURL fields (PublicURL from A2APublicURL, never from Listen). It serves the real
// Agent Card route and asserts the advertised name and URL.
var _ = Describe("application.newInteractiveService", func() {
	It("advertises the configured A2A public URL in the Agent Card", func() {
		app := &application{
			Listen:               "127.0.0.1:0",
			ProviderBaseURL:      "http://provider.example.test",
			InteractiveAuthToken: "test-token",
			A2APublicURL:         "https://agent.example.test/a2a",
			A2AAgentName:         "claude-interactive",
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

		var card struct {
			Name string `json:"name"`
		}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &card)).To(Succeed())
		Expect(card.Name).To(Equal("claude-interactive"))
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
