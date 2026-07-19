package daemonapi

import (
	"strings"
	"testing"
	"time"
)

func TestTerminalLoginInputValidation(t *testing.T) {
	t.Parallel()

	sessionID := "tls1_" + strings.Repeat("a", 32)
	valid := []TerminalLoginInput{
		{Account: "work"},
		{Account: "work", SessionID: sessionID, Action: &TerminalLoginAction{Type: "refresh"}},
		{Account: "work", SessionID: sessionID, Action: &TerminalLoginAction{
			Type: "activate", ControlID: "control-1",
		}},
		{Account: "work", SessionID: sessionID, Action: &TerminalLoginAction{
			Type: "key", ControlID: "control-1", Key: "ø",
		}},
	}
	for _, input := range valid {
		if err := input.validate(); err != nil {
			t.Fatalf("validate(%+v) error = %v", input, err)
		}
	}

	invalid := []TerminalLoginInput{
		{},
		{Account: "work", Action: &TerminalLoginAction{Type: "refresh"}},
		{Account: "work", SessionID: "invalid", Action: &TerminalLoginAction{Type: "refresh"}},
		{Account: "work", SessionID: sessionID},
		{Account: "work", SessionID: sessionID, Action: &TerminalLoginAction{
			Type: "key", ControlID: "control-1", Key: "secret",
		}},
		{Account: "work", SessionID: sessionID, Action: &TerminalLoginAction{
			Type: "key", ControlID: "../../control-1", Key: "a",
		}},
	}
	for _, input := range invalid {
		if err := input.validate(); err == nil {
			t.Fatalf("validate(%+v) unexpectedly succeeded", input)
		}
	}
}

func TestValidateTerminalLoginResult(t *testing.T) {
	t.Parallel()

	sessionID := "tls1_" + strings.Repeat("b", 32)
	start := TerminalLoginInput{Account: "work"}
	pending := TerminalLoginResult{
		Account: "work", SessionID: sessionID, Status: "pending",
		View: &TerminalLoginView{Controls: []TerminalLoginControl{{
			ID: "control-1", Kind: "input", Name: "Email",
		}}},
	}
	if err := validateTerminalLoginResult(start, pending); err != nil {
		t.Fatalf("pending result error = %v", err)
	}
	authenticated := TerminalLoginResult{
		Account: "work", Status: "authenticated", CapturedAt: time.Unix(2, 0),
	}
	if err := validateTerminalLoginResult(start, authenticated); err != nil {
		t.Fatalf("authenticated result error = %v", err)
	}

	pending.View.Controls[0].ID = "invalid"
	if err := validateTerminalLoginResult(start, pending); err == nil {
		t.Fatal("invalid terminal control unexpectedly accepted")
	}
	if err := validateTerminalLoginResult(start, TerminalLoginResult{
		Account: "other", Status: "authenticated", CapturedAt: time.Unix(2, 0),
	}); err == nil {
		t.Fatal("different account unexpectedly accepted")
	}
}
