---
name: simd
description: aabbworld's SIMD kernels, built with GOEXPERIMENT=simd on amd64, arm64 or wasm — the collision pair sweep a vector of boxes at a time, Query's and sight's cone a vector of pieces at a time, the slabs a vector of rays at a time, written for AVX2 and again on the portable simd package, the heightfield walked four rays in lockstep on AVX2 — the gate and the file pattern, why each is as wide as it is, what stays scalar on purpose, the invariants (the same pairs and sightings to the bit) and the tests that hold them, how a kernel is measured, the traps. Use when touching a *_simd.go, *_lanes.go or wide_*.go file or a scalar twin, adding a kernel, chasing a difference between the SIMD and the plain build, or measuring either.
---

# simd

Everything wide here is deliberate, bounded and measured: a kernel earns its place by giving the
plain code's answer and a number in `BENCHMARKS.md`. What the user sees is `doc.go` ("SIMD"); the
numbers and the method are `BENCHMARKS.md` ("Wide pair sweep", "simd's vectors", "Sight in SIMD",
"Key takeaways", "How to benchmark"); what changed when is `CHANGELOG.md`. The cells the kernels
read, and the still layer, are the `spatial` skill's. This skill is the practice.

## The gate and the files

- Build: `GOEXPERIMENT=simd` on amd64, arm64 or wasm. Every kernel has two writings: one for AVX2
  on `simd/archsimd`, amd64 only, and one on the portable `simd` package at whatever width it
  runs (128 bits on Neon and wasm, 256 or 512 on amd64). `internal/simdx` holds what `simd`
  lacks: a mask as a bitmask (`Bits32`, `Bits64`), the padding a slice loaded a vector at a time
  needs (`Pad32`, `Pad64`), `On` (simd in hardware), `ArchFirst` (AVX2, and simd not emulated)
  and `Settle` (`ClearAVXUpperBits` on amd64, nothing elsewhere).
- Run: in each package `wide_simd.go`, under `goexperiment.simd && (amd64 || arm64 || wasm)`,
  holds the gates — `wide` (`simdx.On() || simdx.ArchFirst()`), `avx2` (`simdx.ArchFirst()`),
  `wideQuery` and `queryWideFrom` in spatial — and the wrappers the callers name (`visitWide`,
  `walkConeWide`, `queryWide`, `fillWide`, `invertWide`), each choosing `avx2`'s kernel or
  simd's. Its twin `wide_scalar.go` holds `const wide = false` and wrappers that do nothing.
  `GODEBUG=simd=0` makes `simdx.On` and `ArchFirst` false: no kernel runs. `aabbworld.SIMD()`
  reports `spatial.Wide()`.
- The AVX2 kernels are `*_simd.go` under `goexperiment.simd && amd64`, with twins `*_scalar.go`
  that do nothing: `pair_sweep_simd.go` (`visitAVX2`), `cone_query_simd.go` (`walkConeAVX2`),
  `slab_simd.go` (`fillAVX2`, `invertAVX2`), `heightfield_simd.go` (`walkWide`, AVX2 only).
  simd's are `*_lanes.go` beside them (`visitLanes`, `walkConeLanes`, `queryLanes`, `fillLanes`,
  `invertLanes`), needing no twin: only `wide_simd.go` calls them. The fork is one `if wide` at
  the caller: `sweep.go` (`walk`, and the lockstep on `wide && avx2`), `slab.go` (`fill`,
  `laid`), `pair_sweep.go`, `grid.go` (`Query` on `wideQuery`), `cone_query.go`.
- `simdx.Settle()` before every callback out of a kernel and at its exit (the SSE code after it
  would pay the transition); `export_wide_test.go` opens `wide` to the tests (`SetWide`) and runs
  a check once for each kernel the CPU runs (`EachKernel`; spatial's `eachKernel`).

## The kernels

- **The pair sweep** (`pair_sweep_simd.go`, `pair_sweep_lanes.go`): one box against the next
  several of a crowded cell (`wideFrom` entries or more), the cell's edges in columns filled per
  cell (`sweepCols`). Where the tests are many to a cell it pays; sparse cells gain nothing.
- **The cone query** (`cone_query_simd.go`, `cone_query_lanes.go`): the pieces of the cells
  under the cone's rectangle tested a vector at a time — the radius, the wedge (a cone narrower
  than a half-turn), the owner cell — from columns of the cells' edges (`cellCols`) built with
  the cells once a scan or a wide query has been asked (`colsAsked`; the plain build never pays
  for them).
