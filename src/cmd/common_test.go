package cmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidConfigOptions(t *testing.T) {
	options := []string{"confirm", "insecure", "uds_cache", "tmp_dir", "log_level", "architecture", "no_log_file", "no_progress", "oci_concurrency"}
	for _, option := range options {
		t.Run("test-"+option, func(t *testing.T) {
			res := isValidConfigOption(option)
			require.True(t, res)
		})
	}

	t.Run("test-invalid-option", func(t *testing.T) {
		res := isValidConfigOption("invalid")
		require.False(t, res)
	})
}
