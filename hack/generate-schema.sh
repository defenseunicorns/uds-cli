#!/usr/bin/env sh

# Create the json schema for the uds-bundle.yaml
go run main.go internal config-uds-schema > uds.schema.json
go run main.go internal config-tasks-schema > tasks.schema.json
