// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package permission routes a turn's permission requests to the channel that
// raised them: an A2A turn's request is published to the attention store and
// answered by the operator there, and every other request keeps resolving
// through the pod's own permission registry.
package permission

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	claudelib "github.com/bborbe/agent/claude"
	"github.com/golang/glog"
)

// PosterPath is where the image installs the pod-side attention poster. The
// Dockerfile copies scripts/pod-attention.py here; the decider invokes it rather
// than reimplementing the store protocol.
const PosterPath = "/usr/local/bin/pod-attention.py"

//counterfeiter:generate -o ../../mocks/permission-gate.go --fake-name PermissionGate . Gate

// Gate decides one permission request by publishing it to the attention store and
// blocking until the operator answers it there.
type Gate interface {
	Decide(ctx context.Context, request claudelib.PermissionRequest) claudelib.PermissionDecision
}

// gate is a Gate backed by the pod-side attention poster.
//
// It holds no store client and no credential: the poster reads the
// attention-store URL and token from the environment it inherits at decision
// time, which is what keeps the token out of the image and the manifest.
type gate struct {
	posterPath string
	producerID string
	counter    atomic.Uint64
	// nonce is a process-uniqueness token, not a business timestamp: it exists
	// only so two processes never mint the same dedup key. libtime's
	// CurrentDateTimeGetter injection does not apply to it.
	nonce int64
}

// NewGate returns a Gate that publishes each request through the poster at
// posterPath and blocks for the operator's answer. producerID is the producer the
// store attributes the item to.
func NewGate(posterPath string, producerID string) Gate {
	return &gate{
		posterPath: posterPath,
		producerID: producerID,
		nonce:      time.Now().UnixNano(),
	}
}

// Decide publishes the request as a permission-class item and blocks until the
// operator answers it or the poster's deadline passes. Every outcome is a verdict
// or a denial, so it returns no error: a denial is the fail-closed answer the
// caller needs.
func (g *gate) Decide(
	ctx context.Context,
	request claudelib.PermissionRequest,
) claudelib.PermissionDecision {
	// The store keys items by dedup_key, so a key shared with another gate would
	// hand this gate the other, already-answered item. The process nonce and the
	// in-process counter together make the key unique across both gates in one
	// process and gates across restarts.
	key := fmt.Sprintf("agent-claude-a2a-gate-%d-%d", g.nonce, g.counter.Add(1))
	var stdout, stderr bytes.Buffer
	// #nosec G204 -- posterPath is a compile-time constant (PosterPath) and the
	// arguments are passed as an argv slice, never through a shell.
	cmd := exec.CommandContext(
		ctx,
		g.posterPath,
		"gate",
		"--dedup-key", key,
		"--payload", payloadFor(request),
		"--producer-id", g.producerID,
	)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		glog.Warningf(
			"attention gate %s did not complete: %v; stderr: %s",
			key,
			err,
			strings.TrimSpace(stderr.String()),
		)
		return denied()
	}
	decision, ok := parseDecision(stdout.String())
	if !ok {
		glog.Warningf(
			"attention gate %s returned no parsable verdict; stdout: %s",
			key,
			strings.TrimSpace(stdout.String()),
		)
		return denied()
	}
	return decision
}

// denied is the fail-closed answer: an unanswered gate, an unreachable store, a
// malformed response and the deadline all resolve to it.
func denied() claudelib.PermissionDecision {
	return claudelib.PermissionDecision{
		Allow:   false,
		Message: "attention gate did not return a verdict",
	}
}

// parseDecision reads the poster's verdict line out of its stdout. The exit code
// alone is never a verdict, so a line that does not carry one of the two accepted
// tokens is reported as unparsable.
func parseDecision(stdout string) (claudelib.PermissionDecision, bool) {
	for _, line := range strings.Split(stdout, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "DECISION: ")
		if !ok {
			continue
		}
		token, reason, _ := strings.Cut(rest, " -- ")
		switch strings.TrimSpace(token) {
		case "allow":
			return claudelib.PermissionDecision{Allow: true}, true
		case "deny":
			return claudelib.PermissionDecision{Allow: false, Message: reason}, true
		}
	}
	return claudelib.PermissionDecision{}, false
}

// payloadFor names the tool the operator is being asked about. It must not
// include the request's input preview under any circumstance: the item is
// readable by anything holding the store token, and the tool's input is not ours
// to publish.
func payloadFor(request claudelib.PermissionRequest) string {
	if request.Description == "" {
		return fmt.Sprintf(`Permission requested for tool "%s"`, request.ToolName)
	}
	return fmt.Sprintf(
		`Permission requested for tool "%s": %s`,
		request.ToolName,
		request.Description,
	)
}