- **The query** (`grid_query_lanes.go`, simd's only, never with AVX2: it lost to the plain test
  on some AVX2 CPUs while gaining on others): `Query` tests a cell's pieces a vector at a time
  from the cone's columns, a cell of fewer than a vector a piece at a time, and only while the
  occupied cells hold `queryWideFrom` vectors of pieces each on average (`spans.occupied`): below
  that the plain test is as fast or faster. A sparse layout answers from its cell list first,
  before the vectors (`spatial` skill).
- **The slabs** (`internal/raycast/slab_simd.go`, `slab_lanes.go`): between two events of a walk
  the active set is fixed, so each active box's `near`/`far` is laid for a vector of angles at a
  time into a scratch buffer `castFlat`/`castElevated` read; blocks under `minWide` go as cast.
- **The heightfield in lockstep** (`internal/raycast/heightfield_simd.go`, AVX2 only): the
  lattice walk four rays at a time, a ray to a lane of `Float64x4` (`Int64x4` for the cells), the
  corners read lane by lane a round ahead (archsimd has no gather) and the points gathered lane by
  lane after; a block whose rays cross more than `maxLaneCells` walks as cast.

**Why those widths.** Four float64, not eight float32, in the slabs and the walk: the walk must
record the plain walk's points to the bit (each lane does the scalar operations in the scalar
order; `fused`/`mulAdd` probe whether the compiler fuses the plain walk's multiply-adds, as
`GOAMD64=v3` does, and fuse alike), and eight float32 rays flipped ties on boxes with a face on a
cell's line — a systematic thing on a grid — changing the set of entities seen, for a percent of
time. Float32 where the data already is float32: the cells' edges, the boxes.

**Why AVX2 first.** On amd64 the AVX2 kernels are as fast as simd's or faster at every width
simd gives, and twice as wide where simd holds a CPU without VPCLMULQDQ to 128 bits; simd's
kernels lost to them at the same width. So `avx2` picks the AVX2 kernels wherever AVX2 runs, and
simd's are for arm64 and wasm. Measure both before moving that line.

**Scalar on purpose**: the crossing order and the budget (`insertCrossing`, `spend`), the cover
(`castCover`, a callback a stretch), `refine` (one ray), grounds without a lattice (`Ground`,
`Profile`: a call a point), the loads of corners and the appends of points (no gather, no
scatter), every callback. A kernel over them would move the cost, not remove it.

## Invariants and the tests that hold them

- Each check of a kernel runs once for every kernel the CPU runs (`eachKernel`, `EachKernel`): on
  an AVX2 CPU the AVX2 one and simd's, on Neon simd's.
- The pair sweep finds the same pairs in the same order as the plain sweep:
  `TestPairSweep_WideFindsWhatNarrowFinds`, `_WideAgreesOnEdgesAndNaN`,
  `_WideNeverPairsABoxWithItself`.
- The cone query keeps what the plain filter keeps: `TestQueryCone_WideFindsWhatNarrowFinds`,
  `_WideAgreesOnEdgesAndNaN`, `_ColumnsFollowTheCells`, `_ManyGoroutinesAskAFreshGridAsOneDoes`.
- Query reports what the plain Query reports, in the same order, and goes wide only over crowded
  cells: `TestQuery_WideFindsWhatNarrowFinds`, `_WideAgreesOnEdgesAndNaN`,
  `_ColumnsFollowTheCells`, `_WideOnlyOverCrowdedCells`, `_ManyGoroutinesAskAFreshGridAsOneDoes`,
  `_ColumnsServeConesAndQueriesAlike`; with AVX2 it stays plain:
  `_WideOnSimdsVectorsOnlyWithoutAVX2`. Through a Space, the seams' images go wide too:
  `TestSpace_Query_FindsEveryBoxAcrossTheSeams` (a crowded torus against a brute force).
