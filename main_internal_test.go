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

	a2a "github.com/a2aproject/a2a-go/v2/a2a"
	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	interactive "github.com/bborbe/agent/interactive"
	agentmocks "github.com/bborbe/agent/mocks"
	"github.com/bborbe/argument/v2"
	"github.com/bborbe/errors"
	"github.com/bborbe/k8s"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

// fakeK8sInterface is the zero-value clientset the heartbeat specs pass in.
// It replaces k8smocks.K8sInterface, whose generated package no longer compiles
// against k8s.io/client-go v0.38.0-alpha.2 (the typed interfaces gained
// DeleteWithResult and bborbe/k8s v1.14.23's mocks predate it). The helper only
// wraps the clientset, so embedding the interface is enough — no method is
// called on it.
type fakeK8sInterface struct {
	k8s.Interface
}

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

// TASK_TYPE is the boundary this change guards: an absent variable falls back
// to the flag's `unknown` default, while a present-but-empty variable overrides
// that default with "". Both are map misses in the dispatch table, so these two
// specs pin how the value arrives through the real parser — if either the
// default tag or the parser's present-but-empty handling changes, the fail-fast
// guard in Run would silently stop covering one of the two paths.
var _ = Describe("TASK_TYPE parsing", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		// Same reset as the argument-parsing specs above: argument.Parse
		// registers flags on the global flag.CommandLine, so a second Parse in
		// the same process panics with "flag redefined".
		flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
		os.Args = []string{"agent-claude"}
	})

	It("defaults to unknown when TASK_TYPE is absent", func() {
		Expect(os.Unsetenv("TASK_TYPE")).To(Succeed())

		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.TaskType).To(Equal("unknown"))
	})

	It("keeps the empty string when TASK_TYPE is set empty", func() {
		Expect(os.Setenv("TASK_TYPE", "")).To(Succeed())
		DeferCleanup(func() { _ = os.Unsetenv("TASK_TYPE") })

		app := &application{}
		Expect(argument.Parse(ctx, app)).To(Succeed())
		Expect(app.TaskType).To(Equal(""))
	})
})

