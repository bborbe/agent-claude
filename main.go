// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Command agent-claude is the canonical AI-heavy agent: one Claude
// invocation per phase, all logic in the prompt + allowed tools.
//
// This binary is the Kafka entry point — spawned as a K8s Job by
// task/executor with TASK_CONTENT + TASK_ID + PHASE + KAFKA_BROKERS env.
// For local CLI mode (file-based), see cmd/run-task/main.go.
//
// Reference implementation for AI-heavy agents using the agent framework
// (lib.NewAgent + claude.NewAgentStep). Other agents (trade-analysis,
// pr-reviewer) follow the same shape — copy this main.go and swap
// prompts/tools.
package main

import (
	"context"
	"os"
	"strings"
	"time"

	a2apush "github.com/a2aproject/a2a-go/v2/a2asrv/push"
	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	delivery "github.com/bborbe/agent/delivery"
	interactive "github.com/bborbe/agent/interactive"
	libmetrics "github.com/bborbe/agent/metrics"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	"github.com/bborbe/k8s"
	libkafka "github.com/bborbe/kafka"
	"github.com/bborbe/run"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/bborbe/agent-claude/pkg/envbag"
	"github.com/bborbe/agent-claude/pkg/factory"
	heartbeat "github.com/bborbe/agent-claude/pkg/heartbeat"
	"github.com/bborbe/agent-claude/pkg/permission"
)

// agentName is the identity string used for Prometheus metric grouping and logging.
const agentName = "claude-agent"

func main() {
	app := &application{}
	os.Exit(service.Main(context.Background(), app, &app.SentryDSN, &app.SentryProxy))
}

