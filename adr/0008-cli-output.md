# 8. UDS CLI Output

Date: 26 June 2024

## Status

Accepted

## Context

Today, UDS CLI outputs virtually all CLI output to `stderr`. However, as the team begins to implement CLI commands that are designed to be consumed by other tools or automation, we need to norm on where logs are sent

## Alternatives

1. **`stderr`**: Continue to output all logs to `stderr`. This is the current behavior and is the simplest to implement.
2. **`stdout`**: Output all logs to `stdout`. This is the most common behavior for CLI tools and is the most likely to be consumed by other tools or automation.
3. **`stdout` and `stderr`**: Strategically output logs to both `stdout` and `stderr`. This is the most flexible option, but is slightly more code to implement.

## Decision

We will strategically output CLI messages to both `stdout` and `stderr`. Typical log output such as progress messages, spinners, etc, will be sent to `stderr`, and output that can be acted upon or is designed to be consumed by other tools will be sent to `stdout`.

## Consequences

The team needs to identify and refactor log output that is meant to be consumed by other tools and ensure it is sent to `stdout`. We will also need to ensure future CLI output adheres to this standard.
```
