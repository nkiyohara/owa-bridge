// Package localipc provides authenticated, same-user local transports for the
// owa-bridge session owner. It never opens a TCP listener.
package localipc

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nkiyohara/owa-bridge/internal/paths"
)

const namespaceVersion = "owa-bridge-ipc-v1"

// Endpoint identifies one daemon namespace without exposing configuration
// content. Different config paths and state directories cannot collide.
type Endpoint struct {
	ID             string
	Address        string
	CredentialPath string
	lockPath       string
	runtimeDir     string
}

// Resolve derives the current platform endpoint for an absolute config path.
func Resolve(configPath string) (Endpoint, error) {
	if !filepath.IsAbs(configPath) {
		return Endpoint{}, errors.New("IPC config path must be absolute")
	}
	stateDirectory, err := paths.StateDir()
	if err != nil {
		return Endpoint{}, err
	}
	return deriveEndpoint(filepath.Clean(configPath), stateDirectory)
}

// ResolveInState derives an endpoint with an explicit absolute state directory.
// It is useful to isolate embedded runtimes and contract tests.
func ResolveInState(configPath, stateDirectory string) (Endpoint, error) {
	return deriveEndpoint(filepath.Clean(configPath), filepath.Clean(stateDirectory))
}

func deriveEndpoint(configPath, stateDirectory string) (Endpoint, error) {
	if !filepath.IsAbs(configPath) || !filepath.IsAbs(stateDirectory) {
		return Endpoint{}, errors.New("IPC inputs must be absolute")
	}
	digest := sha256.Sum256([]byte(
		namespaceVersion + "\x00" + filepath.Clean(configPath) + "\x00" + filepath.Clean(stateDirectory),
	))
	id := hex.EncodeToString(digest[:16])
	address, runtimeDirectory, lockPath, err := platformEndpoint(id)
	if err != nil {
		return Endpoint{}, fmt.Errorf("resolve local IPC endpoint: %w", err)
	}
	return Endpoint{
		ID:             id,
		Address:        address,
		CredentialPath: filepath.Join(stateDirectory, "ipc", id+".token"),
		lockPath:       lockPath,
		runtimeDir:     runtimeDirectory,
	}, nil
}

func currentTempDir() (string, error) {
	directory := os.TempDir()
	if !filepath.IsAbs(directory) {
		return "", errors.New("temporary directory must be absolute")
	}
	return filepath.Clean(directory), nil
}
