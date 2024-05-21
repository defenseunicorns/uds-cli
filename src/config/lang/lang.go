// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package lang contains the language strings in english used by UDS
package lang

const (
	// root UDS-CLI cmds
	RootCmdShort              = "CLI for UDS Bundles"
	RootCmdFlagSkipLogFile    = "Disable log file creation"
	RootCmdFlagNoProgress     = "Disable fancy UI progress bars, spinners, logos, etc"
	RootCmdFlagCachePath      = "Specify the location of the Zarf cache directory"
	RootCmdFlagTempDir        = "Specify the temporary directory to use for intermediate files"
	RootCmdFlagInsecure       = "Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture."
	RootCmdFlagLogLevel       = "Log level when running UDS-CLI. Valid options are: warn, info, debug, trace"
	RootCmdErrInvalidLogLevel = "Invalid log level. Valid options are: warn, info, debug, trace."
	RootCmdFlagArch           = "Architecture for UDS bundles and Zarf packages"

	// logs
	CmdBundleLogsShort = "View most recent UDS CLI logs"

	// bundle
	CmdBundleShort           = "Commands for creating, deploying, removing, pulling, and inspecting bundles"
	CmdBundleFlagConcurrency = "Number of concurrent layer operations to perform when interacting with a remote bundle."

	// bundle create
	CmdBundleCreateShort = "Create a bundle from a given directory or the current directory"
	//CmdBundleCreateFlagConfirm            = "Confirm bundle creation without prompting"
	CmdBundleCreateFlagOutput             = "Specify the output (an oci:// URL) for the created bundle"
	CmdBundleCreateFlagSigningKey         = "Path to private key file for signing bundles"
	CmdBundleCreateFlagSigningKeyPassword = "Password to the private key file used for signing bundles"

	// bundle deploy
	CmdBundleDeployShort        = "Deploy a bundle from a local tarball or oci:// URL"
	CmdBundleDeployFlagConfirm  = "Confirms bundle deployment without prompting. ONLY use with bundles you trust. Skips prompts to review SBOM, configure variables, select optional components and review potential breaking changes."
	CmdBundleDeployFlagPackages = "Specify which zarf packages you would like to deploy from the bundle. By default all zarf packages in the bundle are deployed."
	CmdBundleDeployFlagResume   = "Only deploys packages from the bundle which haven't already been deployed"
	CmdBundleDeployFlagSet      = "Specify deployment variables to set on the command line (KEY=value)"
	CmdBundleDeployFlagRetries  = "Specify the number of retries for package deployments (applies to all pkgs in a bundle)"

	// bundle inspect
	CmdBundleInspectShort            = "Display the metadata of a bundle"
	CmdBundleInspectFlagKey          = "Path to a public key file that will be used to validate a signed bundle"
	CmdPackageInspectFlagSBOM        = "Create a tarball of SBOMs contained in the bundle"
	CmdPackageInspectFlagExtractSBOM = "Create a folder of SBOMs contained in the bundle"

	// bundle remove
	CmdBundleRemoveShort        = "Remove a bundle that has been deployed already"
	CmdBundleRemoveFlagConfirm  = "REQUIRED. Confirm the removal action to prevent accidental deletions"
	CmdBundleRemoveFlagPackages = "Specify which zarf packages you would like to remove from the bundle. By default all zarf packages in the bundle are removed."

	// bundle publish
	CmdPublishShort = "Publish a bundle from the local file system to a remote registry"

	// bundle pull
	CmdBundlePullShort      = "Pull a bundle from a remote registry and save to the local file system"
	CmdBundlePullFlagOutput = "Specify the output directory for the pulled bundle"
	CmdBundlePullFlagKey    = "Path to a public key file that will be used to validate a signed bundle"

	// cmd viper setup
	CmdViperErrLoadingConfigFile = "failed to load config file: %s"
	CmdViperInfoUsingConfigFile  = "Using config file %s"

	// bundle picker during deployment
	CmdPackageChoose    = "Choose or type the bundle file"
	CmdPackageChooseErr = "Bundle path selection canceled: %s"

	// uds-cli version
	CmdVersionShort = "Shows the version of the running UDS-CLI binary"
	CmdVersionLong  = "Displays the version of the UDS-CLI release that the current binary was built from."

	// uds-cli internal
	CmdInternalShort             = "Internal cmds used by UDS-CLI"
	CmdInternalConfigSchemaShort = "Generates a JSON schema for the uds-bundle.yaml configuration"
	CmdInternalConfigSchemaErr   = "Unable to generate the uds-bundle.yaml schema"

	// uds run
	CmdRunShort = "Run a task using maru-runner"

	// uds zarf
	CmdZarfShort = "Run a zarf command"

	// uds dev
	CmdDevShort       = "Commands useful for developing bundles"
	CmdDevDeployShort = "[beta] Creates and deploys a UDS bundle from a given directory in dev mode"
	CmdDevDeployLong  = "[beta] Creates and deploys a UDS bundle from a given directory in dev mode, setting package options like YOLO mode for faster iteration."
)
