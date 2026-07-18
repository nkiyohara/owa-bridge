package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nkiyohara/owa-bridge/internal/approval"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/policy"
)

const CalendarMeetingUpdateModeOWADefault = "owa_default"

// CalendarUpdateInput applies a closed patch to one exact event version.
// Nil fields remain unchanged; an empty pointed-to string clears that field.
type CalendarUpdateInput struct {
	Account   domain.AccountID `json:"account"`
	EventID   string           `json:"eventId"`
	ChangeKey string           `json:"changeKey"`
	Subject   *string          `json:"subject,omitempty"`
	Body      *string          `json:"body,omitempty"`
	Start     *string          `json:"start,omitempty"`
	End       *string          `json:"end,omitempty"`
	Location  *string          `json:"location,omitempty"`
}

// CalendarUpdateReview displays the exact patch without exposing an unbounded
// body. MeetingUpdateMode records that attendee notification behavior remains
// under OWA's default calendar policy.
type CalendarUpdateReview struct {
	EventID           string              `json:"eventId"`
	ChangeKey         string              `json:"changeKey"`
	Subject           *string             `json:"subject,omitempty"`
	Body              *CalendarBodyReview `json:"body,omitempty"`
	Start             *string             `json:"start,omitempty"`
	End               *string             `json:"end,omitempty"`
	Location          *string             `json:"location,omitempty"`
	MeetingUpdateMode string              `json:"meetingUpdateMode"`
}

// CalendarUpdateResult contains a refreshed identity when OWA returns one.
type CalendarUpdateResult struct {
	ID        string `json:"id,omitempty"`
	ChangeKey string `json:"changeKey,omitempty"`
}

// CalendarUpdateAccess is an immutable preview or a completed update.
type CalendarUpdateAccess struct {
	Status  string                `json:"status"`
	Updated *CalendarUpdateResult `json:"updated,omitempty"`
	Review  CalendarUpdateReview  `json:"review"`
	Preview *approval.Preview     `json:"preview,omitempty"`
}

// CalendarUpdater is the narrow OWA port for the supported calendar fields.
type CalendarUpdater interface {
	UpdateCalendarEvent(context.Context, CalendarUpdateInput) (CalendarUpdateResult, error)
}

// Update always prepares an exact external-write preview.
func (service *CalendarService) Update(
	ctx context.Context,
	input CalendarUpdateInput,
	caller domain.Caller,
) (CalendarUpdateAccess, error) {
	if err := input.Validate(); err != nil {
		return CalendarUpdateAccess{}, err
	}
	operation, err := domain.NewOperation(
		"calendar.update", domain.EffectExternalWrite, input.Account, input,
	)
	if err != nil {
		return CalendarUpdateAccess{}, fmt.Errorf("create calendar update operation: %w", err)
	}
	prepared, err := service.guard.Prepare(ctx, operation, caller)
	if err != nil {
		return CalendarUpdateAccess{}, err
	}
	switch prepared.Decision.Verdict {
	case policy.VerdictPreview:
		return CalendarUpdateAccess{
			Status: "approval_required", Review: input.Review(), Preview: prepared.Preview,
		}, nil
	case policy.VerdictDeny:
		return CalendarUpdateAccess{}, errors.New("calendar update operation was denied")
	case policy.VerdictAllow:
		return CalendarUpdateAccess{}, errors.New("calendar update policy attempted to bypass mandatory preview")
	default:
		return CalendarUpdateAccess{}, errors.New("calendar update operation received an unknown policy verdict")
	}
}

