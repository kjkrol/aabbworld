---
name: spatial
description: aabbworld's index and its edges — the grid behind a Space (internal/spatial): two counting-sorted layouts, one for queries and one for the pair sweep, their cells listed while sparse, the columns the wide kernels read; the still layer (RebuildStill) indexed once and paired with the moving boxes alone; what Query, QueryCone and Pairs promise at the world's last unit, past an open edge and on a wrapping seam; the invariants, the oracles that hold them, the traps and the ideas dropped. Use when touching internal/spatial, space.go's Rebuild*, Query or Pairs, the edge rules, or a test comparing the grid with a brute force, and before changing how the cells are laid out.
---

# spatial

What the user sees is `doc.go` ("Space and Item", "Edges", "Tick lifecycle", "Queries and
sight") and `internal/spatial/doc.go`; the numbers are `BENCHMARKS.md` ("Still layer and sparse
cells", "Spatial grid", "Records in columns"); what changed when is `CHANGELOG.md`. The wide
kernels over the cells are the `simd` skill's. This skill is what the code cannot say.

## The model

- A `Grid` is told records, never copies: `RebuildRuns` keeps `columns` over the caller's memory
  (a `Run` a column, the items one more) and reads boxes and capabilities where they lie, at the
  first question that needs them (`stale`, `unlisted`, `Ready`). A record moves only by a
  Rebuild; its box may move any time, `Invalidate` saying so. Positions count on across the
  columns in order, and `Pairs` reports positions, not ids.
- Two layouts, kept apart on purpose: the query cells (`pieces`, `spans`, `cells`) and the pair
  sweep's (`sweep`, the boxes grown by the reach, only the capability wanted). One shared layout
  was measured to cost more than it saves. Both are counting sorts into cells: count a cell's
  pieces, lay the cells out one after another, deal the pieces out last to first.
- `spans` is the counting sort's cells. While no more than one cell in `sparseShare` holds
  anything, the cells holding anything are listed as they are met (`touched`), sorted once at
  `place`, and that list alone is cleared, laid out, visited and asked (`listedIn` by binary
  search); past that share every cell is walked. `occupied` is the count either way, and the
  wide Query's gate reads it.
- A box on a wrapping seam is a main piece and up to three images (`plane.AABB.VisitFragments`,
  `addImage`); a query or a pair met through more than one piece is reported once, by the cell
  that owns the top-left corner of the overlap (`ownedFrom`, `flushSeam`).
- The still layer is a second `Grid` of the same cells (`still`), rebuilt only by `RebuildStill`,
  brought up by the same `Ready`; `Query`, `QueryCone`, `Item` and `EntryAABB` ask both, `Items`
  the moving alone, `StillItems` the still, a still position counted on past the moving ones.
  `Pairs` lays the still boxes out once per `RebuildStill`, reach and want (`laidFor`) and
  crosses the moving sweep with it (`cross`), never still with still. The collide engine takes a
  still item into its batch as `Static` the first time a pair meets it (`hold`), so the solver,
  the field and the reports are untouched, and a tick holds the still boxes it met, not all.

## What the edges promise

- The grid holds a box and an area to its cell indices only (`clamp`: the corners to the
  coordinates there are cells for, false for a box turned inside out); the box and the area
  themselves are tested as they come. So a query meets a box where it truly lies, in the world's
  last unit and past an open edge too, and never a box that merely shares the last cell.
- `Space.Query` asks the grid once per axis span and crosses them (`spans` in `space.go`): on an
  axis that does not wrap the asked range as it is, so an area past a closed edge meets what hangs
  out there and nothing else; on a wrapping axis the folded range, plus the overhang's range from
  0, plus the seam's line when the area ends exactly on the edge or begins exactly at 0 (a box
  ending on `size` has no image; the line is asked for from the other side).
- The same rules for both layers: a still box across a seam stops a moving one on the other side,
  a still box past an open edge is neither found nor paired.