type application struct {
	SentryDSN   string `required:"false" arg:"sentry-dsn"   env:"SENTRY_DSN"   usage:"SentryDSN"    display:"length"`
	SentryProxy string `required:"false" arg:"sentry-proxy" env:"SENTRY_PROXY" usage:"Sentry Proxy" display:"length"`

	// Claude Code CLI configuration
	ClaudeConfigDir claudelib.ClaudeConfigDir `required:"false" arg:"claude-config-dir" env:"CLAUDE_CONFIG_DIR" usage:"Claude Code config directory"`

	// Agent directory (contains .claude/ with CLAUDE.md and commands)
	AgentDir claudelib.AgentDir `required:"false" arg:"agent-dir" env:"AGENT_DIR" usage:"Agent directory with .claude/ config" default:"agent"`

	// Allowed tools (comma-separated)
	AllowedToolsRaw string `required:"false" arg:"allowed-tools" env:"ALLOWED_TOOLS" usage:"Comma-separated list of allowed tools"`

	// Task content from agent pipeline. Required for a task-routed agent; a
	// service agent has no task to run, so the requirement is enforced in Run
	// where the agent shape is known.
	TaskContent string `required:"false" arg:"task-content" env:"TASK_CONTENT" usage:"Raw task markdown from vault; required unless AGENT_TYPE=service"`

	// Environment context passed to prompt (comma-separated KEY=VALUE pairs)
	EnvContextRaw envbag.KeyValueList `required:"false" arg:"env-context" env:"ENV_CONTEXT" usage:"Comma-separated KEY=VALUE pairs for prompt context"`

	// Environment variables passed to Claude CLI process (comma-separated KEY=VALUE pairs).
	// Use this for ad-hoc / less-common env vars. The three load-bearing Anthropic provider
	// vars below have dedicated arg slots so they don't have to be packed into this string.
	ClaudeEnvRaw envbag.KeyValueList `required:"false" arg:"claude-env" env:"CLAUDE_ENV" usage:"Comma-separated KEY=VALUE pairs for Claude CLI environment"`

	// Anthropic-compatible provider routing. Setting AnthropicBaseURL + AnthropicAuthToken
	// routes the claude CLI to an alt-provider (e.g. MiniMax via https://api.minimax.io/anthropic).
	// AnthropicModel drives both the `--model` CLI flag and the ANTHROPIC_MODEL env var seen by
	// the claude subprocess. Non-empty values override the same keys in ClaudeEnvRaw.
	AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
	AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL"                                  display:"length"`
	AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name; also exposed to the claude subprocess as ANTHROPIC_MODEL"                  default:"sonnet"`

	// Outbound HTTP proxy, forwarded to the Claude child rather than left on the pod.
	// The library replaces the child environment with a fixed allowlist (HOME, PATH,
	// USER, TZ, ZONEINFO, TMPDIR, LANG, LC_ALL), so a proxy set only on the pod is
	// invisible to the Bash tool — and therefore to git, the Go toolchain,
	// govulncheck, osv-scanner and trivy, which are the processes that actually need
	// it. NO_PROXY matters as much as the other two: without it an in-cluster endpoint
	// (the model router, the attention store, the vault writer) is sent to the proxy
	// and refused, so a worker loses its own model rather than degrading.
	//
	// ⚠️ These are dedicated fields rather than CLAUDE_ENV entries because CLAUDE_ENV
	// is a comma-separated bag and NO_PROXY is itself a comma-separated list — packing
	// it there would truncate it to its first entry and silently drop the rest.
	HTTPProxy  string `required:"false" arg:"http-proxy"  env:"HTTP_PROXY"  usage:"HTTP proxy forwarded to the Claude child"        display:"length"`
	HTTPSProxy string `required:"false" arg:"https-proxy" env:"HTTPS_PROXY" usage:"HTTPS proxy forwarded to the Claude child"       display:"length"`
	NoProxy    string `required:"false" arg:"no-proxy"    env:"NO_PROXY"    usage:"Proxy bypass list forwarded to the Claude child" display:"length"`

	// Attention-store credentials, forwarded to the Claude child rather than kept
	// on the pod. The pod's own environment carries both names, but the library
	// replaces the child environment with a fixed allowlist, so without this
	// forwarding the child never sees them and the poster falls back to
	// http://localhost:18080, where nothing listens inside a pod. The values are
	// read from the pod environment at runtime (POD_ATTENTION_TOKEN via a
	// secretRef), so no credential is ever a literal in a CR or manifest.
	AttentionStoreURL string `required:"false" arg:"attention-store-url" env:"POD_ATTENTION_STORE_URL" usage:"Attention store base URL forwarded to the Claude child"`
	AttentionToken    string `required:"false" arg:"attention-token"     env:"POD_ATTENTION_TOKEN"     usage:"Attention store bearer token forwarded to the Claude child" display:"length"`

	// GatewaySecret is the shared secret the deployed vault service (`git-rest`)
	// requires in the `X-Gateway-Secret` header on every `/api/v1/*` request.
	// Forwarded to the Claude child for exactly the reason the pair above is: the
	// pod's own environment carries it, but the library replaces the child
	// environment with a fixed allowlist, so without this forwarding the child
	// cannot read or write a vault file at all — and the service answers `401`,
	// which reads like a wrong secret rather than a missing forwarding, sending a
	// worker to hunt for a credential the pod already holds. The value arrives from the
	// pod environment (a secretRef), so no credential is ever a literal in a CR or
	// manifest; `display:"length"` keeps it out of the startup log.
	GatewaySecret string `required:"false" arg:"gateway-secret" env:"GATEWAY_SECRET" usage:"Shared secret for the vault service's X-Gateway-Secret header, forwarded to the Claude child" display:"length"`

	// GitHub App credential, forwarded to the Claude child for exactly the reason
	// the attention-store pair above is: the library replaces the child
	// environment with a fixed allowlist (HOME, PATH, USER, TZ, ZONEINFO, TMPDIR,
	// LANG, LC_ALL), so a pod that holds the App is still not a child that can
	// mint from it. Without this forwarding the `git` a worker runs sees none of
	// the three, and `git-credential-github-app` fails with "GITHUB_APP_ID is
	// unset or empty" — a push that cannot succeed, from a pod whose own
	// environment plainly carries the credential. GITHUB_APP_PEM arrives from the
	// pod environment (a secretRef), so no credential is ever a literal in a CR or
	// manifest; display:"length" keeps its value out of the startup log.
	GitHubAppID             string `required:"false" arg:"github-app-id"              env:"GITHUB_APP_ID"              usage:"GitHub App id forwarded to the Claude child"`
	GitHubAppInstallationID string `required:"false" arg:"github-app-installation-id" env:"GITHUB_APP_INSTALLATION_ID" usage:"GitHub App installation id forwarded to the Claude child"`
	GitHubAppPEM            string `required:"false" arg:"github-app-pem"             env:"GITHUB_APP_PEM"             usage:"GitHub App private key forwarded to the Claude child"     display:"length"`

	// Branch for Kafka result delivery
	Branch base.Branch `required:"false" arg:"branch" env:"BRANCH" usage:"branch"`

	// TopicPrefix is an explicit Kafka topic prefix, independent of Branch.
	TopicPrefix base.TopicPrefix `required:"false" arg:"topic-prefix" env:"TOPIC_PREFIX" usage:"Explicit Kafka topic prefix; empty means unprefixed topics"`

	// Phase to run (framework requires explicit phase)
	Phase domain.TaskPhase `required:"false" arg:"phase" env:"PHASE" usage:"Agent phase: planning | execution | ai_review" default:"execution"`

	// AgentType is the agent shape stamped by the executor from the Config's
	// spec.type: "service" for a long-running identity agent, empty for a
	// task-routed one. See factory.AgentTypeService.
	AgentType string `required:"false" arg:"agent-type" env:"AGENT_TYPE" usage:"Agent shape: 'service' for a long-running identity agent; empty for a task-routed one"`

	// Listen is the address a service agent binds for readiness, metrics and
	// prompt intake. A task-routed agent runs one task and exits, so it never serves.
	Listen string `required:"false" arg:"listen" env:"LISTEN" usage:"Address for readiness/metrics (service agents only)" default:":9090"`

	// ProviderBaseURL is the provider endpoint a service agent dials for its
	// readiness check. Empty means the check is skipped and reported as skipped
	// rather than guessed at.
	ProviderBaseURL string `required:"false" arg:"provider-base-url" env:"PROVIDER_BASE_URL" usage:"Provider endpoint; a service agent dials it for readiness"`

	// InteractiveAuthToken is the bearer credential the interactive service requires on
	// every gated route. It is delivered as a runtime-only pod secret and is never
	// written into source, an image layer or a committed manifest. The requirement is
	// enforced in runService, where the agent shape is known: a service that cannot
	// authenticate must fail to start rather than serve unauthenticated.
	InteractiveAuthToken string `required:"false" arg:"interactive-auth-token" env:"INTERACTIVE_AUTH_TOKEN" usage:"Bearer token required by the interactive service's gated routes" display:"length"`

	// A2APublicURL is the externally reachable address of the interactive service's A2A
	// endpoint, advertised verbatim in its Agent Card. It is configuration, never derived from
	// Listen, so the card cannot advertise the listen address. Required for a service agent;
	// enforced in runService rather than by a required tag, for the same reason as
	// InteractiveAuthToken.
	A2APublicURL string `required:"false" arg:"a2a-public-url" env:"A2A_PUBLIC_URL" usage:"Externally reachable A2A endpoint URL the Agent Card advertises (service agents only)"`

	// A2AAgentName names this deployed agent in its A2A Agent Card. Required for a service
	// agent; enforced in runService rather than by a required tag, for the same reason as
	// A2APublicURL.
	A2AAgentName string `required:"false" arg:"a2a-agent-name" env:"A2A_AGENT_NAME" usage:"Name the A2A Agent Card advertises for this agent (service agents only)"`

	// Kafka delivery (optional — only active when TASK_ID is set). TaskID is a
	// plain string rather than agentlib.TaskIdentifier so a service agent, which
	// has no task id, is not rejected by that type's Validate method at parse time.
	KafkaBrokers libkafka.Brokers `required:"false" arg:"kafka-brokers" env:"KAFKA_BROKERS" usage:"Comma separated list of Kafka brokers"`
	TaskID       string           `required:"false" arg:"task-id"       env:"TASK_ID"       usage:"Agent task identifier for publishing results back to task controller"`

	PushgatewayURL string `required:"false" arg:"pushgateway-url" env:"PUSHGATEWAY_URL" usage:"Prometheus PushGateway URL"          default:"http://pushgateway:9090"`
	TaskType       string `required:"false" arg:"task-type"       env:"TASK_TYPE"       usage:"Task type label for metric grouping" default:"unknown"`
}

