package owa

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/application"
)

func testCalendarCreateInput() application.CalendarCreateInput {
	return application.CalendarCreateInput{
		Account:           "work",
		Calendar:          application.CalendarFolder{Kind: application.CalendarFolderDistinguished, ID: "calendar"},
		Subject:           "Synthetic design review",
		Body:              "Discuss synthetic fixture data only.",
		Start:             "2026-07-20T09:00:00+01:00",
		End:               "2026-07-20T09:45:00+01:00",
		Location:          "Room Example",
		RequiredAttendees: []string{"alice@example.invalid"},
		OptionalAttendees: []string{"bob@example.invalid"},
		TeamsMeeting:      true,
	}
}

func TestCalendarCreateRequestMatchesGoldenFixture(t *testing.T) {
	t.Parallel()

	payload, err := buildCalendarCreateEnvelope(testCalendarCreateInput())
	if err != nil {
		t.Fatalf("buildCalendarCreateEnvelope() error = %v", err)
	}
	actual := marshalJSON(t, payload)
	want := readFixture(t, "create_calendar_event_request.json")
	assertJSONEqual(t, actual, want)
}

func TestCreateCalendarEventNormalizesResponseAndNeverRetries(t *testing.T) {
	t.Parallel()

	fixture := readFixture(t, "create_calendar_event_response.json")
	expectedRequest := readFixture(t, "create_calendar_event_request.json")
	requestBodies := make(chan []byte, 1)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if got := request.URL.Query().Get("action"); got != "CreateCalendarEvent" {
			t.Errorf("action query = %q, want CreateCalendarEvent", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
			return
		}
		requestBodies <- body
		_, _ = writer.Write(fixture)
	}))
	defer server.Close()

	client := testClient(t, server, nil)
	created, err := client.CreateCalendarEvent(context.Background(), testCalendarCreateInput())
	if err != nil {
		t.Fatalf("CreateCalendarEvent() error = %v", err)
	}
	assertJSONEqual(t, <-requestBodies, expectedRequest)
	if created.ID != "synthetic-event-created-1" || created.ChangeKey != "synthetic-change-created-1" ||
		!created.IsOnlineMeeting || created.OnlineMeetingProvider != teamsForBusinessProvider ||
		created.OnlineMeetingJoinURL != "https://teams.example.invalid/l/meetup-join/synthetic" {
		t.Fatalf("unexpected created event: %+v", created)
	}
	if calls.Load() != 1 {
		t.Fatalf("CreateCalendarEvent calls = %d, want exactly 1", calls.Load())
	}
}

func TestCalendarCreateWithoutAttendeesDoesNotSendInvitations(t *testing.T) {
	t.Parallel()

	input := testCalendarCreateInput()
	input.RequiredAttendees = nil
	input.OptionalAttendees = nil
	payload, err := buildCalendarCreateEnvelope(input)
	if err != nil {
		t.Fatalf("buildCalendarCreateEnvelope() error = %v", err)
	}
	if payload.Body.SendMeetingInvitations != "SendToNone" ||
		payload.Body.Items[0].RequiredAttendees != nil ||
		payload.Body.Items[0].OptionalAttendees != nil {
		t.Fatalf("unexpected request: %+v", payload.Body)
	}
}

func TestCalendarCreateWithoutTeamsMeetingOmitsOnlineMeetingFields(t *testing.T) {
	t.Parallel()

	input := testCalendarCreateInput()
	input.TeamsMeeting = false
	payload, err := buildCalendarCreateEnvelope(input)
	if err != nil {
		t.Fatalf("buildCalendarCreateEnvelope() error = %v", err)
	}
	item := payload.Body.Items[0]
	if item.IsOnlineMeeting || item.OnlineMeetingProvider != "" {
		t.Fatalf("unexpected online meeting request: %+v", payload.Body)
	}
}

func TestCalendarCreatePreservesOpaqueCalendarID(t *testing.T) {
	t.Parallel()

	input := testCalendarCreateInput()
	input.Calendar = application.CalendarFolder{
		Kind: application.CalendarFolderOpaque, ID: "AAMkCaseSensitiveCalendarID==",
	}
	payload, err := buildCalendarCreateEnvelope(input)
	if err != nil {
		t.Fatalf("buildCalendarCreateEnvelope() error = %v", err)
	}
	folder := payload.Body.SavedItemFolderID.BaseFolderID
	if folder.ID != input.Calendar.ID || folder.Type != "FolderId:#Exchange" {
		t.Fatalf("opaque calendar ID changed: %+v", folder)
	}
}

func TestCalendarCreateValidatesBeforeNetwork(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be called for invalid input")
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	input := testCalendarCreateInput()
	input.End = input.Start
	if _, err := client.CreateCalendarEvent(context.Background(), input); err == nil {
		t.Fatal("CreateCalendarEvent() unexpectedly accepted an empty event")
	}
}

func TestCalendarCreateAcceptsWorstCaseEscapedBoundedBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write(readFixture(t, "create_calendar_event_response.json"))
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	input := testCalendarCreateInput()
	input.Body = strings.Repeat("\x01", application.MaxCalendarBodyBytes)
	if _, err := client.CreateCalendarEvent(t.Context(), input); err != nil {
		t.Fatalf("CreateCalendarEvent() rejected a bounded escaped body: %v", err)
	}
}

func TestCalendarCreateMarksMalformedSuccessResponseUnknown(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"Body":{"ResponseMessages":{"Items":[{"ResponseClass":"Success","ResponseCode":"NoError","Items":[]}]}}}`))
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	_, err := client.CreateCalendarEvent(t.Context(), testCalendarCreateInput())
	if !errors.Is(err, application.ErrWriteOutcomeUnknown) {
		t.Fatalf("CreateCalendarEvent() error = %v, want ErrWriteOutcomeUnknown", err)
	}
}
