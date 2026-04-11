# common
Common helper utilities and wrappers for code reuse.
 
When we code projects, we constantly encounter a similar set of functionality and logic. This package's intent is to wrap those commonly recurring functionalities and access points into a reusable helper package, so that we don't have to keep maintaining separate code bases.

This package will continue to be updated with more reusable code as well.

# Compatibility and Stability

`github.com/aldelo/common` is a shared foundation library for 36+ downstream
repositories. Stability of observable behavior is treated as a **hard
invariant** across minor and patch releases, not a best-effort.

## What "observable contract" means here

For every exported helper in this package, these properties are part of the
contract and MUST NOT change between minor/patch versions:

- **Return type and nullability** — e.g., `(string, error)` stays `(string, error)`.
- **Empty-input behavior** — e.g., `Base64StdDecode("")` returns `("", nil)`,
  not an error.
- **Error-path behavior** — e.g., `LenTrim` on a string of only whitespace
  returns `0`, not `-1`.
- **Byte-vs-rune semantics** — `LenTrim`, `Left`, `Right`, `Mid`, and
  `NextFixedLength` operate on **bytes**, not runes. Downstream code
  (including `crypto/crypto.go` which derives AES-256 keys via
  `Left(passphrase, 32)`) depends on this.
- **Numeric format** — e.g., `Float64ToCurrencyString` uses `"%.2f"`.

These invariants are enforced by regression tests in
`helper-str-contract_test.go` and `helper-conv_test.go`. Any PR that changes
a pinned contract must either (a) update a version of the contract that
downstream repos don't yet depend on, or (b) be bundled into a coordinated
major-version (`v2.x`) release with all consumers migrated in one batch.

## How we version

This module follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html):

| Change | Version bump |
|---|---|
| Bug fix that restores a prior contract | PATCH (`v1.7.8 → v1.7.9`) |
| New additive helper (new function, new file, new type) | MINOR (`v1.7.x → v1.8.0`) |
| Any observable-contract change on an existing exported symbol | MAJOR (`v1.x → v2.0`) |

**Deprecation policy:** Helpers that are being phased out (e.g., `Md5`,
AES-CBC with NUL-byte padding, RSA envelope with unkeyed SHA-256 integrity
tag) are marked with `Deprecated:` in their godoc and remain callable for at
least one minor release cycle before removal. Safer replacements are added
as **additive siblings** in a minor release so consumers can migrate before
the old helpers are removed in the next major release.

**Monetary arithmetic:** `Float64ToCurrencyString` is a **display-only**
helper. Do not use `float64` for monetary computation — see the godoc on
the helper and the `TestFloat64ToCurrencyString_*` tests in
`helper-conv_test.go` for the rationale. Use `int64` cents or
`github.com/shopspring/decimal` for monetary arithmetic, then format via
`Float64ToCurrencyString` only at the final render step.

## Release notes

See [`CHANGELOG.md`](CHANGELOG.md) for the per-release list of fixes,
contract restorations, dependency bumps, and deferred breaking changes.

# Usage
- To use the common package:
  - in the project folder root: 
  - go mod init
  - go mod tidy
- For example, if project is "HelloWorld":
  - /HelloWorld/go mod init
  - /HelloWorld/go mod tidy

# types of helpers
- string helpers
- number helpers
- io helpers
- converter helpers
- db type helpers
- net helpers
- reflection helpers
- regex helpers
- time and date helpers
- uuid helpers
- crypto helpers (aes, gcm, rsa, sha, etc)
- csv parser helpers
- wrappers for aws related services
  - service discovery / cloud map wrapper (using aws sdk)
  - dynamodb / dax wrapper (using aws sdk)
  - kms wrapper (using aws sdk)
  - redis wrapper (using go-redis package)
  - s3 wrapper (using aws sdk)
  - ses wrapper (using aws sdk)
  - sqs wrapper (using aws sdk)
  - sns wrapper (using aws sdk)
  - gin web server 
  - xray wrapper (using aws sdk)
    - use xray.Init() to config
    - use xray.SetXRayServiceOn() to enable xray tracing
    - xray tracing is already coded into the following services:
      - kms, cloudmap, dynamodb, redis, s3, ses, sns, sqs, mysql, gin
- wrappers for relational database access
  - mysql wrapper (using sqlx package)
  - sqlite wrapper (using sqlx package)
  - sqlserver wrapper (using sqlx package)
- other wrappers
  - for running as systemd service
  - for logging and config
  - for circuit breaker and rate limit
  - etc.
  
# build and deploy automation
- Create a file such as 'build.sh' within project
- Edit file content as:
```
    #!/bin/zsh

    GOOS=linux GOARCH=amd64 go build
    scp -i ~/.ssh/YourKey.pem YourBinary hostUserName@hostIP:/home/hostUserName/targetFolder
```
- Descriptions:
  - YourKey.pem = the Linux Host SSH Certificate Key
  - YourBinary = the Binary Build by go build to Upload
  - hostUserName = the Linux Host Login Username
  - hostIP = the Linux Host IP Address
  - hostUserName = the Linux Host Login Username
  - targetFolder = the Linux Host Folder Where SCP Will Upload YourBinary To
 
