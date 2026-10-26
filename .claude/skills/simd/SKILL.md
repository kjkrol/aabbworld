---
name: simd
description: aabbworld's SIMD kernels, built with GOEXPERIMENT=simd on amd64 and run on AVX2 — the collision pair sweep eight boxes at a time, sight's cone query eight pieces at a time, its slabs four rays at a time and the heightfield walked four rays in lockstep: the gate and the file pattern, why each is as wide as it is, what stays scalar on purpose, the invariants (the same pairs and sightings to the bit) and the tests that hold them, how much each gives and how to measure it. Use when touching a *_simd.go file or its scalar twin, adding a kernel, chasing a difference between the SIMD and the plain build, or measuring either.
---

# simd

Everything wide here is deliberate, bounded and measured: a kernel earns its place by giving the
plain code's answer and a number in `BENCHMARKS.md`. The user's summary is `doc.go`, "SIMD"; the
numbers and the method are `BENCHMARKS.md` ("Wide pair sweep", "Sight in SIMD", "Key takeaways",
"How to benchmark"); the history is `CHANGELOG.md`. This skill is the practice.

## The gate and the files

- Build: `GOEXPERIMENT=simd` on amd64 (`simd/archsimd`). Run: `var wide = archsimd.X86.AVX2()`
  (`internal/spatial/pair_sweep_simd.go`, `internal/raycast/slab_simd.go`): the masks are joined
  on 256 bits and the broadcasts take AVX2; without it the plain path runs in the SIMD build
  too. `aabbworld.SIMD()` (`simd.go`) reports `spatial.Wide()`.
- A kernel is a file `*_simd.go` under `//go:build goexperiment.simd && amd64` with a twin under
  `//go:build !(goexperiment.simd && amd64)` holding `const wide = false` (or a `walkWide` that
  walks nothing), so the callers read one set of names: `slab_simd.go`/`slab_scalar.go`,
  `heightfield_simd.go`/`heightfield_scalar.go`, `pair_sweep_simd.go`/`pair_sweep_scalar.go`;
  the cone query's plain path is `cone_query.go` itself (`if !wide`). The fork is one `if wide`
  at the caller: `sweep.go` (`walk`), `slab.go` (`fill`, `laid`), `pair_sweep.go`, `grid.go`
  (the columns), `cone_query.go`.
- `archsimd.ClearAVXUpperBits()` before every callback out of a kernel and at its exit (the SSE
  code after it would pay the transition); `export_simd_test.go` opens `wide` to the tests, which
  turn it off to compare the two paths on one scene.

## The kernels

- **The pair sweep** (`internal/spatial/pair_sweep_simd.go`, 2026-10-23): one box against the
  next eight of a crowded cell (`wideFrom` 8 entries), `Float32x8`, the cell's edges in columns
  filled per cell. Where the tests are many to a cell: `BroadPhase` 12,000 boxes 6.95 → 3.10 ms
  (−55%), `Grid_RebuildPairsQuery` −31%; sparse cells nothing.
- **The cone query** (`internal/spatial/cone_query_simd.go`, the sight stage): the pieces of the
  cells under the cone's rectangle tested eight at a time, `Float32x8` — the radius, the wedge
  (a cone narrower than a half-turn), the owner cell — from columns of the cells' edges built
  with the cells once a scan has been asked (`g.coned`; the plain build never pays for them).
  The gather was half a scan: `View_Visible` 1,000 entities 25.4 → 15.7 µs with the slabs; the
  plain build 26.1 → 16.8 from the tight filter alone.
- **The slabs** (`internal/raycast/slab_simd.go`): between two events of a walk the active set
  is fixed, so each active box's `near`/`far` is laid for four angles at a time, `Float64x4`,
  into a scratch buffer `castFlat`/`castElevated` read; blocks under `minWide` (2) go as cast.
  With the query: `View_Visible`/`Both`/`Depths`/`Translucent` −33…37%.
