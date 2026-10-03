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
	"time"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	delivery "github.com/bborbe/agent/delivery"
	"github.com/bborbe/agent/envparse"
	interactive "github.com/bborbe/agent/interactive"
	libmetrics "github.com/bborbe/agent/metrics"
	"github.com/bborbe/cqrs/base"
	"github.com/bborbe/errors"
	libkafka "github.com/bborbe/kafka"
	libsentry "github.com/bborbe/sentry"
	"github.com/bborbe/service"
	libtime "github.com/bborbe/time"
	"github.com/bborbe/vault-cli/pkg/domain"
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"

	"github.com/bborbe/agent-claude/pkg/factory"
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
	EnvContextRaw string `required:"false" arg:"env-context" env:"ENV_CONTEXT" usage:"Comma-separated KEY=VALUE pairs for prompt context"`

	// Environment variables passed to Claude CLI process (comma-separated KEY=VALUE pairs).
	// Use this for ad-hoc / less-common env vars. The three load-bearing Anthropic provider
	// vars below have dedicated arg slots so they don't have to be packed into this string.
	ClaudeEnvRaw string `required:"false" arg:"claude-env" env:"CLAUDE_ENV" usage:"Comma-separated KEY=VALUE pairs for Claude CLI environment"`

	// Anthropic-compatible provider routing. Setting AnthropicBaseURL + AnthropicAuthToken
	// routes the claude CLI to an alt-provider (e.g. MiniMax via https://api.minimax.io/anthropic).
	// AnthropicModel drives both the `--model` CLI flag and the ANTHROPIC_MODEL env var seen by
	// the claude subprocess. Non-empty values override the same keys in ClaudeEnvRaw.
	AnthropicBaseURL   string                `required:"false" arg:"anthropic-base-url"   env:"ANTHROPIC_BASE_URL"   usage:"Anthropic-compatible API base URL"`
	AnthropicAuthToken string                `required:"false" arg:"anthropic-auth-token" env:"ANTHROPIC_AUTH_TOKEN" usage:"Bearer token for ANTHROPIC_BASE_URL"                                  display:"password"`
	AnthropicModel     claudelib.ClaudeModel `required:"false" arg:"anthropic-model"      env:"ANTHROPIC_MODEL"      usage:"Model name; also exposed to the claude subprocess as ANTHROPIC_MODEL"                    default:"sonnet"`

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

	provider := factory.CreateAgentProvider(
		a.ClaudeConfigDir,
		a.AgentDir,
		claudelib.ParseAllowedTools(a.AllowedToolsRaw),
		a.AnthropicModel,
		claudeEnv,
		envparse.KeyValuePairs(a.EnvContextRaw),
	)
	agent, err := provider.Get(ctx, agentlib.TaskType(a.TaskType))
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "select agent for task_type")
	}

	result, err := agent.Run(ctx, a.Phase, a.TaskContent, deliverer)
	if err != nil {
		jobMetrics.RecordRun(agentlib.AgentStatusFailed)
		jobMetrics.RecordDuration(time.Since(start))
		return errors.Wrap(ctx, err, "agent run failed")
	}
	jobMetrics.RecordRun(result.Status)
	jobMetrics.RecordDuration(time.Since(start))
	return agentlib.PrintResult(ctx, result)
}

// buildClaudeEnv assembles the environment passed to the Claude CLI process:
// the ad-hoc CLAUDE_ENV pairs, with the three load-bearing Anthropic provider
// vars overriding the same keys. Both agent shapes use it — the one-shot runner
// and the long-lived session take the same config.
func (a *application) buildClaudeEnv() map[string]string {
	claudeEnv := envparse.KeyValuePairs(a.ClaudeEnvRaw)
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
	return claudeEnv
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
	glog.V(2).Infof(
		"agent-claude service mode: serving readiness, metrics, prompt intake and the permission endpoint on %s",
		a.Listen,
	)
	// One registry, passed to the session factory and to the service together, so
	// a turn that pauses on a permission request is visible on the service's own
	// /permission route and answerable from outside the pod.
	permissions := interactive.NewPermissionRegistry()
	sessions := factory.CreateClaudeSessionFactory(
		a.ClaudeConfigDir,
		a.AgentDir,
		claudelib.ParseAllowedTools(a.AllowedToolsRaw),
		a.AnthropicModel,
		claudeEnv,
		permissions,
	)
	return interactive.NewServiceWithPermissions(
		sessions,
		a.Listen,
		a.ProviderBaseURL,
		registry,
		interactive.NewAuthToken(a.InteractiveAuthToken),
		permissions,
	).Run(ctx)
}