- `simdx.Bits32` and `Bits64` set one bit a lane, and the way through memory, wasm's, sets the
  same: `TestBits_OneBitALane`, `_StoredAgreesWithTheMask`.
- Every lane of a slab is `hitDistance`: `TestFillWide_EveryLaneIsHitDistance`,
  `TestInvertWide_IsOneOverEach`; the View benchmarks' scenes see exactly what they saw (an
  entity seen over less than the sweep resolves may differ once boxes out of reach add no
  casts: said in the CHANGELOG, accepted).
- The lockstep walk records the plain walk's points to the bit:
  `TestHeights_RaysWalkedSideBySideRecordWhatEachWalksAlone` (random fields, fans of angles, the
  oracle `fieldOracle`); a scan over a field sees what one over its profile sees.
- `make test-simd` runs the suite built wide, `make test` plain, `make test-simd-128` built wide
  with simd's vectors held to 128 bits (on AVX2, the width of a CPU without VPCLMULQDQ),
  `make test-simd-emulated` built wide with no kernel running, and `make vet-simd` vets the build
  for amd64, arm64 and wasm; CI (`.github/workflows/simd-amd64.yml`) runs them all on amd64 and
  benchmarks a pull request against its base. A change to a kernel runs them, the walk once under
  `GOAMD64=v3`, and the suite on an arm64 machine.

## Measuring

`make bench-simd` and `make bench-save-simd` beside `make bench`. A change is judged on a baseline
copy of the tree run alternately with the working one on an idle machine, medians of several runs
(`BENCHMARKS.md`, "How to benchmark"), in both builds — the plain build must not pay for the
columns or the buffers — on the scenes a kernel aims at and on the sparse ones (`BroadPhase`
sparse, `View_Visible` with 100 entities), which must stay within noise. A number for
`BENCHMARKS.md` names the CPU: the kernels differ by machine (the query's gate above), and a
contributor's arm64 numbers sit beside the amd64 ones, never over them.

## Traps

- AVX2 on every hyperthread lowers the clock: scalar code called from inside a wide scan (a
  callback a point, `castCover`) runs slower under a parallel benchmark than alone, and a few
  percent faster on one CPU. Compare `-cpu=1` and the plain build before blaming a kernel.
- A kernel is as fast as its lanes are full: count them on an instrumented copy before widening;
  a block shorter than the width is not worth the gate (`minWide`, `wideFrom`).
- Columns built always cost the plain path: build them where they pay and lazily (`colsAsked`,
  the crowded cell's `fill`).
- `.Add(zero)` after `Max` turns a −0 into +0 as `math.Max` has it; without it a lane differs
  from the scalar on a ray along an axis.
- The twin files keep every name the callers use (`wide`, `fillWide`, `walkWide`,
  `invertWide`, `visitWide`, `walkConeWide`, `queryWide`): the plain build is what `make test`
  and the plain CI job compile.
- A kernel over simd's vectors is a function, not a method: the compiler's rewrite of simd code
  for each width fails on methods ("missing Types entry"). It has other crashes (a nil pointer
  on a test holding its vectors among `t.Fatalf`s): keep vectors in small functions of their own.
- simd's width is the CPU's: a slice loaded a vector at a time runs `simdx.Pad32` or `Pad64` past
  its data, and a test that lays its own slices pads them so (an AVX-512 runner caught one).
- simd has no move-mask: `simdx.Bits32` is `VMOVMSKPS` through `ToArch` on amd64 and an AND with
  the lane weights and `ADDV` on Neon; a hit loop takes `bits.TrailingZeros64` of it.
- Every kernel reads the cells' layout three times over (plain, AVX2, lanes): a change to how the
  cells are laid out (`spans`, `cellCols`) is made in all three, and `make test-simd` plus
  `make test-simd-emulated` is what tells a copy was missed.
- A test forcing the wide Query turns the sparse list off too (`keepWideQuery`): a sparse
  layout answers from its listed cells before any vector, so a small scene never reaches the
  kernel otherwise.
