# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

**aabbworld** (module `github.com/kjkrol/aabbworld`) is a public Go library for 2D worlds of
axis-aligned boxes: a plane with edge rules (stop, wrap or open on each axis), a spatial index over
the boxes placed on it, a collision engine separating the ones that overlap, and a line-of-sight
scan. `Space` is the whole of it; `geom` and `plane` are its vocabulary. It has no opinion about
how a simulation stores its entities: it is told the world as a slice of items or as runs over the
caller's own columns, reads the boxes where they lie, and names them by the `uid.UID64` it was
given, never minting one. The concepts, the tick lifecycle and the package graph are the root
`doc.go`; every package has a `doc.go` of its own; the numbers and how they were taken are
`BENCHMARKS.md`; what changed when is `CHANGELOG.md`.

**Read the skill for the layer before working in it** (`.claude/skills`, loaded by their
descriptions): `spatial` (the grid behind a Space: its two layouts, the cells listed while sparse,
the still layer, what the edges promise, the oracles, the traps) and `simd` (the wide kernels:
the gate and the file pattern, the widths and why, the invariants, how a kernel is measured).

## Commands

```bash
go build ./... && go vet ./... && gofmt -l . && go test ./...   # standard verification sequence
go test ./internal/spatial -run TestName -v                     # a single test
make test                 # every test, the plain build (what every other build runs)
make test-simd            # GOEXPERIMENT=simd: every wide kernel this CPU runs, vs the plain code
make test-simd-128        # the same, simd's vectors held to 128 bits (no VPCLMULQDQ, Neon, wasm)
make test-simd-emulated   # built wide with no kernel running (GODEBUG=simd=0)
make vet-simd             # vets the wide build for amd64, arm64 and wasm from any machine
make bench                # every benchmark once, with allocations; bench-simd the wide build
make bench-save           # BENCH_COUNT repeats into bench_results/ (git-ignored); bench-save-simd
```

A change to a kernel or to the cells' layout runs all four test builds and the vet: the plain
build is what CI's first job compiles and what a reader without the experiment gets. CI
(`.github/workflows/simd-amd64.yml`) runs them on amd64 and benchmarks a pull request against its
base on one runner; an arm64 machine runs the suite by hand. Benchmarks live in `bench/`
(package `bench_test`), one scene a sub-benchmark.

## Rules of the house

Decisions made; they are not up for revisiting in passing.

- **The world is a slice, never a copy.** A Space keeps the slice or the runs it was last handed
  and reads the boxes as they are now; the caller moves boxes between Rebuilds and adds, drops or
  reorders records only through one. Nothing in the library owns an entity or a store of its own.
- **No hidden costs.** `Rebuild` records and defers; indexing waits for the first question that
  needs it; the columns the wide kernels read are built only once a cone or a wide query has been
  asked; the plain build pays nothing for the wide code. A path that costs on scenes it does not
  serve is gated or dropped (`BENCHMARKS.md`, "Key takeaways").
- **Every build gives the same answer to the bit.** The pair sweep finds the same pairs in the
  same order, the cone and Query report the same pieces, the scan sees the same entities and
  records the same points, built plain, with AVX2 or on simd's vectors at any width. A kernel is
  held to that by a test run once for every kernel the CPU runs, and earns its place with a
  number in `BENCHMARKS.md` on the scenes it aims at while the sparse ones stay within noise.
- **A world number is a float32.** Every coordinate, size, distance and angle a Space takes or
  gives; a box is 32 bytes. float64 stays inside a computation that sums (a ray's walk), and the
  answer comes back as float32. Two boxes overlapping by a few float32 steps at their edges are
  touching, never pushed apart.
- **The edge rules are one set.** Stop, wrap or open per axis (`Edges`), applied alike to a box
  placed, moved, indexed, queried, paired or scanned, in the moving layer and the still one. A
  box and an area are tested as they truly lie; the grid holds only its cell indices to the cells
  it has. On a wrapping axis the seam's line belongs to both sides.
- **Still is a declaration, never an observation.** What stands is handed to `RebuildStill` and
  indexed once; a box that merely did not move stays a moving box. A still box is paired with the
  moving ones alone and never pushed, whatever its Capability.
- **The public surface is the root package**, `collide` (the engine's contract), `geom` and
  `plane`; everything else is `internal`. A type an internal package needs on the seam is defined
  once with the public layout (`Item`, `Run`, `Capability` in `internal/spatial`), converted by
  the root, never aliased.
- **Nothing of a game.** No entity system, no sprite, no cell kind, no tick loop: a Space answers
  questions about boxes. What a caller does with a pair or a sighting is the caller's.
- **The user tags and publishes releases**, never Claude; a fix reaches a user through a tag and
  their `go.mod` bump.

## Testing conventions

External tests (`package aabbworld_test`, `spatial_test`, ...) with the package's own helpers in
its test files (`hooks`, a Handler of optional funcs; `stillSpace`, `box10`, `found`,
`stillTick`); internal tests only where unexported state must be read (`*_internal_test.go`,
`export_*_test.go` opening a gate to the tests). A test's name is a sentence saying what holds
(`TestTick_PushesAMovingBoxOutOfAStillOneAndNeverTheStillOne`), so it needs no comment. The grid
is checked against a brute force over the true boxes and the true area, never against a clamped
copy; a kernel's check runs once for every kernel the CPU runs (`eachKernel`, `EachKernel`); a
test forcing a kernel turns the sparse list off, or a small scene never reaches it.

## Docs, benchmarks and commits

Every package has a `doc.go` with `# Type` sections: what the package brings and what a caller does
with it, never how it does it inside, no history; the root `doc.go` carries the concepts, the tick
lifecycle and the package graph. README leads with what the library is and keeps its architecture
table in step with the graph. `BENCHMARKS.md` records every number with its scene, its method
(a baseline copy of the tree run alternately with the working one on an idle machine, medians) and
its CPU: the kernels differ by machine, so a contributor's arm64 numbers sit beside the amd64 ones.
`CHANGELOG.md` gets a `## vX.Y.Z — date` section per tag, grouped in bold (**Added**, **Fixed**,
**Changed**, **Improved**, ...), each bullet what it brings and the numbers it moved.

**A comment says only what the code cannot.** An exported type, function, method or field gets at
most two sentences: what it is and what it is for, never how, never its history. Unexported code
and tests get a comment only for a non-obvious why — a unit, a direction, a trap, a source. Never a
comment repeating the name, the signature or the next line.

**Lines stay within 100 columns**, a tab counted as 4; one value a line; a struct literal of many
fields a field a line.

Commit messages use the conventional prefixes (`feat`, `fix`, `docs`, `refactor`, `perf`, `test`,
`chore`; `doc/Contributor_Recommendations.md`), `!` where the API breaks, and carry no
`Co-Authored-By` trailer. Commit and push only when asked. A PR of the maintainer's goes to review
squashed to one commit whose message is the PR's title and body: what it brings, grouped as the
CHANGELOG is, short, no word of the tool that wrote it; a contributor's PR is merged as it comes,
its commits kept. A release is one `docs: vX.Y.Z` commit on `main` dating the CHANGELOG's section,
then the tag, both the user's.
