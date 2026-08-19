// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package zarf

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/defenseunicorns/uds-cli/internal/filesystem"
	"github.com/defenseunicorns/uds-cli/pkg/bundle/spec"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/zarf-dev/zarf/src/pkg/packager/layout"
	"github.com/zarf-dev/zarf/src/pkg/signing"
)

// ValidatePackageSignatureVerification validates the create-time signature policy for a package.
func ValidatePackageSignatureVerification(packageName string, verification *spec.PackageSignatureVerification) error {
	if verification == nil {
		return fmt.Errorf("package %q: signature_verification block is required", packageName)
	}

	verify := true
	if verification.Verify != nil {
		verify = *verification.Verify
	}
	if verification.PublicKey != "" && strings.TrimSpace(verification.PublicKey) == "" {
		return fmt.Errorf("package %q: signature_verification.public_key must not be blank", packageName)
	}
	hasPublicKey := strings.TrimSpace(verification.PublicKey) != ""
	hasKeyless := verification.Keyless != nil

	if !verify {
		if hasPublicKey || hasKeyless {
			return fmt.Errorf("package %q: signature_verification.verify = false cannot be combined with public_key or keyless", packageName)
		}
		return nil
	}
	if hasPublicKey == hasKeyless {
		return fmt.Errorf("package %q: signature_verification must configure exactly one of public_key or keyless when verification is enabled", packageName)
	}
	if hasPublicKey {
		return nil
	}

	keyless := verification.Keyless
	hasIdentity := strings.TrimSpace(keyless.CertificateIdentity) != ""
	hasIdentityRegexp := strings.TrimSpace(keyless.CertificateIdentityRegexp) != ""
	if hasIdentity == hasIdentityRegexp {
		return fmt.Errorf("package %q: keyless verification requires exactly one of certificate_identity or certificate_identity_regexp", packageName)
	}
	if hasIdentityRegexp {
		if _, err := regexp.Compile(keyless.CertificateIdentityRegexp); err != nil {
			return fmt.Errorf("package %q: invalid certificate_identity_regexp: %w", packageName, err)
		}
	}
	hasIssuer := strings.TrimSpace(keyless.CertificateOIDCIssuer) != ""
	hasIssuerRegexp := strings.TrimSpace(keyless.CertificateOIDCIssuerRegexp) != ""
	if hasIssuer == hasIssuerRegexp {
		return fmt.Errorf("package %q: keyless verification requires exactly one of certificate_oidc_issuer or certificate_oidc_issuer_regexp", packageName)
	}
	if hasIssuerRegexp {
		if _, err := regexp.Compile(keyless.CertificateOIDCIssuerRegexp); err != nil {
			return fmt.Errorf("package %q: invalid certificate_oidc_issuer_regexp: %w", packageName, err)
		}
	}
	return nil
}

// PackageSignatureVerificationOptions translates a package signature policy into Zarf layout options.
func PackageSignatureVerificationOptions(pkg *spec.Package, verificationDir, tmpDir string) (layout.PackageLayoutOptions, error) {
	if pkg == nil {
		return layout.PackageLayoutOptions{}, fmt.Errorf("package is required")
	}
	if pkg.SignatureVerification == nil {
		return layout.PackageLayoutOptions{}, fmt.Errorf("package %q: signature_verification block is required", pkg.Name)
	}
	verification := pkg.SignatureVerification
	verify := true
	if verification.Verify != nil {
		verify = *verification.Verify
	}
	if !verify {
		return layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyNever}, nil
	}

	verifyOptions := signing.DefaultVerifyBlobOptions()
	verifyOptions.TempDir = tmpDir
	if strings.TrimSpace(verification.PublicKey) != "" {
		keyPath, err := writeVerificationMaterial(verificationDir, pkg.Name, "public-key.pem", verification.PublicKey)
		if err != nil {
			return layout.PackageLayoutOptions{}, err
		}
		verifyOptions.Key = keyPath
		return layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyAlways, VerifyBlobOptions: &verifyOptions}, nil
	}

	keyless := verification.Keyless
	verifyOptions.CertVerify.CertIdentity = keyless.CertificateIdentity
	verifyOptions.CertVerify.CertIdentityRegexp = keyless.CertificateIdentityRegexp
	verifyOptions.CertVerify.CertOidcIssuer = keyless.CertificateOIDCIssuer
	verifyOptions.CertVerify.CertOidcIssuerRegexp = keyless.CertificateOIDCIssuerRegexp
	verifyOptions.CommonVerifyOptions.IgnoreTlog = keyless.InsecureIgnoreTlog
	verifyOptions.CertVerify.IgnoreSCT = keyless.InsecureIgnoreSCT
	verifyOptions.CommonVerifyOptions.UseSignedTimestamps = keyless.UseSignedTimestamps
	if keyless.TrustedRoot != "" {
		trustedRootPath, err := writeVerificationMaterial(verificationDir, pkg.Name, "trusted-root.json", keyless.TrustedRoot)
		if err != nil {
			return layout.PackageLayoutOptions{}, err
		}
		verifyOptions.CommonVerifyOptions.TrustedRootPath = trustedRootPath
	}
	return layout.PackageLayoutOptions{VerificationStrategy: layout.VerifyAlways, VerifyBlobOptions: &verifyOptions}, nil
}

