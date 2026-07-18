package daemonapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/nkiyohara/owa-bridge/internal/application"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/localipc"
)

// Client calls one daemon namespace and reloads its rotating credential for
// every operation. It never retries an ambiguous application call.
type Client struct {
	endpoint localipc.Endpoint
	http     *http.Client
}

// NewClient creates a no-TCP HTTP transport over Unix socket or named pipe.
func NewClient(endpoint localipc.Endpoint) (*Client, error) {
	if endpoint.Address == "" || endpoint.CredentialPath == "" {
		return nil, errors.New("complete daemon endpoint is required")
	}
	transport := &http.Transport{
		Proxy:              nil,
		DialContext:        func(ctx context.Context, _, _ string) (net.Conn, error) { return localipc.DialContext(ctx, endpoint) },
		DisableCompression: true,
		DisableKeepAlives:  true,
		MaxConnsPerHost:    maxConcurrentCalls,
	}
	return &Client{endpoint: endpoint, http: &http.Client{
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}}, nil
}

// Close releases idle transport resources and no daemon state.
func (client *Client) Close() error {
	client.http.CloseIdleConnections()
	return nil
}

// Status negotiates the protocol and exposes the daemon default account.
func (client *Client) Status(ctx context.Context, caller domain.Caller) (Status, error) {
	var result Status
	if err := client.call(ctx, MethodStatus, caller, struct{}{}, &result); err != nil {
		return Status{}, err
	}
	if result.ProtocolVersion != ProtocolVersion {
		return Status{}, fmt.Errorf("daemon protocol %d is unsupported", result.ProtocolVersion)
	}
	if err := result.DefaultAccount.Validate(); err != nil {
		return Status{}, errors.New("daemon returned an invalid default account")
	}
	if err := validateConfigDigest(result.ConfigDigest); err != nil {
		return Status{}, errors.New("daemon returned an invalid config digest")
	}
	return result, nil
}

// Shutdown requests graceful termination after the response is written.
func (client *Client) Shutdown(ctx context.Context, caller domain.Caller) error {
	var result struct {
		Stopping bool `json:"stopping"`
	}
	if err := client.call(ctx, MethodShutdown, caller, struct{}{}, &result); err != nil {
		return err
	}
	if !result.Stopping {
		return errors.New("daemon did not acknowledge shutdown")
	}
	return nil
}

// Login asks the session owner to ensure an interactive account session.
func (client *Client) Login(ctx context.Context, account domain.AccountID, caller domain.Caller) (LoginResult, error) {
	if err := account.Validate(); err != nil {
		return LoginResult{}, err
	}
	var result LoginResult
	if err := client.call(ctx, MethodLogin, caller, LoginInput{Account: account}, &result); err != nil {
		return LoginResult{}, err
	}
	if result.Account != account || !result.Authenticated || result.CapturedAt.IsZero() {
		return LoginResult{}, errors.New("daemon returned invalid login state")
	}
	return result, nil
}

func (client *Client) ListMail(ctx context.Context, input application.MailListInput, caller domain.Caller) (application.MailPage, error) {
	var result application.MailPage
	return result, client.call(ctx, MethodMailList, caller, input, &result)
}

// SearchMail executes one bounded, read-only AQS search through the session owner.
func (client *Client) SearchMail(ctx context.Context, input application.MailSearchInput, caller domain.Caller) (application.MailPage, error) {
	var result application.MailPage
	return result, client.call(ctx, MethodMailSearch, caller, input, &result)
}

// ListMailFolders discovers bounded folder metadata through the session owner.
func (client *Client) ListMailFolders(ctx context.Context, input application.MailFolderListInput, caller domain.Caller) (application.MailFolderPage, error) {
	var result application.MailFolderPage
	return result, client.call(ctx, MethodMailFolders, caller, input, &result)
}

func (client *Client) GetMailBody(ctx context.Context, input application.MailBodyInput, caller domain.Caller) (application.MailBodyAccess, error) {
	var result application.MailBodyAccess
	return result, client.call(ctx, MethodMailGetBody, caller, input, &result)
}

