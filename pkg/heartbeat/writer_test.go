// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/bborbe/k8s"
	k8smocks "github.com/bborbe/k8s/mocks"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

var _ = Describe("ConfigMapWriter", func() {
	var (
		ctx             context.Context
		deployer        *k8smocks.K8sConfigMapDeployer
		currentDateTime libtime.CurrentDateTime
		namespace       k8s.Namespace
		writer          heartbeat.Writer
	)

	BeforeEach(func() {
		ctx = context.Background()
		deployer = &k8smocks.K8sConfigMapDeployer{}
		currentDateTime = libtime.NewCurrentDateTime()
		currentDateTime.SetNow(libtime.DateTime(time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)))
		namespace = k8s.Namespace("default")
		writer = heartbeat.NewConfigMapWriter(
			deployer,
			namespace,
			heartbeat.ConfigMapName,
			currentDateTime,
		)
	})

	It("writes an entry keyed by the session id", func() {
		deployer.GetReturns(nil, errors.New("not found"))
		Expect(writer.Write(ctx, "abc")).To(Succeed())
		Expect(deployer.DeployCallCount()).To(Equal(1))
		_, deployed := deployer.DeployArgsForCall(0)
		Expect(deployed.Name).To(Equal(heartbeat.ConfigMapName.String()))
		Expect(deployed.Namespace).To(Equal(namespace.String()))
		Expect(deployed.Data).To(HaveKey("abc"))

		var entry map[string]any
		Expect(json.Unmarshal([]byte(deployed.Data["abc"]), &entry)).To(Succeed())
		Expect(entry).To(HaveLen(1))
		Expect(entry).To(HaveKey("refreshedAt"))
		refreshedAt, ok := entry["refreshedAt"].(string)
		Expect(ok).To(BeTrue())
		_, err := time.Parse(time.RFC3339, refreshedAt)
		Expect(err).To(BeNil())
	})

	It("merges into existing data", func() {
		deployer.GetReturns(&corev1.ConfigMap{
			Data: map[string]string{
				"other-session": `{"refreshedAt":"2026-10-05T11:59:00Z"}`,
			},
		}, nil)
		Expect(writer.Write(ctx, "abc")).To(Succeed())
		Expect(deployer.DeployCallCount()).To(Equal(1))
		_, deployed := deployer.DeployArgsForCall(0)
		Expect(deployed.Data).To(HaveKey("other-session"))
		Expect(deployed.Data).To(HaveKey("abc"))
	})

	It("publishes a single-entry ConfigMap when the read finds nothing", func() {
		deployer.GetReturns(nil, errors.New("not found"))
		Expect(writer.Write(ctx, "abc")).To(Succeed())
		Expect(deployer.DeployCallCount()).To(Equal(1))
		_, deployed := deployer.DeployArgsForCall(0)
		Expect(deployed.Data).To(HaveLen(1))
		Expect(deployed.Data).To(HaveKey("abc"))
	})

	It("returns the deploy error to the caller", func() {
		deployer.GetReturns(nil, errors.New("not found"))
		deployer.DeployReturns(errors.New("configmaps is forbidden"))
		err := writer.Write(ctx, "abc")
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("forbidden"))
	})

	It("uses a valid ConfigMap name", func() {
		Expect(heartbeat.ConfigMapName.Validate(ctx)).To(BeNil())
	})
})
