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

### Changed (behavioral — review before upgrading)

- **CORS fail-closed (wrapper/gin):** When `AllowAllOrigins` is false and
  `AllowOrigins` is empty and `AllowOriginFunc` is nil, CORS middleware is
  NO LONGER installed. Previously, an empty config silently allowed all
  origins. Now the server returns no `Access-Control-Allow-Origin` header,
  causing browsers to reject cross-origin requests. Consumers relying on
  the previous permissive-by-default behavior must explicitly set
  `AllowOrigins` or `AllowAllOrigins = true`. (gin.go:842-856, SEC-003)

- **REST error wrapping (wrapper/rest):** `rest.GET`, `rest.POST`,
  `rest.PUT`, `rest.DELETE` now wrap response-body read errors with
  `fmt.Errorf("reading response body: %w", err)` instead of returning
  the raw `io.ReadAll` error. Callers using `errors.Is(err, ...)` on the
  returned error should use `errors.Unwrap` or `errors.As` if they need
  the inner error. (rest.go:188,266,343,409)

### Fixed

- **SES/SQS deadline enforcement:** All 12 SES and 15 SQS methods now
  enforce a 30s default context deadline via `ensureSESCtx`/`ensureSQSCtx`,
  matching the existing `ensureSNSCtx` pattern. Prevents goroutine leaks
  during AWS regional degradation. (A1-F1)
- **TCPServer data race:** Eliminated `_tcpListener` read/write race
  between Accept goroutine and Close() via goroutine-local capture under
  RLock + `net.ErrClosed` clean shutdown. (A1-F3)
- **ginxray silent error discard:** All 7 `_ = seg.AddMetadata(...)` sites
  in ginxray now route through `LogXrayAddFailure()`. (A1-F2)
- **ginxray raw body PII:** Removed raw request/response body from xray
  metadata to prevent PII exposure in traces. (A1-F6)

## [v1.8.2] — 2026-04-15

Patch release. Closes five `wrapper/sns` findings from the **SP-010
pass-5 contrarian review** (review `_src/docs/repos/connector/findings/
2026-04-15-contrarian-pass5/`). All fixes target surfaces already
touched by `v1.8.1`'s `ensureSNSCtx` / `maskPhoneForXray` rollout —
this release is the correctness sweep that the pass-5 review demanded
before the surfaces could be considered fully hardened.

No observable helper contract change from `v1.8.1`. Every public
function signature in `wrapper/sns` is preserved. Consumers pinning
`common v1.8.1` should bump to `v1.8.2` as a drop-in for the PII-safety
and UTF-8-safety guarantees below.

Context: SP-010 pass-5 re-audited `v1.8.1` against a contrarian rule
set — "assume the v1.8.1 fixes are incomplete, find what was missed".
The review found five findings in `wrapper/sns` (A1-F1 → A1-F5) and
five in `connector` (A2/A4 class) — five on each sibling, landed as
ten per-gap commits under the standing directive *"one gap at a time,
review+audit between gap groups, version ceiling v1.8.2."* The
per-gap protocol was: fix → regression test → mutation probe
(causality validation) → full suite green → per-finding commit.
Three independent reviewer audits (Gap 1.A / 2.A / 3.A) returned
PASS or PASS-WITH-NOTES with zero blockers.

### Fixed — SP-010 A1-F1 (`wrapper/sns` — `ensureSNSCtx`)

- **`ensureSNSCtx` now enforces the deadline even when xray is on.**
  The `v1.8.1` helper applied the default-30s / caller-supplied
  `timeOutDuration` deadline only on the xray-disabled path. When
  xray was enabled, the helper returned the xray-derived ctx **as
  provided by the caller**, with no deadline check — so a caller
  that had xray enabled and had also configured a long
  `timeOutDuration` upstream would observe the xray ctx's implicit
  lifetime, not their own timeout. The fix wraps the xray ctx with
  `context.WithTimeout(segCtx, resolvedDeadline)` before returning,
  preserving both the xray trace plumbing and the observable
  deadline. Every SNS SDK call site now observes the intended
  deadline regardless of the xray toggle. Commit `ae59793`.

