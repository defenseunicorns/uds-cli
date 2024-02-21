// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2023-Present The UDS Authors

// Package utils provides utility fns for UDS-CLI
package utils

import (
	"os"
	"testing"
)

func TestCalculateFileChecksum(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "testfile-")
	if err != nil {
		t.Fatalf("Unable to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	// Write some content to the file
	content := []byte("hello world")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Unable to write to temporary file: %v", err)
	}
	tmpFile.Close()

	// Expected checksum of "hello world"
	expectedChecksum := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	checksum, err := CalculateFileChecksum(tmpFile.Name())
	if err != nil {
		t.Errorf("CalculateFileChecksum returned an error: %v", err)
	}

	if checksum != expectedChecksum {
		t.Errorf("Expected checksum %s, got %s", expectedChecksum, checksum)
	}
}

func TestVerifyFileChecksum(t *testing.T) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "testfile-")
	if err != nil {
		t.Fatalf("Unable to create temporary file: %v", err)
	}
	defer os.Remove(tmpFile.Name()) // clean up

	// Write some content to the file
	content := []byte("hello world")
	if _, err := tmpFile.Write(content); err != nil {
		t.Fatalf("Unable to write to temporary file: %v", err)
	}
	tmpFile.Close()

	// Expected checksum of "hello world"
	expectedChecksum := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"

	valid, err := VerifyFileChecksum(tmpFile.Name(), expectedChecksum)
	if err != nil {
		t.Errorf("VerifyFileChecksum returned an error: %v", err)
	}

	if !valid {
		t.Errorf("Expected checksum verification to be true, got false")
	}

	// Test with incorrect checksum
	invalidChecksum := "invalidchecksum"
	valid, err = VerifyFileChecksum(tmpFile.Name(), invalidChecksum)
	if err != nil {
		t.Errorf("Didn't expect an error for incorrect checksum, got %v", err)
	}

	if valid {
		t.Errorf("Expected checksum verification to be false, got true")
	}
}
