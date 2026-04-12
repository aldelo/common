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
  `true` for empty input (was returning `false`).
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

### Fixed — DynamoDB data integrity (release-readiness remediation)

Findings from `_src/docs/repos/common/reviews/deep-review-2026-04-11-release-readiness.md`,
section "P0 — ship-gate defects". These are widening fixes (accept more input,
retry more residuals) so no downstream behavior is narrowed and rule #10 holds.

- **DDB-P0-1** — `wrapper/dynamodb`: raised `TransactWriteItems` /
  `TransactGetItems` item limit 25 → 100 to match the AWS service limit (moved
  from 25 to 100 on 2022-09-27). Introduced exported package constants
  `MaxTransactItems = 100` and `MaxBatchWriteItems = 25` so the two limits are
  named, impossible to conflate, and contract-pinned by tests. `dynamodb.go`
  validators and `crud.go` chunkers for `TransactionWriteItemsWithRetry`,
  `TransactionGetItemsWithRetry`, `TransactionSet`, `Update` (transaction
  branch), and `Delete` (chunker) all now reference `MaxTransactItems`.
  `BatchWriteItemsWithRetry` / `BatchDeleteItemsWithRetry` intentionally stay
  at 25 because `BatchWriteItem`'s AWS limit is unchanged — the new
  `TestTransactAndBatchLimits_AreDistinct` contract-pin test guards against a
  future naive "25 → 100 everywhere" refactor. Widening-only: any
  previously-accepted call with ≤ 25 items remains accepted.
