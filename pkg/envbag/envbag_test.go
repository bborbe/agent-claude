// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package envbag_test

import (
	"github.com/bborbe/agent/envparse"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/bborbe/agent-claude/pkg/envbag"
)

var _ = Describe("KeyValueList", func() {
	Describe("String", func() {
		It("returns the sorted key names and no value", func() {
			Expect(envbag.KeyValueList("B=2,A=1").String()).To(Equal("A,B"))
		})

		It("renders an empty bag as the empty string", func() {
			Expect(envbag.KeyValueList("").String()).To(Equal(""))
		})

		It("renders only the parseable key when an entry has no '='", func() {
			Expect(envbag.KeyValueList("OK=yes,stray-fragment").String()).To(Equal("OK"))
		})

		It("keeps a value containing '=' out of the rendering", func() {
			Expect(envbag.KeyValueList("EQ=a=b=c").String()).To(Equal("EQ"))
		})

		It("trims whitespace around keys and sorts the result", func() {
			Expect(envbag.KeyValueList(" B = 2 , A = 1 ").String()).To(Equal("A,B"))
		})

		It("never renders a value", func() {
			Expect(
				envbag.KeyValueList("SECRET=hunter2").String(),
			).NotTo(ContainSubstring("hunter2"))
		})
	})

	Describe("Pairs", func() {
		It("returns the same map as envparse.KeyValuePairs for the same input", func() {
			const raw = "GH_TOKEN=abc,FOO=bar"
			Expect(envbag.KeyValueList(raw).Pairs()).To(Equal(envparse.KeyValuePairs(raw)))
		})

		It("returns nil for the empty bag", func() {
			Expect(envbag.KeyValueList("").Pairs()).To(BeNil())
		})
	})
})
