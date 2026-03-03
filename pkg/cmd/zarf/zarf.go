// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

// Package zarf wraps the vendored Zarf CLI with two entry points:
//
//   - NewZarfCommand — user-facing "uds tools zarf" (strips 2 os.Args elements)
//   - NewInternalZarfCommand — hidden root-level "uds zarf" for internal callbacks
//     (strips 1 os.Args element)
//
// # Callback mechanism
//
// Zarf actions (wait, hooks) shell out via GetFinalExecutableCommand() (utils/io.go),
// which returns "<binary> <ActionsCommandZarfPrefix>". The prefix is set at build time:
//
//	-X 'github.com/zarf-dev/zarf/src/config.ActionsCommandZarfPrefix=zarf'
//
// The spawned process runs IsVendorCmd() (cmd/vendor.go) at package init to decide
// whether to register real or stub vendor commands (kubectl, helm, etc.):
//
//	if prefix != "" { args = args[1:] }   // strips exactly ONE element
//	return len(args) > 2 && args[1] == "tools" && slices.Contains(vendorCmds, args[2])
//
// If IsVendorCmd returns false, vendor commands become no-op stubs and actions
// silently fail with exit code 1.
//
// # Why prefix must be a single word
//
// IsVendorCmd strips exactly one element when a prefix is set, then expects "tools"
// at args[1]. A multi-word prefix (e.g. "tools zarf") shifts the args so that
// args[1] != "tools", causing IsVendorCmd to return false.
//
// Zarf config (config/config.go) exposes two callback routing variables:
//
//	ActionsCommandZarfPrefix  string   (settable via ldflags -X)
//	ActionsUseSystemZarf      bool     (NOT settable via ldflags; -X only works on strings)
//
// All options evaluated:
//
//	Approach              Callback                       Result
//	--------------------  -----------------------------  ----------------------------------------
//	prefix="zarf"         uds zarf tools kubectl ...     IsVendorCmd: strip 1 → args[1]="tools" ✓
//	  (chosen)              routes via hidden root cmd    No runtime init, no vendorCmds coupling.
//
//	prefix="tools zarf"   uds tools zarf tools kubectl   IsVendorCmd: strip 1 → args[1]="zarf" ✗
//	                                                     Stub commands, actions fail silently.
//
//	No prefix,            uds tools kubectl ...          IsVendorCmd works (no strip, args[1]=
//	useSystemZarf=false                                  "tools" ✓), but: bool not settable via
//	                                                     ldflags (requires runtime init()); cobra
//	                                                     needs every vendor cmd registered under
//	                                                     "tools" and kept in sync with Zarf.
//
//	No prefix,            zarf tools kubectl ...         Invokes a separate system-installed zarf
//	useSystemZarf=true                                   binary. Defeats vendoring.
package zarf

import (
	"context"
	"os"

	"github.com/spf13/cobra"
	zarfCLI "github.com/zarf-dev/zarf/src/cmd"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
)

// NewZarfCommand creates the user-facing zarf subcommand under "uds tools".
// This command passes all arguments directly to the vendored Zarf CLI.
//
// Limitation: Zarf's vendored tool commands (kubectl, helm, etc.) require the
// internal root-level "uds zarf" entry point because IsVendorCmd checks os.Args
// at package init time. Running "uds tools zarf tools kubectl ..." causes
// IsVendorCmd to see the wrong arg layout and register stubs instead of real
// vendor commands.  Use "uds zarf tools <tool>" for vendored tools.
//
// Usage:
//
//	uds tools zarf <zarf-commands>
//	uds tools zarf version
//	uds tools zarf package list
func NewZarfCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "zarf",
		Short: "Vendored Zarf CLI (non-vendored-tool commands)",
		Long: `Wraps the vendored Zarf CLI, providing access to Zarf commands.

NOTE: Zarf's vendored tool commands (kubectl, helm, etc.) do NOT work through
this path due to Zarf's init-time vendor-command detection. The command
"uds tools zarf tools kubectl ..." will silently fail because Zarf's
IsVendorCmd() registers stub commands instead of real ones when os.Args
has the extra "tools" nesting.

For vendored tool access, use the root-level entry point instead:

  uds zarf tools kubectl ...
  uds zarf tools helm ...

This path is suitable for all other Zarf commands:

  uds tools zarf version
  uds tools zarf package list`,
		Run: func(_ *cobra.Command, _ []string) {
			// Remove "uds" and "tools" from args, keeping "zarf" and everything after.
			// os.Args: ["/path/to/uds", "tools", "zarf", ...] → ["zarf", ...]
			os.Args = os.Args[2:]
			util.CheckErr(zarfCLI.Execute(context.Background()))
		},
		// Disable flag parsing so all flags are passed to Zarf
		DisableFlagParsing: true,
	}

	return cmd
}

// NewInternalZarfCommand creates the hidden root-level zarf command for internal
// Zarf callbacks. When Zarf runs actions (e.g., wait), it calls back to
// "<binary> zarf tools kubectl ..." which routes through this command.
//
// This must be at root level because Zarf's IsVendorCmd checks os.Args at package
// init time and expects the prefix ("zarf") to be at os.Args[1].
func NewInternalZarfCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "zarf",
		Short:  "Vendored Zarf CLI (internal)",
		Hidden: true,
		Run: func(_ *cobra.Command, _ []string) {
			// Remove "uds" from args, keeping "zarf" and everything after.
			// os.Args: ["/path/to/uds", "zarf", ...] → ["zarf", ...]
			os.Args = os.Args[1:]
			util.CheckErr(zarfCLI.Execute(context.Background()))
		},
		DisableFlagParsing: true,
	}

	return cmd
}