- **DDB-P0-2** — `wrapper/dynamodb/BatchWriteItemsWithRetry` now actually
  retries `BatchWriteItemOutput.UnprocessedItems`. Previous behavior: a
  successful AWS response with a non-empty `UnprocessedItems` map (items
  deferred by DynamoDB due to throttling / provisioned-throughput-exceeded)
  was returned to the caller unretried, silently dropping those items. New
  behavior: after the initial call succeeds, the residual items are retried
  via `do_BatchWriteItem` in a local exponential-backoff loop
  (100 ms → 200 ms → 400 ms → 800 ms → 1.6 s, capped at 2 s) up to the
  caller-supplied `maxRetries` budget. A hard error from the retry path
  returns the initial call's successCount plus typed residual items rather
  than clobbering the successful initial signal. Two new pure helper
  functions — `unprocessedItemsToAwsRequestItems` and
  `awsRequestItemsToUnprocessedItems` — handle the typed ↔ AWS-SDK-shape
  conversion and are unit-tested without any AWS connection. Minor
  observability loss: the retry path calls the raw SDK bypassing
  `batchWriteItemsWithTrace`, so xray segments cover only the initial call;
  documented in the helper's godoc. AWS reference:
  [BatchWriteItem error-handling guidance](https://docs.aws.amazon.com/amazondynamodb/latest/APIReference/API_BatchWriteItem.html#API_BatchWriteItem_Errors).

### Added — additive siblings (rule #10 compliant)

- **P0-5** — `crypto.RsaAesPublicKeyEncryptAndSignHmac` /
  `crypto.RsaAesPrivateKeyDecryptAndVerifyHmac`, the additive v1.7.9
  replacement for the deprecated `RsaAesPublicKeyEncryptAndSign` /
  `RsaAesPrivateKeyDecryptAndVerify` pair. The V2 pair keeps the same
  security goals (hybrid RSA-OAEP-SHA256 key wrap + AES-GCM body
  encryption + sender signature verified by recipient) but fixes two
  hazards in the V1 envelope format:
  (1) V1 appended an unkeyed `Sha256(recipientPublicKey, "TPK@2019")`
  hash whose presence alongside the ciphertext invited readers to
  mistake it for an integrity tag — it is actually only a
  recipient-key lookup identifier (see
  `RsaAesParseTPKHashFromEncryptedPayload`) and any attacker can
  recompute it. V2 replaces this with a real HMAC-SHA256 tag keyed by
  the per-envelope 32-byte AES key, computed over
  `rsaEncryptedAesKey || aesGcmEncryptedBody`, so tampering of either
  component is detected BEFORE the GCM decrypter is invoked
  (fail-fast).
  (2) V1's AES-GCM inner plaintext was the 0x0B (VT) delimited triple
  `plainText<VT>senderPublicKey<VT>signature`, so any plaintext
  containing a VT byte silently broke the `strings.Split` parser and
  rendered the envelope unrecoverable. V2 replaces the VT delimiter
  with length-prefixed fields (8-char uppercase-hex uint32 length
  followed by raw bytes per field), so arbitrary byte sequences —
  including every control byte 0x00..0x1F, leading/trailing NUL, and
  embedded VT — round-trip exactly.
  The V2 wire format starts with `<STX>V2<rsaEncryptedAesKey 512
  hex><aesGcmBody><hmacTag 64 hex><ETX>`. The literal "V2" marker is
  not valid hex, so a V1 decrypter fed a V2 payload fails its "aes
  key must be 512 hex chars" check and a V2 decrypter fed a V1
  payload fails its "first two bytes must be V2" check — V1 and V2
  payloads are unambiguously distinguishable and never cross-decode.
  The V1 pair remains fully callable through the entire v1.x series
  per workspace rule #10 (observable-contract stability); removal is
  scheduled for v2.0.0 once all 36+ consumers have migrated to the
  V2 sibling. New regression tests in `crypto_test.go` cover:
  (A) V2 round-trip with ASCII plaintext,
  (B) V2 round-trip with a plaintext containing every control byte
  0x00..0x1F plus leading/trailing NUL and VT — the category of
  inputs V1 corrupts,
  (C) V2 round-trip with a ~4 KiB plaintext (length-prefix decoder
  past trivial sizes),
  (D) V1/V2 cross-version isolation (each decrypter rejects the
  other's envelope format),
  (E) HMAC tamper detection (flipping a byte inside the GCM body
  triggers an integrity-check failure, not a successful decrypt of
  corrupted data),
  (F) signature verification still runs on the V2 path (an envelope
  signed by sender A but presented as signed by sender B fails).

### Deprecated

- **P0-4** — `crypto.AesCbcEncrypt` / `crypto.AesCbcDecrypt` marked
  `Deprecated:` in godoc. The CBC helpers pad plaintext with 0x00 bytes on
  encrypt and strip ALL trailing 0x00 bytes on decrypt
  (`strings.ReplaceAll(..., NUL, "")`), which silently corrupts any plaintext
  whose natural last byte is 0x00 — the trailing NUL is indistinguishable
  from padding and is removed. CBC also lacks authentication, so a tampered
  ciphertext decrypts without error. The godoc directs callers to the
  already-existing `crypto.AesGcmEncrypt` / `crypto.AesGcmDecrypt` AEAD pair
  which preserves arbitrary byte sequences exactly and detects tampering
  via an authentication tag (NIST SP 800-38D). The CBC functions remain
  fully callable through the entire v1.x series per workspace rule #10
  (observable-contract stability); removal is scheduled for v2.0.0. New
  regression tests in `crypto_test.go`
  (`TestAesCbc_DeprecationObservableContracts`) pin three contracts:
  (1) block-aligned plaintext round-trips cleanly, (2) non-block-aligned
  plaintext without trailing NULs round-trips cleanly, (3) plaintext with
  a trailing 0x00 byte is CORRUPTED on the round-trip — this BUG is
  pinned intentionally so a future refactor cannot silently change the
  observable behavior without forcing a v2.0.0 release. The same test
  also demonstrates that `AesGcmEncrypt`/`AesGcmDecrypt` preserve the
  trailing NUL exactly, proving the migration target is correct.

- **P0-5** — `crypto.RsaAesPublicKeyEncryptAndSign` /
  `crypto.RsaAesPrivateKeyDecryptAndVerify` marked `Deprecated:` in
  godoc. The V1 envelope format has two hazards documented in full in
  the "Added — additive siblings" section above: an unkeyed hash that
  looks like an integrity tag but provides none, and a VT-delimited
  inner plaintext parser that silently breaks when plaintext contains a
  VT byte. The godoc directs callers to the new V2 siblings
  `RsaAesPublicKeyEncryptAndSignHmac` /
  `RsaAesPrivateKeyDecryptAndVerifyHmac`. The V1 functions remain fully
  callable through the entire v1.x series per workspace rule #10
  (observable-contract stability); removal is scheduled for v2.0.0.

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
- Added `wrapper/dynamodb/dynamodb_contract_test.go` — first test file in the
  package. Pins `MaxTransactItems = 100` and `MaxBatchWriteItems = 25` and
  guards that they remain distinct (DDB-P0-1), plus unit tests for the new
  `unprocessedItemsToAwsRequestItems` / `awsRequestItemsToUnprocessedItems`
  pure helpers covering empty input, put-only / delete-only / mixed residuals,
  nil-skip behavior, and typed↔AWS round-trip symmetry (DDB-P0-2).

### Deferred to v2.0.0 (coordinated breaking-change release)

Findings that would break observable contracts of `aldelo/common` are deferred
to a future major release so downstream repos can migrate in one coordinated
batch. See `_src/docs/repos/common/reviews/deep-review-2026-04-11-release-readiness.md`
in the workspace for the full list, including:

- **P0-4** — Removal of `AesCbcEncrypt` / `AesCbcDecrypt`. The NUL-padding
  hazard was documented and pinned via deprecation godoc + regression tests
  in v1.7.9 (see "Deprecated" section above); the already-existing
  `AesGcmEncrypt` / `AesGcmDecrypt` pair is the migration target for v1.x
  callers. Function removal is scheduled for v2.0.0 once all 36+ consumer
  repos have migrated.
- **P0-5** — Removal of `RsaAesPublicKeyEncryptAndSign` /
  `RsaAesPrivateKeyDecryptAndVerify`. The V1 envelope hazards were
  documented and the V2 sibling pair
  (`RsaAesPublicKeyEncryptAndSignHmac` /
  `RsaAesPrivateKeyDecryptAndVerifyHmac`) was added in v1.7.9 (see
  "Added — additive siblings" section above); the V1 pair is the
  migration source and remains callable through the entire v1.x
  series. Function removal is scheduled for v2.0.0 once all 36+
  consumer repos have migrated to the V2 pair.
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
