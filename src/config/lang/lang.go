// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package lang contains the language strings in english used by UDS
package lang

const (
	// root UDS-CLI cmds
	RootCmdShort                       = "CLI for UDS Bundles"
	RootCmdFlagSkipLogFile             = "Disable log file creation"
	RootCmdFlagNoProgress              = "Disable fancy UI progress bars, spinners, logos, etc"
	RootCmdFlagCachePath               = "Specify the location of the UDS cache directory"
	RootCmdFlagTempDir                 = "Specify the temporary directory to use for intermediate files"
	RootCmdFlagInsecure                = "Allow access to insecure registries and disable other recommended security enforcements such as package checksum and signature validation. This flag should only be used if you have a specific reason and accept the reduced security posture."
	RootCmdFlagSkipSignatureValidation = "Skip signature validation for packages"
	RootCmdFlagNoColor                 = "Disable color output"
	RootCmdFlagLogLevel                = "Log level when running UDS-CLI. Valid options are: warn, info, debug, trace"
	RootCmdErrInvalidLogLevel          = "Invalid log level. Valid options are: warn, info, debug, trace."
	RootCmdFlagArch                    = "Architecture for UDS bundles and Zarf packages"

	// completion
	CompletionCmdShort          = "Generate the autocompletion script for the specified shell"
	CompletionCmdLong           = "Generate the autocompletion script for uds for the specified shell.\nSee each sub-command's help for details on how to use the generated script.\n"
	CompletionCmdShortBash      = "Generate the autocompletion script for bash"
	CompletionCmdShortZsh       = "Generate the autocompletion script for zsh"
	CompletionCmdShortFish      = "Generate the autocompletion script for fish"
	CompletionNoDescFlagName    = "no-descriptions"
	CompletionNoDescFlagDesc    = "disable completion descriptions"
	CompletionNoDescFlagDefault = false

	// logs
	CmdBundleLogsShort = "View most recent UDS CLI logs"

	// bundle
	CmdBundleFlagConcurrency = "Number of concurrent layer operations to perform when interacting with a remote bundle."

	// bundle create
	CmdBundleCreateShort                  = "Create a bundle from a given directory or the current directory"
	CmdBundleCreateFlagConfirm            = "Confirm bundle creation without prompting"
	CmdBundleCreateFlagOutput             = "Specify the output directory or oci:// URL for the created bundle"
	CmdBundleCreateFlagSigningKey         = "Path to private key file for signing bundles"
	CmdBundleCreateFlagSigningKeyPassword = "Password to the private key file used for signing bundles"
	CmdBundleCreateFlagVersion            = "Specify the version of the bundle"
	CmdBundleCreateFlagName               = "Specify the name of the bundle"

	// bundle deploy
	CmdBundleDeployShort        = "Deploy a bundle from a local tarball or oci:// URL"
	CmdBundleDeployFlagConfirm  = "Confirms bundle deployment without prompting. ONLY use with bundles you trust"
	CmdBundleDeployFlagPackages = "Specify which zarf packages you would like to deploy from the bundle. By default all zarf packages in the bundle are deployed."
	CmdBundleDeployFlagResume   = "Only deploys packages from the bundle which haven't already been deployed"
	CmdBundleDeployFlagSet      = "Specify deployment variables to set on the command line (KEY=value)"
	CmdBundleDeployFlagRetries  = "Specify the number of retries for package deployments (applies to all pkgs in a bundle)"
	CmdBundleDeployFlagRef      = "Specify which zarf package ref you want to deploy. By default the ref set in the bundle yaml is used."

	// bundle inspect
	CmdBundleInspectShort             = "Display the metadata of a bundle"
	CmdBundleInspectFlagKey           = "Path to a public key file that will be used to validate a signed bundle"
	CmdPackageInspectFlagSBOM         = "Create a tarball of SBOMs contained in the bundle"
	CmdPackageInspectFlagExtractSBOM  = "Create a folder of SBOMs contained in the bundle"
	CmdBundleInspectFlagFindImages    = "Derive images from a uds-bundle.yaml file and list them"
	CmdBundleInspectFlagListVariables = "List all configurable variables in a bundle (including zarf variables)"

	// bundle remove
	CmdBundleRemoveShort        = "Remove a bundle that has been deployed already"
	CmdBundleRemoveFlagConfirm  = "REQUIRED. Confirm the removal action to prevent accidental deletions"
	CmdBundleRemoveFlagPackages = "Specify which zarf packages you would like to remove from the bundle. By default all zarf packages in the bundle are removed."

	// bundle publish
	CmdPublishShort       = "Publish a bundle from the local file system to a remote registry"
	CmdPublishVersionFlag = "[Deprecated] Specify the version of the bundle to be published. This flag will be removed in a future version. Users should use the --version flag during creation to override the version defined in uds-bundle.yaml"

	// bundle pull
	CmdBundlePullShort      = "Pull a bundle from a remote registry and save to the local file system"
	CmdBundlePullFlagOutput = "Specify the output directory for the pulled bundle"
	CmdBundlePullFlagKey    = "Path to a public key file that will be used to validate a signed bundle"

	// bundle list
	CmdBundleListShort = "[alpha] List deployed bundles in the cluster"

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
	CmdInternalConfigSchemaErr   = "unable to generate the uds-bundle.yaml schema"

	// uds run
	CmdRunShort = "Run a task using maru-runner"

	// uds zarf
	CmdZarfShort = "Run a zarf command"

	// uds internal
	CmdInternalGenerateCliDocsShort   = "Generate CLI documentation for UDS-CLI"
	CmdInternalGenerateCliDocsSuccess = "Successfully generated CLI documentation"

	// uds dev
	CmdDevShort                = "[beta] Commands useful for developing bundles"
	CmdDevDeployShort          = "[beta] Creates and deploys a UDS bundle in dev mode"
	CmdBundleCreateFlagFlavor  = "[beta] Specify which zarf package flavor you want to use."
	CmdDevDeployLong           = "[beta] Creates and deploys a UDS bundle from a given directory or OCI repository in dev mode, setting package options like YOLO mode for faster iteration."
	CmdBundleCreateForceCreate = "[beta] For local bundles with local packages, specify whether to create a zarf package even if it already exists."

	// uds monitor
	CmdMonitorShort = "Monitor a UDS Cluster"
	CmdMonitorLong  = "Tools for monitoring a UDS Cluster and connecting to the UDS Engine for advanced troubleshooting"

	CmdMonitorNamespaceFlag = "Limit monitoring to a specific namespace"

	CmdMonitorPeprShort         = "Observe Pepr operations in a UDS Cluster"
	CmdMonitorPeprLong          = "View UDS Policy enforcements, UDS Operator events and additional Pepr operations"
	CmdPeprMonitorFollowFlag    = "Continuously stream Pepr logs"
	CmdPeprMonitorTimestampFlag = "Show timestamps in Pepr logs"
	CmdPeprMonitorSinceFlag     = "Only return logs newer than a relative duration like 5s, 2m, or 3h. Defaults to all logs."
	CmdPeprMonitorJSONFlag      = "Return the raw JSON output of the logs"
)
