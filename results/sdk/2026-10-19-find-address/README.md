# `solana.FindAddress` on `sol_try_find_program_address`

October 19, 2026. Local only; runners 0.5.1 (fogo-v0, LiteSVM 0.8.2) and 0.6.0
(solana-v3, LiteSVM 0.13.0), platform-tools v1.57. API:
[SDK2.md](../../../docs/SDK2.md#interfaces-and-bounds).

## Same answer as `find_program_address`

`TestFindAddressSBF` (`internal/compiler/findaddress_sbf_test.go`) derives 27
seed sets (none, one byte, 24 Hyperlane-shaped `process_authority` groups, 15
seeds of 32 bytes) under different program keys. On both profiles every
address and bump matches the host oracle `fixture.FindProgramAddress` and a
`CreateAddress` loop over bumps 255..1 in the same program. Four malformed
groups (16 seeds, a 33-byte seed, a trailing byte, a short seed) return 2007
before the syscall.

## CU (whole transaction, from `fogo-v0-summary.json`; solana-v3 is identical)

| seeds | bump | tries | `FindAddress` | `CreateAddress` loop | saved |
| --- | ---: | ---: | ---: | ---: | ---: |
| one byte | 255 | 1 | 2,113 | 2,520 | 407 |
| Hyperlane `process_authority` | 252 | 4 | 6,681 | 7,964 | 1,283 |
| Hyperlane `process_authority` | 251 | 5 | 8,181 | 9,588 | 1,407 |
| 15 × 32 bytes | 252 | 4 | 6,851 | 12,084 | 5,233 |

Each extra bump costs exactly 1,500 CU with `FindAddress`, the runtime's price.
The loop pays 1,500 plus the seed parsing and the 32-byte staging copy on every
try. Median saving over the 27 vectors: 911 CU.

## Hyperlane Go programs

`hyperlane-monorepo` `sealevel` with `svm.FindProgramAddress` switched from a
`CreateAddress` loop to `FindAddress`, both built with this repo's
`hyperlane-integration-3` CLI (so the loop already had the
`opt-runtime-word-access` direct output), run through all 23 Rust functional
suites (584/584 pass both ways). Total CU change per program: fee 2.2% less,
token-native 1.6% less, token-cross-collateral 1.2% less, IGP 0.8% less,
mailbox 0.9% less, composite ISM 0.7% less. Bump searches depend on the suites'
random keys, so single calls move by whole 1,500 CU steps between runs; the
per-call median is unchanged.
