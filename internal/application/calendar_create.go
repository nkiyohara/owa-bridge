package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nkiyohara/owa-bridge/internal/approval"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/policy"
)

const (
	MaxCalendarAttendees     = 1000
	MaxCalendarSubjectBytes  = 998
	MaxCalendarBodyBytes     = 1 << 20
	MaxCalendarLocationBytes = 4096
	MaxCalendarEventDuration = 31 * 24 * time.Hour
	calendarBodyPreviewRunes = 500
)

// CalendarCreateInput creates one non-recurring, non-all-day calendar item.
// Required and optional attendees turn it into a meeting invitation.
type CalendarCreateInput struct {
	Account           domain.AccountID `json:"account"`
	Calendar          CalendarFolder   `json:"calendar"`
	Subject           string           `json:"subject,omitempty"`
	Body              string           `json:"body,omitempty"`
	Start             string           `json:"start"`
	End               string           `json:"end"`
	Location          string           `json:"location,omitempty"`
	RequiredAttendees []string         `json:"requiredAttendees,omitempty"`
	OptionalAttendees []string         `json:"optionalAttendees,omitempty"`
	TeamsMeeting      bool             `json:"teamsMeeting,omitempty"`
}

// CalendarCreateReview is the exact bounded review shown before creation.
type CalendarCreateReview struct {
	Calendar              CalendarFolder `json:"calendar"`
	Subject               string         `json:"subject,omitempty"`
	BodyPreview           string         `json:"bodyPreview,omitempty"`
	BodyBytes             int            `json:"bodyBytes"`
	BodySHA256            string         `json:"bodySha256"`
	Start                 string         `json:"start"`
	End                   string         `json:"end"`
	Location              string         `json:"location,omitempty"`
	RequiredAttendees     []string       `json:"requiredAttendees,omitempty"`
	OptionalAttendees     []string       `json:"optionalAttendees,omitempty"`
	InvitationsWillBeSent bool           `json:"invitationsWillBeSent"`
	TeamsMeeting          bool           `json:"teamsMeeting"`
}

// CalendarBodyReview exposes bounded text while binding the complete body.
type CalendarBodyReview struct {
	Preview string `json:"preview,omitempty"`
	Bytes   int    `json:"bytes"`
	SHA256  string `json:"sha256"`
}

// CalendarCreateResult identifies the created event returned by OWA.
type CalendarCreateResult struct {
	ID                    string `json:"id"`
	ChangeKey             string `json:"changeKey,omitempty"`
	IsOnlineMeeting       bool   `json:"isOnlineMeeting"`
	OnlineMeetingProvider string `json:"onlineMeetingProvider,omitempty"`
	OnlineMeetingJoinURL  string `json:"onlineMeetingJoinUrl,omitempty"`
}

// CalendarCreateAccess is either an immutable preview or a created event.
type CalendarCreateAccess struct {
	Status  string                `json:"status"`
	Created *CalendarCreateResult `json:"created,omitempty"`
	Review  CalendarCreateReview  `json:"review"`
	Preview *approval.Preview     `json:"preview,omitempty"`
}

// CalendarCreator is the narrow OWA port for one new event.
type CalendarCreator interface {
	CreateCalendarEvent(context.Context, CalendarCreateInput) (CalendarCreateResult, error)
}

// Create prepares a calendar write. It always requires an exact preview,
// including when no attendees are present.
func (service *CalendarService) Create(
	ctx context.Context,
	input CalendarCreateInput,
	caller domain.Caller,
) (CalendarCreateAccess, error) {
	if err := input.Validate(service.maxAttendees); err != nil {
		return CalendarCreateAccess{}, err
	}
	operation, err := domain.NewOperation(
		"calendar.create", domain.EffectExternalWrite, input.Account, input,
	)
	if err != nil {
		return CalendarCreateAccess{}, fmt.Errorf("create calendar operation: %w", err)
	}
	prepared, err := service.guard.Prepare(ctx, operation, caller)
	if err != nil {
		return CalendarCreateAccess{}, err
	}
	switch prepared.Decision.Verdict {
	case policy.VerdictPreview:
		return CalendarCreateAccess{
			Status: "approval_required", Review: input.Review(), Preview: prepared.Preview,
		}, nil
	case policy.VerdictDeny:
		return CalendarCreateAccess{}, errors.New("calendar create operation was denied")
	case policy.VerdictAllow:
		return CalendarCreateAccess{}, errors.New("calendar create policy attempted to bypass mandatory preview")
	default:
		return CalendarCreateAccess{}, errors.New("calendar create operation received an unknown policy verdict")
	}
}

