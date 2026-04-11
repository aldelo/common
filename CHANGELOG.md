# Changelog

All notable changes to `github.com/aldelo/common` are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Observable contracts of helpers in this library are preserved across minor/patch
versions per workspace rule #10 — downstream consumers (36+ repos) depend on
byte-indexed, empty-input, and error-path behavior staying stable between
releases. Breaking changes require a coordinated major-version bump.

---

## [Unreleased]

## [v1.7.9] — 2026-04-11

Release-readiness remediation pass. Primary themes: restoring observable
contracts silently changed in v1.7.0–v1.7.8, closing dependency CVEs flagged
by `govulncheck`, and tightening monetary-math documentation.

### Security

- **P0-7** — Pinned `toolchain go1.26.2` in `go.mod` to close 5 standard-library
  CVEs flagged by `govulncheck`: `GO-2026-4865` (`html/template` XSS context
  tracking), plus 4 additional stdlib advisories in `crypto/x509` and related
  packages reachable from `wrapper/hystrixgo` and `wrapper/gin`. The `go` directive
  remains at `1.24.1` so downstream module compatibility is unchanged — only the
  preferred auto-downloaded toolchain version moves forward.
- **P0-8** — Bumped `google.golang.org/grpc` v1.67.3 → v1.79.3 to resolve
  `CVE-2026-33186` (grpc-go TLS handshake handling). Transitive bumps: `x/crypto`,
  `x/net`, `x/sys`, `x/text`, `genproto/googleapis/rpc`.
- **P0-9** — Bumped `aws-sdk-go-v2/service/bedrockruntime` v1.50.1 → v1.50.4 to
  resolve `GHSA-xmrv-pmrh-hhx2`. Transitive bumps to `aws-sdk-go-v2` core,
  `eventstream` v1.7.6 → v1.7.8, `configsources`, and `endpoints/v2`.

### Fixed — restore v1.6.7 observable contracts (rule #10)

These fixes restore helper behavior that v1.7.0–v1.7.8 silently changed and
that 36+ downstream repos depend on. Each fix has a dedicated contract-pin
regression test in `helper-str-contract_test.go`.

- **P0-1** — `LenTrim` reverted to byte-count semantics (was rune-count since
  `af0d217`). Downstream `crypto/crypto.go` uses `Left(passphrase, 32)` to
  derive AES-256 keys that MUST be 32 bytes long.
- **P0-2** — `Left` / `Right` / `Mid` reverted to byte-indexed slicing.
- **P0-3** — `NextFixedLength` reverted to always-advance byte formula.
- **P1-1** — `Base64StdDecode("")` restored to return `("", nil)` (was erroring).
- **P1-2** — `Is*Only` family (`IsAlphanumericOnly`, `IsAlphabeticOnly`,
  `IsNumericIntOnly`, `IsAlphanumericAndPunctuationOnly`) restored to return
  `false` for empty input (was returning `true`).
- **P1-7** — `SliceStringToCSVString` restored to dumb-join contract (no quoting).
- **P1-8** — `Replace` restored to stdlib-passthrough contract.
- **P1-9** — `ParseKeyValue` restored to strict v1.6.7 validation.
- **P1-10** — `GenerateRandomChar`: silenced `go vet` `stringintconv` warning
  without changing return type.

### Fixed — error handling and safety

- **P1-4** — `wrapper/xray`, `wrapper/cloudmap`, `wrapper/dynamodb`: added
  nil-guards on `xray.seg.Seg` field accesses (1322 sites combined). Prevents
  nil-deref panics when X-Ray is disabled via `AWS_XRAY_SDK_DISABLED=TRUE`.
- **P1-5** — `wrapper/gin/ginhelper`: two-value type assertion on context secret
  lookup — panic → `false, nil` on missing/wrong type.
- **P3-1** — Non-constant format strings in `fmt.Errorf` / `fmt.Printf` replaced
  with constant format + `%s` / `%w` (48 sites, silences `go vet`).
- **P3-2** — Dropped impossible and tautological nil guards (8 sites in
  `wrapper/dynamodb/crud.go` and helpers).

### Changed — documentation only (observable contracts unchanged)

- **P0-12** — `Float64ToCurrencyString` godoc rewritten. The v1.7.8 docstring
  said "Use for monetary amounts (payment fields, transaction amounts, prices)",
  inviting callers to use `float64` for monetary arithmetic. IEEE-754 binary
  floats accumulate drift across add/sub/mul/div, and the same total reached
  by different paths can fail `==` comparison — the helper's `%.2f` rounding
  silently hides this. The new docstring marks the helper as **display-only**,
  explains the hazard, and recommends `int64` cents or
  `github.com/shopspring/decimal` for monetary arithmetic. Format contract
  (`"%.2f"`) is unchanged. New regression tests in `helper-conv_test.go` pin
  both the hazard (drift-hiding) and the contract (two-decimal format).

### Hygiene

- **S1031** — Dropped unnecessary nil-checks before `for-range` (6 sites).
- **S1040** — Dropped redundant `context.Context` type assertion (1 site).
- **SA6005** — Replaced `ToLower/ToUpper` equality comparisons with
  `strings.EqualFold` (18 sites).

### Testing

- **P1-3** — Added `helper-str-contract_test.go` pinning byte-indexed `util.Left`
  contract across 8 call sites in `crypto/crypto.go`.
- **P1-4** — Added `wrapper/xray` end-to-end panic-path test with
  `AWS_XRAY_SDK_DISABLED=TRUE`.
- Added `helper-conv_test.go` with float-drift hazard tests (P0-12, above).

### Deferred to v2.0.0 (coordinated breaking-change release)

Findings that would break observable contracts of `aldelo/common` are deferred
to a future major release so downstream repos can migrate in one coordinated
batch. See `_src/docs/repos/common/reviews/deep-review-2026-04-11-release-readiness.md`
in the workspace for the full list, including:

- **P0-4** — AES-CBC with NUL-byte (non-PKCS#7) padding — AES-GCM sibling will
  be added in v1.8.0 as an additive API; CBC helpers remain supported until
  v2.0.0.
- **P0-5** — RSA envelope (sign-then-encrypt with unkeyed SHA-256 integrity
  tag) — an HMAC-keyed sibling will be added in v1.8.0 as an additive API; the
  existing helpers remain supported until v2.0.0.
- **P0-6** — `Md5` helper — will be marked `Deprecated:` in godoc in v1.7.9,
  removed in v2.0.0. Callers must migrate to `Sha256` / `Sha512`.
- Unreachable `aws-sdk-go` v1 S3 Crypto SDK vulnerabilities `GO-2022-0646` (CBC
  padding oracle) and `GO-2022-0635` (in-band key negotiation) — deferred to
  the v1 → v2 AWS SDK migration epic tracked separately.

---

## [v1.7.8] — 2025-11 (pre-CHANGELOG)

- Fixed AWS region input validation for `AWS_us_east_1_nvirginia`. (#76)

## [v1.7.0] – [v1.7.7] — historical

Historical releases predate this CHANGELOG. See git log for commit-level history:
`git log v1.6.9..v1.7.7`.