// CommitUpdate consumes one caller-bound preview and submits its exact patch.
func (service *CalendarService) CommitUpdate(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (CalendarUpdateAccess, error) {
	operation, err := service.guard.CommitFor(
		ctx, token, caller, "calendar.update", domain.EffectExternalWrite,
	)
	if err != nil {
		return CalendarUpdateAccess{}, err
	}
	var input CalendarUpdateInput
	if err := operation.DecodePayload(&input); err != nil {
		return CalendarUpdateAccess{}, err
	}
	if err := input.Validate(); err != nil {
		return CalendarUpdateAccess{}, err
	}
	updated, err := service.executeUpdate(ctx, input, caller, operation)
	if err != nil {
		return CalendarUpdateAccess{}, err
	}
	return CalendarUpdateAccess{
		Status: "updated", Updated: &updated, Review: input.Review(),
	}, nil
}

func (service *CalendarService) executeUpdate(
	ctx context.Context,
	input CalendarUpdateInput,
	caller domain.Caller,
	operation domain.Operation,
) (CalendarUpdateResult, error) {
	updated, callErr := service.updater.UpdateCalendarEvent(ctx, input)
	outcome, reason := calendarWriteAuditOutcome(callErr)
	auditErr := service.guard.audit.Record(context.WithoutCancel(ctx), AuditEvent{
		Phase: AuditPhaseExecuted, Outcome: outcome, Reason: reason,
		Caller: caller, Operation: operation.View(),
	})
	if callErr != nil || auditErr != nil {
		return CalendarUpdateResult{}, errors.Join(callErr, auditErr)
	}
	return updated, nil
}

// Validate rejects empty patches, stale-unsafe identities, and unsupported
// independent start/end changes before policy or network use.
func (input CalendarUpdateInput) Validate() error {
	if err := input.Account.Validate(); err != nil {
		return err
	}
	if err := validateOpaqueValue("calendar event ID", input.EventID); err != nil {
		return err
	}
	if err := validateOpaqueValue("calendar event change key", input.ChangeKey); err != nil {
		return err
	}
	if input.Subject == nil && input.Body == nil && input.Start == nil &&
		input.End == nil && input.Location == nil {
		return errors.New("calendar update must change at least one supported field")
	}
	if input.Subject != nil && (!utf8.ValidString(*input.Subject) ||
		len(*input.Subject) > MaxCalendarSubjectBytes || strings.ContainsAny(*input.Subject, "\r\n\x00")) {
		return errors.New("calendar subject is malformed or too large")
	}
	if input.Body != nil && (!utf8.ValidString(*input.Body) ||
		len(*input.Body) > MaxCalendarBodyBytes || strings.ContainsRune(*input.Body, '\x00')) {
		return errors.New("calendar body is malformed or too large")
	}
	if input.Location != nil && (!utf8.ValidString(*input.Location) ||
		len(*input.Location) > MaxCalendarLocationBytes || strings.ContainsAny(*input.Location, "\r\n\x00")) {
		return errors.New("calendar location is malformed or too large")
	}
	if (input.Start == nil) != (input.End == nil) {
		return errors.New("calendar start and end must be updated together")
	}
	if input.Start != nil {
		start, err := time.Parse(time.RFC3339, *input.Start)
		if err != nil {
			return errors.New("calendar start must be RFC3339")
		}
		end, err := time.Parse(time.RFC3339, *input.End)
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
	}
	return nil
}

// Review returns an immutable bounded representation of the exact patch.
func (input CalendarUpdateInput) Review() CalendarUpdateReview {
	review := CalendarUpdateReview{
		EventID: input.EventID, ChangeKey: input.ChangeKey,
		Subject: cloneString(input.Subject), Start: cloneString(input.Start),
		End: cloneString(input.End), Location: cloneString(input.Location),
		MeetingUpdateMode: CalendarMeetingUpdateModeOWADefault,
	}
	if input.Body != nil {
		body := reviewCalendarBody(*input.Body)
		review.Body = &body
	}
	return review
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func calendarWriteAuditOutcome(callErr error) (AuditOutcome, string) {
	if callErr == nil {
		return AuditOutcomeSuccess, "completed"
	}
	if errors.Is(callErr, ErrWriteOutcomeUnknown) {
		return AuditOutcomeUnknown, "outcome_unknown"
	}
	return AuditOutcomeFailure, "transport_error"
}
