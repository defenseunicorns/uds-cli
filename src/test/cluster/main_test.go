// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

//go:build cluster_integration

package cluster

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/defenseunicorns/uds-cli/src/test/testutil"
)

var suite *testutil.ClusterSuite //nolint:gochecknoglobals

func TestMain(m *testing.M) {
	setupContext, cancelSetup := context.WithTimeout(context.Background(), 30*time.Minute)
	configuredSuite, err := testutil.SetupDevelopmentCluster(setupContext)
	cancelSetup()
	if err != nil {
		fmt.Fprintf(os.Stderr, "set up development cluster suite: %v\n", err)
		os.Exit(1)
	}
	suite = configuredSuite

	exitCode := m.Run()
	cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), 10*time.Minute)
	if err := suite.Close(cleanupContext); err != nil {
		fmt.Fprintf(os.Stderr, "clean up development cluster suite: %v\n", err)
		exitCode = 1
	}
	cancelCleanup()
	os.Exit(exitCode)
}