func (client *Client) CommitMailBody(ctx context.Context, token string, caller domain.Caller) (application.MailBodyAccess, error) {
	var result application.MailBodyAccess
	return result, client.call(ctx, MethodMailCommitBody, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) CreateMailDraft(ctx context.Context, input application.MailDraftInput, caller domain.Caller) (application.MailDraftAccess, error) {
	var result application.MailDraftAccess
	return result, client.call(ctx, MethodMailCreateDraft, caller, input, &result)
}

func (client *Client) CommitMailDraft(ctx context.Context, token string, caller domain.Caller) (application.MailDraftAccess, error) {
	var result application.MailDraftAccess
	return result, client.call(ctx, MethodMailCommitDraft, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) SendMail(ctx context.Context, input application.MailSendInput, caller domain.Caller) (application.MailSendAccess, error) {
	var result application.MailSendAccess
	return result, client.call(ctx, MethodMailSend, caller, input, &result)
}

func (client *Client) CommitMailSend(ctx context.Context, token string, caller domain.Caller) (application.MailSendAccess, error) {
	var result application.MailSendAccess
	return result, client.call(ctx, MethodMailCommitSend, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) MoveMail(ctx context.Context, input application.MailMoveInput, caller domain.Caller) (application.MailMoveAccess, error) {
	var result application.MailMoveAccess
	return result, client.call(ctx, MethodMailMove, caller, input, &result)
}

func (client *Client) CommitMailMove(ctx context.Context, token string, caller domain.Caller) (application.MailMoveAccess, error) {
	var result application.MailMoveAccess
	return result, client.call(ctx, MethodMailCommitMove, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) SetMailReadState(ctx context.Context, input application.MailReadStateInput, caller domain.Caller) (application.MailReadStateAccess, error) {
	var result application.MailReadStateAccess
	return result, client.call(ctx, MethodMailReadState, caller, input, &result)
}

func (client *Client) CommitMailReadState(ctx context.Context, token string, caller domain.Caller) (application.MailReadStateAccess, error) {
	var result application.MailReadStateAccess
	return result, client.call(ctx, MethodMailCommitState, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) ListCalendar(ctx context.Context, input application.CalendarListInput, caller domain.Caller) (application.CalendarPage, error) {
	var result application.CalendarPage
	return result, client.call(ctx, MethodCalendarList, caller, input, &result)
}

// CreateCalendar prepares an immutable calendar event preview.
func (client *Client) CreateCalendar(ctx context.Context, input application.CalendarCreateInput, caller domain.Caller) (application.CalendarCreateAccess, error) {
	var result application.CalendarCreateAccess
	return result, client.call(ctx, MethodCalendarCreate, caller, input, &result)
}

// CommitCalendarCreate consumes one caller-bound calendar event preview.
func (client *Client) CommitCalendarCreate(ctx context.Context, token string, caller domain.Caller) (application.CalendarCreateAccess, error) {
	var result application.CalendarCreateAccess
	return result, client.call(ctx, MethodCalendarCommit, caller, ApprovalInput{Token: token}, &result)
}

// UpdateCalendar prepares an immutable patch preview for one event version.
func (client *Client) UpdateCalendar(ctx context.Context, input application.CalendarUpdateInput, caller domain.Caller) (application.CalendarUpdateAccess, error) {
	var result application.CalendarUpdateAccess
	return result, client.call(ctx, MethodCalendarUpdate, caller, input, &result)
}

// CommitCalendarUpdate consumes one caller-bound calendar update preview.
func (client *Client) CommitCalendarUpdate(ctx context.Context, token string, caller domain.Caller) (application.CalendarUpdateAccess, error) {
	var result application.CalendarUpdateAccess
	return result, client.call(ctx, MethodCalendarCommitUpdate, caller, ApprovalInput{Token: token}, &result)
}

// CancelCalendar prepares a destructive preview for one event version.
func (client *Client) CancelCalendar(ctx context.Context, input application.CalendarCancelInput, caller domain.Caller) (application.CalendarCancelAccess, error) {
	var result application.CalendarCancelAccess
	return result, client.call(ctx, MethodCalendarCancel, caller, input, &result)
}

// CommitCalendarCancel consumes one caller-bound cancellation preview.
func (client *Client) CommitCalendarCancel(ctx context.Context, token string, caller domain.Caller) (application.CalendarCancelAccess, error) {
	var result application.CalendarCancelAccess
	return result, client.call(ctx, MethodCalendarCommitCancel, caller, ApprovalInput{Token: token}, &result)
}

func (client *Client) call(ctx context.Context, method Method, caller domain.Caller, input, output any) error {
	if !method.valid() {
		return errors.New("invalid daemon method")
	}
	if err := caller.Validate(); err != nil {
		return err
	}
	params, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode daemon params: %w", err)
	}
	id, err := newRequestID()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(requestEnvelope{
		Version: ProtocolVersion, ID: id, Method: method, Caller: caller, Params: params,
	})
	if err != nil {
		return fmt.Errorf("encode daemon request: %w", err)
	}
	if len(payload) > maxRequestBytes {
		return errors.New("daemon request exceeds maximum size")
	}
	credential, err := localipc.LoadCredential(client.endpoint)
	if err != nil {
		return fmt.Errorf("load daemon credential: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+requestHost+requestPath, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build daemon request: %w", err)
	}
	request.Header.Set("Authorization", authorizationType+credential)
	request.Header.Set("Content-Type", contentType)
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("call local daemon: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read daemon response: %w", err)
	}
	if len(body) > maxResponseBytes {
		return errors.New("daemon response exceeds maximum size")
	}
	var envelope responseEnvelope
	if err := decodeStrict(bytes.NewReader(body), &envelope); err != nil {
		return errors.New("daemon returned an invalid response")
	}
	if envelope.Version != ProtocolVersion || envelope.ID != id && envelope.ID != "" {
		return errors.New("daemon returned a mismatched response")
	}
	if envelope.Error != nil {
		return envelope.Error
	}
	if response.StatusCode != http.StatusOK || len(envelope.Result) == 0 {
		return fmt.Errorf("daemon returned HTTP %d without a result", response.StatusCode)
	}
	if err := decodeStrict(bytes.NewReader(envelope.Result), output); err != nil {
		return errors.New("daemon returned an invalid result")
	}
	return nil
}

func newRequestID() (string, error) {
	random := make([]byte, 18)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate daemon request ID: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(random), nil
}
