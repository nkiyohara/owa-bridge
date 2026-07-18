// Package domain contains protocol- and adapter-independent business types.
package domain

import "fmt"

// Effect describes the externally observable impact of an operation.
type Effect string

const (
	EffectRead             Effect = "read"
	EffectSensitiveRead    Effect = "sensitive_read"
	EffectReversibleWrite  Effect = "reversible_write"
	EffectExternalWrite    Effect = "external_write"
	EffectDestructiveWrite Effect = "destructive_write"
)

// Validate rejects effect classes unknown to this version of the application.
func (effect Effect) Validate() error {
	switch effect {
	case EffectRead,
		EffectSensitiveRead,
		EffectReversibleWrite,
		EffectExternalWrite,
		EffectDestructiveWrite:
		return nil
	default:
		return fmt.Errorf("unknown operation effect %q", effect)
	}
}

// IsWrite reports whether the effect can change mailbox state.
func (effect Effect) IsWrite() bool {
	switch effect {
	case EffectReversibleWrite, EffectExternalWrite, EffectDestructiveWrite:
		return true
	case EffectRead, EffectSensitiveRead:
		return false
	default:
		return false
	}
}
