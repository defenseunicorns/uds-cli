## Bundle Anatomy
A UDS Bundle is an OCI artifact with the following form:

![UDS Bundle OCI Layout Diagram](.images/uds-bundle.png)

## Schema Validation

When working with UDS Bundle definitions it can be useful to setup your IDE to know about the schema that UDS Runner uses.

### VS Code

To do this in VS Code you can install the [YAML Extension](https://marketplace.visualstudio.com/items?itemName=redhat.vscode-yaml) and add the following to your `settings.json` (pinning `main` to your UDS CLI version if desired):

```json
    "yaml.schemas": {
        "https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/uds.schema.json": "uds-bundle.yaml"
    },
```

You can also add the following line to the top of a yaml file as well:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/defenseunicorns/uds-cli/main/uds.schema.json
```