func (a *application) Run(ctx context.Context, _ libsentry.Client) error {
	registry := prometheus.NewRegistry()
	jobMetrics := libmetrics.NewJobMetrics(registry, libtime.NewCurrentDateTime())
	pusher := push.New(a.PushgatewayURL, libmetrics.BuildJobMetricsName(agentName)).
		Grouping("agent", agentName).
		Grouping("task_type", a.TaskType).
		Collector(registry)
	defer func() {
		if err := pusher.PushContext(ctx); err != nil {
			glog.Warningf("prometheus push failed: %v", err)
			return
		}
		glog.V(2).Infof("prometheus push completed")
	}()
	start := libtime.NewCurrentDateTime().Now().Time()

	glog.V(2).Infof("agent-claude started phase=%s", a.Phase)

	deliverer := delivery.NewNoopResultDeliverer()
	if a.TaskID != "" {
		if len(a.KafkaBrokers) == 0 {
			jobMetrics.RecordRun(agentlib.AgentStatusFailed)
			jobMetrics.RecordDuration(time.Since(start))
			return errors.Errorf(ctx, "KAFKA_BROKERS must be set when TASK_ID is set")
		}
		syncProducer, err := factory.CreateSyncProducer(ctx, a.KafkaBrokers)
		if err != nil {
			jobMetrics.RecordRun(agentlib.AgentStatusFailed)
			jobMetrics.RecordDuration(time.Since(start))
			return errors.Wrap(ctx, err, "create sync producer")
		}
		defer func() {
			if err := syncProducer.Close(); err != nil {
				glog.Warningf("close sync producer failed: %v", err)
			}
		}()
		deliverer = factory.CreateKafkaResultDeliverer(
			syncProducer, a.TopicPrefix, agentlib.TaskIdentifier(a.TaskID), a.TaskContent,
			libtime.NewCurrentDateTime(),
		)
	}

	claudeEnv := a.buildClaudeEnv()

	if a.AgentType == factory.AgentTypeService {
		return a.runService(ctx, registry, claudeEnv)
	}
	if a.TaskContent == "" {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Errorf(
			ctx,
			"TASK_CONTENT is required for a task-routed agent; it is optional only when AGENT_TYPE=%s",
			factory.AgentTypeService,
		)
	}

	if err := validateTaskType(ctx, agentlib.TaskType(a.TaskType)); err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "validate task_type")
	}

	provider := createAgentProvider(
		a.ClaudeConfigDir,
		a.AgentDir,
		claudelib.ParseAllowedTools(a.AllowedToolsRaw),
		a.AnthropicModel,
		claudeEnv,
		a.EnvContextRaw.Pairs(),
	)
	agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "select agent for task_type")
	}

	result, err := agent.Run(ctx, a.Phase, a.TaskContent, deliverer)
	return recordAgentResult(ctx, jobMetrics, start, result, err)
}