- An entity is still or moving, never both; `RebuildStill(nil, nil)` empties the layer and a
  Space never given one behaves as it did.
- Two boxes meeting exactly on a torus seam, one ending on `size` and one beginning at 0, make no
  pair, and neither do two meeting so anywhere else: boxes sharing an edge are touching, and a
  touching pair is never put to `Touch`. Two overlapping across the seam are paired at every
  reach, the overhang making the image. Checked under #27; not a defect.

## Invariants and the oracles that hold them

- Listed cells answer as every cell walked, the same pairs in the same order:
  `TestSpans_ListedCellsAnswerAsEveryCellWalked`, `_ListedPairsAgreeWithBruteForce`,
  `_ACellListedLastTimeIsEmptyNow`; tests set `sparse = false` to compare.
- The grid against a brute force over the true boxes and the true area, no clamp on either side:
  `inside` in `grid_test.go`, `checkCone` in `cone_query_test.go`; a Space on a torus against a
  brute force over the images: `TestSpace_Query_FindsEveryBoxAcrossTheSeams`; the last unit and
  the seam's line by cases: `TestSpace_Query_InTheWorldsLastUnitMeetsOnlyWhatReachesIntoIt`,
  `_OnTheSeamMeetsWhatBeginsOnItsOtherSide`.
- The still layer: `TestGrid_PairsAStillItemWithEveryMovingOneItTouchesAndNoStillOne`,
  `_QueriesSeeBothLayers`, `_ItemFindsAnIDInEitherLayer`,
  `_RebuildStillReplacesTheStillLayerAndRebuildLeavesIt`, `_StillLayerIsLaidOutAgainForANewReach`;
  through a Space and a tick: `space_still_test.go` (pushed out whole, a still sensor touched and
  pushing nothing, never two still boxes, across the seam, past an open edge).
- Many goroutines read a grid at once while nothing writes it; the first reader after a Rebuild
  or Invalidate brings it up under `mu`: `_ManyGoroutinesAskAFreshGridAsOneDoes`.

## Traps

- Three readers of the cells: the plain loops, the AVX2 kernels and the lanes kernels
  (`*_lanes.go`) all read `spans.entries` and `cells`; a change to the layout is made in all
  three and run under `make test-simd` and `make test-simd-emulated`, or the plain build passes
  and a kernel reads stale starts.
- The sparse list pre-empts the wide paths: a scene small enough to be listed never reaches
  `queryWide` or `walkConeWide`. A test of a kernel turns `sparse` off (`keepWideQuery`); a
  benchmark of a kernel fills its grid.
- `clamp` is for indices. Testing a clamped box or area against the pieces was the cause of #26:
  an area from 511.5 in a world 512 wide met a box ending at 511. An oracle in a test compares
  the true boxes, so a clamp creeping back fails the oracle, not the user.
- The seam's line is not an image: a box ending exactly on `size` is not folded, so a query ending
  there meets it only because `Space.Query` asks for the line from the other side. A new path that
  takes an area to the grid directly repeats the asking or misses the seam.
- The still layer's sweep is laid out for one reach and one want; a tick asking another lays it
  out again (`laidFor`). Two engines of different reach over one Space pay a layout each tick.
- The list is worth it only sorted: unsorted, the cells of a sparse scene lay scattered and a
  view's query was slower than walking every cell; sorted, the sweep visits them in the order the
  full walk did, which is what keeps the pairs' order.
- The cell's claim on a piece or a pair is tested by the cell's first edge (`ownedFrom`), never by
  working the corner's cell out: the conversion kept a zero live across the loop and cost on the
  dense scenes.

## Dropped

- One layout shared by queries and the pair sweep: each holds only what its reader needs.
- A still layer found by diffing the boxes between Rebuilds: it would cost a pass over every
  standing box a step, and a box that happened not to move for a frame would become a wall. Still
  is a declaration (`RebuildStill`), never an observation.
- Pairing still boxes with each other: the overlapping houses of a forest are hundreds of
  contacts a tick between two Static boxes, every one moving neither.
