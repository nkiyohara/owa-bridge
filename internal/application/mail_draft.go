package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"unicode/utf8"

	"github.com/nkiyohara/owa-bridge/internal/approval"
	"github.com/nkiyohara/owa-bridge/internal/domain"
	"github.com/nkiyohara/owa-bridge/internal/policy"
)

const (
	MaxMailRecipients     = 500
	MaxMailSubjectBytes   = 998
	MaxMailDraftBodyBytes = 1 << 20
	mailDraftPreviewRunes = 500
)

// MailDraftInput creates a save-only plain-text draft. It never sends mail.
type MailDraftInput struct {
	Account domain.AccountID `json:"account"`
	To      []string         `json:"to,omitempty"`
	CC      []string         `json:"cc,omitempty"`
	BCC     []string         `json:"bcc,omitempty"`
	Subject string           `json:"subject,omitempty"`
	Body    string           `json:"body,omitempty"`
}

// MailDraft identifies the saved draft returned by OWA.
type MailDraft struct {
	ID        string `json:"id"`
	ChangeKey string `json:"changeKey,omitempty"`
}

// MailReview is safe to show before saving or sending an exact composition.
type MailReview struct {
	To          []string `json:"to,omitempty"`
	CC          []string `json:"cc,omitempty"`
	BCC         []string `json:"bcc,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	BodyPreview string   `json:"bodyPreview,omitempty"`
	BodyBytes   int      `json:"bodyBytes"`
	BodySHA256  string   `json:"bodySha256"`
}

// MailDraftAccess represents a completed save or an approval preview.
type MailDraftAccess struct {
	Status  string            `json:"status"`
	Draft   *MailDraft        `json:"draft,omitempty"`
	Review  MailReview        `json:"review"`
	Preview *approval.Preview `json:"preview,omitempty"`
}

// MailDraftWriter is the narrow OWA port for save-only drafts.
type MailDraftWriter interface {
	CreateMailDraft(context.Context, MailDraftInput) (MailDraft, error)
}

// CreateDraft prepares a save-only draft and executes it when policy allows.
func (service *MailService) CreateDraft(
	ctx context.Context,
	input MailDraftInput,
	caller domain.Caller,
) (MailDraftAccess, error) {
	if err := input.Validate(service.maxRecipients); err != nil {
		return MailDraftAccess{}, err
	}
	review := input.Review()
	operation, err := domain.NewOperation("mail.create_draft", domain.EffectReversibleWrite, input.Account, input)
	if err != nil {
		return MailDraftAccess{}, fmt.Errorf("create mail draft operation: %w", err)
	}
	prepared, err := service.guard.Prepare(ctx, operation, caller)
	if err != nil {
		return MailDraftAccess{}, err
	}
	switch prepared.Decision.Verdict {
	case policy.VerdictAllow:
		draft, err := service.executeDraft(ctx, input, caller, operation)
		if err != nil {
			return MailDraftAccess{}, err
		}
		return MailDraftAccess{Status: "completed", Draft: &draft, Review: review}, nil
	case policy.VerdictPreview:
		return MailDraftAccess{Status: "approval_required", Review: review, Preview: prepared.Preview}, nil
	case policy.VerdictDeny:
		return MailDraftAccess{}, errors.New("mail draft operation was denied")
	default:
		return MailDraftAccess{}, errors.New("mail draft operation received an unknown policy verdict")
	}
}

// CommitDraft consumes a caller-bound preview and saves its immutable draft.
func (service *MailService) CommitDraft(
	ctx context.Context,
	token string,
	caller domain.Caller,
) (MailDraftAccess, error) {
	operation, err := service.guard.CommitFor(
		ctx, token, caller, "mail.create_draft", domain.EffectReversibleWrite,
	)
	if err != nil {
		return MailDraftAccess{}, err
	}
	var input MailDraftInput
	if err := operation.DecodePayload(&input); err != nil {
		return MailDraftAccess{}, err
	}
	if err := input.Validate(service.maxRecipients); err != nil {
		return MailDraftAccess{}, err
	}
	draft, err := service.executeDraft(ctx, input, caller, operation)
	if err != nil {
		return MailDraftAccess{}, err
	}
	return MailDraftAccess{Status: "completed", Draft: &draft, Review: input.Review()}, nil
}

func (service *MailService) executeDraft(
	ctx context.Context,
	input MailDraftInput,
	caller domain.Caller,
	operation domain.Operation,
) (MailDraft, error) {
	draft, callErr := service.draftWriter.CreateMailDraft(ctx, input)
	outcome := AuditOutcomeSuccess
	reason := "completed"
	if callErr != nil {
		outcome = AuditOutcomeFailure
		reason = "transport_error"
		if errors.Is(callErr, ErrWriteOutcomeUnknown) {
			outcome = AuditOutcomeUnknown
			reason = "outcome_unknown"
		}
	}
	auditErr := service.guard.audit.Record(context.WithoutCancel(ctx), AuditEvent{
		Phase:     AuditPhaseExecuted,
		Outcome:   outcome,
		Reason:    reason,
		Caller:    caller,
		Operation: operation.View(),
	})
	if callErr != nil || auditErr != nil {
		return MailDraft{}, errors.Join(callErr, auditErr)
	}
	return draft, nil
}

// Validate enforces configured and absolute recipient/content bounds.
func (input MailDraftInput) Validate(maxRecipients int) error {
	if err := input.Account.Validate(); err != nil {
		return err
	}
	if maxRecipients < 1 || maxRecipients > MaxMailRecipients {
		return errors.New("invalid mail recipient limit")
	}
	count := len(input.To) + len(input.CC) + len(input.BCC)
	if count > maxRecipients {
		return fmt.Errorf("mail draft has %d recipients; maximum is %d", count, maxRecipients)
	}
	for _, recipient := range append(append(append([]string{}, input.To...), input.CC...), input.BCC...) {
		if err := validateMailAddress(recipient); err != nil {
			return err
		}
	}
	if !utf8.ValidString(input.Subject) || len(input.Subject) > MaxMailSubjectBytes ||
		strings.ContainsAny(input.Subject, "\r\n\x00") {
		return errors.New("mail subject is malformed or too large")
	}
	if !utf8.ValidString(input.Body) || len(input.Body) > MaxMailDraftBodyBytes ||
		strings.ContainsRune(input.Body, '\x00') {
		return errors.New("mail draft body is malformed or too large")
	}
	if count == 0 && input.Subject == "" && input.Body == "" {
		return errors.New("mail draft must contain a recipient, subject, or body")
	}
	return nil
}

func validateMailAddress(value string) error {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("mail recipient address is malformed")
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return fmt.Errorf("mail recipient %q must be a bare email address", value)
	}
	return nil
}

// Review summarizes content while binding the full body through SHA-256.
func (input MailDraftInput) Review() MailReview {
	preview := input.Body
	if utf8.RuneCountInString(preview) > mailDraftPreviewRunes {
		runes := []rune(preview)
		preview = string(runes[:mailDraftPreviewRunes-1]) + "…"
	}
	digest := sha256.Sum256([]byte(input.Body))
	return MailReview{
		To: append([]string(nil), input.To...), CC: append([]string(nil), input.CC...),
		BCC: append([]string(nil), input.BCC...), Subject: input.Subject,
		BodyPreview: preview, BodyBytes: len(input.Body), BodySHA256: hex.EncodeToString(digest[:]),
	}
}
