package agent

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrDeviceCodePending   = errors.New("device-code pending")
	ErrDeviceCodeTimeout   = errors.New("device-code timeout")
	ErrDeviceCodeCancelled = errors.New("device-code cancelled")
	ErrDeviceCodeFailed    = errors.New("device-code failed")
	ErrDeviceCodeRefresh   = errors.New("device-code refresh failed")
)

type DeviceCodeEffectKind string

const (
	DeviceCodeEffectStart   DeviceCodeEffectKind = "device_code_start"
	DeviceCodeEffectPoll    DeviceCodeEffectKind = "device_code_poll"
	DeviceCodeEffectRefresh DeviceCodeEffectKind = "device_code_refresh"
	DeviceCodeEffectCancel  DeviceCodeEffectKind = "device_code_cancel"
)

type DeviceCodeResultStatus string

const (
	DeviceCodeStatusStarted   DeviceCodeResultStatus = "started"
	DeviceCodeStatusPending   DeviceCodeResultStatus = "pending"
	DeviceCodeStatusSucceeded DeviceCodeResultStatus = "succeeded"
	DeviceCodeStatusTimedOut  DeviceCodeResultStatus = "timed_out"
	DeviceCodeStatusRefreshed DeviceCodeResultStatus = "refreshed"
	DeviceCodeStatusCancelled DeviceCodeResultStatus = "cancelled"
	DeviceCodeStatusFailed    DeviceCodeResultStatus = "failed"
)

type DeviceCodePollStatus string

const (
	DeviceCodePollPending DeviceCodePollStatus = "pending"
	DeviceCodePollSuccess DeviceCodePollStatus = "success"
	DeviceCodePollFailure DeviceCodePollStatus = "failure"
)

type DeviceCodeClock interface {
	Now() time.Time
}

type DeviceCodeClient interface {
	StartDeviceCode(context.Context, DeviceCodeStartRequest) (DeviceCodeStartResponse, error)
	PollDeviceCode(context.Context, DeviceCodePollRequest) (DeviceCodePollResponse, error)
	RefreshDeviceCodeToken(context.Context, DeviceCodeRefreshRequest) (DeviceCodeRefreshResponse, error)
}

type DeviceCodeStartRequest struct {
	Provider string
	Model    string
	Scopes   []string
	Interval time.Duration
	Timeout  time.Duration
}

type DeviceCodeStartResponse struct {
	DeviceCode              Secret
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                time.Duration
	ExpiresIn               time.Duration
}

type DeviceCodePollRequest struct {
	Provider   string
	DeviceCode Secret
}

type DeviceCodePollResponse struct {
	Status     DeviceCodePollStatus
	Token      DeviceCodeToken
	RetryAfter time.Duration
	Err        error
}

type DeviceCodeRefreshRequest struct {
	Provider     string
	RefreshToken Secret
}

type DeviceCodeRefreshResponse struct {
	Token DeviceCodeToken
	Err   error
}

type DeviceCodeToken struct {
	AccessToken  Secret
	RefreshToken Secret
	ExpiresAt    time.Time
}

func (token DeviceCodeToken) String() string {
	return fmt.Sprintf("access_token %s refresh_token %s expires_at %s", token.AccessToken, token.RefreshToken, token.ExpiresAt.Format(time.RFC3339))
}

type DeviceCodeSession struct {
	Provider                string
	Model                   string
	DeviceCode              Secret
	UserCode                string
	VerificationURI         string
	VerificationURIComplete string
	Interval                time.Duration
	ExpiresAt               time.Time
	Cancelled               bool
}

func (session DeviceCodeSession) String() string {
	return fmt.Sprintf("provider %q model %q device_code %s user_code %q expires_at %s cancelled %t", session.Provider, session.Model, session.DeviceCode, session.UserCode, session.ExpiresAt.Format(time.RFC3339), session.Cancelled)
}

type DeviceCodeEffectResult struct {
	Kind       DeviceCodeEffectKind
	Status     DeviceCodeResultStatus
	Provider   string
	Session    DeviceCodeSession
	Token      DeviceCodeToken
	NextPollAt time.Time
	RetryAfter time.Duration
	Err        error
}

func StartDeviceCodeEffect(ctx context.Context, request DeviceCodeStartRequest, client DeviceCodeClient, clock DeviceCodeClock) (DeviceCodeEffectResult, error) {
	response, err := client.StartDeviceCode(ctx, request)
	if err != nil {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeFailed, Provider: request.Provider, Cause: err}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectStart, Status: DeviceCodeStatusFailed, Provider: request.Provider, Err: wrapped}, wrapped
	}

	now := clock.Now()
	interval := response.Interval
	if interval == 0 {
		interval = request.Interval
	}
	timeout := response.ExpiresIn
	if timeout == 0 {
		timeout = request.Timeout
	}
	session := DeviceCodeSession{
		Provider:                request.Provider,
		Model:                   request.Model,
		DeviceCode:              response.DeviceCode,
		UserCode:                response.UserCode,
		VerificationURI:         response.VerificationURI,
		VerificationURIComplete: response.VerificationURIComplete,
		Interval:                interval,
		ExpiresAt:               now.Add(timeout),
	}
	return DeviceCodeEffectResult{Kind: DeviceCodeEffectStart, Status: DeviceCodeStatusStarted, Provider: request.Provider, Session: session, NextPollAt: now.Add(interval), RetryAfter: interval}, nil
}

