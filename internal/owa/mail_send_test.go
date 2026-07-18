package owa

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/nkiyohara/owa-bridge/internal/application"
)

func testSendInput() application.MailSendInput {
	return application.MailSendInput{
		Account: "work", To: []string{"alice@example.invalid"},
		Subject: "Synthetic send", Body: "Send body",
	}
}

func TestSendRequestMatchesGoldenFixture(t *testing.T) {
	t.Parallel()

	actual := marshalJSON(t, buildSendEnvelope(testSendInput()))
	want := readFixture(t, "send_mail_request.json")
	assertJSONEqual(t, actual, want)
}

func TestSendMailAcceptsEmptySuccessItemsAndNeverRetries(t *testing.T) {
	t.Parallel()

	fixture := readFixture(t, "send_mail_response.json")
	expectedRequest := readFixture(t, "send_mail_request.json")
	requestBodies := make(chan []byte, 1)
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
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
	sent, err := client.SendMail(context.Background(), testSendInput())
	if err != nil {
		t.Fatalf("SendMail() error = %v", err)
	}
	assertJSONEqual(t, <-requestBodies, expectedRequest)
	if sent.ID != "" || sent.ChangeKey != "" {
		t.Fatalf("unexpected sent copy: %+v", sent)
	}
	if calls.Load() != 1 {
		t.Fatalf("CreateItem send calls = %d, want exactly 1", calls.Load())
	}
}

func TestSendMailMarksMalformedSuccessResponseUnknown(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
          "Body":{"ResponseMessages":{"Items":[{
            "ResponseClass":"Success","ResponseCode":"NoError",
            "Items":[{"ItemId":{"Id":"one"}},{"ItemId":{"Id":"two"}}]
          }]}}
        }`))
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	_, err := client.SendMail(t.Context(), testSendInput())
	if !errors.Is(err, application.ErrWriteOutcomeUnknown) {
		t.Fatalf("SendMail() error = %v, want ErrWriteOutcomeUnknown", err)
	}
}

func TestSendMailAcceptsSuccessItemWithoutSentCopyIdentity(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"Body":{"ResponseMessages":{"Items":[{
				"ResponseClass":"Success","ResponseCode":"NoError",
				"Items":[{"ItemId":{"Id":"","ChangeKey":""}}]
			}]}}
		}`))
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	sent, err := client.SendMail(t.Context(), testSendInput())
	if err != nil {
		t.Fatalf("SendMail() error = %v", err)
	}
	if sent.ID != "" || sent.ChangeKey != "" {
		t.Fatalf("unexpected sent copy: %+v", sent)
	}
}

func TestSendMailRejectsChangeKeyWithoutSentCopyID(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{
			"Body":{"ResponseMessages":{"Items":[{
				"ResponseClass":"Success","ResponseCode":"NoError",
				"Items":[{"ItemId":{"Id":"","ChangeKey":"synthetic-change"}}]
			}]}}
		}`))
	}))
	defer server.Close()
	client := testClient(t, server, nil)
	_, err := client.SendMail(t.Context(), testSendInput())
	if !errors.Is(err, application.ErrWriteOutcomeUnknown) {
		t.Fatalf("SendMail() error = %v, want ErrWriteOutcomeUnknown", err)
	}
}