### Documented — SP-010 A1-F2 (`wrapper/sns` — 29 callsite comments)

- **Callsite comments rewritten post-A1-F1.** Twenty-nine SNS client
  callsites in `wrapper/sns/sns.go` had comments that described the
  pre-A1-F1 helper semantics ("default deadline applied only when
  xray off"). Those comments are now aligned with the post-fix
  contract: the helper always returns a deadline-bearing ctx, xray
  or not. Pure comment cleanup — zero code change, no behavior
  impact — but the correctness of future review passes depends on
  these comments matching what the code actually does. Commit
  `22e96f1`.

### Fixed — SP-010 A1-F3 (`wrapper/sns` — `SendSMS` phone PII)

- **`SendSMS` xray metadata now masks the destination phone number.**
  `v1.8.1`'s `maskPhoneForXray` was wired into `OptInPhoneNumber`,
  `CheckIfPhoneNumberIsOptedOut`, and `ListPhoneNumbersOptedOut`
  but not `SendSMS` — the pass-3 F5 rationale comment had argued
  that `SendSMS` treats the phone number as a delivery address, not
  metadata, and therefore should not be masked. The pass-5 review
  rejected that reasoning: xray metadata is trace plumbing, not
  application data, and any trace reader with metadata access can
  pivot a raw `SendSMS` xray segment dump to a natural-person
  identity. The fix applies `maskPhoneForXray` at the `SendSMS`
  xray emit site so the destination phone is redacted to
  `+X******NNNN` before hitting the tracer. The SNS SDK call itself
  still receives the unredacted destination — only the metadata
  surface is masked. Commit `549e7a2`.

### Fixed — SP-010 A1-F4 (`wrapper/sns` — UTF-8 safety)

- **`maskPhoneForXray` now slices by rune, not byte.** The
  `v1.8.1` implementation of `maskPhoneForXray` indexed the input
  string by byte offset to extract the country-code prefix and
  last-four suffix. For ASCII E.164 phone numbers (the intended
  shape), byte-indexing is correct — but a phone number containing
  any multi-byte codepoint (Arabic-Indic digits ٠–٩, Devanagari,
  etc.) silently produced a garbage mask by slicing through the
  middle of a UTF-8 sequence. No panic, no log — just a leaked or
  malformed redaction. The fix converts to `[]rune` once, slices
  on rune indices, and reconverts. A table-driven test covers
  US / UK / Arabic-Indic / Devanagari / non-E.164 / empty / `+`-only
  inputs; a property test pins that the middle digits are never
  revealed across five country formats. Commit `57cf00c`. This
  fix is the origin of lesson **L18** (*rune-based string slicing
  as default for every unvalidated-input redact/truncate/mask
  helper*).

### Added — SP-010 A1-F5 (`wrapper/sns` — `ensureSNSCtx` dead-guard test)

- **Nil-segCtx reachability test for `ensureSNSCtx`.** `v1.8.1`
  added a nil-segCtx guard to `ensureSNSCtx` (the xray-derived ctx
  is nil-safe), but no test existed that actually reached the guard
  under realistic conditions — the only path that produces a nil
  segCtx in production is an SNS callsite invoked from a request
  flow that has xray disabled AND no upstream-propagated ctx, and
  the `v1.8.1` tests either passed a non-nil mock segCtx or
  exercised the caller-supplied-deadline branch. The new test
  (`TestEnsureSNSCtx_NilSegCtxDeadGuard_A1F5`) replays the nil-ctx
  branch explicitly and asserts that the returned ctx is non-nil
  and deadline-bearing. Without the guard, the helper would return
  a nil ctx that would trip the SDK on first use. Mutation-probe
  validated: temporarily removing the `segCtx == nil` guard
  produces the expected nil-return panic. Commit `258803d`.

### Verified

- `go build ./...` clean
- `go vet ./...` clean
- `go test -race -short ./...` clean (full package tree; matches
  the `v1.8.1` release convention at this repo's sibling
  `connector/CHANGELOG.md:148-151`)
- Three independent reviewer audits (Gap 1.A / 2.A / 3.A) by
  `pr-review-toolkit:code-reviewer` (opus) returned PASS or
  PASS-WITH-NOTES with zero blockers. Reports archived in the
  workspace at `_src/docs/repos/connector/findings/2026-04-15-
  contrarian-pass5/_gap{1,2,3}A-reviewer-audit.md`.

### Upgrade notes

- **Drop-in from v1.8.1.** No `go.mod` directive moves; the `go`
  toolchain pin remains `1.26.2` from v1.8.0.
- **Consumer sweep.** All 38 workspace consumer repos should bump
  their `common` pin `v1.8.1 → v1.8.2` in coordination with the
  sibling `connector v1.8.2` release cut from the same review cycle.
  The sibling release's CHANGELOG entry is the canonical record of
  that sibling tag — see `github.com/aldelo/connector/CHANGELOG.md`
  at whichever tag is current on origin.
- **Lessons promoted.** This cycle promoted four lessons (L17–L20)
  into the workspace lessons-learned file, including L18
  (rune-based slicing as default) which originated here.

## [v1.8.1] — 2026-04-15

Patch release. Closes the two `wrapper/sns` + `wrapper/kms` P1 findings
from the `deep-review-2026-04-15-contrarian-pass4` cycle that landed on
`master` after the `v1.8.0` tag was cut. No observable helper contract
changes from `v1.8.0`; every public function signature is preserved.
Consumers should bump `v1.8.0 → v1.8.1` as a drop-in.

Context: `v1.8.0` narrated hardening of `wrapper/kms` / `wrapper/sns`
against nil / torn-read / hung-endpoint failure modes, but the two
*implementations* of that hardening (`ensureSNSCtx` full rollout +
`atomic.Pointer[kms.KMS]` migration) merged after the tag. `v1.8.1`
brings the tag in line with the narrative. This is the first release
cut under workspace rule #15 (release-artifact parity), which is why
the gap was surfaced as a P0 contrarian finding rather than silently
shipped.

### Fixed — SP-008 P1-COMMON-SNS-01 (`wrapper/sns`)

- **ensureSNSCtx helper rollout (25 callsites).** Every SNS client
  callsite in `wrapper/sns/sns.go` now funnels through a single
  `ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)` helper with
  precedence: caller-provided `timeOutDuration` > xray-derived ctx >
  default 30s deadline. The helper is also nil-segCtx safe — a nil
  xray ctx no longer slips into an SDK call site. Before the
  rollout, 17 of the 25 callsites had no deadline at all when xray
  was disabled and the caller did not pass an explicit timeout,
  leaving the caller goroutine blocked indefinitely on a hung AWS
  SNS endpoint. The rollout is internal plumbing — no public
  function signature changed, and callers that already pass
  `timeOutDuration` see zero behavioral change. Commit `f982aee`.
- **Phone-PII mask for xray emit sites.** A new internal
  `maskPhoneForXray` redacts E.164 phone numbers to `+X*****NNNN`
  (keeps the country-code prefix and last four digits, asterisks the
  middle) so trace readers with metadata access can no longer pivot
  a raw xray segment dump to a natural-person identity. Wired into
  `OptInPhoneNumber`, `CheckIfPhoneNumberIsOptedOut`, and
  `ListPhoneNumbersOptedOut` xray emit sites. `SendSMS` continues
  to emit the unredacted destination on purpose — the F5 pass-3
  rationale comment at ~L1935 documents that SendSMS treats the
  phone as a delivery address, not PII metadata. Table-driven tests
  (`TestMaskPhoneForXray` — US / UK / minimum / below-threshold /
  non-E.164 / empty / no-plus) and a property test
  (`TestMaskPhoneForXray_NeverRevealsMiddleDigits` across five
  country formats) freeze the redaction invariant against future
  regression. Commit `f982aee`.

### Fixed — SP-008 P1-COMMON-KMS-01 (`wrapper/kms`)

- **`kmsClient` migrated to `atomic.Pointer[kms.KMS]`.** The hot-path
  KMS client publication in `wrapper/kms/kms.go` is now an
  `atomic.Pointer[kms.KMS]` instead of a plain pointer protected by
  `k.mu.RLock`. Every reader path does a single acquire `Load()` +
  nil check — lock-free and faster — and, more importantly, the
  compiler now **enforces** the torn-read invariant: a future
  refactor cannot silently reintroduce the hazard by adding a new
  method that reads the field without taking the mutex, because the
  field is no longer directly readable. Torn reads under the old
  plain-pointer scheme were benign on amd64 but theoretically
  observable on arm64 (the Go memory model does not guarantee
  pointer-write atomicity without explicit synchronization), so
  this also future-proofs the library for Graviton-based AWS
  deployments. Commit `00d2a13`.
- **Reconfigure path unchanged.** `setSessionAndClient` still holds
  `k.mu` across the session + client mutation, and the `Store()` now
  happens *under* that lock. Multi-field snapshot readers (the four
  hot-path methods `EncryptViaCmkAes256`, `DecryptViaCmkAes256`,
  `EncryptViaCmkRsa2048`, `DecryptViaCmkRsa2048`) keep their
  function-entry `RLock` — the `RLock` still pins `AesKmsKeyName` /
  `RsaKmsKeyName` + `_parentSegment` to the same publication
  generation as the client, so an `RLock` + `Load` observes a
  consistent snapshot even if a concurrent `setSessionAndClient`
  fires mid-call.
- **New regression tests.** `TestKMS_ConcurrentReconfigureDoesNotRace`
  pins the writer-reader invariant under `go test -race` with eight
  parallel readers and a flip-flop writer; a regressed migration
  that removes an `RLock` hoist or reintroduces a plain pointer
  trips the race detector. `TestKMS_GetClientReturnsErrWhenUnset`
  and `TestKMS_DisconnectClearsPublishedClient` pin the nil-`Load()`
  contract that the sentinel "Client is Required" error path depends
  on.

### Verified

- `go build ./...` clean
- `go vet ./...` clean
- `go test -race ./...` clean (full package tree, including new
  tests above)
- `govulncheck ./...` — same baseline as v1.8.0 (no new advisories)

### Upgrade notes

- **Drop-in from v1.8.0.** No `go.mod` directive moves; the `go`
  toolchain pin remains `1.26.2` from v1.8.0.
- **Consumer sweep.** All 38 workspace consumer repos should bump
  their `common` pin `v1.8.0 → v1.8.1` in coordination with the
  sibling `connector` release cut from the same review cycle. The
  sibling release's CHANGELOG entry is the canonical record of that
  sibling tag — see `github.com/aldelo/connector/CHANGELOG.md` at
  whichever tag is current on origin.

## [v1.8.0] — 2026-04-15

Minor release. Primary themes: **coordinated `go 1.26.2` baseline bump**
(the `go` directive in `go.mod` moves from `1.24.1` → `1.26.2` so the
security toolchain pin from v1.7.9 is now also the declared language
level); hardening of observability helpers in `wrapper/xray`, `wrapper/kms`,
and `wrapper/sns` against nil / torn-read / hung-endpoint failure modes;
and changelog-level callouts for the SNS F4/F5 xray metadata-key rename
shipped in v1.7.9 that external downstream observability tooling may still
be matching on.

This is a **coordinated-bump release** — per workspace rule #10, the `go`
directive move is not silent: all 38 workspace consumer repos are expected
to bump their own `go` directive to `1.26.2` and their `common` pin to
`v1.8.0` in the same wave. No observable helper contract in this release
has changed from `v1.7.10`.

### Changed — language baseline (coordinated bump)

- **GOMOD-F1** — `go` directive in `common/go.mod` moved from `1.24.1` to
  `1.26.2`. The `toolchain` directive (pinned to `go1.26.2` in v1.7.9 to
  pick up the `GO-2026-4865` `html/template` fix) is now joined by the
  matching `go` directive, so consumers no longer get the "toolchain
  newer than go directive" mismatch. Every downstream repo that pins
  `common` must bump to `go 1.26.2` in the same wave. Existing
  `github.com/aldelo/connector` is the reference consumer for the sweep
  pattern.

### Added — defensive timeout defaults (SP-008 P2-CMN-2 / P2-CMN-3)

- **P2-CMN-2** — `wrapper/kms/kms.go`: all `Encrypt*` / `Decrypt*` /
  `ReEncrypt*` / `GetRSAPublicKey` / `Sign*` / `Verify*` SDK call sites
  now funnel through a new internal `ensureKMSCtx(segCtx)` helper that
  applies a **default 30-second deadline** when the xray-derived ctx is
  nil and the caller passed no deadline of its own. Previously a hung
  AWS KMS endpoint could block the caller indefinitely when xray was
  disabled. The helper preserves any existing deadline verbatim — only
  the "no deadline at all" case is defaulted.
- **P2-CMN-3** — `wrapper/sns/sns.go`: `Publish` / `SendSMS` now use the
  matching `ensureSNSCtx(segCtx, segCtxSet, timeOutDuration)` helper.
  When `timeOutDuration` is empty AND the xray-derived ctx is unset,
  a 30-second default deadline is applied. Existing callers that pass
  an explicit `timeOutDuration` see **zero** behavioral change.

### Changed — nil / torn-read hardening (SP-008 P1, P3-CMN-3)

- **P1 KMS godoc (P1 fix-up from deep-review-2026-04-15)** — `EncryptViaCmkAes256`
  and surrounding KMS methods now accurately describe the AWS SDK
  pointer-reassignment semantics (the xray segment ctx *may* be replaced
  by the SDK during retries, so callers must not assume the ctx they
  passed is the one the deferred closure observes). The previous godoc
  described an older pre-retry internal pattern that no longer holds.
- **P3-CMN-3** — `wrapper/kms/kms.go`: the four hot-path methods
  `EncryptViaCmkAes256`, `DecryptViaCmkAes256`, `EncryptViaCmkRsa2048`,
  and `DecryptViaCmkRsa2048` now take a **single `RLock` snapshot** of
  `kmsClient` / `AesKmsKeyName` (or `RsaKmsKeyName`) / `_parentSegment`
  at function entry instead of 3 independent getter `RLock` pairs. This
  closes a torn-read hazard where a concurrent `SetKmsClient(...)`
  between getter calls could surface a new client paired with an old
  key name (or vice versa). The xray defer closure now references the
  captured local key-name, so metadata annotations always reflect the
  value actually used for the KMS call. Inline `cli == nil` check
  replaces `k.getClient()`'s error-returning form — the error message
  normalizes to `"KMS CMK Failed: KMS Client is Required"` to match the
  existing `"Required"` validation wording. No observable contract
  change from v1.7.10.

### Fixed — minor style (SP-008 P3-CMN-2)

- **P3-CMN-2** — `wrapper/sns/sns.go`: two `len([]byte(s))` idioms
  (lines 1818 and 2732 in v1.7.10 addressing) replaced with plain
  `len(message)`. The Go compiler already optimizes `len([]byte(s))`
  to zero allocations since Go 1.5, so this is **pure readability**
  with no behavioral or performance change.

### Observable-contract migration notice (SP-008 P2-CMN-1)

**External xray observability tooling outside the 38-repo workspace may
match on SNS Publish/SendSMS metadata keys.** v1.7.9 renamed these keys
(originally to stop leaking PII through xray segment dumps):

| v1.7.8 key (removed in v1.7.9) | v1.7.9+ key |
|---|---|
| `SNS-Publish-Message` | `SNS-Publish-Message-Length` |
| `SNS-Publish-Attributes` | `SNS-Publish-Attribute-Keys` |
| `SNS-SendSMS-Message` | `SNS-SendSMS-Message-Length` |

The **values** also changed: the old `*-Message` keys emitted the full
payload; the new `*-Message-Length` keys emit the byte length, and the
new `*-Attribute-Keys` key emits a sorted, comma-joined list of
attribute names (never values). Any downstream xray dashboard, alarm,
or log query matching on the old keys must migrate before bumping past
`v1.7.8`. Cross-repo grep across the 38-workspace-repo set returned
zero matches, so this notice is primarily for external operators whose
repos are not visible from the workspace.

### Fixed — cosmetic (SP-008 P3-CMN-1)

- **P3-CMN-1** — commit `6fc1625` v1.7.10 subject line said
  `SliceElementAtIndex`; the test and fix both target
  `SliceDeleteElement`. The commit body is correct. Noted here per rule
  #10 ("do not amend pushed commits") — no code change, changelog record
  only.

### Consumer impact

- Every downstream repo MUST bump **both** the `common` pin
  (`v1.7.10` → `v1.8.0`) **AND** the `go` directive (`1.24.1` → `1.26.2`)
  in the same commit. Bumping only one produces a build-tree diagnostic
  for half the workspace. The coordinated sweep wave for the 38-workspace
  repos is tracked in
  `_src/docs/plans/2026-04-15__pass3-f4f7-pushed__checkpoint.md`.
- No helper observable contract changed. All fixes are nil-guard /
  timeout / readability / documentation / internal-locking. The release
  is bumped to **minor** (not patch) solely because the `go` directive
  move is a coordinated, caller-observable change.

## [v1.7.10] — 2026-04-13

Patch release. Single fix for a `SliceDeleteElement` panic that shipped in
v1.7.0–v1.7.9 and was discovered after `v1.7.9` was already tagged on origin.
Since published Git tags are immutable for downstream Go module consumers
(the Go module proxy caches tag-to-commit hashes — moving a published tag
causes `go.sum` checksum mismatches across every consumer), this fix is
delivered as a new patch release rather than retagging v1.7.9.

### Fixed — error handling and safety

- **P0-13** — `SliceDeleteElement`: fixed `reflect: reflect.Value.Set using
  unaddressable value` panic on value-type slice inputs (the most common call
  pattern, e.g. `SliceDeleteElement([]int{1,2,3}, -1)`). The "settable copy"
  fallback introduced in `af0d217` used `reflect.MakeSlice`, which does NOT
  produce an addressable `Value` — so the downstream `v.Set(v.Slice(...))` still
  panicked. Replaced with the canonical `reflect.New(v.Type()).Elem()` trick
  (allocate a `*T`, dereference to get a settable `T`-Value, copy the input
  header into it). The documented negative-index contract (`-1` removes last,
  `-2` removes 2nd-last, etc.) now actually works. Added 17 unit tests in the
  new `helper-other_test.go` covering value slice / pointer slice / nil / empty /
  single element / out-of-bounds positive+negative / struct slices — previously
  `SliceDeleteElement` had **zero** tests, which is how the bug shipped.
  Rule #10: observable contract is what the godoc promises, not what the buggy
  implementation happened to do.

### Consumer impact

- All 36+ downstream repos pinned at `v1.7.9` should bump to `v1.7.10`. The
  fix is a strict bug fix (panic → correct return) with no API changes; bumping
  is safe under workspace rule #10.
- `connector` (the first consumer to bump) tracked the panic via its
  `service/service_test.go::TestSliceDeleteFunc` test, which was temporarily
  skipped pending this release. Once `connector/go.mod` is bumped to v1.7.10,
  that skip can be removed.

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