func PollDeviceCodeEffect(ctx context.Context, session DeviceCodeSession, client DeviceCodeClient, clock DeviceCodeClock) (DeviceCodeEffectResult, error) {
	if session.Cancelled {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeCancelled, Provider: session.Provider}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusCancelled, Provider: session.Provider, Session: session, Err: wrapped}, wrapped
	}
	now := clock.Now()
	if !now.Before(session.ExpiresAt) {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeTimeout, Provider: session.Provider}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusTimedOut, Provider: session.Provider, Session: session, Err: wrapped}, wrapped
	}

	response, err := client.PollDeviceCode(ctx, DeviceCodePollRequest{Provider: session.Provider, DeviceCode: session.DeviceCode})
	if err != nil {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeFailed, Provider: session.Provider, Cause: err}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusFailed, Provider: session.Provider, Session: session, Err: wrapped}, wrapped
	}
	switch response.Status {
	case DeviceCodePollPending:
		retryAfter := response.RetryAfter
		if retryAfter == 0 {
			retryAfter = session.Interval
		}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusPending, Provider: session.Provider, Session: session, NextPollAt: now.Add(retryAfter), RetryAfter: retryAfter, Err: ErrDeviceCodePending}, nil
	case DeviceCodePollSuccess:
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusSucceeded, Provider: session.Provider, Session: session, Token: response.Token}, nil
	case DeviceCodePollFailure:
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeFailed, Provider: session.Provider, Cause: response.Err}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusFailed, Provider: session.Provider, Session: session, Err: wrapped}, wrapped
	default:
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeFailed, Provider: session.Provider, Cause: fmt.Errorf("unsupported device-code poll status %q", response.Status)}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectPoll, Status: DeviceCodeStatusFailed, Provider: session.Provider, Session: session, Err: wrapped}, wrapped
	}
}

func RefreshDeviceCodeEffect(ctx context.Context, provider string, refreshToken Secret, client DeviceCodeClient) (DeviceCodeEffectResult, error) {
	response, err := client.RefreshDeviceCodeToken(ctx, DeviceCodeRefreshRequest{Provider: provider, RefreshToken: refreshToken})
	if err != nil {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeRefresh, Provider: provider, Cause: err}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectRefresh, Status: DeviceCodeStatusFailed, Provider: provider, Err: wrapped}, wrapped
	}
	if response.Err != nil {
		wrapped := DeviceCodeFailureError{Kind: ErrDeviceCodeRefresh, Provider: provider, Cause: response.Err}
		return DeviceCodeEffectResult{Kind: DeviceCodeEffectRefresh, Status: DeviceCodeStatusFailed, Provider: provider, Err: wrapped}, wrapped
	}
	return DeviceCodeEffectResult{Kind: DeviceCodeEffectRefresh, Status: DeviceCodeStatusRefreshed, Provider: provider, Token: response.Token}, nil
}

func CancelDeviceCodeEffect(session DeviceCodeSession, clock DeviceCodeClock) DeviceCodeEffectResult {
	session.Cancelled = true
	now := clock.Now()
	return DeviceCodeEffectResult{Kind: DeviceCodeEffectCancel, Status: DeviceCodeStatusCancelled, Provider: session.Provider, Session: session, NextPollAt: now}
}

func ValidateDefaultDeviceCodeEffects(request ReadinessRequest) (ProviderReadiness, error) {
	readiness, err := ClassifyFakeReadiness(request)
	if err != nil {
		return ProviderReadiness{}, err
	}
	if readiness.Family != ProviderFamilyDeviceCode {
		return ProviderReadiness{}, UnsupportedProviderError{Provider: request.Provider}
	}
	return readiness, nil
}

type DeviceCodeFailureError struct {
	Kind     error
	Provider string
	Cause    error
}

func (err DeviceCodeFailureError) Error() string {
	message := err.Kind.Error()
	if err.Cause != nil {
		message = fmt.Sprintf("%s: cause <redacted>", message)
	}
	return fmt.Sprintf("%s for provider %q value <redacted>", message, err.Provider)
}

func (err DeviceCodeFailureError) Unwrap() error {
	return err.Kind
}

type FakeDeviceCodeClock struct {
	Current time.Time
}

func (clock *FakeDeviceCodeClock) Now() time.Time {
	return clock.Current
}

func (clock *FakeDeviceCodeClock) Advance(duration time.Duration) {
	clock.Current = clock.Current.Add(duration)
}

type FakeDeviceCodeClient struct {
	StartResponse   DeviceCodeStartResponse
	StartErr        error
	PollResponses   []DeviceCodePollResponse
	PollErrs        []error
	RefreshResponse DeviceCodeRefreshResponse
	RefreshErr      error
	Starts          []DeviceCodeStartRequest
	Polls           []DeviceCodePollRequest
	Refreshes       []DeviceCodeRefreshRequest
}

func (client *FakeDeviceCodeClient) StartDeviceCode(_ context.Context, request DeviceCodeStartRequest) (DeviceCodeStartResponse, error) {
	client.Starts = append(client.Starts, request)
	return client.StartResponse, client.StartErr
}

func (client *FakeDeviceCodeClient) PollDeviceCode(_ context.Context, request DeviceCodePollRequest) (DeviceCodePollResponse, error) {
	client.Polls = append(client.Polls, request)
	if len(client.PollErrs) > 0 {
		err := client.PollErrs[0]
		client.PollErrs = client.PollErrs[1:]
		return DeviceCodePollResponse{}, err
	}
	if len(client.PollResponses) == 0 {
		return DeviceCodePollResponse{Status: DeviceCodePollPending}, nil
	}
	response := client.PollResponses[0]
	client.PollResponses = client.PollResponses[1:]
	return response, nil
}

func (client *FakeDeviceCodeClient) RefreshDeviceCodeToken(_ context.Context, request DeviceCodeRefreshRequest) (DeviceCodeRefreshResponse, error) {
	client.Refreshes = append(client.Refreshes, request)
	return client.RefreshResponse, client.RefreshErr
}
