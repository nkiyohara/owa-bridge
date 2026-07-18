package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

const maximumPayloadBytes = 4 << 20

var operationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._][a-z0-9]+)*$`)

// Operation is an immutable, normalized request evaluated by the policy core.
type Operation struct {
	name    string
	effect  Effect
	account AccountID
	payload json.RawMessage
}

// OperationView is the non-secret metadata safe to return in a preview.
type OperationView struct {
	Name    string    `json:"name"`
	Effect  Effect    `json:"effect"`
	Account AccountID `json:"account"`
	Digest  string    `json:"digest"`
}

// NewOperation validates and snapshots a typed operation payload.
func NewOperation(name string, effect Effect, account AccountID, payload any) (Operation, error) {
	if !operationNamePattern.MatchString(name) || len(name) > 96 {
		return Operation{}, fmt.Errorf("invalid operation name %q", name)
	}
	if err := effect.Validate(); err != nil {
		return Operation{}, err
	}
	if err := account.Validate(); err != nil {
		return Operation{}, err
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return Operation{}, fmt.Errorf("encode operation payload: %w", err)
	}
	if len(encoded) > maximumPayloadBytes {
		return Operation{}, fmt.Errorf("operation payload exceeds %d bytes", maximumPayloadBytes)
	}

	return Operation{
		name:    name,
		effect:  effect,
		account: account,
		payload: encoded,
	}, nil
}

// Name returns the stable operation name.
func (operation Operation) Name() string {
	return operation.name
}

// Effect returns the operation's effect class.
func (operation Operation) Effect() Effect {
	return operation.effect
}

// Account returns the mailbox account boundary for the operation.
func (operation Operation) Account() AccountID {
	return operation.account
}

// DecodePayload decodes the immutable payload into a typed destination.
func (operation Operation) DecodePayload(destination any) error {
	if err := json.Unmarshal(operation.payload, destination); err != nil {
		return fmt.Errorf("decode operation payload: %w", err)
	}
	return nil
}

// View returns metadata and a digest without exposing operation content.
func (operation Operation) View() OperationView {
	digest := operation.digest()
	return OperationView{
		Name:    operation.name,
		Effect:  operation.effect,
		Account: operation.account,
		Digest:  hex.EncodeToString(digest[:]),
	}
}

// Validate rejects fabricated or incomplete operation metadata.
func (view OperationView) Validate() error {
	if !operationNamePattern.MatchString(view.Name) || len(view.Name) > 96 {
		return fmt.Errorf("invalid operation view name %q", view.Name)
	}
	if err := view.Effect.Validate(); err != nil {
		return err
	}
	if err := view.Account.Validate(); err != nil {
		return err
	}
	if len(view.Digest) != 2*sha256.Size {
		return errors.New("operation view digest must be a SHA-256 hex string")
	}
	if _, err := hex.DecodeString(view.Digest); err != nil {
		return fmt.Errorf("decode operation view digest: %w", err)
	}
	return nil
}

func (operation Operation) digest() [sha256.Size]byte {
	encoded, err := json.Marshal(struct {
		Version int             `json:"version"`
		Name    string          `json:"name"`
		Effect  Effect          `json:"effect"`
		Account AccountID       `json:"account"`
		Payload json.RawMessage `json:"payload"`
	}{
		Version: 1,
		Name:    operation.name,
		Effect:  operation.effect,
		Account: operation.account,
		Payload: operation.payload,
	})
	if err != nil {
		panic("domain operation contains invalid normalized JSON: " + err.Error())
	}
	return sha256.Sum256(encoded)
}
