package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

const (
	testDeviceCode   = "synthetic-device-code"
	testAccessToken  = "synthetic-access-token"
	testRefreshToken = "synthetic-refresh-token"
)

var testDeviceCodeNow = time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)

func TestDeviceCodeStartAndPollSuccessAreDeterministic(t *testing.T) {
	t.Parallel()

	clock := &FakeDeviceCodeClock{Current: testDeviceCodeNow}
	client := &FakeDeviceCodeClient{
		StartResponse: DeviceCodeStartResponse{
			DeviceCode:              NewSecret(testDeviceCode),
			UserCode:                "AILA-123",
			VerificationURI:         "https://example.test/device",
			VerificationURIComplete: "https://example.test/device?user_code=AILA-123",
			Interval:                5 * time.Second,
			ExpiresIn:               2 * time.Minute,
		},
		PollResponses: []DeviceCodePollResponse{{
			Status: DeviceCodePollSuccess,
			Token:  testDeviceCodeToken(testDeviceCodeNow.Add(time.Hour)),
		}},
	}
	startRequest := DeviceCodeStartRequest{Provider: "codex", Model: "codex-high", Scopes: []string{"model:read"}, Interval: time.Second, Timeout: time.Minute}

	start, err := StartDeviceCodeEffect(context.Background(), startRequest, client, clock)
	if err != nil {
		t.Fatalf("start device code: %v", err)
	}
	if start.Kind != DeviceCodeEffectStart || start.Status != DeviceCodeStatusStarted || start.Provider != "codex" {
		t.Fatalf("start result = %+v, want started codex", start)
	}
	if start.Session.ExpiresAt != testDeviceCodeNow.Add(2*time.Minute) || start.NextPollAt != testDeviceCodeNow.Add(5*time.Second) || start.RetryAfter != 5*time.Second {
		t.Fatalf("start timing = expires %s next %s retry %s", start.Session.ExpiresAt, start.NextPollAt, start.RetryAfter)
	}
	if !reflect.DeepEqual(client.Starts, []DeviceCodeStartRequest{startRequest}) {
		t.Fatalf("start calls = %#v, want %#v", client.Starts, []DeviceCodeStartRequest{startRequest})
	}

	poll, err := PollDeviceCodeEffect(context.Background(), start.Session, client, clock)
	if err != nil {
		t.Fatalf("poll device code: %v", err)
	}
	if poll.Kind != DeviceCodeEffectPoll || poll.Status != DeviceCodeStatusSucceeded || poll.Token.AccessToken.Value() != testAccessToken {
		t.Fatalf("poll result = %+v, want succeeded token", poll)
	}
	if len(client.Polls) != 1 || client.Polls[0].Provider != "codex" || client.Polls[0].DeviceCode.Value() != testDeviceCode {
		t.Fatalf("poll calls = %#v, want codex synthetic device code", client.Polls)
	}
}

func TestDeviceCodePendingRetryScheduleIsDeterministic(t *testing.T) {
	t.Parallel()

	clock := &FakeDeviceCodeClock{Current: testDeviceCodeNow}
	client := &FakeDeviceCodeClient{PollResponses: []DeviceCodePollResponse{{Status: DeviceCodePollPending, RetryAfter: 7 * time.Second}}}
	session := testDeviceCodeSession(testDeviceCodeNow.Add(time.Minute))

	result, err := PollDeviceCodeEffect(context.Background(), session, client, clock)
	if err != nil {
		t.Fatalf("pending poll returned error: %v", err)
	}
	if result.Status != DeviceCodeStatusPending || !errors.Is(result.Err, ErrDeviceCodePending) {
		t.Fatalf("pending result = %+v, want pending sentinel", result)
	}
	if result.RetryAfter != 7*time.Second || result.NextPollAt != testDeviceCodeNow.Add(7*time.Second) {
		t.Fatalf("retry schedule = %s %s, want 7s", result.RetryAfter, result.NextPollAt)
	}
}

func TestDeviceCodeTimeoutDoesNotPollClient(t *testing.T) {
	t.Parallel()

	clock := &FakeDeviceCodeClock{Current: testDeviceCodeNow}
	client := &FakeDeviceCodeClient{}
	session := testDeviceCodeSession(testDeviceCodeNow)

	result, err := PollDeviceCodeEffect(context.Background(), session, client, clock)
	if !errors.Is(err, ErrDeviceCodeTimeout) {
		t.Fatalf("timeout error = %v, want ErrDeviceCodeTimeout", err)
	}
	if result.Status != DeviceCodeStatusTimedOut || len(client.Polls) != 0 {
		t.Fatalf("timeout result = %+v polls=%d, want timed_out and no poll", result, len(client.Polls))
	}
}

