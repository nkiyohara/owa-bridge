package updatecheck

import (
	"context"
	"fmt"
	"os"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// VerifyProvenance validates a checksum manifest against the public Sigstore
// trust root, a transparency-log entry, an observer timestamp, an embedded SCT,
// and the exact release workflow identity.
func VerifyProvenance(
	ctx context.Context,
	manifestPath, bundlePath, workflowIdentity, trustCachePath string,
) error {
	signedBundle, err := bundle.LoadJSONFromPath(bundlePath)
	if err != nil {
		return fmt.Errorf("load Sigstore bundle: %w", err)
	}
	options := tuf.DefaultOptions().WithContext(ctx).WithCacheValidity(1)
	if trustCachePath != "" {
		if err := os.MkdirAll(trustCachePath, 0o700); err != nil {
			return fmt.Errorf("create Sigstore trust cache: %w", err)
		}
		if err := os.Chmod(trustCachePath, 0o700); err != nil { // #nosec G302 -- private directory requires owner execute.
			return fmt.Errorf("protect Sigstore trust cache: %w", err)
		}
		options.WithCachePath(trustCachePath)
	}
	trustedRoot, err := root.FetchTrustedRootWithOptions(options)
	if err != nil {
		return fmt.Errorf("refresh Sigstore trusted root: %w", err)
	}
	verifier, err := verify.NewVerifier(
		trustedRoot,
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
		verify.WithSignedCertificateTimestamps(1),
	)
	if err != nil {
		return fmt.Errorf("create Sigstore verifier: %w", err)
	}
	identity, err := verify.NewShortCertificateIdentity(
		sigstoreOIDCIssuer,
		"",
		workflowIdentity,
		"",
	)
	if err != nil {
		return fmt.Errorf("define release workflow identity: %w", err)
	}
	manifest, err := os.Open(manifestPath) // #nosec G304,G703 -- private bounded release asset selected by the installer.
	if err != nil {
		return fmt.Errorf("open signed checksum manifest: %w", err)
	}
	defer func() { _ = manifest.Close() }()
	if _, err := verifier.Verify(
		signedBundle,
		verify.NewPolicy(
			verify.WithArtifact(manifest),
			verify.WithCertificateIdentity(identity),
		),
	); err != nil {
		return fmt.Errorf("sigstore verification failed: %w", err)
	}
	return nil
}
