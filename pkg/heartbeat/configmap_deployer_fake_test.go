// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"errors"
	"sync"

	"github.com/bborbe/k8s"
	corev1 "k8s.io/api/core/v1"
)

// fakeCluster is an in-memory stand-in for the cluster's ConfigMap store. Its
// Deploy replaces the stored object wholesale — the same semantics as
// k8s.NewConfigMapDeployer, whose mergeConfigMap only carries the ResourceVersion
// across and lets the supplied Data replace the stored one. Modelling that
// faithfully is what makes the merge test meaningful: a writer that builds a fresh
// single-key ConfigMap loses every other writer's key here exactly as it would
// against a real cluster.
//
// It is hand-written rather than a counterfeiter mock because the integration
// specs assert the stored state after the loop has run, and counterfeiter fakes
// are stateless.
type fakeCluster struct {
	mutex       sync.Mutex
	items       map[string]corev1.ConfigMap
	deployCount int
}

// The fake is compile-time checked against the interface, so a future signature
// change fails to build rather than silently dropping the fake.
var _ k8s.ConfigMapDeployer = (*fakeCluster)(nil)

func newFakeCluster() *fakeCluster {
	return &fakeCluster{items: map[string]corev1.ConfigMap{}}
}

// key namespaces the store by "<namespace>/<name>", the identity the real
// deployer uses.
func (f *fakeCluster) key(namespace k8s.Namespace, name k8s.Name) string {
	return namespace.String() + "/" + name.String()
}

// Get returns a deep copy of the stored object, or an error when nothing is
// stored. The copy is load-bearing: the writer merges in place on the map it got
// from Get, so a shallow struct copy would alias the stored map and a writer that
// mutated the result without ever calling Deploy would still appear to have
// merged — a false pass the real API server cannot produce, because it hands back
// a freshly deserialized object.
func (f *fakeCluster) Get(
	ctx context.Context,
	namespace k8s.Namespace,
	name k8s.Name,
) (*corev1.ConfigMap, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	item, ok := f.items[f.key(namespace, name)]
	if !ok {
		return nil, errors.New("configmap not found")
	}
	return cloneConfigMap(item), nil
}

// Deploy stores the object it was handed under the namespace and name carried by
// that object, replacing anything already stored including its Data map — it does
// not merge, mirroring mergeConfigMap's whole-object replace.
func (f *fakeCluster) Deploy(ctx context.Context, configmap corev1.ConfigMap) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.items[f.key(k8s.Namespace(configmap.Namespace), k8s.Name(configmap.Name))] = configmap
	f.deployCount++
	return nil
}

// Undeploy deletes the entry. A missing entry is not an error, matching the real
// deployer's skip.
func (f *fakeCluster) Undeploy(
	ctx context.Context,
	namespace k8s.Namespace,
	name k8s.Name,
) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	delete(f.items, f.key(namespace, name))
	return nil
}

// stored returns the stored object and whether it exists.
func (f *fakeCluster) stored(namespace k8s.Namespace, name k8s.Name) (corev1.ConfigMap, bool) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	item, ok := f.items[f.key(namespace, name)]
	return item, ok
}

// deployCalls returns how many times Deploy has been called. The specs use it to
// prove the loop kept ticking without a fixed sleep.
func (f *fakeCluster) deployCalls() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.deployCount
}

// cloneConfigMap copies the object and its Data map, so a caller mutating the
// result cannot reach the stored map.
func cloneConfigMap(item corev1.ConfigMap) *corev1.ConfigMap {
	clone := item
	clone.Data = make(map[string]string, len(item.Data))
	for key, value := range item.Data {
		clone.Data[key] = value
	}
	return &clone
}
