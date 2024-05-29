#!/usr/bin/env sh

# Create the json schema for the uds-bundle.yaml
go run main.go internal config-uds-schema > uds.schema.json
go run main.go internal config-tasks-schema > tasks.schema.json

# Adds pattern properties to all definitions to allow for yaml extensions
jq '.definitions |= map_values(. + {"patternProperties": {"^x-": {}}})' tasks.schema.json > temp_tasks.schema.json
mv temp_tasks.schema.json tasks.schema.json

# Modifies pattern properties to allow input parameters
jq '.definitions.Task.properties.inputs.patternProperties = {"^[_a-zA-Z][a-zA-Z0-9_-]*$": {"$schema": "http://json-schema.org/draft-04/schema#","$ref": "#/definitions/InputParameter"}}' tasks.schema.json > temp_tasks.schema.json
mv temp_tasks.schema.json tasks.schema.json
jq '.definitions.Action.properties.with.patternProperties = {"^[_a-zA-Z][a-zA-Z0-9_-]*$": {"additionalProperties": true}}' tasks.schema.json > temp_tasks.schema.json
mv temp_tasks.schema.json tasks.schema.json


# Create the json schema for zarf.yaml
go run main.go zarf internal gen-config-schema > zarf.schema.json

# Adds pattern properties to all definitions to allow for yaml extensions
jq '
  def addPatternProperties:
    . +
    if has("properties") then
      {"patternProperties": {"^x-": {}}}
    else
      {}
    end;

  walk(if type == "object" then addPatternProperties else . end)
' zarf.schema.json > temp_zarf.schema.json
mv temp_zarf.schema.json zarf.schema.json