// recordAgentResult records the metrics for a finished agent run and reports the
// outcome. agent.Run returns (nil, nil) when every step of the requested phase is
// skipped, so the nil result is a reachable outcome rather than a defensive check:
// dereferencing it panicked the pod and burned a Job attempt. A nil result is
// reported as a failed run with a named error so the metrics and delivery paths
// see a failure instead of a crash.
func recordAgentResult(
	ctx context.Context,
	jobMetrics libmetrics.JobMetrics,
	start time.Time,
	result *agentlib.Result,
	runErr error,
) error {
	if runErr != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, runErr, "agent run failed")
	}
	if result == nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.New(ctx, "agent run returned no result")
	}
	jobMetrics.RecordRun(result.Status)
	jobMetrics.RecordDuration(time.Since(start))
	return agentlib.PrintResult(ctx, result)
}

// buildClaudeEnv assembles the environment passed to the Claude CLI process:
// the ad-hoc CLAUDE_ENV pairs, with the three load-bearing Anthropic provider
// vars, the two attention-store vars and the three GitHub App vars overriding
// the same keys. Both agent shapes use it — the one-shot runner and the
// long-lived session take the same config.
//
// The returned map is handed to the runner as ClaudeRunnerConfig.Env, which the
// library applies as its final layer over the child environment. That layer is
// what lets a pod's Claude child reach the attention store and mint a GitHub App
// token: the library otherwise replaces the child env with a fixed allowlist
// (HOME, PATH, USER, TZ, ZONEINFO, TMPDIR, LANG, LC_ALL), which carries neither.
func (a *application) buildClaudeEnv() map[string]string {
	claudeEnv := a.ClaudeEnvRaw.Pairs()
	if claudeEnv == nil {
		claudeEnv = map[string]string{}
	}
	if a.AnthropicBaseURL != "" {
		claudeEnv["ANTHROPIC_BASE_URL"] = a.AnthropicBaseURL
	}
	if a.AnthropicAuthToken != "" {
		claudeEnv["ANTHROPIC_AUTH_TOKEN"] = a.AnthropicAuthToken
	}
	if a.AnthropicModel != "" {
		claudeEnv["ANTHROPIC_MODEL"] = a.AnthropicModel.String()
	}
	if a.AttentionStoreURL != "" {
		claudeEnv["POD_ATTENTION_STORE_URL"] = a.AttentionStoreURL
	}
	if a.AttentionToken != "" {
		claudeEnv["POD_ATTENTION_TOKEN"] = a.AttentionToken
	}
	if a.GatewaySecret != "" {
		claudeEnv["GATEWAY_SECRET"] = a.GatewaySecret
	}
	// The proxy triple rides the same final layer, and needs it for the same reason
	// as the vars above: the pod carries all three, but the library's allowlist drops
	// them before the child starts, so a Bash tool running `git clone` or `go mod
	// download` would otherwise see no proxy at all.
	if a.HTTPProxy != "" {
		claudeEnv["HTTP_PROXY"] = a.HTTPProxy
	}
	if a.HTTPSProxy != "" {
		claudeEnv["HTTPS_PROXY"] = a.HTTPSProxy
	}
	if a.NoProxy != "" {
		claudeEnv["NO_PROXY"] = a.NoProxy
	}
	// The GitHub App triple rides the same final layer, and needs it for the same
	// reason: the pod's own environment carries all three, but the library's
	// allowlist drops them before the child starts, so the helper `git` invokes
	// would otherwise see an empty environment and every push would fail.
	if a.GitHubAppID != "" {
		claudeEnv["GITHUB_APP_ID"] = a.GitHubAppID
	}
	if a.GitHubAppInstallationID != "" {
		claudeEnv["GITHUB_APP_INSTALLATION_ID"] = a.GitHubAppInstallationID
	}
	if a.GitHubAppPEM != "" {
		claudeEnv["GITHUB_APP_PEM"] = a.GitHubAppPEM
	}
	return claudeEnv
}

