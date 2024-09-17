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

Allow users to launch UDS Runtime locally from uds-cli. When running UDS Runtime locally, API token authentication is required. This is implemented programmatically and is transparent to the user.

### Implementation Details

To execute UDS Runtime from uds-cli, the appropriate runtime binary is pulled into the `src/cmd/bin` directory during the uds-cli build process. This is based on the specific build task being run (e.g., `build-cli-linux-arm`). The runtime binary is then packaged with the uds-cli binary.

To ensure the `uds ui` command works correctly, the uds-cli must be built using the build tasks specified in `tasks.yaml`. If not, the runtime binary might be missing, causing the `uds ui` command to fail.

### Alternatives Considered

1. Save runtime binaries directly in the uds-cli repo to avoid having to pull at build time. This approach was discarded because it adds extra size and complexity to the uds-cli repo
2. Vendor UDS Runtime in uds-cli. This approach was discarded because although the Runtime backend can be vendored, it required static assets from the frontend which would need to be embedded in the uds-cli binary, thus negating the benefits of vendoring.
