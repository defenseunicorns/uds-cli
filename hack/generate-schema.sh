#!/usr/bin/env sh

# Create the json schema for the uds-bundle.yaml
go run main.go internal config-schema > uds.schema.json