func TestDeviceCodeCancelPreventsFurtherPoll(t *testing.T) {
	t.Parallel()

	clock := &FakeDeviceCodeClock{Current: testDeviceCodeNow}
	client := &FakeDeviceCodeClient{}
	cancel := CancelDeviceCodeEffect(testDeviceCodeSession(testDeviceCodeNow.Add(time.Minute)), clock)
	if cancel.Status != DeviceCodeStatusCancelled || !cancel.Session.Cancelled || cancel.NextPollAt != testDeviceCodeNow {
		t.Fatalf("cancel result = %+v, want cancelled at fake now", cancel)
	}

	result, err := PollDeviceCodeEffect(context.Background(), cancel.Session, client, clock)
	if !errors.Is(err, ErrDeviceCodeCancelled) {
		t.Fatalf("cancelled poll error = %v, want ErrDeviceCodeCancelled", err)
	}
	if result.Status != DeviceCodeStatusCancelled || len(client.Polls) != 0 {
		t.Fatalf("cancelled poll result = %+v polls=%d, want cancelled and no poll", result, len(client.Polls))
	}
}

func TestDeviceCodeRefreshIsTypedAndDeterministic(t *testing.T) {
	t.Parallel()

	client := &FakeDeviceCodeClient{RefreshResponse: DeviceCodeRefreshResponse{Token: testDeviceCodeToken(testDeviceCodeNow.Add(2 * time.Hour))}}
	result, err := RefreshDeviceCodeEffect(context.Background(), "copilot", NewSecret(testRefreshToken), client)
	if err != nil {
		t.Fatalf("refresh device token: %v", err)
	}
	if result.Kind != DeviceCodeEffectRefresh || result.Status != DeviceCodeStatusRefreshed || result.Provider != "copilot" || result.Token.ExpiresAt != testDeviceCodeNow.Add(2*time.Hour) {
		t.Fatalf("refresh result = %+v, want refreshed copilot token", result)
	}
	if len(client.Refreshes) != 1 || client.Refreshes[0].RefreshToken.Value() != testRefreshToken {
		t.Fatalf("refresh calls = %#v, want synthetic refresh token", client.Refreshes)
	}
}

func TestDeviceCodeFailureIsTypedAndRedacted(t *testing.T) {
	t.Parallel()

	clock := &FakeDeviceCodeClock{Current: testDeviceCodeNow}
	client := &FakeDeviceCodeClient{PollResponses: []DeviceCodePollResponse{{Status: DeviceCodePollFailure, Err: errors.New("remote rejected " + testAccessToken)}}}
	result, err := PollDeviceCodeEffect(context.Background(), testDeviceCodeSession(testDeviceCodeNow.Add(time.Minute)), client, clock)
	if !errors.Is(err, ErrDeviceCodeFailed) {
		t.Fatalf("failure error = %v, want ErrDeviceCodeFailed", err)
	}
	var typed DeviceCodeFailureError
	if !errors.As(err, &typed) {
		t.Fatalf("failure error = %T %[1]v, want DeviceCodeFailureError", err)
	}
	if result.Status != DeviceCodeStatusFailed || !errors.Is(result.Err, ErrDeviceCodeFailed) {
		t.Fatalf("failure result = %+v, want failed typed error", result)
	}
	for _, text := range []string{err.Error(), fmt.Sprint(result.Session), fmt.Sprintf("%+v", result.Session), fmt.Sprint(result.Token), fmt.Sprintf("%+v", result.Token)} {
		assertNoDeviceCodeSyntheticSecrets(t, text)
		if strings.Contains(text, "remote rejected") {
			t.Fatalf("error text leaked provider cause detail: %q", text)
		}
	}
}

func TestValidateDefaultDeviceCodeEffectsUseNoBrowserNetworkOrCredentialLookup(t *testing.T) {
	t.Parallel()

	readiness, err := ValidateDefaultDeviceCodeEffects(ReadinessRequest{Provider: "codex", Model: "codex-high"})
	if err != nil {
		t.Fatalf("validate default device-code effects: %v", err)
	}
	if readiness.Family != ProviderFamilyDeviceCode || readiness.RequiresCredentialCheck {
		t.Fatalf("readiness = %+v, want device-code without credential check", readiness)
	}

	_, err = ValidateDefaultDeviceCodeEffects(ReadinessRequest{Provider: "openai", Model: "gpt-4.1"})
	if !errors.Is(err, ErrUnsupportedProvider) {
		t.Fatalf("api-key provider error = %v, want ErrUnsupportedProvider", err)
	}
}

func testDeviceCodeSession(expiresAt time.Time) DeviceCodeSession {
	return DeviceCodeSession{Provider: "codex", Model: "codex-high", DeviceCode: NewSecret(testDeviceCode), UserCode: "AILA-123", Interval: 5 * time.Second, ExpiresAt: expiresAt}
}

func testDeviceCodeToken(expiresAt time.Time) DeviceCodeToken {
	return DeviceCodeToken{AccessToken: NewSecret(testAccessToken), RefreshToken: NewSecret(testRefreshToken), ExpiresAt: expiresAt}
}

func assertNoDeviceCodeSyntheticSecrets(t *testing.T, text string) {
	t.Helper()
	for _, secret := range []string{testDeviceCode, testAccessToken, testRefreshToken} {
		if strings.Contains(text, secret) {
			t.Fatalf("text leaked synthetic secret %q in %q", secret, text)
		}
	}
}
