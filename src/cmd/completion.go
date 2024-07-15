// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package cmd contains the CLI commands for UDS.
package cmd

import (
	"fmt"
	"os"

	"github.com/defenseunicorns/uds-cli/src/config/lang"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [command]",
	Short: lang.CmdCompletionShort,
}

var bashCompletionCmd = &cobra.Command{
	Use:                   "bash",
	Short:                 lang.CmdCompletionShortBash,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Long: fmt.Sprintf(`%s

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
`, lang.CmdCompletionShortBash),
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenBashCompletionV2(os.Stdout, true)
		if err != nil {
			return err
		}
		return nil
	},
}

var zshCompletionCmd = &cobra.Command{
	Use:                   "zsh",
	Short:                 lang.CmdCompletionShortZsh,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Long: fmt.Sprintf(`%s

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
`, lang.CmdCompletionShortZsh),
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenZshCompletion(os.Stdout)
		if err != nil {
			return err
		}
		return nil
	},
}

var fishCompletionCmd = &cobra.Command{
	Use:                   "fish",
	Short:                 lang.CmdCompletionShortFish,
	DisableFlagsInUseLine: true,
	Args:                  cobra.NoArgs,
	Long: fmt.Sprintf(`%s

To load completions in your current shell session:

        uds completion fish | source

To load completions for every new session, execute once:

        uds completion fish > ~/.config/fish/completions/uds.fish

You will need to start a new shell for this setup to take effect.
`, lang.CmdCompletionShortFish),
	RunE: func(cmd *cobra.Command, _ []string) error {
		err := cmd.Root().GenFishCompletion(os.Stdout, true)
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
}