// CommitCreate consumes a caller-bound preview and submits its immutable event
// exactly once.
func (service *CalendarService) CommitCreate(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (CalendarCreateAccess, error) {
	operation, err := service.guard.CommitFor(
		ctx, token, caller, "calendar.create", domain.EffectExternalWrite,
	)
	if err != nil {
		return CalendarCreateAccess{}, err
	}
	var input CalendarCreateInput
	if err := operation.DecodePayload(&input); err != nil {
		return CalendarCreateAccess{}, err
	}
	if err := input.Validate(service.maxAttendees); err != nil {
		return CalendarCreateAccess{}, err
	}
	created, err := service.executeCreate(ctx, input, caller, operation)
	if err != nil {
		return CalendarCreateAccess{}, err
	}
	return CalendarCreateAccess{
		Status: "created", Created: &created, Review: input.Review(),
	}, nil
}

func (service *CalendarService) executeCreate(
	ctx context.Context,
	input CalendarCreateInput,
	caller domain.Caller,
	operation domain.Operation,
) (CalendarCreateResult, error) {
	created, callErr := service.creator.CreateCalendarEvent(ctx, input)
	outcome, reason := calendarWriteAuditOutcome(callErr)
	auditErr := service.guard.audit.Record(context.WithoutCancel(ctx), AuditEvent{
		Phase: AuditPhaseExecuted, Outcome: outcome, Reason: reason,
		Caller: caller, Operation: operation.View(),
	})
	if callErr != nil || auditErr != nil {
		return CalendarCreateResult{}, errors.Join(callErr, auditErr)
	}
	return created, nil
}

// Validate bounds all content, addresses, and absolute times before policy or
// network use.
func (input CalendarCreateInput) Validate(maxAttendees int) error {
	if err := input.Account.Validate(); err != nil {
		return err
	}
	if err := validateCalendarFolder(input.Calendar); err != nil {
		return err
	}
	if maxAttendees < 1 || maxAttendees > MaxCalendarAttendees {
		return errors.New("invalid calendar attendee limit")
	}
	if !utf8.ValidString(input.Subject) || len(input.Subject) > MaxCalendarSubjectBytes ||
		strings.ContainsAny(input.Subject, "\r\n\x00") {
		return errors.New("calendar subject is malformed or too large")
	}
	if !utf8.ValidString(input.Body) || len(input.Body) > MaxCalendarBodyBytes ||
		strings.ContainsRune(input.Body, '\x00') {
		return errors.New("calendar body is malformed or too large")
	}
	if !utf8.ValidString(input.Location) || len(input.Location) > MaxCalendarLocationBytes ||
		strings.ContainsAny(input.Location, "\r\n\x00") {
		return errors.New("calendar location is malformed or too large")
	}
	start, err := time.Parse(time.RFC3339, input.Start)
	if err != nil {
		return errors.New("calendar start must be RFC3339")
	}
	end, err := time.Parse(time.RFC3339, input.End)
	if err != nil {
		return errors.New("calendar end must be RFC3339")
	}
	duration := end.Sub(start)
	if duration <= 0 {
		return errors.New("calendar end must be after start")
	}
	if duration > MaxCalendarEventDuration {
		return fmt.Errorf("calendar event duration must not exceed %s", MaxCalendarEventDuration)
	}
	attendees := append(append([]string(nil), input.RequiredAttendees...), input.OptionalAttendees...)
	if len(attendees) > maxAttendees {
		return fmt.Errorf("calendar event has %d attendees; maximum is %d", len(attendees), maxAttendees)
	}
	seen := make(map[string]struct{}, len(attendees))
	for _, attendee := range attendees {
		if err := validateMailAddress(attendee); err != nil {
			return fmt.Errorf("validate calendar attendee: %w", err)
		}
		normalized := strings.ToLower(attendee)
		if _, exists := seen[normalized]; exists {
			return fmt.Errorf("calendar attendee %q appears more than once", attendee)
		}
		seen[normalized] = struct{}{}
	}
	return nil
}

// Review binds the complete body while limiting visible preview text.
func (input CalendarCreateInput) Review() CalendarCreateReview {
	body := reviewCalendarBody(input.Body)
	return CalendarCreateReview{
		Calendar: input.Calendar, Subject: input.Subject,
		BodyPreview: body.Preview, BodyBytes: body.Bytes, BodySHA256: body.SHA256,
		Start: input.Start, End: input.End, Location: input.Location,
		RequiredAttendees:     append([]string(nil), input.RequiredAttendees...),
		OptionalAttendees:     append([]string(nil), input.OptionalAttendees...),
		InvitationsWillBeSent: len(input.RequiredAttendees)+len(input.OptionalAttendees) > 0,
		TeamsMeeting:          input.TeamsMeeting,
	}
}

func reviewCalendarBody(value string) CalendarBodyReview {
	preview := value
	if utf8.RuneCountInString(preview) > calendarBodyPreviewRunes {
		runes := []rune(preview)
		preview = string(runes[:calendarBodyPreviewRunes-1]) + "…"
	}
	digest := sha256.Sum256([]byte(value))
	return CalendarBodyReview{
		Preview: preview, Bytes: len(value), SHA256: hex.EncodeToString(digest[:]),
	}
}
