package browser

import (
	"strings"
	"testing"
)

func TestNormalizeTerminalSnapshotBoundsAndSanitizesPage(t *testing.T) {
	t.Parallel()

	controls := []terminalControl{
		{ID: "control-1", Kind: "input", Name: "Email\x1b[31m", Sensitive: false},
		{ID: "control-2", Kind: "activate", Name: "Next"},
		{ID: "invalid", Kind: "activate", Name: "Ignored"},
	}
	view := normalizeTerminalSnapshot(terminalSnapshot{
		Origin:   "https://LOGIN.EXAMPLE:443/path?token=secret",
		Title:    " Sign\x00 in ",
		Text:     "Microsoft\n\n  Continue   to Outlook\x1b[2J",
		Controls: controls,
	})
	if view.Origin != "https://login.example:443" {
		t.Fatalf("Origin = %q", view.Origin)
	}
	if strings.ContainsAny(view.Title+view.Text+view.Controls[0].Name, "\x00\x1b") {
		t.Fatalf("view retained terminal controls: %+v", view)
	}
	if view.Text != "Microsoft\nContinue to Outlook [2J" {
		t.Fatalf("Text = %q", view.Text)
	}
	if len(view.Controls) != 2 || view.Controls[0].ID != "control-1" {
		t.Fatalf("Controls = %+v", view.Controls)
	}
}

func TestValidateTerminalAction(t *testing.T) {
	t.Parallel()

	valid := []TerminalAction{
		{Kind: TerminalActivate, ElementID: "control-1"},
		{Kind: TerminalFocus, ElementID: "control-64"},
		{Kind: TerminalKey, ElementID: "control-2", Key: "a"},
		{Kind: TerminalKey, ElementID: "control-2", Key: "Enter"},
	}
	for _, action := range valid {
		if err := validateTerminalAction(action); err != nil {
			t.Fatalf("validateTerminalAction(%+v) error = %v", action, err)
		}
	}

	invalid := []TerminalAction{
		{},
		{Kind: TerminalActivate, ElementID: "control-0"},
		{Kind: TerminalActivate, ElementID: "control-1", Key: "x"},
		{Kind: TerminalKey, ElementID: "control-1"},
		{Kind: TerminalKey, ElementID: "control-1", Key: "ab"},
		{Kind: TerminalKey, ElementID: "control-1", Key: "\x1b"},
	}
	for _, action := range invalid {
		if err := validateTerminalAction(action); err == nil {
			t.Fatalf("validateTerminalAction(%+v) unexpectedly succeeded", action)
		}
	}
}
