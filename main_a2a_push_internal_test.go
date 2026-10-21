// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// This file is package main (not main_test) for the same reason
// main_internal_test.go is: the application struct and its newInteractiveService
// method are unexported, so an external test cannot build the service under test
// through the worker's own constructor. Ginkgo registers specs in one global suite
// per test binary, so the RunSpecs already in main_test.go runs these too.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	a2a "github.com/a2aproject/a2a-go/v2/a2a"
	a2apush "github.com/a2aproject/a2a-go/v2/a2asrv/push"
	agentlib "github.com/bborbe/agent"
	interactive "github.com/bborbe/agent/interactive"
	agentmocks "github.com/bborbe/agent/mocks"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
)

// testPushToken is the bearer credential the service under test is built with. It is a
// test literal, never a real credential, and it is never printed in a failure message.
const testPushToken = "test-push-token"

// longTurn is the turn's duration. It must exceed 30 seconds so the fixed mid-turn
// emission (10 seconds after the turn starts) lands with room on both sides: at least
// 10 seconds after the turn start and at least 10 seconds before the turn end.
const longTurn = 35 * time.Second

// midTurnDelay mirrors the library's fixed midTurnStatusDelay (10 * time.Second). It is a
// mirror for the assertion's arithmetic only — it is not a knob, and nothing in this
// repository configures it.
const midTurnDelay = 10 * time.Second

// BeforeSuite turns on glog's stderr sink and raises the verbosity the worker's turn
// markers need. It must run before any spec: the argument-parsing specs in
// main_internal_test.go replace flag.CommandLine in a BeforeEach, and after that
// replacement glog's flags are no longer registered and flag.Set would fail. A
// BeforeSuite always runs first, so this is the only place the calls can live.
var _ = BeforeSuite(func() {
	Expect(flag.Set("logtostderr", "true")).To(Succeed())
	Expect(flag.Set("v", "2")).To(Succeed())
})

// captureStderr redirects the process's stderr into a buffer for the duration of fn and
// returns what was written. glog's stderr sink reads os.Stderr when it emits, so the swap
// is observed as long as -logtostderr is on.
func captureStderr(fn func()) string {
	old := os.Stderr
	r, w, err := os.Pipe()
	Expect(err).To(BeNil())
	os.Stderr = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	// Restore and close on every path, including a panic from a failed Expect inside fn:
	// a leaked write end would keep the copier goroutine alive for the rest of the run.
	defer func() {
		os.Stderr = old
		_ = w.Close()
	}()

	fn()

	_ = w.Close()
	return <-done
}

// glogLineTime parses the timestamp out of a glog stderr line. glog's header carries no
// year, so the year is taken from now: time.Parse with layout "0102 ..." leaves the year
// at 0, and AddDate(now.Year()-parsed.Year(), 0, 0) sets it to the current one.
func glogLineTime(line string, now time.Time) (time.Time, bool) {
	if len(line) < 21 || line[0] != 'I' {
		return time.Time{}, false
	}
	parsed, err := time.Parse("0102 15:04:05.000000", line[1:21])
	if err != nil {
		return time.Time{}, false
	}
	return parsed.AddDate(now.Year()-parsed.Year(), 0, 0), true
}

// turnWindow scans captured stderr for the two turn-boundary lines of one session id and
// returns their parsed instants. It fails the spec — never skips it — when a marker is
// absent or unparseable, because a silently unparsed marker would turn the whole mid-turn
// window check into a no-op. The test runs exactly one turn on the id, so more than one of
// either marker is a failure too.
func turnWindow(output, sessionID string, now time.Time) (time.Time, time.Time) {
	startMarker := "turn start id=" + sessionID
	endMarker := "turn end id=" + sessionID

	var starts, ends []time.Time
	for _, line := range strings.Split(output, "\n") {
		header := strings.Index(line, "] ")
		if header < 0 {
			continue
		}
		message := strings.TrimSpace(line[header+2:])
		if message != startMarker && message != endMarker {
			continue
		}
		at, ok := glogLineTime(line, now)
		Expect(ok).To(BeTrue(), "unparseable glog line for %q: %q", message, line)
		if message == startMarker {
			starts = append(starts, at)
			continue
		}
		ends = append(ends, at)
	}

	Expect(
		starts,
	).To(HaveLen(1), "expected exactly one %q marker in the captured stderr", startMarker)
	Expect(ends).To(HaveLen(1), "expected exactly one %q marker in the captured stderr", endMarker)
	return starts[0], ends[0]
}

