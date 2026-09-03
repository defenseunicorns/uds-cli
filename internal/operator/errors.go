// Copyright 2026 Defense Unicorns
// SPDX-License-Identifier: AGPL-3.0-or-later OR LicenseRef-Defense-Unicorns-Commercial

package operator

import "errors"

var (
	ErrNoTargetsFound         = errors.New("no potential targets for monitoring found")
	ErrConnectCluster         = errors.New("connecting to cluster")
	ErrCreateKubernetesClient = errors.New("creating Kubernetes client")
	ErrListPeprPods           = errors.New("listing Pepr pods")
	ErrOpenLogStream          = errors.New("opening log stream")
	ErrStreamPodLogs          = errors.New("streaming logs for pod")
	ErrProcessPodLogs         = errors.New("processing logs for pod")
	ErrFlushMonitorOutput     = errors.New("flushing monitor output")
	ErrMutationPatchMissing   = errors.New("mutation patch is missing")
	ErrDecodeMutationPatch    = errors.New("decoding mutation patch")
	ErrParseMutationPatch     = errors.New("parsing mutation patch")
)
