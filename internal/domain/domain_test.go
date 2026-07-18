package domain

import (
	"strings"
	"testing"
)

func TestEffectClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		effect  Effect
		valid   bool
		isWrite bool
	}{
		{EffectRead, true, false},
		{EffectSensitiveRead, true, false},
		{EffectReversibleWrite, true, true},
		{EffectExternalWrite, true, true},
		{EffectDestructiveWrite, true, true},
		{Effect("future_effect"), false, false},
	}

	for _, test := range tests {
		t.Run(string(test.effect), func(t *testing.T) {
			t.Parallel()
			if got := test.effect.Validate() == nil; got != test.valid {
				t.Fatalf("Validate() success = %v, want %v", got, test.valid)
			}
			if got := test.effect.IsWrite(); got != test.isWrite {
				t.Fatalf("IsWrite() = %v, want %v", got, test.isWrite)
			}
		})
	}
}

func TestNewOperationSnapshotsPayload(t *testing.T) {
	t.Parallel()

	payload := map[string]string{"subject": "Original"}
	operation, err := NewOperation("mail.send", EffectExternalWrite, "work", payload)
	if err != nil {
		t.Fatalf("NewOperation() error = %v", err)
	}
	payload["subject"] = "Modified"

	var decoded map[string]string
	if err := operation.DecodePayload(&decoded); err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	if decoded["subject"] != "Original" {
		t.Fatalf("payload was not snapshotted: %#v", decoded)
	}

	view := operation.View()
	if view.Name != "mail.send" || view.Effect != EffectExternalWrite || view.Account != "work" {
		t.Fatalf("unexpected view: %+v", view)
	}
	if len(view.Digest) != 2*32 {
		t.Fatalf("digest length = %d, want 64", len(view.Digest))
	}
}

func TestNewOperationRejectsInvalidBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		effect  Effect
		account AccountID
		payload any
	}{
		{"Mail.Send", EffectExternalWrite, "work", nil},
		{"mail.send", Effect("unknown"), "work", nil},
		{"mail.send", EffectExternalWrite, "", nil},
		{"mail.send", EffectExternalWrite, " work", nil},
		{"mail.send", EffectExternalWrite, "work\nother", nil},
		{"mail.send", EffectExternalWrite, "work", func() {}},
		{"mail.send", EffectExternalWrite, "work", strings.Repeat("x", maximumPayloadBytes+1)},
	}

	for _, test := range tests {
		if _, err := NewOperation(test.name, test.effect, test.account, test.payload); err == nil {
			t.Fatalf("NewOperation(%q, %q, %q) unexpectedly succeeded", test.name, test.effect, test.account)
		}
	}
}

func TestCallerValidation(t *testing.T) {
	t.Parallel()

	if err := (Caller{Surface: "mcp", Instance: "codex:session-1"}).Validate(); err != nil {
		t.Fatalf("valid caller rejected: %v", err)
	}
	for _, caller := range []Caller{
		{},
		{Surface: "mcp", Instance: ""},
		{Surface: "mcp\n", Instance: "session"},
		{Surface: "cli", Instance: " session"},
	} {
		if err := caller.Validate(); err == nil {
			t.Fatalf("invalid caller unexpectedly accepted: %+v", caller)
		}
	}
}
