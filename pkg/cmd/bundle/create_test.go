// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package bundle

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/defenseunicorns/uds-cli/pkg/cmd/util"
	"github.com/defenseunicorns/uds-cli/pkg/iostreams"
	"github.com/spf13/cobra"
)

type fakeFileInfo struct {
	name  string
	isDir bool
}

func (fi fakeFileInfo) Name() string       { return fi.name }
func (fi fakeFileInfo) Size() int64        { return 0 }
func (fi fakeFileInfo) Mode() os.FileMode  { return 0 }
func (fi fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (fi fakeFileInfo) IsDir() bool        { return fi.isDir }
func (fi fakeFileInfo) Sys() any           { return nil }

func TestCreateOptions_Validate(t *testing.T) {
	dir := filepath.Join("some", "dir")
	bundlePath := filepath.Join(dir, util.BundleFileName)
	emptyDir := filepath.Join("some", "empty")

	statMap := map[string]os.FileInfo{
		dir:        fakeFileInfo{name: filepath.Base(dir), isDir: true},
		bundlePath: fakeFileInfo{name: util.BundleFileName, isDir: false},
		emptyDir:   fakeFileInfo{name: filepath.Base(emptyDir), isDir: true},
	}
	statFn := func(path string) (os.FileInfo, error) {
		if fi, ok := statMap[path]; ok {
			return fi, nil
		}
		return nil, os.ErrNotExist
	}

	t.Run("empty string", func(t *testing.T) {
		o := &CreateOptions{BundleFile: "", StatFn: statFn}
		require.Error(t, o.Validate())
	})

	t.Run("path not found", func(t *testing.T) {
		o := &CreateOptions{BundleFile: filepath.Join("does", "not", "exist"), StatFn: statFn}
		require.Error(t, o.Validate())
	})

	t.Run("not a directory", func(t *testing.T) {
		o := &CreateOptions{BundleFile: bundlePath, StatFn: statFn}
		require.Error(t, o.Validate())
	})

	t.Run("directory missing bundle file", func(t *testing.T) {
		o := &CreateOptions{BundleFile: emptyDir, StatFn: statFn}
		require.Error(t, o.Validate())
	})

	t.Run("directory containing bundle file", func(t *testing.T) {
		o := &CreateOptions{BundleFile: dir, StatFn: statFn}
		require.NoError(t, o.Validate())
		require.Equal(t, bundlePath, o.BundleFile)
	})
}

func TestCreateOptions_Complete(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{"my-bundle-dir"})
	require.NoError(t, err)
	assert.Equal(t, "my-bundle-dir", o.BundleFile)
}

func TestCreateOptions_Complete_NoArgs(t *testing.T) {
	streams, _, _, _ := iostreams.NewTestIOStreams()
	o := &CreateOptions{IOStreams: streams}
	cmd := &cobra.Command{}

	err := o.Complete(cmd, []string{})
	require.NoError(t, err)
	assert.Equal(t, ".", o.BundleFile)
}
