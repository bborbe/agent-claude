// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	claudelib "github.com/bborbe/agent/claude"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/permission"
)

// writePoster drops an executable shim at dir/poster.sh and returns its path.
// Driving the real NewGate against a shim is the point of this file: the poster
// is a subprocess boundary, so a shape assertion would not prove the mapping.
func writePoster(dir string, script string) string {
	path := filepath.Join(dir, "poster.sh")
	Expect(os.WriteFile(path, []byte(script), 0600)).To(Succeed())
	Expect(os.Chmod(path, 0700)).To(Succeed())
	return path
}

// recordedCalls splits a record file written by the argv-recording shim into one
// argument slice per invocation.
func recordedCalls(record string) [][]string {
	data, err := os.ReadFile(record)
	Expect(err).To(BeNil())
	var calls [][]string
	var current []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if line == "CALL" {
			if current != nil {
				calls = append(calls, current)
			}
			current = []string{}
			continue
		}
		current = append(current, line)
	}
	if current != nil {
		calls = append(calls, current)
	}
	return calls
}

// valueAfter returns the argument following flag, or "" when it is absent.
func valueAfter(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

var _ = Describe("Gate", func() {
	var (
		ctx     context.Context
		dir     string
		request claudelib.PermissionRequest
	)

	BeforeEach(func() {
		ctx = context.Background()
		dir = GinkgoT().TempDir()
		request = claudelib.PermissionRequest{
			ToolName:     "Bash",
			Description:  "run a command",
			InputPreview: "rm -rf /tmp/secret",
			Source:       claudelib.A2APermissionSource,
		}
	})

	It("allows when the poster reads an allow verdict", func() {
		path := writePoster(
			dir,
			"#!/bin/sh\necho 'ITEM_ID: 1'\necho 'DECISION: allow -- operator said yes'\nexit 0\n",
		)

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeTrue())
	})

	It("denies with the reason when the poster reads a deny verdict", func() {
		path := writePoster(dir, "#!/bin/sh\necho 'DECISION: deny -- operator said no'\nexit 0\n")

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeFalse())
		Expect(decision.Message).To(Equal("operator said no"))
	})

	It("denies when the poster reaches its deadline without a verdict", func() {
		path := writePoster(dir, "#!/bin/sh\necho 'NO_DECISION: store unreachable'\nexit 1\n")

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeFalse())
	})

	It("denies when the poster exits zero but printed no parsable verdict", func() {
		path := writePoster(dir, "#!/bin/sh\nexit 0\n")

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeFalse())
	})

	It("denies when the poster refuses", func() {
		path := writePoster(dir, "#!/bin/sh\necho 'REFUSED: no token'\nexit 2\n")

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeFalse())
	})

	It("does not treat a non-zero exit beside an allow line as a verdict", func() {
		path := writePoster(dir, "#!/bin/sh\necho 'DECISION: allow -- x'\nexit 1\n")

		decision := permission.NewGate(path, "pod-1").Decide(ctx, request)

		Expect(decision.Allow).To(BeFalse())
	})

	It("denies without panicking when the poster does not exist", func() {
		var decision claudelib.PermissionDecision
		Expect(func() {
			decision = permission.NewGate("/nonexistent/poster", "p").Decide(ctx, request)
		}).NotTo(Panic())

		Expect(decision.Allow).To(BeFalse())
	})

	It("passes the gate subcommand, the producer and a payload naming the tool only", func() {
		record := filepath.Join(dir, "argv.txt")
		path := writePoster(dir, fmt.Sprintf(
			"#!/bin/sh\n{ echo CALL; for a in \"$@\"; do echo \"$a\"; done; } >> %s\necho 'DECISION: allow -- ok'\n",
			record,
		))

		permission.NewGate(path, "pod-1").Decide(ctx, request)

		calls := recordedCalls(record)
		Expect(calls).To(HaveLen(1))
		args := calls[0]
		Expect(args[0]).To(Equal("gate"))
		Expect(valueAfter(args, "--producer-id")).To(Equal("pod-1"))
		payload := valueAfter(args, "--payload")
		Expect(payload).To(ContainSubstring("Bash"))
		Expect(payload).NotTo(ContainSubstring(request.InputPreview))
	})

	It("gives two consecutive decisions different dedup keys", func() {
		record := filepath.Join(dir, "argv.txt")
		path := writePoster(dir, fmt.Sprintf(
			"#!/bin/sh\n{ echo CALL; for a in \"$@\"; do echo \"$a\"; done; } >> %s\necho 'DECISION: allow -- ok'\n",
			record,
		))
		gate := permission.NewGate(path, "pod-1")

		gate.Decide(ctx, request)
		gate.Decide(ctx, request)

		calls := recordedCalls(record)
		Expect(calls).To(HaveLen(2))
		first := valueAfter(calls[0], "--dedup-key")
		second := valueAfter(calls[1], "--dedup-key")
		Expect(first).NotTo(BeEmpty())
		Expect(second).NotTo(BeEmpty())
		Expect(first).NotTo(Equal(second))
	})
})
