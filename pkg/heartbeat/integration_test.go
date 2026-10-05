// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package heartbeat_test

import (
	"context"
	"encoding/json"
	"time"

	"github.com/bborbe/k8s"
	libtime "github.com/bborbe/time"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/bborbe/agent-claude/pkg/heartbeat"
)

// pinnedNow is the instant the pinned-clock specs freeze the injected clock at.
var pinnedNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

// readerTTL is the reader's fixed liveness TTL: it calls a worker live while its
// stamp is younger than 60 seconds. The writer never applies this value — it only
// re-stamps on RefreshInterval.
const readerTTL = 60 * time.Second

// parseReaderEntry mirrors the reader's parse of one ConfigMap value:
// json.loads, read refreshedAt, datetime.fromisoformat. The reader lives in
// bborbe/claude-supervisor and is not modified; this mirrors its contract so a
// drifting data shape fails a test here rather than silently disappearing from
// the reader's view on the cluster.
func parseReaderEntry(value string) (time.Time, error) {
	var entry struct {
		RefreshedAt string `json:"refreshedAt"`
	}
	if err := json.Unmarshal([]byte(value), &entry); err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, entry.RefreshedAt)
}

// readerLive mirrors the reader's liveness verdict: the stamp is live when its
// age is non-negative and below the TTL. An unparseable value is skipped by the
// reader, so parseReaderEntry's error is the caller's to handle.
func readerLive(refreshedAt time.Time, now time.Time, ttl time.Duration) bool {
	age := now.Sub(refreshedAt)
	return age >= 0 && age < ttl
}

var _ = Describe("cluster heartbeat integration", func() {
	var (
		ctx       context.Context
		fake      *fakeCluster
		recorder  heartbeat.ActivityRecorder
		publisher heartbeat.Publisher
	)

	// start runs the publisher's real loop in the background and returns the
	// cancel func plus the channel Run's result arrives on.
	start := func() (context.CancelFunc, chan error) {
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- publisher.Run(runCtx)
		}()
		return cancel, done
	}

	// stop cancels the loop and asserts Run returned nil, the publisher's
	// documented behaviour on cancellation.
	stop := func(cancel context.CancelFunc, done chan error) {
		cancel()
		Eventually(done).Should(Receive(BeNil()))
	}

	BeforeEach(func() {
		ctx = context.Background()
		fake = newFakeCluster()
	})

	Describe("with a pinned clock", func() {
		var currentDateTime libtime.CurrentDateTime

		BeforeEach(func() {
			currentDateTime = libtime.NewCurrentDateTime()
			currentDateTime.SetNow(libtime.DateTime(pinnedNow))
			recorder = heartbeat.NewActivityRecorder(currentDateTime)
			publisher = heartbeat.NewPublisher(
				recorder,
				heartbeat.NewConfigMapWriter(
					fake,
					k8s.Namespace("dev"),
					heartbeat.ConfigMapName,
					currentDateTime,
				),
				10*time.Millisecond,
			)
		})

		It("stamps a live entry that parses through the reader's parser", func() {
			recorder.Record(ctx, "abc")
			cancel, done := start()
			defer cancel()

			Eventually(func() bool {
				_, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
				return ok
			}).Should(BeTrue())

			stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
			Expect(ok).To(BeTrue())
			value, ok := stored.Data["abc"]
			Expect(ok).To(BeTrue())

			stamp, err := parseReaderEntry(value)
			Expect(err).NotTo(HaveOccurred())
			Expect(stamp).To(Equal(pinnedNow))
			Expect(readerLive(stamp, stamp, readerTTL)).To(BeTrue())
			Expect(readerLive(stamp, stamp.Add(61*time.Second), readerTTL)).To(BeFalse())
			Expect(readerLive(stamp, stamp.Add(-1*time.Second), readerTTL)).To(BeFalse())

			var entry map[string]any
			Expect(json.Unmarshal([]byte(value), &entry)).To(Succeed())
			Expect(entry).To(HaveLen(1))
			Expect(entry).To(HaveKey("refreshedAt"))

			stop(cancel, done)
		})

		It("merges, so another writer's entry survives", func() {
			const otherEntry = `{"refreshedAt":"2026-10-05T11:00:00Z"}`
			Expect(fake.Deploy(ctx, corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      heartbeat.ConfigMapName.String(),
					Namespace: "dev",
				},
				Data: map[string]string{"other": otherEntry},
			})).To(Succeed())

			recorder.Record(ctx, "abc")
			cancel, done := start()
			defer cancel()

			Eventually(func() bool {
				stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
				if !ok {
					return false
				}
				return len(stored.Data) == 2
			}).Should(BeTrue())

			stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
			Expect(ok).To(BeTrue())
			Expect(stored.Data).To(HaveKey("abc"))
			Expect(stored.Data).To(HaveKey("other"))
			Expect(stored.Data["other"]).To(Equal(otherEntry))

			stop(cancel, done)
		})

		It("stops refreshing a session that has been idle past the cutoff", func() {
			recorder.Record(ctx, "abc")
			currentDateTime.SetNow(libtime.DateTime(pinnedNow.Add(2 * time.Minute)))
			cancel, done := start()
			defer cancel()

			Consistently(
				fake.deployCalls,
				100*time.Millisecond,
				10*time.Millisecond,
			).Should(Equal(0))
			_, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
			Expect(ok).To(BeFalse())

			stop(cancel, done)
		})
	})

	Describe("with a real clock", func() {
		BeforeEach(func() {
			currentDateTime := libtime.NewCurrentDateTime()
			recorder = heartbeat.NewActivityRecorder(currentDateTime)
			publisher = heartbeat.NewPublisher(
				recorder,
				heartbeat.NewConfigMapWriter(
					fake,
					k8s.Namespace("dev"),
					heartbeat.ConfigMapName,
					currentDateTime,
				),
				10*time.Millisecond,
			)
		})

		It("keeps the entry live while the loop runs", func() {
			recorder.Record(ctx, "abc")
			cancel, done := start()
			defer cancel()

			Eventually(fake.deployCalls).Should(BeNumerically(">=", 5))

			stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
			Expect(ok).To(BeTrue())
			stamp, err := parseReaderEntry(stored.Data["abc"])
			Expect(err).NotTo(HaveOccurred())
			Expect(readerLive(stamp, time.Now(), readerTTL)).To(BeTrue())

			stop(cancel, done)
		})
	})
})

var _ = Describe("fakeCluster", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("Get returns a copy that does not alias the stored map", func() {
		fake := newFakeCluster()
		Expect(fake.Deploy(ctx, corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      heartbeat.ConfigMapName.String(),
				Namespace: "dev",
			},
			Data: map[string]string{"other": "seed"},
		})).To(Succeed())
		got, err := fake.Get(ctx, k8s.Namespace("dev"), heartbeat.ConfigMapName)
		Expect(err).NotTo(HaveOccurred())
		got.Data["injected"] = "x"
		stored, ok := fake.stored(k8s.Namespace("dev"), heartbeat.ConfigMapName)
		Expect(ok).To(BeTrue())
		Expect(stored.Data).NotTo(HaveKey("injected"))
	})
})
