# UDS UI

Author(s): @decleaver  
Date Created: Sept 9, 2024  
Status: IMPLEMENTED  
Ticket: https://github.com/defenseunicorns/uds-cli/issues/870  

### Problem Statement

The goal of the `uds ui` command is to allow uds-cli users to launch the UDS Runtime application from the command line.

### Proposal

Bundle the UDS Runtime binaries as a part of uds-cli allowing users to launch UDS Runtime from the command line.

### Scope and Requirements

Allow users to launch UDS Runtime from uds-cli

### Implementation Details

In order to be able to execute UDS Runtime from uds-cli, the UDS Runtime binaries have been added to the uds-cli repository under src/cmd/bin. Renovate has been set up to automatically create a PR to update the UDS Runtime binaries in the uds-cli repository whenever the latest UDS Runtime release is published. The renovate configuration is defined in `renovate.json` and the script checks and updates the  UDS Runtime binaries is `hack/update-uds-runtime-binaries.sh`

A downside to this implementation is that we are now including 4 additional binaries as a part of uds-cli which are each about 80MB in size.
