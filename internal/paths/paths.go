// Package paths resolves platform-native, account-safe local state paths.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nkiyohara/owa-bridge/internal/domain"
)

// StateDir returns the private application state directory for this platform.
func StateDir() (string, error) {
	if override := os.Getenv("OWA_STATE_DIR"); override != "" {
		if !filepath.IsAbs(override) {
			return "", errors.New("OWA_STATE_DIR must be absolute")
		}
		return filepath.Clean(override), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	configDirectory, configErr := os.UserConfigDir()
	cacheDirectory, cacheErr := os.UserCacheDir()
	return stateDir(runtime.GOOS, home, configDirectory, cacheDirectory, configErr, cacheErr, os.Getenv("XDG_STATE_HOME"))
}

// ProfileDir uses a digest so an account alias can never become a path.
func ProfileDir(account domain.AccountID) (string, error) {
	if err := account.Validate(); err != nil {
		return "", err
	}
	state, err := StateDir()
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(account))
	key := hex.EncodeToString(digest[:16])
	return filepath.Join(state, "profiles", key), nil
}

// AuditPath returns the content-free JSONL audit path.
func AuditPath() (string, error) {
	state, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(state, "audit", "events.jsonl"), nil
}

// UpdateCachePath returns the private, content-free release check cache.
func UpdateCachePath() (string, error) {
	state, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(state, "updates", "latest.json"), nil
}

// UpdateTrustCachePath returns the private cache for Sigstore TUF trust
// metadata used only by an explicit direct self-update.
func UpdateTrustCachePath() (string, error) {
	state, err := StateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(state, "updates", "sigstore"), nil
}

func stateDir(
	goos, home, configDirectory, cacheDirectory string,
	configErr, cacheErr error,
	xdgStateHome string,
) (string, error) {
	switch goos {
	case "linux":
		if xdgStateHome != "" {
			if !filepath.IsAbs(xdgStateHome) {
				return "", errors.New("XDG_STATE_HOME must be absolute")
			}
			return filepath.Join(xdgStateHome, "owa-bridge"), nil
		}
		if home == "" {
			return "", errors.New("user home is empty")
		}
		return filepath.Join(home, ".local", "state", "owa-bridge"), nil
	case "darwin":
		if configErr != nil {
			return "", fmt.Errorf("resolve application support directory: %w", configErr)
		}
		return filepath.Join(configDirectory, "owa-bridge"), nil
	case "windows":
		if cacheErr != nil {
			return "", fmt.Errorf("resolve local application data: %w", cacheErr)
		}
		return filepath.Join(cacheDirectory, "owa-bridge"), nil
	default:
		if cacheErr != nil {
			return "", fmt.Errorf("resolve state directory: %w", cacheErr)
		}
		return filepath.Join(cacheDirectory, "owa-bridge"), nil
	}
}
