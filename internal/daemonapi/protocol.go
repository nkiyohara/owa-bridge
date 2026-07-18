// Package daemonapi defines the private, versioned API between owa-bridge
// adapters and the local session-owning daemon.
package daemonapi

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/nkiyohara/owa-bridge/internal/application"
	"github.com/nkiyohara/owa-bridge/internal/domain"
)

const (
	ProtocolVersion   = 7
	maxRequestBytes   = 8 << 20
	maxResponseBytes  = 16 << 20
	contentType       = "application/json"
	requestPath       = "/v1/call"
	requestHost       = "owa.local"
	authorizationType = "Bearer "
)

// Method is a closed daemon operation name. There is no arbitrary OWA action.
type Method string

const (
	MethodStatus               Method = "status"
	MethodShutdown             Method = "shutdown"
	MethodLogin                Method = "login"
	MethodMailFolders          Method = "mail.folders.list"
	MethodMailList             Method = "mail.list"
	MethodMailSearch           Method = "mail.search"
	MethodMailGetBody          Method = "mail.get_body"
	MethodMailCommitBody       Method = "mail.commit_body"
	MethodMailCreateDraft      Method = "mail.create_draft"
	MethodMailCommitDraft      Method = "mail.commit_draft"
	MethodMailSend             Method = "mail.send"
	MethodMailCommitSend       Method = "mail.commit_send"
	MethodMailMove             Method = "mail.move"
	MethodMailCommitMove       Method = "mail.commit_move"
	MethodMailReadState        Method = "mail.set_read_state"
	MethodMailCommitState      Method = "mail.commit_read_state"
	MethodCalendarList         Method = "calendar.list"
	MethodCalendarCreate       Method = "calendar.create"
	MethodCalendarCommit       Method = "calendar.commit_create"
	MethodCalendarUpdate       Method = "calendar.update"
	MethodCalendarCommitUpdate Method = "calendar.commit_update"
	MethodCalendarCancel       Method = "calendar.cancel"
	MethodCalendarCommitCancel Method = "calendar.commit_cancel"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,96}$`)

type requestEnvelope struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Method  Method          `json:"method"`
	Caller  domain.Caller   `json:"caller"`
	Params  json.RawMessage `json:"params"`
}

type responseEnvelope struct {
	Version int             `json:"version"`
	ID      string          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a stable remote failure without response bodies or credentials.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (failure *Error) Error() string {
	if failure == nil {
		return ""
	}
	return failure.Message
}

// Status describes a daemon without revealing its config or state paths.
type Status struct {
	ProtocolVersion int              `json:"protocolVersion"`
	Version         string           `json:"version"`
	ProcessID       int              `json:"processId"`
	StartedAt       time.Time        `json:"startedAt"`
	DefaultAccount  domain.AccountID `json:"defaultAccount"`
	ConfigDigest    string           `json:"configDigest"`
}

// ApprovalInput commits an exact in-memory operation preview.
type ApprovalInput struct {
	Token string `json:"token"`
}

// LoginInput selects one configured account for interactive authentication.
type LoginInput struct {
	Account domain.AccountID `json:"account"`
}

// LoginResult contains freshness metadata but no authorization material.
type LoginResult struct {
	Account       domain.AccountID `json:"account"`
	Authenticated bool             `json:"authenticated"`
	CapturedAt    time.Time        `json:"capturedAt"`
}

// Backend is the complete typed surface hosted by the session owner.
type Backend interface {
	DefaultAccount() domain.AccountID
	Login(context.Context, domain.AccountID, domain.Caller) (LoginResult, error)
	ListMailFolders(context.Context, application.MailFolderListInput, domain.Caller) (application.MailFolderPage, error)
	ListMail(context.Context, application.MailListInput, domain.Caller) (application.MailPage, error)
	SearchMail(context.Context, application.MailSearchInput, domain.Caller) (application.MailPage, error)
	GetMailBody(context.Context, application.MailBodyInput, domain.Caller) (application.MailBodyAccess, error)
	CommitMailBody(context.Context, string, domain.Caller) (application.MailBodyAccess, error)
	CreateMailDraft(context.Context, application.MailDraftInput, domain.Caller) (application.MailDraftAccess, error)
	CommitMailDraft(context.Context, string, domain.Caller) (application.MailDraftAccess, error)
	SendMail(context.Context, application.MailSendInput, domain.Caller) (application.MailSendAccess, error)
	CommitMailSend(context.Context, string, domain.Caller) (application.MailSendAccess, error)
	MoveMail(context.Context, application.MailMoveInput, domain.Caller) (application.MailMoveAccess, error)
	CommitMailMove(context.Context, string, domain.Caller) (application.MailMoveAccess, error)
	SetMailReadState(context.Context, application.MailReadStateInput, domain.Caller) (application.MailReadStateAccess, error)
	CommitMailReadState(context.Context, string, domain.Caller) (application.MailReadStateAccess, error)
	ListCalendar(context.Context, application.CalendarListInput, domain.Caller) (application.CalendarPage, error)
	CreateCalendar(context.Context, application.CalendarCreateInput, domain.Caller) (application.CalendarCreateAccess, error)
	CommitCalendarCreate(context.Context, string, domain.Caller) (application.CalendarCreateAccess, error)
	UpdateCalendar(context.Context, application.CalendarUpdateInput, domain.Caller) (application.CalendarUpdateAccess, error)
	CommitCalendarUpdate(context.Context, string, domain.Caller) (application.CalendarUpdateAccess, error)
	CancelCalendar(context.Context, application.CalendarCancelInput, domain.Caller) (application.CalendarCancelAccess, error)
	CommitCalendarCancel(context.Context, string, domain.Caller) (application.CalendarCancelAccess, error)
}

func (method Method) valid() bool {
	switch method {
	case MethodStatus, MethodShutdown, MethodLogin, MethodMailFolders, MethodMailList, MethodMailSearch, MethodMailGetBody, MethodMailCommitBody,
		MethodMailCreateDraft, MethodMailCommitDraft, MethodMailSend, MethodMailCommitSend,
		MethodMailMove, MethodMailCommitMove,
		MethodMailReadState, MethodMailCommitState,
		MethodCalendarList, MethodCalendarCreate, MethodCalendarCommit,
		MethodCalendarUpdate, MethodCalendarCommitUpdate,
		MethodCalendarCancel, MethodCalendarCommitCancel:
		return true
	default:
		return false
	}
}

func validateConfigDigest(value string) error {
	if len(value) != 64 {
		return errors.New("config digest must be a SHA-256 hex string")
	}
	if _, err := hex.DecodeString(value); err != nil {
		return errors.New("config digest must be a SHA-256 hex string")
	}
	return nil
}

func (request requestEnvelope) validate() error {
	if request.Version != ProtocolVersion {
		return fmt.Errorf("unsupported daemon protocol version %d", request.Version)
	}
	if !requestIDPattern.MatchString(request.ID) {
		return errors.New("invalid daemon request ID")
	}
	if !request.Method.valid() {
		return errors.New("unknown daemon method")
	}
	if err := request.Caller.Validate(); err != nil {
		return fmt.Errorf("invalid daemon caller: %w", err)
	}
	if len(request.Params) == 0 {
		return errors.New("daemon request params are required")
	}
	return nil
}