// taskTypeUnset is the value the TaskType flag's default tag supplies when
// TASK_TYPE is absent. It is a metrics-grouping placeholder, never a
// dispatched task type: the dispatch table registers only llm, healthcheck
// and oauth-probe. It must stay equal to the value in the TaskType field's
// `default` tag.
const taskTypeUnset agentlib.TaskType = "unknown"

// validateTaskType rejects a task_type the publisher never stamped. Both the
// empty string (TASK_TYPE present but empty) and the flag's default (TASK_TYPE
// absent) are map misses in the dispatch table, so without this guard the
// operator sees a generic "unknown task_type" error that names neither the
// missing environment variable nor the component that must set it. A
// genuinely unknown non-empty value is left to the provider, whose error
// lists the accepted types.
func validateTaskType(ctx context.Context, taskType agentlib.TaskType) error {
	switch taskType {
	case "":
		return errors.Errorf(
			ctx,
			"TASK_TYPE is empty; the task publisher (task/executor) must stamp one of the agent.TaskType* constants before dispatching a job to agent-claude",
		)
	case taskTypeUnset:
		return errors.Errorf(
			ctx,
			"TASK_TYPE is unset (default %q); the task publisher (task/executor) must stamp one of the agent.TaskType* constants before dispatching a job to agent-claude",
			string(taskTypeUnset),
		)
	}
	return nil
}

