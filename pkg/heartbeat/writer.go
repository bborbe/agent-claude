// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bborbe/errors"
	"github.com/bborbe/k8s"
	libtime "github.com/bborbe/time"
	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Writer publishes the liveness entry for one session.
type Writer interface {
	// Write stamps sessionID into the heartbeat ConfigMap with the current time.
	Write(ctx context.Context, sessionID string) error
}

// NewConfigMapWriter returns a Writer that stamps entries into the named
// ConfigMap in the given namespace, reading the existing data first so the
// write merges with what other writers have stored instead of replacing it.
func NewConfigMapWriter(
	deployer k8s.ConfigMapDeployer,
	namespace k8s.Namespace,
	name k8s.Name,
	currentDateTime libtime.CurrentDateTimeGetter,
) Writer {
	return &configMapWriter{
		deployer:        deployer,
		namespace:       namespace,
		name:            name,
		currentDateTime: currentDateTime,
	}
}

type configMapWriter struct {
	deployer        k8s.ConfigMapDeployer
	namespace       k8s.Namespace
	name            k8s.Name
	currentDateTime libtime.CurrentDateTimeGetter
}

// heartbeatEntry is the value the reader parses. Only refreshedAt is written:
// the reader requires no other field.
type heartbeatEntry struct {
	RefreshedAt string `json:"refreshedAt"`
}

func (w *configMapWriter) Write(ctx context.Context, sessionID string) error {
	value, err := json.Marshal(heartbeatEntry{
		RefreshedAt: w.currentDateTime.Now().Time().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return errors.Wrap(ctx, err, "marshal heartbeat entry")
	}
	if err := w.deployer.Deploy(ctx, w.configMap(w.mergeData(ctx, sessionID, string(value)))); err != nil {
		return errors.Wrap(ctx, err, "deploy heartbeat configmap")
	}
	return nil
}

// mergeData returns the data map to publish: the ConfigMap's stored data with
// this session's entry added. Deploy replaces the whole object rather than
// merging the data map, so a writer that published a fresh single-key object
// would silently drop every other writer's keys — hence the read-modify-write.
func (w *configMapWriter) mergeData(
	ctx context.Context,
	sessionID string,
	value string,
) map[string]string {
	current, err := w.deployer.Get(ctx, w.namespace, w.name)
	if err != nil {
		// The existing state is not readable, so this publishes a single-entry
		// ConfigMap. That is safe when the failure is persistent: Deploy's own
		// internal Get re-reads and surfaces it. A transient read failure
		// (timeout, 429, 500) while the ConfigMap holds another writer's keys
		// would let Deploy's whole-object replace drop those keys for one tick —
		// bounded and self-healing, because each writer re-stamps its own entry
		// within RefreshInterval.
		glog.V(3).Infof("read heartbeat configmap failed, publishing single entry: %v", err)
		return map[string]string{sessionID: value}
	}
	data := current.Data
	if data == nil {
		data = map[string]string{}
	}
	data[sessionID] = value
	return data
}

func (w *configMapWriter) configMap(data map[string]string) corev1.ConfigMap {
	return corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      w.name.String(),
			Namespace: w.namespace.String(),
		},
		Data: data,
	}
}
