package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/domain"
)

func stringPointer(value string) *string { return &value }

func validCalendarUpdateInput() CalendarUpdateInput {
	return CalendarUpdateInput{
		Account: "work", EventID: "event-1", ChangeKey: "change-1",
		Subject:  stringPointer("Updated synthetic event"),
		Start:    stringPointer("2026-07-20T09:00:00+01:00"),
		End:      stringPointer("2026-07-20T10:00:00+01:00"),
		Location: stringPointer("Room Updated"),
	}
}

func TestCalendarUpdateAlwaysPreviewsThenCommitsExactPatch(t *testing.T) {
	t.Parallel()

	port := &fakeCalendarReader{updateResult: CalendarUpdateResult{ID: "event-1", ChangeKey: "change-2"}}
	service, recorder := testCalendarService(t, port)
	caller := domain.Caller{Surface: "mcp", Instance: "session-1"}
	input := validCalendarUpdateInput()
	access, err := service.Update(t.Context(), input, caller)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if access.Status != "approval_required" || access.Preview == nil || port.updateCalls != 0 ||
		access.Preview.Operation.Name != "calendar.update" ||
		access.Preview.Operation.Effect != domain.EffectExternalWrite ||
		access.Review.MeetingUpdateMode != CalendarMeetingUpdateModeOWADefault {
		t.Fatalf("unsafe preview: %+v calls=%d", access, port.updateCalls)
	}
	committed, err := service.CommitUpdate(t.Context(), access.Preview.Token, caller)
	if err != nil {
		t.Fatalf("CommitUpdate() error = %v", err)
	}
	if committed.Status != "updated" || committed.Updated == nil ||
		committed.Updated.ChangeKey != "change-2" || port.updateCalls != 1 ||
		port.updateInput.Subject == nil || *port.updateInput.Subject != *input.Subject {
		t.Fatalf("unexpected commit: %+v input=%+v calls=%d", committed, port.updateInput, port.updateCalls)
	}
	if len(recorder.events) != 3 || recorder.events[2].Outcome != AuditOutcomeSuccess {
		t.Fatalf("unexpected audit events: %+v", recorder.events)
	}
}

func TestCalendarUpdateReviewRepresentsClearsAndBoundsBody(t *testing.T) {
	t.Parallel()

	input := CalendarUpdateInput{
		Account: "work", EventID: "event-1", ChangeKey: "change-1",
		Subject: stringPointer(""), Body: stringPointer(strings.Repeat("界", calendarBodyPreviewRunes+10)),
		Location: stringPointer(""),
	}
	review := input.Review()
	if review.Subject == nil || *review.Subject != "" || review.Location == nil || *review.Location != "" ||
		review.Body == nil || !strings.HasSuffix(review.Body.Preview, "…") ||
		review.Body.Bytes != len(*input.Body) || len(review.Body.SHA256) != 64 {
		t.Fatalf("unexpected review: %+v", review)
	}
	*input.Subject = "mutated"
	if *review.Subject != "" {
		t.Fatalf("review alias changed after input mutation: %+v", review)
	}
}

func TestCalendarUpdateValidationPreventsNetworkUse(t *testing.T) {
	t.Parallel()

	port := &fakeCalendarReader{}
	service, _ := testCalendarService(t, port)
	caller := domain.Caller{Surface: "cli", Instance: "process-1"}
	valid := validCalendarUpdateInput()
	tests := []CalendarUpdateInput{
		{},
		{Account: "work", EventID: "event-1", ChangeKey: "change-1"},
		func() CalendarUpdateInput { value := valid; value.EventID = ""; return value }(),
		func() CalendarUpdateInput { value := valid; value.ChangeKey = ""; return value }(),
		func() CalendarUpdateInput {
			value := valid
			value.Subject = stringPointer("bad\nsubject")
			return value
		}(),
		func() CalendarUpdateInput { value := valid; value.Body = stringPointer("bad\x00body"); return value }(),
		func() CalendarUpdateInput {
			value := valid
			value.Location = stringPointer("bad\rlocation")
			return value
		}(),
		func() CalendarUpdateInput { value := valid; value.End = nil; return value }(),
		func() CalendarUpdateInput { value := valid; value.Start = stringPointer("not-a-time"); return value }(),
		func() CalendarUpdateInput { value := valid; value.End = cloneString(value.Start); return value }(),
		func() CalendarUpdateInput {
			value := valid
			value.End = stringPointer("2026-09-01T09:00:00+01:00")
			return value
		}(),
	}
	for _, input := range tests {
		if _, err := service.Update(context.Background(), input, caller); err == nil {
			t.Fatalf("Update(%+v) unexpectedly succeeded", input)
		}
	}
	if port.updateCalls != 0 {
		t.Fatalf("update calls = %d, want 0", port.updateCalls)
	}
}

func TestCalendarUpdateAuditsAmbiguousOutcome(t *testing.T) {
	t.Parallel()

	port := &fakeCalendarReader{updateErr: errors.Join(
		ErrWriteOutcomeUnknown, errors.New("synthetic disconnect"),
	)}
	service, recorder := testCalendarService(t, port)
	caller := domain.Caller{Surface: "cli", Instance: "process-1"}
	access, err := service.Update(t.Context(), validCalendarUpdateInput(), caller)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	_, err = service.CommitUpdate(t.Context(), access.Preview.Token, caller)
	if !errors.Is(err, ErrWriteOutcomeUnknown) {
		t.Fatalf("CommitUpdate() error = %v, want ErrWriteOutcomeUnknown", err)
	}
	last := recorder.events[len(recorder.events)-1]
	if last.Outcome != AuditOutcomeUnknown || last.Reason != "outcome_unknown" {
		t.Fatalf("unexpected audit event: %+v", last)
	}
}

func TestCalendarUpdatePreviewCannotAuthorizeCancellation(t *testing.T) {
	t.Parallel()

	port := &fakeCalendarReader{}
	service, _ := testCalendarService(t, port)
	caller := domain.Caller{Surface: "mcp", Instance: "session-1"}
	access, err := service.Update(t.Context(), validCalendarUpdateInput(), caller)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := service.CommitCancel(t.Context(), access.Preview.Token, caller); err == nil {
		t.Fatal("CommitCancel() accepted a calendar update preview")
	}
	if _, err := service.CommitUpdate(t.Context(), access.Preview.Token, caller); err != nil {
		t.Fatalf("CommitUpdate() error after mismatched operation = %v", err)
	}
	if port.cancelCalls != 0 || port.updateCalls != 1 {
		t.Fatalf("unexpected calls: cancel=%d update=%d", port.cancelCalls, port.updateCalls)
	}
}
