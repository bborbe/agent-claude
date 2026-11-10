// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	agentlib "github.com/bborbe/agent"
	claudelib "github.com/bborbe/agent/claude"
	libkafka "github.com/bborbe/kafka"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/factory"
)

var _ = Describe("CreateAgentProvider", func() {
	var (
		ctx      context.Context
		provider agentlib.AgentProvider
	)

	BeforeEach(func() {
		ctx = context.Background()
		provider = factory.CreateAgentProvider(
			claudelib.ClaudeConfigDir(""),
			claudelib.AgentDir("agent"),
			claudelib.AllowedTools{},
			claudelib.ClaudeModel("sonnet"),
			map[string]string{},
			claudelib.SessionID(""),
			map[string]string{},
		)
	})

	It("returns a non-nil provider", func() {
		Expect(provider).NotTo(BeNil())
	})

	It("Get returns the domain agent for TaskTypeLLM", func() {
		agent, err := provider.Get(ctx, agentlib.TaskTypeLLM)
		Expect(err).To(BeNil())
		Expect(agent).NotTo(BeNil())
	})

	It("Get returns the liveness agent for TaskTypeHealthcheck", func() {
		agent, err := provider.Get(ctx, agentlib.TaskTypeHealthcheck)
		Expect(err).To(BeNil())
		Expect(agent).NotTo(BeNil())
	})

	It("Get returns the SAME liveness agent for TaskTypeOAuthProbe (alias)", func() {
		healthcheckAgent, err := provider.Get(ctx, agentlib.TaskTypeHealthcheck)
		Expect(err).To(BeNil())
		oauthProbeAgent, err := provider.Get(ctx, agentlib.TaskTypeOAuthProbe)
		Expect(err).To(BeNil())
		Expect(oauthProbeAgent).To(BeIdenticalTo(healthcheckAgent))
	})

	Describe("Get with unknown task_type", func() {
		var err error

		BeforeEach(func() {
			_, err = provider.Get(ctx, agentlib.TaskType("bogus"))
		})

		It("returns an error", func() {
			Expect(err).To(HaveOccurred())
		})

		It("error message contains the unknown task_type literal", func() {
			Expect(err.Error()).To(ContainSubstring("unknown task_type"))
		})

		It("error message contains the offending value quoted", func() {
			Expect(err.Error()).To(ContainSubstring(`"bogus"`))
		})

		It("error message contains the binary name", func() {
			Expect(err.Error()).To(ContainSubstring("agent-claude"))
		})

		It("error message contains the sorted accepted-types list", func() {
			Expect(err.Error()).To(ContainSubstring("[healthcheck llm oauth-probe]"))
		})
	})
})

var _ = Describe("CreateSyncProducer", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("returns an error when broker is unreachable", func() {
		producer, err := factory.CreateSyncProducer(ctx, libkafka.Brokers{})
		Expect(producer).To(BeNil())
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("create sync producer"))
	})
})

var _ = Describe("CreateKafkaResultDeliverer", func() {
	It("returns a non-nil ResultDeliverer", func() {
		deliverer := factory.CreateKafkaResultDeliverer(
			nil,
			"",
			"",
			"",
			nil,
		)
		Expect(deliverer).NotTo(BeNil())
	})
})

var _ = Describe("CreateFileResultDeliverer", func() {
	It("returns a non-nil ResultDeliverer", func() {
		deliverer := factory.CreateFileResultDeliverer("/tmp/test-output.md")
		Expect(deliverer).NotTo(BeNil())
	})
})

var _ = Describe("AgentTypeService", func() {
	// The executor owns this value (AgentTypeService in the executor's CRD types
	// package). A mismatch is silent: the binary would simply never leave task
	// mode, with nothing logged as an error. This pins the cross-repo contract
	// that has no compile-time check.
	It(`equals "service"`, func() {
		Expect(factory.AgentTypeService).To(Equal("service"))
	})
})

var _ = Describe("CreateClaudeSessionFactory", func() {
	It("returns a non-nil SessionFactory", func() {
		sessions := factory.CreateClaudeSessionFactory(
			"",
			"",
			nil,
			"",
			nil,
			claudelib.SessionID(""),
			nil,
		)
		Expect(sessions).NotTo(BeNil())
	})
})

var _ = Describe("CreateAgent", func() {
	It("returns a non-nil *agentlib.Agent", func() {
		agent := factory.CreateAgent(
			"",
			"",
			nil,
			"",
			nil,
			claudelib.SessionID(""),
			nil,
		)
		Expect(agent).NotTo(BeNil())
	})
})

// The session id is only useful if it reaches the spawned process, so these
// specs drive a real runner against a stub `claude` on PATH and read the argv
// the process actually received. A config-field assertion would prove only that
// the value was stored, not that the library turned it into --session-id.
var _ = Describe("CreateClaudeRunner session id argv", func() {
	var (
		ctx      context.Context
		binDir   string
		argvDir  string
		origPath string
	)

	BeforeEach(func() {
		ctx = context.Background()
		binDir = GinkgoT().TempDir()
		argvDir = GinkgoT().TempDir()
		// The runner needs a stream-json result event to finish a turn; a bare
		// echo of the args would fail the run before the argv could be asserted.
		script := "#!/bin/sh\n" +
			"printf '%s\\n' \"$@\" > \"$ARGV_FILE\"\n" +
			"printf '{\"type\":\"result\",\"result\":\"ok\"}\\n'\n" +
			"exit 0\n"
		Expect(
			os.WriteFile(filepath.Join(binDir, "claude"), []byte(script), 0755), //nolint:gosec
		).To(Succeed())
		origPath = os.Getenv("PATH")
		Expect(os.Setenv("PATH", binDir+string(os.PathListSeparator)+origPath)).To(Succeed())
	})

	AfterEach(func() {
		_ = os.Setenv("PATH", origPath)
	})

	readArgv := func(argvFile string) []string {
		data, err := os.ReadFile(argvFile)
		Expect(err).NotTo(HaveOccurred())
		return strings.Split(strings.TrimSpace(string(data)), "\n")
	}

	It("passes a non-empty id as --session-id followed by the id", func() {
		argvFile := filepath.Join(argvDir, "argv")
		runner := factory.CreateClaudeRunner(
			"",
			"",
			nil,
			"",
			map[string]string{"ARGV_FILE": argvFile},
			claudelib.SessionID("sess-fixed"),
		)

		result, err := runner.Run(ctx, "prompt")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Result).To(Equal("ok"))

		argv := readArgv(argvFile)
		index := slices.Index(argv, "--session-id")
		Expect(index).NotTo(Equal(-1))
		Expect(argv[index+1]).To(Equal("sess-fixed"))
	})

	It("omits --session-id when the id is empty", func() {
		argvFile := filepath.Join(argvDir, "argv")
		runner := factory.CreateClaudeRunner(
			"",
			"",
			nil,
			"",
			map[string]string{"ARGV_FILE": argvFile},
			claudelib.SessionID(""),
		)

		result, err := runner.Run(ctx, "prompt")
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Result).To(Equal("ok"))

		Expect(readArgv(argvFile)).NotTo(ContainElement("--session-id"))
	})

	It("rejects an invalid id before starting the process", func() {
		runner := factory.CreateClaudeRunner(
			"",
			"",
			nil,
			"",
			map[string]string{"ARGV_FILE": filepath.Join(argvDir, "argv")},
			claudelib.SessionID("bad/id"),
		)

		_, err := runner.Run(ctx, "prompt")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("invalid SessionID"))
	})
})
