// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package lang contains the language strings in english used by UDS
package lang

const (
	CmdBundleShort           = "Commands for creating, deploying, removing, pulling, and inspecting bundles"
	CmdBundleFlagConcurrency = "Number of concurrent layer operations to perform when interacting with a remote bundle."

	CmdBundleCreateShort                  = "Create a bundle from a given directory or the current directory"
	CmdBundleCreateFlagConfirm            = "Confirm bundle creation without prompting"
	CmdBundleCreateFlagOutput             = "Specify the output (an oci:// URL) for the created bundle"
	CmdBundleCreateFlagSigningKey         = "Path to private key file for signing bundles"
	CmdBundleCreateFlagSigningKeyPassword = "Password to the private key file used for signing bundles"
	CmdBundleCreateFlagSet                = "Specify bundle template variables to set on the command line (KEY=value)"

	CmdBundleDeployShort       = "Deploy a bundle from a local tarball or oci:// URL"
	CmdBundleDeployFlagSet     = "Specify deployment variables to set on the command line (KEY=value)"
	CmdBundleDeployFlagConfirm = "Confirms bundle deployment without prompting. ONLY use with bundles you trust. Skips prompts to review SBOM, configure variables, select optional components and review potential breaking changes."

	CmdBundleInspectShort   = "Display the metadata of a bundle"
	CmdBundleInspectFlagKey = "Path to a public key file that will be used to validate a signed bundle"

	CmdBundleRemoveShort       = "Remove a bundle that has been deployed already"
	CmdBundleRemoveFlagConfirm = "REQUIRED. Confirm the removal action to prevent accidental deletions"

	CmdBundlePullShort      = "Pull a bundle from a remote registry and save to the local file system"
	CmdBundlePullFlagOutput = "Specify the output directory for the pulled bundle"
	CmdBundlePullFlagKey    = "Path to a public key file that will be used to validate a signed bundle"
)