// recordedPush is one POST the receiver observed: when it arrived, the headers it carried
// and its raw body.
type recordedPush struct {
	arrived time.Time
	headers http.Header
	body    []byte
}

// pushReceiver records every POST it is sent, under a mutex, in arrival order. It answers
// 200 so the sender treats the push as delivered.
type pushReceiver struct {
	mutex      sync.Mutex
	posts      []recordedPush
	httpServer *httptest.Server
}

// newPushReceiver starts a recording receiver bound to loopback by construction
// (httptest.NewServer listens on 127.0.0.1) and returns it.
func newPushReceiver() *pushReceiver {
	receiver := &pushReceiver{}
	receiver.httpServer = httptest.NewServer(receiver.handler())
	return receiver
}

// handler is the receiver's HTTP surface.
func (r *pushReceiver) handler() http.Handler {
	return http.HandlerFunc(r.record)
}

// record stores one POST. The header map is cloned because http.Header on a request is
// only valid for the duration of the handler call.
func (r *pushReceiver) record(w http.ResponseWriter, req *http.Request) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read push body", http.StatusInternalServerError)
		return
	}
	r.mutex.Lock()
	r.posts = append(r.posts, recordedPush{
		arrived: time.Now(),
		headers: req.Header.Clone(),
		body:    body,
	})
	r.mutex.Unlock()
	w.WriteHeader(http.StatusOK)
}

// all returns a copy of the recorded POSTs, under the mutex.
func (r *pushReceiver) all() []recordedPush {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return append([]recordedPush(nil), r.posts...)
}

// server is the receiver's HTTP server.
func (r *pushReceiver) server() *httptest.Server {
	return r.httpServer
}

// a2aEnvelope is a JSON-RPC response: exactly one of the two halves is set.
type a2aEnvelope struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
}

// a2aCallBody marshals one JSON-RPC request.
func a2aCallBody(id int, method string, params any) string {
	body, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params"`
	}{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	Expect(err).NotTo(HaveOccurred())
	return string(body)
}

// postA2A sends one JSON-RPC body to the service's gated /a2a route and returns the status
// code and the raw response body. Every request carries the bearer token the service was
// built with: without it the gate answers 401 before the handler runs.
func postA2A(serverURL, body string) (int, string, error) {
	request, err := http.NewRequest(
		http.MethodPost,
		serverURL+"/a2a",
		strings.NewReader(body),
	)
	if err != nil {
		return 0, "", err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+testPushToken)
	// a2a-go v2.7.0 rejects a call whose A2A-Version does not match the version the
	// transport serves; an absent header is treated as "0.3", so every call must
	// name the served version explicitly.
	request.Header.Set(a2a.SvcParamVersion, string(a2a.Version))
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return 0, "", err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return response.StatusCode, "", err
	}
	return response.StatusCode, string(content), nil
}

// callA2A posts one request and decodes the envelope, failing the spec when the transport
// fails or the response is not a JSON-RPC envelope.
func callA2A(serverURL, body string) a2aEnvelope {
	status, responseBody, err := postA2A(serverURL, body)
	Expect(err).NotTo(HaveOccurred())
	Expect(status).To(Equal(http.StatusOK))
	var envelope a2aEnvelope
	Expect(json.Unmarshal([]byte(responseBody), &envelope)).To(Succeed())
	return envelope
}

// newWorkingStatusEvent is a status update built directly through the SDK's constructors,
// which is what the sender-level specs push.
func newWorkingStatusEvent() *a2a.TaskStatusUpdateEvent {
	return &a2a.TaskStatusUpdateEvent{
		ContextID: "pushprobe",
		TaskID:    a2a.TaskID("push-task-1"),
		Status:    a2a.TaskStatus{State: a2a.TaskStateWorking},
	}
}

