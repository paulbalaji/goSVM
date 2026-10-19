# Word-at-a-time `==` on byte arrays

October 19, 2026. Local only; runners 0.5.1 (fogo-v0, LiteSVM 0.8.2) and 0.6.0
(solana-v3, LiteSVM 0.13.0), platform-tools v1.57. Contract:
[GO_ARRAYS.md](../../../docs/GO_ARRAYS.md).

## Correctness

`TestByteArrayEqualSBF` compares `[8]byte`, `[13]byte`, `[20]byte`,
`[32]byte` (also as a struct field), `[64]byte`, `[13]int8`, `[32]int8`,
`[32]bool` and `[8]uint16` pairs: equal, and with bit 0 and bit 7 flipped at
every position. All 437 comparisons match Go on both profiles.
`TestByteArrayEqualC` checks that only one-byte integer arrays of at least 8
elements get the word comparison, and `TestCorpusMatchesPreCheckCompiler`
that it is the only change to the frozen corpus's C.

## CU (whole transaction, equal inputs; `*/before-summary.json` is main `79cf25c`)

| arrays | before | after | saved |
| --- | ---: | ---: | ---: |
| `[8]byte` | 301 | 302 | -1 |
| `[13]byte` | 431 | 416 | 15 |
| `[20]byte` | 577 | 541 | 36 |
| `[32]byte` and a struct holding one (2 comparisons) | 1,548 | 750 | 798 |
| `[64]byte` | 3,197 | 1,343 | 1,854 |
| `[32]int8` | 1,167 | 744 | 423 |
| `[32]bool` (unchanged lowering) | 1,345 | 1,345 | 0 |

The transactions also copy the inputs with `copy`, which is a byte loop on
main, so the saving per comparison is the difference, not the ratio: about
400 CU per equal `[32]byte` pair. Short arrays were already cheap.

## Hyperlane Go programs

`hyperlane-monorepo` `sealevel` at `1d4694c13f` (which already compares
account keys with an unrolled XOR, not `==`), built with
`hyperlane-integration-2` with and without this branch, all 23 Rust
functional suites, 584/584 pass both ways. Total CU: fee 1.2% less, IGP 1.4%
less, synthetic token 1.6% less, others within 0.5%; per-call median
unchanged. Programs that compare keys with `==` gain more.