- **The heightfield in lockstep** (`internal/raycast/heightfield_simd.go`): the lattice walk four
  rays at a time, a ray to a lane of `Float64x4` (`Int64x4` for the cells), the corners read lane
  by lane a round ahead (archsimd has no gather) and the points gathered lane by lane after; a
  block whose rays cross more than `maxLaneCells` (4,096) walks as cast. `View_Elevated` `field`
  1,000 entities 242 → 173 µs (−28%), 100 entities 84 → 59; the lanes 95–99% full.

**Why those widths.** Four float64, not eight float32, in the slabs and the walk: the walk must
record the plain walk's points to the bit (each lane does the scalar operations in the scalar
order; `fused`/`mulAdd` probe whether the compiler fuses the plain walk's multiply-adds, as
`GOAMD64=v3` does, and fuse alike), and eight float32 rays flipped ties on boxes with a face on a
cell's line — a systematic thing on a grid — changing the set of entities seen, for ±1% of time.
Float32 where the data already is float32: the cells' edges, the boxes.

**Scalar on purpose**: the crossing order and the budget (`insertCrossing`, `spend`), the cover
(`castCover`, a callback a stretch), `refine` (one ray), grounds without a lattice (`Ground`,
`Profile`: a call a point), the loads of corners and the appends of points (no gather, no
scatter), every callback. A kernel over them would move the cost, not remove it.

## Invariants and the tests that hold them

- The pair sweep finds the same pairs in the same order as the plain sweep:
  `TestPairSweep_WideFindsWhatNarrowFinds`, `_WideAgreesOnEdgesAndNaN`,
  `_WideNeverPairsABoxWithItself`.
- The cone query keeps what the plain filter keeps: `TestQueryCone_WideFindsWhatNarrowFinds`,
  `_WideAgreesOnEdgesAndNaN`, `_ColumnsFollowTheCells`, `_ManyGoroutinesAskAFreshGridAsOneDoes`.
- Every lane of a slab is `hitDistance`: `TestFillWide_EveryLaneIsHitDistance`,
  `TestInvertWide_IsOneOverEach`; the View benchmarks' scenes see exactly what they saw (an
  entity seen over less than the sweep resolves may differ once boxes out of reach add no
  casts: said in the CHANGELOG, accepted).
- The lockstep walk records the plain walk's points to the bit:
  `TestHeights_RaysWalkedSideBySideRecordWhatEachWalksAlone` (random fields, fans of angles, the
  oracle `fieldOracle`); a scan over a field sees what one over its profile sees.
- `make test-simd` runs the suite built wide, `make test` and CI run it plain. A change to a
  kernel runs both, and the walk once under `GOAMD64=v3`.

## Measuring

`make bench-simd` and `make bench-save-simd` beside `make bench`. A change is judged on a baseline
copy of the tree run alternately with the working one on an idle machine, the lower of six runs
(`BENCHMARKS.md`, "How to benchmark"), in both builds — the plain build must not pay for the
columns or the buffers — on the scenes a kernel aims at and on the sparse ones (`BroadPhase`
sparse, `View_Visible` with 100 entities), which must stay within noise. gram's `gram-perf` skill
(`/home/karol/CODE/gram/.claude/skills/gram-perf/`) has the alternating script, `measure.sh`
with `compare.py`, and the fairness rules for the siblings.

## Traps

- AVX2 on every hyperthread lowers the clock: scalar code called from inside a wide scan (a
  callback a point, `castCover`) runs slower under a parallel benchmark than alone — gram's
  sampled-ground regimes +2–6% with the scan in parallel, 2–4% faster on one CPU. Compare
  `-cpu=1` and the plain build before blaming a kernel.
- A kernel is as fast as its lanes are full: count them on an instrumented copy (the lockstep's
  95–99%) before widening; a block shorter than the width is not worth the gate (`minWide`,
  `wideFrom`).
- Columns built always cost the plain path (+7…10% once): build them where they pay and lazily
  (`g.coned`, the crowded cell's `fill`).
- `.Add(zero)` after `Max` turns a −0 into +0 as `math.Max` has it; without it a lane differs
  from the scalar on a ray along an axis.
- The twin files keep every name the callers use (`wide`, `fillWide`, `walkWide`,
  `invertWide`): the plain build is what CI compiles.
