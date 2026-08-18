// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
)

func TestDeployOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundleCmd := NewBundleCommand(streams)
	cmd, _, err := bundleCmd.Find([]string{"deploy"})
	require.NoError(t, err)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "local artifact", args: []string{"bundle.tar.zst"}, want: "bundle.tar.zst"},
		{name: "OCI artifact", args: []string{"oci://example.com/bundle:1.0.0"}, want: "oci://example.com/bundle:1.0.0"},
		{name: "no argument remains empty", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := NewDeployOptions(streams)
			require.NoError(t, o.Complete(cmd, tt.args))
			assert.Equal(t, tt.want, o.BundlePath)
			assert.NotNil(t, o.Printer)
		})
	}
}

func TestDeployOptions_Validate(t *testing.T) {
	tempDir := t.TempDir()
	artifact := filepath.Join(tempDir, "bundle.tar.zst")
	require.NoError(t, os.WriteFile(artifact, []byte("test"), 0o600))
	sourceDir := filepath.Join(tempDir, "source")
	require.NoError(t, os.Mkdir(sourceDir, 0o700))
	sourceFile := filepath.Join(sourceDir, bundleFileName)
	require.NoError(t, os.WriteFile(sourceFile, []byte("test"), 0o600))
	otherFile := filepath.Join(tempDir, "bundle.txt")
	require.NoError(t, os.WriteFile(otherFile, []byte("test"), 0o600))
	specialFile := filepath.Join(tempDir, "device.tar.zst")
	if err := os.Symlink(os.DevNull, specialFile); err != nil {
		t.Logf("special-file validation case unavailable: %v", err)
		specialFile = ""
	}

	tests := []struct {
		name    string
		ref     string
		wantErr string
	}{
		{name: "local artifact", ref: artifact},
		{name: "OCI reference", ref: "oci://ghcr.io/example/bundle:1.0.0"},
		{name: "OCI reference with artifact-like tag", ref: "oci://ghcr.io/example/bundle:release.tar.zst"},
		{name: "bare OCI reference", ref: "ghcr.io/example/bundle:1.0.0"},
		{name: "empty", wantErr: "bundle artifact is required"},
		{name: "missing artifact", ref: filepath.Join(tempDir, "missing.tar.zst"), wantErr: "bundle artifact not found"},
		{name: "source directory", ref: sourceDir, wantErr: "uds bundle dev deploy"},
		{name: "source file", ref: sourceFile, wantErr: "uds bundle dev deploy"},
		{name: "other file", ref: otherFile, wantErr: "local .tar.zst bundle artifact or OCI reference"},
		{name: "special file", ref: specialFile, wantErr: "regular file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.ref == "" && tt.name == "special file" {
				t.Skip("special-file validation case unavailable")
			}
			o := &DeployOptions{
				BundlePath:   tt.ref,
				Verification: VerifyOptions{SkipSignatureVerification: true},
			}
			err := o.Validate()
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}

func TestDeployOptions_Run_OCIPullUsesArtifactPathAndCleansWorkspace(t *testing.T) {
	streams, _, out, _ := iostreams.NewTestIOStreams()
	tmpDir := t.TempDir()
	puller := &recordingPuller{
		pullBundle: func(_ context.Context, ref, targetDir string, opts bundle.PullOptions) (*bundle.PullResult, error) {
			assert.Equal(t, "oci://example.com/test:1.0.0", ref)
			assert.Equal(t, tmpDir, opts.Config.Options.TmpDir)
			assert.Same(t, streams.Out(), opts.Streams.Out())
			artifact := filepath.Join(targetDir, "pulled.tar.zst")
			require.NoError(t, os.WriteFile(artifact, []byte("not an archive"), 0o600))
			return &bundle.PullResult{OCIReference: ref, OutputPath: artifact}, nil
		},
	}

	o := NewDeployOptions(streams)
	o.BundlePath = "oci://example.com/test:1.0.0"
	o.Verification.SkipSignatureVerification = true
	o.pullBundle = puller.PullBundle
	o.flags = CLIFlags{TmpDir: tmpDir, TmpDirChanged: true}

	err := o.Run(t.Context())
	require.ErrorContains(t, err, "extracting bundle artifact")
	assert.Equal(t, 1, puller.bundleCalls)
	assert.Empty(t, out.String(), "pull and failed deploy must not print structured output")
	entries, readErr := os.ReadDir(tmpDir)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "OCI deploy workspace should be removed after failure")
}