// runService is the long-running half of this binary. A service agent has no
// task to run, so instead of executing one and exiting it stays alive and
// answers when addressed, holding one conversation per session id.
func (a *application) runService(
	ctx context.Context,
	registry *prometheus.Registry,
	claudeEnv map[string]string,
) error {
	// The shape-scoped guard lives here rather than in a required:"true" struct tag:
	// argument.Parse evaluates required tags for every agent shape, so a tag would also
	// reject the task-routed jobs, which never serve HTTP. runService is reached only
	// when the agent shape is a service, so this is where the requirement belongs.
	if a.InteractiveAuthToken == "" {
		return errors.Errorf(
			ctx,
			"INTERACTIVE_AUTH_TOKEN is required for a service agent; a service that cannot authenticate must not serve unauthenticated",
		)
	}
	if a.A2APublicURL == "" {
		return errors.Errorf(
			ctx,
			"A2A_PUBLIC_URL is required for a service agent; the Agent Card must advertise a real network endpoint",
		)
	}
	if a.A2AAgentName == "" {
		return errors.Errorf(
			ctx,
			"A2A_AGENT_NAME is required for a service agent; the Agent Card must name the deployed agent",
		)
	}
	glog.V(2).Infof(
		"agent-claude service mode: serving readiness, metrics, prompt intake, the permission endpoint and the A2A Agent Card (/.well-known/agent-card.json, agent %s, endpoint %s) on %s",
		a.A2AAgentName,
		a.A2APublicURL,
		a.Listen,
	)
	// One registry, passed to the session factory and to the service together, so
	// a turn that pauses on a permission request is visible on the service's own
	// /permission route and answerable from outside the pod.
	permissions := interactive.NewPermissionRegistry()

	// A gate raised by an A2A turn is published to the attention store and answered
	// there by the operator; every other request keeps resolving through the
	// registry above, which is what the pod's own /permission route serves. The
	// registry instance is unchanged and still shared with the endpoint.
	decider := permission.NewDecider(
		permissions,
		permission.NewGate(permission.PosterPath, gateProducerID(a)),
	)

	// The clock is created once here and shared by the activity recorder and the
	// heartbeat writer, so a session's activity and its entry's stamp come from the
	// same source of time.
	currentDateTime := libtime.NewCurrentDateTime()
	recorder := heartbeat.NewActivityRecorder(currentDateTime)
	// The observer wraps the Session the factory returns, not the factory's Create:
	// the library calls Create once per session and never again, so only Prompt can
	// report that a session is still being served.
	sessions := heartbeat.NewObservingSessionFactory(
		factory.CreateClaudeSessionFactory(
			a.ClaudeConfigDir,
			a.AgentDir,
			claudelib.ParseAllowedTools(a.AllowedToolsRaw),
			a.AnthropicModel,
			claudeEnv,
			decider,
		),
		recorder,
	)
	interactiveService := a.newInteractiveService(sessions, registry, permissions)

	clientset, err := k8s.CreateClientset("")
	if err != nil {
		// A broken liveness path must not take prompt serving down: the worker goes
		// unlisted rather than unserved. The next pod start retries the setup.
		glog.Warningf("cluster heartbeat disabled: %v", err)
		return interactiveService.Run(ctx)
	}
	publisher, err := buildHeartbeatPublisher(ctx, clientset, recorder, currentDateTime)
	if err != nil {
		// Same rule as above: the heartbeat is best-effort, prompt serving is not.
		glog.Warningf("cluster heartbeat disabled: %v", err)
		return interactiveService.Run(ctx)
	}
	return run.CancelOnFirstFinish(ctx, publisher.Run, interactiveService.Run)
}

// gateProducerID is the producer a permission gate is posted to the attention
// store under: the pod's own name, so the board attributes a gate to the worker
// that raised it and two pods never share one producer. A pod always has a
// hostname; the agent name is the fallback for the case where it does not, and the
// service-mode guard has already established that the agent name is non-empty.
func gateProducerID(a *application) string {
	hostname, err := os.Hostname()
	if err != nil {
		glog.Warningf("permission gate producer falls back to agent name: %v", err)
		return a.A2AAgentName
	}
	return hostname
}

