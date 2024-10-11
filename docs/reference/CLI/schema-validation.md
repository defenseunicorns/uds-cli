---
title: Schema Validation
---
When working with UDS configuration files, it can be useful to setup your IDE to know about the various schemas that UDS uses.

The recommended method of validating schemas is by the use of `yaml-language-server` file headers:

For `uds-bundle.yaml`
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/uds.schema.json
```

For `zarf.yaml`
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/zarf.schema.json
```

For `tasks.yaml`
```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/tasks.schema.json
```

This method works with both VSCode and Goland (Jetbrains IDEs).

### Other IDE-specific Methods

### VS Code

To do this in VS Code you can install the [YAML Extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml) and add the following to your `settings.json` (pinning `main` to your UDS CLI version if desired):

```json
    "yaml.schemas": {
        "https://raw.githubusercontent.com/defenseunicorns/cli-commands/main/uds.schema.json": "uds-bundle.yaml"
    },
```

#### Goland (Jetbrains IDEs)

Use this method if you want to apply the schema to all YAML files in your project without modifying them. Open the IDE settings and navigate to `Languages & Frameworks` -> `Schemas and DTDs` -> `JSON Schema Mappings` and add a new schema using the "+" icon as shown below:

![Goland Schema Mapping](https://github.com/defenseunicorns/uds-cli/blob/main/docs/.images/goland-json-schema.png?raw=true)

Don't forget to set the file path pattern for the JSON schema to apply to.