var _ = Describe("validateTaskType", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("rejects the empty string and names TASK_TYPE and the publisher", func() {
		err := validateTaskType(ctx, agentlib.TaskType(""))
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TASK_TYPE is empty"))
		Expect(err.Error()).To(ContainSubstring("task publisher"))
	})

	It("rejects the flag's unknown default and quotes it", func() {
		err := validateTaskType(ctx, taskTypeUnset)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("TASK_TYPE is unset"))
		Expect(err.Error()).To(ContainSubstring(`"unknown"`))
	})

	// A genuinely unknown non-empty value is the provider's error to report, not
	// this helper's: the provider's message lists the accepted types.
	It("leaves a genuinely unknown non-empty value to the provider", func() {
		Expect(validateTaskType(ctx, agentlib.TaskType("bogus"))).To(Succeed())
	})

	It("accepts every task type the dispatch table registers", func() {
		Expect(validateTaskType(ctx, agentlib.TaskTypeLLM)).To(Succeed())
		Expect(validateTaskType(ctx, agentlib.TaskTypeHealthcheck)).To(Succeed())
		Expect(validateTaskType(ctx, agentlib.TaskTypeOAuthProbe)).To(Succeed())
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

	// GITHUB_APP_PEM is private key material, so it takes the same display:"length"
	// treatment as the two credentials above: the startup log names the field and
	// reports its length, and the key itself never reaches the log.
	It("reports the GitHub App PEM as a length, never its value", func() {
		const sentinel = "sentinel-github-app-pem-7c53"

		var buffer bytes.Buffer
		log.SetOutput(&buffer)
		DeferCleanup(func() { log.SetOutput(os.Stderr) })

		app := &application{GitHubAppPEM: sentinel}
		Expect(argument.Print(context.Background(), app)).To(Succeed())

		output := buffer.String()
		Expect(output).To(ContainSubstring("GitHubAppPEM"))
		Expect(output).To(ContainSubstring(
			fmt.Sprintf("GitHubAppPEM length %d", len(sentinel)),
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

	// The same allowlist gap, with a symptom that reads like a network fault rather
	// than a config one: a worker whose pod env carries HTTP_PROXY still runs its
	// Bash tool without it, so `git clone` and `go mod download` go direct and are
	// refused by the worker's own NetworkPolicy. NO_PROXY is the other half —
	// without it the in-cluster router is sent to the proxy and the worker loses its
	// own model rather than degrading.
	It("forwards the proxy triple to the Claude child", func() {
		app := &application{
			HTTPProxy:  "http://egress-proxy:3128",
			HTTPSProxy: "http://egress-proxy:3128",
			NoProxy:    "localhost,127.0.0.1,.svc,claude-code-router-dev",
		}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"HTTP_PROXY":  "http://egress-proxy:3128",
			"HTTPS_PROXY": "http://egress-proxy:3128",
			"NO_PROXY":    "localhost,127.0.0.1,.svc,claude-code-router-dev",
		}))
	})

	It("omits the proxy triple when unset, leaving a laptop run unchanged", func() {
		app := &application{ClaudeEnvRaw: "FOO=bar"}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{"FOO": "bar"}))
	})

	// NO_PROXY is itself a comma-separated list, so it cannot ride the CLAUDE_ENV
	// bag — envparse splits that on commas and would keep only the first entry,
	// silently dropping every in-cluster bypass after it. This is why the triple
	// has dedicated fields rather than being packed into the bag.
	It("keeps a comma-separated NO_PROXY intact where CLAUDE_ENV would truncate it", func() {
		app := &application{NoProxy: "a.example,b.example,c.example"}
		Expect(app.buildClaudeEnv()["NO_PROXY"]).To(Equal("a.example,b.example,c.example"))
	})

	// The same gap, closed the same way. The pod's own environment carries the
	// GitHub App triple, but the library's allowlist drops it before the child
	// starts, so the `git` a worker runs would invoke the credential helper
	// against an environment holding none of the three — and the helper fails
	// loudly with "GITHUB_APP_ID is unset or empty". A pod that holds a credential
	// is not yet a pod that can use it.
	It("forwards the GitHub App vars to the Claude child", func() {
		app := &application{
			GitHubAppID:             "5252311",
			GitHubAppInstallationID: "169648268",
			GitHubAppPEM:            "sentinel-github-app-pem-7c53",
		}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{
			"GITHUB_APP_ID":              "5252311",
			"GITHUB_APP_INSTALLATION_ID": "169648268",
			"GITHUB_APP_PEM":             "sentinel-github-app-pem-7c53",
		}))
	})

	It("omits the GitHub App vars when unset, leaving a laptop run unchanged", func() {
		app := &application{ClaudeEnvRaw: "FOO=bar"}
		Expect(app.buildClaudeEnv()).To(Equal(map[string]string{"FOO": "bar"}))
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

		// The capability is asserted as a parsed field, not as a substring: the
		// field's zero value marshals into the body as "pushNotifications":false,
		// so a substring check would pass on a service whose push is unwired.
		var card struct {
			Name         string `json:"name"`
			Capabilities struct {
				PushNotifications bool `json:"pushNotifications"`
			} `json:"capabilities"`
		}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &card)).To(Succeed())
		Expect(card.Name).To(Equal("claude-interactive"))
		Expect(card.Capabilities.PushNotifications).To(BeTrue())
	})

	// The capability follows the wiring rather than being configured separately, so a
	// service built with the push configuration left unset — no option at all, which is
	// the state this repository was in before the wiring landed — must both advertise
	// nothing and refuse a registration. Asserting only the card would let a hardcoded
	// capability pass; asserting only the refusal would let a service that refuses
	// everything pass. The refusal must carry the SDK's own push-not-supported reason:
	// a registration naming an unknown task is refused anyway, with TASK_NOT_FOUND
	// (-32001), so "an error came back" is not the property under test.
	It("advertises no push and refuses a registration when built without the wiring", func() {
		service := interactive.NewServiceWithPermissions(
			&agentmocks.SessionFactory{},
			"127.0.0.1:0",
			"http://provider.example.test",
			prometheus.NewRegistry(),
			interactive.NewAuthToken("test-token"),
			interactive.CardConfig{
				Name:      "claude-interactive",
				PublicURL: "https://agent.example.test/a2a",
			},
			interactive.NewPermissionRegistry(),
			interactive.DefaultSessionIdleTimeout,
			interactive.DefaultMaxSessions,
		)

		cardRecorder := httptest.NewRecorder()
		service.Handler().ServeHTTP(
			cardRecorder,
			httptest.NewRequest(http.MethodGet, "/.well-known/agent-card.json", nil),
		)
		Expect(cardRecorder.Code).To(Equal(http.StatusOK))
		var card struct {
			Capabilities struct {
				PushNotifications bool `json:"pushNotifications"`
			} `json:"capabilities"`
		}
		Expect(json.Unmarshal(cardRecorder.Body.Bytes(), &card)).To(Succeed())
		Expect(card.Capabilities.PushNotifications).To(BeFalse())

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(
			http.MethodPost,
			"/a2a",
			bytes.NewBufferString(
				`{"jsonrpc":"2.0","id":1,"method":"CreateTaskPushNotificationConfig",`+
					`"params":{"taskId":"unknown-task","url":"http://127.0.0.1:1/push",`+
					`"token":"tok"}}`,
			),
		)
		request.Header.Set("Content-Type", "application/json")
		// /a2a is gated: without the bearer token the gate answers 401 before the
		// handler runs, and the assertion below would pass for the wrong reason.
		request.Header.Set("Authorization", "Bearer test-token")
		// a2a-go v2.7.0 rejects a call whose A2A-Version does not match the version
		// the transport serves; an absent header is treated as "0.3".
		request.Header.Set(a2a.SvcParamVersion, string(a2a.Version))
		service.Handler().ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var response struct {
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Code    int             `json:"code"`
				Message string          `json:"message"`
				Data    json.RawMessage `json:"data"`
			} `json:"error"`
		}
		Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
		GinkgoWriter.Printf("refused registration body: %s", recorder.Body.String())
		Expect(response.Result).To(BeEmpty())
		Expect(response.Error).NotTo(BeNil())
		Expect(response.Error.Code).To(Equal(-32003))
		// -32003 is the SDK's push-not-supported code; the reason string it carries
		// alongside it is what distinguishes this refusal from the task-not-found one.
		Expect(string(response.Error.Data)).To(ContainSubstring("PUSH_NOTIFICATION_NOT_SUPPORTED"))
	})

	// The interactive path's own route, served end to end. The A2A gate routes a
	// request raised by an /a2a turn to the store; a request raised by a /prompt
	// turn must still appear here and be answerable here. This spec drives the real
	// registry the service was built with, so it fails if the service stops serving
	// the route or starts reading a gate off the store instead.
	It("serves a /prompt-raised gate on GET /permission", func() {
		app := &application{
			Listen:               "127.0.0.1:0",
			ProviderBaseURL:      "http://provider.example.test",
			InteractiveAuthToken: "test-token",
			A2APublicURL:         "https://agent.example.test/a2a",
			A2AAgentName:         "claude-interactive",
			// Set so the "body carries no store URL" assertion below has teeth: with
			// the field left empty, a body that leaked the store URL would leak an
			// empty string and the assertion could never fail.
			AttentionStoreURL: "https://store.example.test",
		}
		registry := interactive.NewPermissionRegistry()
		service := app.newInteractiveService(
			&agentmocks.SessionFactory{},
			prometheus.NewRegistry(),
			registry,
		)

		ctx, cancel := context.WithCancel(context.Background())
		DeferCleanup(cancel)
		go func() {
			_, _ = registry.DecidePermission(ctx, claudelib.PermissionRequest{
				ToolName:    "Bash",
				Description: "run a command",
				Source:      claudelib.PromptPermissionSource,
			})
		}()

		Eventually(registry.List).Should(HaveLen(1))
		pendingID := registry.List()[0].ID

		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/permission", nil)
		// /permission is gated: readiness, metrics and the Agent Card are the only
		// exempt routes, so this spec sends the token the service was built with.
		request.Header.Set("Authorization", "Bearer test-token")
		service.Handler().ServeHTTP(recorder, request)

		Expect(recorder.Code).To(Equal(http.StatusOK))
		var pending []interactive.PendingPermission
		Expect(json.Unmarshal(recorder.Body.Bytes(), &pending)).To(Succeed())
		Expect(pending).To(HaveLen(1))
		Expect(pending[0].ID).To(Equal(pendingID))
		Expect(pending[0].ToolName).To(Equal("Bash"))
		// The route answers from the registry, not the store: a body naming the store
		// would mean the pod had gone looking for a gate somewhere it cannot reach.
		Expect(recorder.Body.String()).NotTo(ContainSubstring("store.example.test"))
	})
})