// newInteractiveService builds the interactive HTTP surface from the application's
// configuration. The card's name and URL travel together in one struct, and which values
// fill the card's Name and PublicURL fields is checked by a spec that serves the Agent
// Card rather than by the compiler.
//
// The A2A push-notification store and sender are built here, in this repository, rather
// than inside the library: the A2A protocol puts the registration on the agent's own
// endpoint, so the worker holds its own per-task registrations, and building them here is
// what makes a2a-go a direct dependency of this module. The library derives both the Agent
// Card's pushNotifications capability and the handler's push wiring from the one option
// carrying them, so the card can never advertise a channel that is not wired.
func (a *application) newInteractiveService(
	sessions agentlib.SessionFactory,
	registry *prometheus.Registry,
	permissions interactive.PermissionRegistry,
) interactive.Service {
	store := a2apush.NewInMemoryStore()
	// AllowPrivateNetworks is mandatory, not a preference. The SDK's sender refuses
	// loopback, private, link-local and unspecified targets by default, and the callback
	// the spawner registers is exactly such an address: a container reaches the Mac host
	// through the Docker gateway, which is private. Without the opt-out every push is
	// refused, the refusal is logged only (FailOnError stays at its false default, so a
	// failing push never cancels the turn), and the channel is silently inert while
	// looking deployed. The opt-out relaxes the resolved-IP range check alone — the
	// sender's redirect cap and cross-host token stripping still apply, and its Timeout
	// stays unset so its own 30-second default governs.
	sender := a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{
		AllowPrivateNetworks: true,
	})
	return interactive.NewServiceWithPermissions(
		sessions,
		a.Listen,
		a.ProviderBaseURL,
		registry,
		interactive.NewAuthToken(a.InteractiveAuthToken),
		interactive.CardConfig{
			Name:      a.A2AAgentName,
			PublicURL: a.A2APublicURL,
		},
		permissions,
		interactive.DefaultSessionIdleTimeout,
		interactive.DefaultMaxSessions,
		interactive.WithPushNotifications(store, sender),
	)
}

// buildHeartbeatPublisher builds the cluster heartbeat publisher from the
// in-cluster client. An error means the pod cannot resolve the cluster API or its
// own namespace; the caller treats that as "run without a heartbeat" rather than as
// a startup failure, because the liveness path is best-effort and prompt serving is
// not.
func buildHeartbeatPublisher(
	ctx context.Context,
	clientset k8s.Interface,
	recorder heartbeat.ActivityRecorder,
	currentDateTime libtime.CurrentDateTimeGetter,
) (heartbeat.Publisher, error) {
	namespace, err := inClusterNamespace(ctx, inClusterNamespacePath)
	if err != nil {
		return nil, errors.Wrap(ctx, err, "resolve pod namespace")
	}
	writer := heartbeat.NewConfigMapWriter(
		k8s.NewConfigMapDeployer(clientset),
		namespace,
		heartbeat.ConfigMapName,
		currentDateTime,
	)
	return heartbeat.NewPublisher(recorder, writer, heartbeat.RefreshInterval), nil
}

// createAgentProvider is the seam tests use to inject an agent provider.
// Production uses factory.CreateAgentProvider.
var createAgentProvider = factory.CreateAgentProvider

// inClusterNamespacePath is where the kubelet mounts the pod's own namespace — the
// same service-account mount k8s_rest.InClusterConfig reads from. It is a var
// rather than a const so a spec can point the reader at a temp file; production
// never reassigns it.
var inClusterNamespacePath = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

// inClusterNamespace reads the pod's own namespace from the file the kubelet mounts
// at the standard service-account path. The heartbeat writer publishes into the
// namespace the pod runs in — the same namespace the reader reads.
func inClusterNamespace(ctx context.Context, path string) (k8s.Namespace, error) {
	content, err := os.ReadFile(path) // #nosec G304 -- path is a package constant
	if err != nil {
		return "", errors.Wrap(ctx, err, "read pod namespace")
	}
	namespace := strings.TrimSpace(string(content))
	if namespace == "" {
		return "", errors.Errorf(ctx, "pod namespace is empty in %s", path)
	}
	return k8s.Namespace(namespace), nil
}