func TestDeployOptions_Run_LocalArtifactDoesNotPull(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	artifact := filepath.Join(t.TempDir(), "bundle.tar.zst")
	require.NoError(t, os.WriteFile(artifact, []byte("not an archive"), 0o600))
	puller := &recordingPuller{}

	o := NewDeployOptions(streams)
	o.BundlePath = artifact
	o.Verification.SkipSignatureVerification = true
	o.pullBundle = puller.PullBundle
	err := o.Run(t.Context())
	require.ErrorContains(t, err, "extracting bundle artifact")
	assert.Zero(t, puller.bundleCalls)
}

func TestDeployOptions_Run_OCIWorkspaceAndOutputLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		result     *bundle.DeployResult
		wantOutput string
	}{
		{
			name:       "successful deploy prints only deploy result",
			result:     &bundle.DeployResult{BundleName: "test-bundle", Packages: []bundle.DeployPackageResult{{Name: "one"}, {Name: "two"}}},
			wantOutput: "Bundle Name:  test-bundle",
		},
		{name: "cancelled deploy prints no result"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, out, _ := iostreams.NewTestIOStreams()
			tmpDir := t.TempDir()
			puller := &recordingPuller{
				pullBundle: func(_ context.Context, ref, targetDir string, _ bundle.PullOptions) (*bundle.PullResult, error) {
					artifact := filepath.Join(targetDir, "pulled.tar.zst")
					require.NoError(t, os.WriteFile(artifact, []byte("artifact"), 0o600))
					return &bundle.PullResult{OCIReference: ref, OutputPath: artifact}, nil
				},
			}
			runnerCalls := 0
			runner := func(_ context.Context, _ iostreams.IOStreams, _ *bundle.UDSBundleConfig, bundlePath string, _ []string, _, _ bool) (*bundle.DeployResult, error) {
				runnerCalls++
				assert.FileExists(t, bundlePath)
				return tt.result, nil
			}

			o := NewDeployOptions(streams)
			o.BundlePath = "oci://example.com/test:1.0.0"
			o.Verification.SkipSignatureVerification = true
			o.pullBundle = puller.PullBundle
			o.runDeploy = runner
			bundleCmd := NewBundleCommand(streams)
			deployCmd, _, err := bundleCmd.Find([]string{"deploy"})
			require.NoError(t, err)
			require.NoError(t, o.Complete(deployCmd, []string{o.BundlePath}))
			o.flags = CLIFlags{TmpDir: tmpDir, TmpDirChanged: true}

			require.NoError(t, o.Run(t.Context()))
			assert.Equal(t, 1, runnerCalls)
			assert.Equal(t, 1, puller.bundleCalls)
			entries, err := os.ReadDir(tmpDir)
			require.NoError(t, err)
			assert.Empty(t, entries)
			if tt.wantOutput == "" {
				assert.Empty(t, out.String())
			} else {
				assert.Contains(t, out.String(), tt.wantOutput)
				assert.NotContains(t, out.String(), "OCI Reference")
			}
		})
	}
}