func writeVerificationMaterial(dir, packageName, filename, contents string) (string, error) {
	packageDir := filepath.Join(dir, sanitizeFileComponent(packageName))
	if err := os.MkdirAll(packageDir, filesystem.PrivateDirectoryMode); err != nil {
		return "", fmt.Errorf("creating verification material directory: %w", err)
	}
	path := filepath.Join(packageDir, filename)
	if err := os.WriteFile(path, []byte(contents), filesystem.PrivateFileMode); err != nil {
		return "", fmt.Errorf("writing verification material: %w", err)
	}
	return path, nil
}

func sanitizeFileComponent(value string) string {
	var result strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			result.WriteRune(r)
		} else {
			result.WriteRune('-')
		}
	}
	return strings.Trim(result.String(), "-")
}

func advisoryVerifyPackage(ctx context.Context, pkgLayout *layout.PackageLayout, pkg *spec.Package, tmpDir string, streams iostreams.IOStreams) {
	if err := ValidatePackageSignatureVerification(pkg.Name, pkg.SignatureVerification); err != nil {
		streams.Warn("package signature policy would fail bundle create; continuing development deploy", "name", pkg.Name, "error", err)
		return
	}
	verify := pkg.SignatureVerification.Verify
	if verify != nil && !*verify {
		streams.Warn("package signature verification is disabled; bundle create will include an unverified package", "name", pkg.Name)
		return
	}

	verificationDir, err := os.MkdirTemp(tmpDir, "uds-package-dev-verify-*")
	if err != nil {
		streams.Warn("unable to prepare temporary workspace for package signature verification; continuing development deploy without signature verification", "name", pkg.Name, "tmpDir", tmpDir, "error", err)
		return
	}
	defer func() {
		if err := os.RemoveAll(verificationDir); err != nil {
			streams.Warn("failed to remove development verification workspace", "path", verificationDir, "error", err)
		}
	}()

	loadOptions, err := PackageSignatureVerificationOptions(pkg, filepath.Join(verificationDir, "material"), tmpDir)
	if err != nil {
		streams.Warn("package signature verification would fail bundle create; continuing development deploy", "name", pkg.Name, "error", err)
		return
	}
	if pkg.SignatureVerification.Keyless != nil {
		keyless := pkg.SignatureVerification.Keyless
		if keyless.InsecureIgnoreTlog || keyless.InsecureIgnoreSCT {
			streams.Warn("keyless package signature verification has reduced protections", "name", pkg.Name, "ignoreTlog", keyless.InsecureIgnoreTlog, "ignoreSCT", keyless.InsecureIgnoreSCT)
		}
	}
	if err := pkgLayout.VerifyPackageSignature(ctx, *loadOptions.VerifyBlobOptions); err != nil {
		streams.Warn("package signature verification would fail bundle create; continuing development deploy", "name", pkg.Name, "error", err)
		return
	}
	streams.Info("package signature verified", "name", pkg.Name)
}