// The agent provider is a package-level seam, so this spec stubs it for the
// duration of the spec and restores the original afterwards — the same pattern
// the inClusterNamespacePath specs use. The call under test is application.Run
// itself: a test against a helper beside it would pass while the production
// dereference stayed unguarded.
var _ = Describe("application.Run nil result", func() {
	var ctx context.Context
	var originalCreateAgentProvider func(
		claudelib.ClaudeConfigDir,
		claudelib.AgentDir,
		claudelib.AllowedTools,
		claudelib.ClaudeModel,
		map[string]string,
		map[string]string,
	) agentlib.AgentProvider

	BeforeEach(func() {
		ctx = context.Background()
		originalCreateAgentProvider = createAgentProvider
		DeferCleanup(func() { createAgentProvider = originalCreateAgentProvider })
	})

	// A step whose ShouldRun is false makes StepRunner.Run return (nil, nil),
	// which Agent.Run propagates as its own (nil, nil) — the exact library
	// behaviour that reached the unguarded result.Status dereference.
	It("returns a named error instead of panicking when the agent returns no result", func() {
		step := &agentmocks.AgentStep{}
		step.NameReturns("skipped")
		step.ShouldRunReturns(false, nil)
		provider := &agentmocks.AgentAgentProvider{}
		provider.GetReturns(
			agentlib.NewAgent(agentlib.NewPhase("execution", step)),
			nil,
		)
		createAgentProvider = func(
			_ claudelib.ClaudeConfigDir,
			_ claudelib.AgentDir,
			_ claudelib.AllowedTools,
			_ claudelib.ClaudeModel,
			_ map[string]string,
			_ map[string]string,
		) agentlib.AgentProvider {
			return provider
		}

		app := &application{
			Phase:          "execution",
			TaskContent:    "# a task",
			TaskType:       string(agentlib.TaskTypeLLM),
			PushgatewayURL: "http://127.0.0.1:1",
		}

		// A panic here fails the spec through Ginkgo's own recovery, so one call
		// proves both halves: no panic, and a named error instead of a result.
		err := app.Run(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("agent run returned no result"))
	})

	// The success path moved into recordAgentResult with the rest of the tail,
	// so it needs a spec of its own now that Run no longer contains it.
	It("returns nil and records the result when the agent run succeeds", func() {
		step := &agentmocks.AgentStep{}
		step.NameReturns("finish")
		step.ShouldRunReturns(true, nil)
		step.RunReturns(&agentlib.Result{
			Status:    agentlib.AgentStatusDone,
			NextPhase: "done",
		}, nil)
		provider := &agentmocks.AgentAgentProvider{}
		provider.GetReturns(
			agentlib.NewAgent(agentlib.NewPhase("execution", step)),
			nil,
		)
		createAgentProvider = func(
			_ claudelib.ClaudeConfigDir,
			_ claudelib.AgentDir,
			_ claudelib.AllowedTools,
			_ claudelib.ClaudeModel,
			_ map[string]string,
			_ map[string]string,
		) agentlib.AgentProvider {
			return provider
		}

		app := &application{
			Phase:          "execution",
			TaskContent:    "# a task",
			TaskType:       string(agentlib.TaskTypeLLM),
			PushgatewayURL: "http://127.0.0.1:1",
		}

		Expect(app.Run(ctx, nil)).To(Succeed())
	})

	// The runErr != nil branch moved out of Run into recordAgentResult, so it
	// needs its own spec now that it no longer sits in the covered function.
	It("returns a wrapped agent error when the agent run fails", func() {
		step := &agentmocks.AgentStep{}
		step.NameReturns("boom")
		step.ShouldRunReturns(true, nil)
		step.RunReturns(nil, errors.New(ctx, "boom"))
		provider := &agentmocks.AgentAgentProvider{}
		provider.GetReturns(
			agentlib.NewAgent(agentlib.NewPhase("execution", step)),
			nil,
		)
		createAgentProvider = func(
			_ claudelib.ClaudeConfigDir,
			_ claudelib.AgentDir,
			_ claudelib.AllowedTools,
			_ claudelib.ClaudeModel,
			_ map[string]string,
			_ map[string]string,
		) agentlib.AgentProvider {
			return provider
		}

		app := &application{
			Phase:          "execution",
			TaskContent:    "# a task",
			TaskType:       string(agentlib.TaskTypeLLM),
			PushgatewayURL: "http://127.0.0.1:1",
		}

		err := app.Run(ctx, nil)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("agent run failed"))
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
			&fakeK8sInterface{},
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
			&fakeK8sInterface{},
			heartbeat.NewActivityRecorder(libtime.NewCurrentDateTime()),
			libtime.NewCurrentDateTime(),
		)

		Expect(err).To(HaveOccurred())
		Expect(publisher).To(BeNil())
	})
})