func TestDeployOptions_Run_RejectsMalformedPullResults(t *testing.T) {
	tests := []struct {
		name   string
		result *bundle.PullResult
		want   string
	}{
		{name: "nil result", want: "puller returned no result"},
		{name: "empty output path", result: &bundle.PullResult{}, want: "empty output path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			o := NewDeployOptions(streams)
			o.BundlePath = "oci://example.com/test:1.0.0"
			o.Verification.SkipSignatureVerification = true
			o.pullBundle = (&recordingPuller{
				pullBundle: func(context.Context, string, string, bundle.PullOptions) (*bundle.PullResult, error) {
					return tt.result, nil
				},
			}).PullBundle
			err := o.Run(t.Context())
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestDeployOptions_Run_RejectsUnsafePulledArtifacts(t *testing.T) {
	externalArtifact := filepath.Join(t.TempDir(), "external.tar.zst")
	require.NoError(t, os.WriteFile(externalArtifact, []byte("artifact"), 0o600))

	tests := []struct {
		name       string
		outputPath func(string) string
		want       string
	}{
		{
			name: "path outside workspace",
			outputPath: func(string) string {
				return externalArtifact
			},
			want: "outside its workspace",
		},
		{
			name: "symlink outside workspace",
			outputPath: func(targetDir string) string {
				path := filepath.Join(targetDir, "pulled.tar.zst")
				require.NoError(t, os.Symlink(externalArtifact, path))
				return path
			},
			want: "outside its workspace",
		},
		{
			name: "source directory",
			outputPath: func(targetDir string) string {
				return targetDir
			},
			want: "outside its workspace",
		},
		{
			name: "non artifact file",
			outputPath: func(targetDir string) string {
				path := filepath.Join(targetDir, "bundle.uds.hcl")
				require.NoError(t, os.WriteFile(path, []byte("source"), 0o600))
				return path
			},
			want: "non-artifact output path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, _, _, _ := iostreams.NewTestIOStreams()
			runnerCalled := false
			o := NewDeployOptions(streams)
			o.BundlePath = "oci://example.com/test:1.0.0"
			o.Verification.SkipSignatureVerification = true
			o.pullBundle = (&recordingPuller{
				pullBundle: func(_ context.Context, ref, targetDir string, _ bundle.PullOptions) (*bundle.PullResult, error) {
					return &bundle.PullResult{OCIReference: ref, OutputPath: tt.outputPath(targetDir)}, nil
				},
			}).PullBundle
			o.runDeploy = func(context.Context, iostreams.IOStreams, *bundle.UDSBundleConfig, string, []string, bool, bool) (*bundle.DeployResult, error) {
				runnerCalled = true
				return nil, nil
			}

			err := o.Run(t.Context())
			require.ErrorContains(t, err, tt.want)
			assert.False(t, runnerCalled)
		})
	}
}

func TestDeployCommands_Flags(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	bundleCmd := NewBundleCommand(streams)

	for _, path := range [][]string{{"deploy"}, {"dev", "deploy"}} {
		cmd, _, err := bundleCmd.Find(path)
		require.NoError(t, err)
		require.NotNil(t, cmd.Flags().Lookup("packages"))
		assert.Equal(t, "p", cmd.Flags().Lookup("packages").Shorthand)
		require.NotNil(t, cmd.Flags().Lookup("force"))
		assert.Equal(t, "f", cmd.Flags().Lookup("force").Shorthand)
		if len(path) == 1 {
			require.NotNil(t, cmd.Flags().Lookup("public-key"))
			require.NotNil(t, cmd.Flags().Lookup("skip-signature-verification"))
		}
		for _, inherited := range []string{"architecture", "plain-http", "skip-tls-verify", "tmp-dir", "concurrency", "config", "output"} {
			require.NotNil(t, cmd.InheritedFlags().Lookup(inherited), "%s should inherit --%s", cmd.CommandPath(), inherited)
		}
	}
}

func TestPromptConfirmation(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantYes bool
	}{
		{name: "y confirms", input: "y\n", wantYes: true},
		{name: "Y confirms", input: "Y\n", wantYes: true},
		{name: "yes confirms", input: "yes\n", wantYes: true},
		{name: "YES confirms", input: "YES\n", wantYes: true},
		{name: "n declines", input: "n\n"},
		{name: "empty declines", input: "\n"},
		{name: "random text declines", input: "maybe\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			streams, in, _, _ := iostreams.NewTestIOStreams()
			in.WriteString(tt.input)
			confirmed, err := PromptConfirmation(streams, "Test this?")
			require.NoError(t, err)
			assert.Equal(t, tt.wantYes, confirmed)
		})
	}
}

func TestPromptConfirmation_BrokenReader(t *testing.T) {
	brokenErr := fmt.Errorf("read error: disk failure")
	streams := iostreams.New(&brokenReader{err: brokenErr}, &bytes.Buffer{}, &bytes.Buffer{})

	confirmed, err := PromptConfirmation(streams, "Test this?")
	require.ErrorContains(t, err, "reading confirmation")
	assert.False(t, confirmed)
}

type recordingPuller struct {
	bundleCalls int
	pullBundle  func(context.Context, string, string, bundle.PullOptions) (*bundle.PullResult, error)
}

func (p *recordingPuller) PullBundle(ctx context.Context, ref, targetDir string, opts bundle.PullOptions) (*bundle.PullResult, error) {
	p.bundleCalls++
	if p.pullBundle == nil {
		return nil, fmt.Errorf("unexpected PullBundle call")
	}
	return p.pullBundle(ctx, ref, targetDir, opts)
}

type brokenReader struct {
	err error
}

// Read always fails to exercise prompt input errors.
func (r *brokenReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

var _ io.Reader = (*brokenReader)(nil)
