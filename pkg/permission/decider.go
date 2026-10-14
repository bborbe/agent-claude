// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package permission

import (
	"context"

	claudelib "github.com/bborbe/agent/claude"
)

// decider routes each permission request to the channel that raised it.
type decider struct {
	registry claudelib.PermissionDecider
	gate     Gate
}

// NewDecider returns a PermissionDecider that sends a request to the channel that
// raised it: a request raised by an A2A turn goes to the gate, and every other
// request — one raised by a /prompt turn, or one that named no route at all — goes
// to registry, which is the decider the pod's own /permission endpoint serves.
func NewDecider(registry claudelib.PermissionDecider, gate Gate) claudelib.PermissionDecider {
	return &decider{
		registry: registry,
		gate:     gate,
	}
}

// DecidePermission sends an A2A-raised request to the attention gate and every
// other request to the registry unchanged. The empty Source falls into the
// registry branch: a caller outside the interactive service is unaffected.
func (d *decider) DecidePermission(
	ctx context.Context,
	request claudelib.PermissionRequest,
) (claudelib.PermissionDecision, error) {
	if request.Source != claudelib.A2APermissionSource {
		return d.registry.DecidePermission(ctx, request)
	}
	return d.gate.Decide(ctx, request), nil
}
