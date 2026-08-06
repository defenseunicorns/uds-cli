# Package Fixtures

`zarf-package-real-simple-amd64-0.0.1.tar.zst` is intentionally committed so
unit tests use a stable Zarf package representation. Do not regenerate it as
part of normal test setup.

- Zarf version: `v0.82.0`
- SHA-256: `c99fca76edecf0f70424b4c677408ab0bd09aae40146ece5426a3c1c7856566d`

To update it deliberately, run from the repository root:

```bash
go run . zarf package create src/test/packages/no-cluster/real-simple \
  -o src/pkg/testdata -a amd64 --confirm
```

Update the recorded version and checksum when replacing the fixture.
