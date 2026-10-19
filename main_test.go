// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main_test

//go:generate go run -mod=mod github.com/maxbrunsfeld/counterfeiter/v6 -generate

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"
)

// NOTE: Explicit "Compiles" spec removed because spawning a child
// process from this race-instrumented test binary segfaults on the
// GH Actions runner (works locally; only reproduces on Linux CI under
// -race). The test binary itself IS package main built — if main.go
// does not compile, `go test` fails immediately, so the assertion is
// redundant. See vault note [[Github Workflow Actions]] gotchas.

func TestSuite(t *testing.T) {
	time.Local = time.UTC
	format.TruncatedDiff = false
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	// A spec that runs a deliberately long turn (~35s) must not be aborted mid-turn by
	// the suite ceiling and reported as a timeout instead of as its real assertion.
	suiteConfig.Timeout = 5 * time.Minute
	RunSpecs(t, "Main Suite", suiteConfig, reporterConfig)
}
