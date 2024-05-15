---
title: Files
type: docs
weight: 4
---

The `files` key is used to copy local or remote files to the current working directory:

```yaml
tasks:
  - name: copy-local
    files:
      - source: /tmp/foo
        target: foo
  - name: copy-remote
    files:
      - source: https://cataas.com/cat
        target: cat.jpeg
```

`files` blocks can also use the following attributes:

- `executable`: boolean value indicating if the file is executable.
- `shasum`: SHA string to verify the integrity of the file.
- `symlinks`: list of strings referring to symlink the file to.
