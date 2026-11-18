// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	internalbundle "github.com/defenseunicorns/uds-cli/internal/bundle"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

func newConfigCommand(streams iostreams.IOStreams) *cobra.Command {
	cmd := &cobra.Command{Use: "config", Short: "Manage bundle configuration"}
	cmd.AddCommand(newConfigRenderCommand(streams))
	return cmd
}

func newConfigRenderCommand(streams iostreams.IOStreams) *cobra.Command {
	var output string
	var force bool
	cmd := &cobra.Command{
		Use:   "render <config.uds.hcl>",
		Short: "Render a bundle configuration",
		Long: `Evaluate a bundle configuration into static HCL without merging defaults.
Contents referenced by file() are embedded in the output, which can contain
plaintext secrets. Protect output files and avoid exposing stdout in logs.

Use --output-file to write to a file instead of stdout.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Parent().Parent().PersistentFlags().Changed("output") {
				return errors.New("--output is not supported by config render; use --output-file")
			}
			if cmd.Flags().Changed("output-file") && output == "" {
				return errors.New("--output-file must not be empty")
			}
			result, err := internalbundle.RenderBundleConfigFile(args[0])
			if err != nil {
				return fmt.Errorf("render config: %w", err)
			}
			if output == "" {
				n, err := streams.Out().Write(result)
				if err != nil {
					return fmt.Errorf("write rendered config: %w", err)
				}
				if n != len(result) {
					return fmt.Errorf("write rendered config: %w", io.ErrShortWrite)
				}
				return nil
			}
			return writeRenderedConfig(args[0], output, result, force)
		},
	}
	cmd.Flags().StringVar(&output, "output-file", "", "write rendered config to a file instead of stdout")
	cmd.Flags().BoolVar(&force, "force", false, "replace an existing output file")
	return cmd
}

func writeRenderedConfig(input, output string, content []byte, force bool) error {
	return writeRenderedConfigWithLink(input, output, content, force, os.Link)
}

func checkRenderedConfigOutput(input, output string, force bool) error {
	inputInfo, err := os.Stat(input)
	if err != nil {
		return fmt.Errorf("stat config input: %w", err)
	}
	outputInfo, err := os.Lstat(output)
	if err == nil {
		// Do not follow output symlinks or replace other special files.
		if outputInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output %q is a symbolic link", output)
		}
		if os.SameFile(inputInfo, outputInfo) {
			return errors.New("output must not be the input file")
		}
		if !outputInfo.Mode().IsRegular() {
			return fmt.Errorf("output %q is not a regular file", output)
		}
		if !force {
			return fmt.Errorf("output %q already exists; use --force to replace it", output)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat output: %w", err)
	}
	return nil
}

func writeRenderedConfigWithLink(input, output string, content []byte, force bool, link func(string, string) error) error {
	return writeRenderedConfigWithOperations(input, output, content, force, link, os.Remove)
}

func writeRenderedConfigWithOperations(input, output string, content []byte, force bool, link func(string, string) error, remove func(string) error) (resultErr error) {
	if err := checkRenderedConfigOutput(input, output, force); err != nil {
		return err
	}

	// Stage in the destination directory with owner-only permissions.
	tmp, err := os.CreateTemp(filepath.Dir(output), ".uds-config-render-*")
	if err != nil {
		return fmt.Errorf("create temporary output: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmpPath != "" {
			if err := remove(tmpPath); err != nil {
				resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary output: %w", err))
			}
		}
	}()
	if n, err := tmp.Write(content); err != nil || n != len(content) {
		_ = tmp.Close()
		if err == nil {
			err = io.ErrShortWrite
		}
		return fmt.Errorf("write temporary output: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary output: %w", err)
	}
	if force {
		// Recheck after staging to narrow the race. Rename cannot portably be
		// conditional on the destination's identity, so a concurrently modified
		// directory is outside this protection.
		if err := checkRenderedConfigOutput(input, output, true); err != nil {
			return err
		}
		err = os.Rename(tmp.Name(), output)
		if err == nil {
			tmpPath = ""
		}
	} else {
		err = link(tmp.Name(), output)
		if isUnsupportedLink(err) {
			// Some filesystems do not support hard links. Exclusive creation still
			// prevents replacement, but readers can see the file while it is written.
			// Remove the staged copy first so the fallback does not need double space.
			removeErr := remove(tmpPath)
			tmpPath = ""
			if removeErr != nil {
				return fmt.Errorf("remove temporary output: %w", removeErr)
			}
			return writeRenderedConfigExclusive(output, content)
		}
	}
	if err != nil {
		return fmt.Errorf("publish rendered config: %w", err)
	}
	return nil
}

func isUnsupportedLink(err error) bool {
	return errors.Is(err, errors.ErrUnsupported) ||
		errors.Is(err, syscall.ENOSYS) ||
		errors.Is(err, syscall.ENOTSUP) ||
		errors.Is(err, syscall.EOPNOTSUPP)
}

func writeRenderedConfigExclusive(output string, content []byte) error {
	return writeRenderedConfigExclusiveWithWrite(output, content, (*os.File).Write)
}

func writeRenderedConfigExclusiveWithWrite(output string, content []byte, write func(*os.File, []byte) (int, error)) error {
	f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	n, writeErr := write(f, content)
	if writeErr == nil && n != len(content) {
		writeErr = io.ErrShortWrite
	}
	closeErr := f.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		// Cleanup is best effort: removal can itself fail or the directory can
		// be changed by another process. Do not hide a cleanup failure.
		removeErr := os.Remove(output)
		return fmt.Errorf("write output: %w", errors.Join(err, removeErr))
	}
	return nil
}
