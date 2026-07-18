package domain

import (
	"fmt"
	"strings"
	"unicode"
)

// AccountID is an opaque identifier for one configured mailbox account.
type AccountID string

// Validate ensures an account identifier is safe to use in policy boundaries.
func (account AccountID) Validate() error {
	return validateIdentifier("account", string(account), 128)
}

// Caller identifies the local adapter instance requesting an operation.
type Caller struct {
	Surface  string `json:"surface"`
	Instance string `json:"instance"`
}

// Validate ensures a caller can be safely bound to an approval token.
func (caller Caller) Validate() error {
	if err := validateIdentifier("caller surface", caller.Surface, 32); err != nil {
		return err
	}
	return validateIdentifier("caller instance", caller.Instance, 128)
}

func validateIdentifier(field, value string, maximum int) error {
	if value == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	if len(value) > maximum {
		return fmt.Errorf("%s exceeds %d bytes", field, maximum)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s must not start or end with whitespace", field)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}
