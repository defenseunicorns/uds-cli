// Copyright 2024 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"os"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

// We manually create the completion cmds because autogen'ing the docs includes Powershell completions,
// which we don't support. Cobra doesn't provide a neat mechanism for excluding just Powershell so if we want
// these completion cmds built into the CLI we have to create them manually
var completionCmd = &cobra.Command{
	Use:   "completion [command]",
	Short: lang.CompletionCmdShort,
	Long:  lang.CompletionCmdLong,
}

var noDesc = rootCmd.CompletionOptions.DisableDescriptions

var bashCompletionCmd = &cobra.Command{
	Use:                   "bash",
	Short:                 lang.CompletionCmdShortBash,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Long: `Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(uds completion bash)

To load completions for every new session, execute once:

#### Linux:

	uds completion bash > /etc/bash_completion.d/uds

#### macOS:

	uds completion bash > $(brew --prefix)/etc/bash_completion.d/uds

You will need to start a new shell for this setup to take effect.
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenBashCompletionV2(os.Stdout, !noDesc)
		if err != nil {
			return err
		}
		return nil
	},
}

var zshCompletionCmd = &cobra.Command{
	Use:   "zsh [flags]",
	Short: lang.CompletionCmdShortZsh,
	Args:  cobra.NoArgs,
	Long: `Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(uds completion zsh)

To load completions for every new session, execute once:

#### Linux:

	uds completion zsh > "${fpath[1]}/_uds"

#### macOS:

	uds completion zsh > $(brew --prefix)/share/zsh/site-functions/_uds

You will need to start a new shell for this setup to take effect.
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenZshCompletion(os.Stdout)
		if err != nil {
			return err
		}
		return nil
	},
}

var fishCompletionCmd = &cobra.Command{
	Use:   "fish [flags]",
	Short: lang.CompletionCmdShortFish,
	Args:  cobra.NoArgs,
	Long: `Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	uds completion fish | source

To load completions for every new session, execute once:

	uds completion fish > ~/.config/fish/completions/uds.fish

You will need to start a new shell for this setup to take effect.
`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenFishCompletion(os.Stdout, !noDesc)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	initViper()
	rootCmd.AddCommand(completionCmd)
	completionCmd.AddCommand(bashCompletionCmd)
	completionCmd.AddCommand(zshCompletionCmd)
	completionCmd.AddCommand(fishCompletionCmd)

	haveNoDescFlag := !rootCmd.CompletionOptions.DisableNoDescFlag && !rootCmd.CompletionOptions.DisableDescriptions
	if haveNoDescFlag {
		bashCompletionCmd.Flags().BoolVar(&noDesc, lang.CompletionNoDescFlagName, lang.CompletionNoDescFlagDefault, lang.CompletionNoDescFlagDesc)
		fishCompletionCmd.Flags().BoolVar(&noDesc, lang.CompletionNoDescFlagName, lang.CompletionNoDescFlagDefault, lang.CompletionNoDescFlagDesc)
		zshCompletionCmd.Flags().BoolVar(&noDesc, lang.CompletionNoDescFlagName, lang.CompletionNoDescFlagDefault, lang.CompletionNoDescFlagDesc)
	}
}
