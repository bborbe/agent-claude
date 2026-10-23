// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"sync"

	"github.com/bborbe/k8s"
	corev1 "k8s.io/api/core/v1"
)

// fakeConfigMapDeployer is the counterfeiter-style stub the writer and publisher
// specs drive. It replaces k8smocks.K8sConfigMapDeployer, whose generated package
// no longer compiles against k8s.io/client-go v0.38.0-alpha.2: the typed
// interfaces gained DeleteWithResult while bborbe/k8s v1.14.23 ships mocks
// generated before that change, and there is no newer release to bump to.
//
// Unlike fakeCluster it does not model a cluster — the specs here only need a
// canned Get result, a canned Deploy error and the recorded Deploy arguments.
type fakeConfigMapDeployer struct {
	mutex sync.Mutex

	getReturns    *corev1.ConfigMap
	getReturnsErr error

	deployErr  error
	deployArgs []context.Context
	deployed   []corev1.ConfigMap
}

var _ k8s.ConfigMapDeployer = (*fakeConfigMapDeployer)(nil)

func (f *fakeConfigMapDeployer) GetReturns(configMap *corev1.ConfigMap, err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.getReturns = configMap
	f.getReturnsErr = err
}

func (f *fakeConfigMapDeployer) Get(
	ctx context.Context,
	namespace k8s.Namespace,
	name k8s.Name,
) (*corev1.ConfigMap, error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.getReturns, f.getReturnsErr
}

func (f *fakeConfigMapDeployer) DeployReturns(err error) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.deployErr = err
}

func (f *fakeConfigMapDeployer) Deploy(ctx context.Context, configMap corev1.ConfigMap) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.deployArgs = append(f.deployArgs, ctx)
	f.deployed = append(f.deployed, configMap)
	return f.deployErr
}

func (f *fakeConfigMapDeployer) DeployCallCount() int {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return len(f.deployed)
}

func (f *fakeConfigMapDeployer) DeployArgsForCall(i int) (context.Context, corev1.ConfigMap) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.deployArgs[i], f.deployed[i]
}

func (f *fakeConfigMapDeployer) Undeploy(
	ctx context.Context,
	namespace k8s.Namespace,
	name k8s.Name,
) error {
	return nil
}