// deliverOneStatusPush sends one working-state status update through the given sender and
// returns the single POST the receiver recorded, failing the spec when the call errored or
// the receiver recorded anything other than exactly one POST.
func deliverOneStatusPush(
	ctx context.Context,
	sender *a2apush.HTTPPushSender,
	receiver *pushReceiver,
	token string,
) recordedPush {
	config := &a2a.PushConfig{
		TaskID: a2a.TaskID("push-task-1"),
		URL:    receiver.server().URL,
		Token:  token,
	}
	Expect(sender.SendPush(ctx, config, newWorkingStatusEvent())).To(Succeed())
	posts := receiver.all()
	Expect(posts).To(HaveLen(1))
	return posts[0]
}

// pushedStatus is the subset of a pushed status-update body the specs assert on. A text
// part flattens to {"text": "..."}, which is where a message's text is read from.
type pushedStatus struct {
	TaskID string `json:"taskId"`
	Status struct {
		State   string `json:"state"`
		Message *struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"message"`
	} `json:"status"`
}

// statusPush is a recorded push that carried a statusUpdate, with the update decoded.
type statusPush struct {
	recordedPush
	status pushedStatus
}

// pushedStatusUpdate returns the status update a recorded push carried, and whether it
// carried one at all.
func pushedStatusUpdate(post recordedPush) (pushedStatus, bool) {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(post.body, &envelope); err != nil {
		return pushedStatus{}, false
	}
	raw, ok := envelope["statusUpdate"]
	if !ok {
		return pushedStatus{}, false
	}
	var status pushedStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		return pushedStatus{}, false
	}
	return status, true
}

// statusPushes selects the recorded POSTs that carry a statusUpdate. The turn also pushes
// the submitted-task and artifact events, so the status updates are selected by shape
// rather than by counting POSTs.
func statusPushes(posts []recordedPush) []statusPush {
	var selected []statusPush
	for _, post := range posts {
		status, ok := pushedStatusUpdate(post)
		if !ok {
			continue
		}
		selected = append(selected, statusPush{recordedPush: post, status: status})
	}
	return selected
}

// statusInState returns the one recorded status push carrying the given state. It fails the
// spec when there is not exactly one: the update happening, and happening once, are both
// the contract. The state is compared against the SDK's own constant, never a typed
// literal.
func statusInState(pushes []statusPush, state string) statusPush {
	var found []statusPush
	for _, push := range pushes {
		if push.status.Status.State == state {
			found = append(found, push)
		}
	}
	Expect(found).To(HaveLen(1), "expected exactly one %s status push", state)
	return found[0]
}

// sendMessageBody is one SendMessage request that starts a turn on the named conversation.
//
// It carries no task id: the SDK's exec-context loader refuses a message naming a task its
// task store does not hold, so a task id cannot be chosen by the client for a new task. It
// carries the callback inline instead, which the SDK saves before the executor runs — that
// is what puts the callback in place for every event of the turn, including the first
// pushed event, which is where the server-generated task id becomes visible.
func sendMessageBody(sessionID, callbackURL, token string) string {
	return a2aCallBody(1, "SendMessage", map[string]any{
		"message": map[string]any{
			"messageId": "m1",
			"role":      "ROLE_USER",
			"contextId": sessionID,
			"parts":     []map[string]any{{"text": "hello"}},
		},
		"configuration": map[string]any{
			"taskPushNotificationConfig": map[string]any{
				"url":   callbackURL,
				"token": token,
			},
		},
	})
}

// pushedTaskID returns the id of the first pushed task event, or "" when none has arrived.
func pushedTaskID(posts []recordedPush) string {
	for _, post := range posts {
		var envelope map[string]json.RawMessage
		if json.Unmarshal(post.body, &envelope) != nil {
			continue
		}
		raw, ok := envelope["task"]
		if !ok {
			continue
		}
		var task struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &task) != nil || task.ID == "" {
			continue
		}
		return task.ID
	}
	return ""
}

// waitForTaskID polls the receiver for the task the turn created and returns its id. The
// id is read off the pushed submitted-task event rather than chosen by the test, because
// the SDK refuses a message naming a task it does not hold and the push is the only place
// the server-generated id becomes visible while the turn is still running.
func waitForTaskID(receiver *pushReceiver) string {
	var taskID string
	Eventually(func() string {
		taskID = pushedTaskID(receiver.all())
		return taskID
	}, 20*time.Second, 100*time.Millisecond).ShouldNot(
		BeEmpty(),
		"the pushed submitted-task event never carried a task id",
	)
	return taskID
}

// registerPushCallback registers the receiver as the task's push callback and returns the
// id the store assigned. A refused registration fails the spec loudly rather than being
// logged and skipped, because a skipped registration makes every later assertion vacuous.
func registerPushCallback(serverURL, taskID, callbackURL, token string) string {
	envelope := callA2A(
		serverURL,
		a2aCallBody(2, "CreateTaskPushNotificationConfig", map[string]any{
			"taskId": taskID,
			"url":    callbackURL,
			"token":  token,
		}),
	)
	Expect(envelope.Error).To(BeNil())
	Expect(envelope.Result).NotTo(BeEmpty())
	var saved struct {
		ID string `json:"id"`
	}
	Expect(json.Unmarshal(envelope.Result, &saved)).To(Succeed())
	Expect(saved.ID).NotTo(BeEmpty())
	return saved.ID
}

// assertPushCallbackRoundTrips reads the registration back and proves it carries the
// address and the token it was created with.
func assertPushCallbackRoundTrips(serverURL, taskID, configID, callbackURL, token string) {
	envelope := callA2A(serverURL, a2aCallBody(3, "GetTaskPushNotificationConfig", map[string]any{
		"taskId": taskID,
		"id":     configID,
	}))
	Expect(envelope.Error).To(BeNil())
	var read struct {
		URL   string `json:"url"`
		Token string `json:"token"`
	}
	Expect(json.Unmarshal(envelope.Result, &read)).To(Succeed())
	Expect(read.URL).To(Equal(callbackURL))
	Expect(read.Token).To(Equal(token))
}

// assertUnknownTaskRefused proves a registration naming a task the worker has never seen is
// refused with the SDK's own task-not-found reason. The reason string is what distinguishes
// it from the push-not-supported refusal an unwired service answers.
func assertUnknownTaskRefused(serverURL, callbackURL string) {
	envelope := callA2A(
		serverURL,
		a2aCallBody(4, "CreateTaskPushNotificationConfig", map[string]any{
			"taskId": "never-seen",
			"url":    callbackURL,
			"token":  "tok",
		}),
	)
	Expect(envelope.Result).To(BeEmpty())
	Expect(envelope.Error).NotTo(BeNil())
	Expect(envelope.Error.Code).To(Equal(-32001))
	Expect(string(envelope.Error.Data)).To(ContainSubstring("TASK_NOT_FOUND"))
}

// assertMidTurnWorkingStatus proves the working-state push landed in the middle of the
// turn: strictly inside the worker's own turn window, at least midTurnDelay-1s after the
// turn start and at least midTurnDelay before its end. The last two bounds are what reject
// a send fired at the turn's first instant or at its last.
//
// The 1s slack on the after-start bound is deliberate and not a weakening: turn start is
// emitted inside the Prompt goroutine while the delay timer is armed on the executor's
// goroutine, so the two instants are separated by sub-second scheduling skew. The
// before-end bound keeps the full delay, because there the delay works in the assertion's
// favour.
func assertMidTurnWorkingStatus(working statusPush, start, end time.Time, pushToken string) {
	Expect(working.arrived).To(BeTemporally(">", start))
	Expect(working.arrived).To(BeTemporally("<", end))
	Expect(working.arrived.Sub(start)).To(BeNumerically(">=", midTurnDelay-1*time.Second))
	Expect(end.Sub(working.arrived)).To(BeNumerically(">=", midTurnDelay))
	Expect(working.status.Status.Message).NotTo(BeNil())
	Expect(working.status.Status.Message.Parts).NotTo(BeEmpty())
	Expect(working.status.Status.Message.Parts[0].Text).NotTo(BeEmpty())
	Expect(working.headers.Get("A2A-Notification-Token")).To(Equal(pushToken))
}

// assertCompletedAfterTurn proves the completion landed at or after the turn end.
func assertCompletedAfterTurn(completed statusPush, end time.Time) {
	Expect(completed.arrived).NotTo(BeTemporally("<", end))
}

// assertStatusEnvelopes proves every status POST is the SDK's own envelope — exactly one
// top-level key, statusUpdate — and names the task the turn ran on.
func assertStatusEnvelopes(statuses []statusPush, taskID string) {
	for _, push := range statuses {
		var envelope map[string]json.RawMessage
		Expect(json.Unmarshal(push.body, &envelope)).To(Succeed())
		// The single-key assertion is the discriminator check: the SDK's
		// StreamResponse.MarshalJSON writes exactly one of message / task /
		// statusUpdate / artifactUpdate, so a body keyed on "event", or a hand-rolled
		// {worker_id, state, text, ts} object, fails here.
		Expect(envelope).To(HaveLen(1))
		Expect(envelope).To(HaveKey("statusUpdate"))
		Expect(push.status.TaskID).To(Equal(taskID))
	}
}

// assertPushTurn asserts the whole wire contract of one long turn: the two status updates
// selected by state, the mid-turn one inside the turn, the completion at or after its end,
// the envelope shape, and a turn that the status it emits did not lengthen.
func assertPushTurn(posts []recordedPush, taskID, pushToken string, start, end time.Time) {
	statuses := statusPushes(posts)
	Expect(len(statuses)).To(
		BeNumerically(">=", 2),
		"expected at least a working and a completed status push",
	)
	working := statusInState(statuses, string(a2a.TaskStateWorking))
	completed := statusInState(statuses, string(a2a.TaskStateCompleted))

	assertMidTurnWorkingStatus(working, start, end, pushToken)
	assertCompletedAfterTurn(completed, end)
	assertStatusEnvelopes(statuses, taskID)

	// The turn must not have been lengthened by the status it emits: the window is closer
	// to the work's own duration than to the work plus the mid-turn delay. A
	// sleep; yield; work implementation lands inside the window and fails only here.
	Expect(end.Sub(start)).To(BeNumerically("<", longTurn+midTurnDelay/2))
}

// runPushTurn starts one long turn on the service under test while the worker's own
// turn-boundary lines are captured, and drives the registration, read-back and refusal
// calls that must happen while the turn is still running. The inline receiver carries the
// callback the request itself registers, so it sees every event of the turn; the explicit
// receiver is the address the separate registration below names. It returns the captured
// stderr.
func runPushTurn(
	serverURL string,
	inline *pushReceiver,
	explicit *pushReceiver,
	sessionID, pushToken string,
) string {
	done := make(chan error, 1)
	return captureStderr(func() {
		go func() {
			_, _, err := postA2A(
				serverURL,
				sendMessageBody(sessionID, inline.server().URL, pushToken),
			)
			done <- err
		}()

		taskID := waitForTaskID(inline)

		// Register promptly once the task id is known: the mid-turn emission is
		// midTurnDelay into the turn, so the registration has roughly that much slack. A
		// refused registration fails the spec loudly rather than being logged and
		// skipped, because a skipped registration makes every later assertion vacuous.
		configID := registerPushCallback(serverURL, taskID, explicit.server().URL, pushToken)
		assertPushCallbackRoundTrips(serverURL, taskID, configID, explicit.server().URL, pushToken)
		assertUnknownTaskRefused(serverURL, explicit.server().URL)

		// A nil error means the turn completed normally, which is also the evidence that
		// the push never failed the turn.
		Eventually(done, 2*longTurn).Should(Receive(BeNil()))
	})
}

var _ = Describe("A2A push sender", func() {
	var (
		ctx      context.Context
		receiver *pushReceiver
	)

	BeforeEach(func() {
		ctx = context.Background()
		receiver = newPushReceiver()
		DeferCleanup(receiver.server().Close)
	})

	// The production configuration, which is the only shape that can reach a loopback
	// callback at all: the private-network guard is opted out.
	productionSender := func() *a2apush.HTTPPushSender {
		return a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{AllowPrivateNetworks: true})
	}

	It("delivers to a loopback receiver and sends the token as the protocol header", func() {
		post := deliverOneStatusPush(ctx, productionSender(), receiver, "push-token")

		Expect(post.headers.Get("A2A-Notification-Token")).To(Equal("push-token"))
	})

	It("sends no token header at all when the config carries no token", func() {
		post := deliverOneStatusPush(ctx, productionSender(), receiver, "")

		// The raw header map is read rather than Header.Get: Get cannot tell an absent
		// header from an empty-string one, and "an absent token is not an empty-string
		// token" is exactly what this half asserts. The canonical form of the name is
		// A2a-Notification-Token, which is why CanonicalHeaderKey is used on both sides.
		_, present := post.headers[http.CanonicalHeaderKey("A2A-Notification-Token")]
		Expect(present).To(BeFalse())
	})

	It("refuses a loopback target with the guard at its default, swallowing the refusal", func() {
		sender := a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{})

		// No error: a failing push is logged and never returned to the caller, so a push
		// failure cannot cancel a turn.
		Expect(
			sender.SendPush(
				ctx,
				&a2a.PushConfig{URL: receiver.server().URL},
				newWorkingStatusEvent(),
			),
		).To(Succeed())
		Expect(receiver.all()).To(BeEmpty())
	})

	It("names the blocked address range when the guard's refusal is surfaced", func() {
		sender := a2apush.NewHTTPPushSender(&a2apush.HTTPSenderConfig{FailOnError: true})

		err := sender.SendPush(
			ctx,
			&a2a.PushConfig{URL: receiver.server().URL},
			newWorkingStatusEvent(),
		)
		Expect(err).To(MatchError(ContainSubstring("blocked address range")))
		Expect(receiver.all()).To(BeEmpty())
	})

	It("pushes the SDK's stream-response envelope rather than a bespoke shape", func() {
		post := deliverOneStatusPush(ctx, productionSender(), receiver, "push-token")

		var envelope map[string]json.RawMessage
		Expect(json.Unmarshal(post.body, &envelope)).To(Succeed())
		// The single-key assertion is the discriminator check: the SDK's
		// StreamResponse.MarshalJSON writes exactly one of message / task /
		// statusUpdate / artifactUpdate, so a body keyed on "event", or a hand-rolled
		// {worker_id, state, text, ts} object, fails here.
		Expect(envelope).To(HaveLen(1))
		Expect(envelope).To(HaveKey("statusUpdate"))

		var status pushedStatus
		Expect(json.Unmarshal(envelope["statusUpdate"], &status)).To(Succeed())
		Expect(status.TaskID).To(Equal("push-task-1"))
		Expect(status.Status.State).To(Equal(string(a2a.TaskStateWorking)))
	})
})

// One spec only, because the turn it runs takes about 35 seconds and the suite must not run
// it more than once. The window is measured against the worker's own two glog lines rather
// than the test's clock around the HTTP POST, so a skewed host cannot break the check.
var _ = Describe("A2A push notification end to end", func() {
	It("pushes a mid-turn working status inside the turn and a completion at its end", func() {
		const sessionID = "pushprobe"
		const pushToken = "push-token"

		inline := newPushReceiver()
		DeferCleanup(inline.server().Close)
		explicit := newPushReceiver()
		DeferCleanup(explicit.server().Close)

		session := &agentmocks.Session{}
		session.PromptStub = func(_ context.Context, _ string) (string, error) {
			time.Sleep(longTurn)
			return "push-probe reply", nil
		}
		sessions := &agentmocks.SessionFactory{}
		sessions.CreateStub = func(string) agentlib.Session { return session }

		// The service under test is built through the worker's own constructor, so this
		// drives the real wiring — the real push store and the real HTTP sender — rather
		// than a hand-assembled service.
		app := &application{
			Listen:               "127.0.0.1:0",
			InteractiveAuthToken: testPushToken,
			A2APublicURL:         "https://agent.example.test/a2a",
			A2AAgentName:         "claude-interactive",
		}
		service := app.newInteractiveService(
			sessions,
			prometheus.NewRegistry(),
			interactive.NewPermissionRegistry(),
		)
		server := httptest.NewServer(service.Handler())
		DeferCleanup(server.Close)

		output := runPushTurn(server.URL, inline, explicit, sessionID, pushToken)

		taskID := pushedTaskID(inline.all())
		Expect(taskID).NotTo(BeEmpty())

		start, end := turnWindow(output, sessionID, time.Now())
		assertPushTurn(inline.all(), taskID, pushToken, start, end)

		// The separately registered callback is live too: it received the turn's
		// completion, which the worker emits after the registration.
		statusInState(statusPushes(explicit.all()), string(a2a.TaskStateCompleted))
	})
})
