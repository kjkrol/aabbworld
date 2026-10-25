# Changelog

## v1.17.0 — 2026-10-25

**Numbers**
- Breaking: every number of the geometry is a float32. `geom.Vec{X, Y}` is float32, and with it
  the corners of `geom.AABB` and the corners, `Size` and `Overhang` of `plane.AABB`; so are
  `geom.NewVec`, `geom.NewAABBAt`, `plane.NewAABB`, `AABB.AxisDistanceX/Y`,
  `collide.Config.Reach`, `collide.FieldWall.Half`, every field of `Cone` (`HalfAngle`, `Radius`,
  `Eye`, `GroundStep`, and what `Transparency`, `Elevation` and `Ground` return), the distance
  `View.Entities` hands out, the step of `View.Outline`, and `CoverField.Walk` and
  `ProfileField.Along`. A `plane.AABB` is 32 bytes instead of 64, so a record of gram's (a box, a
  velocity of four float64, a kind and a capability) is 72 bytes instead of 104. Whole units are
  exact up to 2^24; the step between two values is about 0.001 at 10,000.
- A coordinate a hair below zero folds to 0 on a wrapping axis, not onto the far edge, where
  float32 would round it (-1e-7 + 4 is 4).
- Sight works its rays out in float64 and answers in float32. The eye and each ray's direction are
  rounded to float32 first, so the boxes, the cover and the profile are met along the very ray
  `Walk` and `Along` are handed; a ray along cell corners no longer sees a wall's sliver in the
  boxes that the cover walk misses.
- An entity stands on the ground when its bottom lies within a millionth of the ground's height
  (and at least 1e-6) above it, no longer within 1e-6 alone, which float32 heights above 32 cannot
  resolve.

**Index**
- Built with `GOEXPERIMENT=simd` on a CPU with AVX2, the pair sweep tests a crowded cell eight
  boxes at a time instead of four, still from eight entries, and finds the same pairs in the same
  order as the plain sweep. Against v1.16.0, the `BroadPhase` crowd in 256-unit buckets ticks in
  2.32 ms instead of 2.99, the mixed-size `Grid_RebuildPairsQuery` in 4.61 ms instead of 5.24, the
  crowd of 10×10 boxes covering 40% in 4.09 ms instead of 4.60.
- Without the experiment the sweep's entries are 32 bytes, padded from 24 so the scalar pair test
  loads them as it loaded float64 ones; the scenes of small buckets tick as fast or up to 5.5%
  faster, the 256-unit crowd 5% slower.
- In both builds the dense solver is 21% faster (3.54 → 2.78 ms), a crowded column's tick 15.5%
  (3.81 → 3.22 ms), `Column_Move` over runs 5–6%, `Surface.WrapAABB` 55%.
- Scenes of fixed boxes find the same pairs as with float64; random positions rounded to float32
  can make a pair more or fewer (53,867 instead of 53,866 among 33,554 boxes), and a crowd moved
  for a hundred ticks drifts (2,648 contacts a tick instead of 2,659).
- Sight is 3–9.5% slower, most where it samples the ground: every point handed to `Cone.Ground`
  and every height it returns is converted. See [BENCHMARKS.md](BENCHMARKS.md#float32--v1170).

## v1.16.0 — 2026-10-24

**Index**
- Built with `GOEXPERIMENT=simd` on amd64 and run on a CPU with AVX2, the pair sweep behind the
  collision tick tests a cell of eight entries or more four boxes at a time (`simd/archsimd`). It
  finds the same pairs in the same order as before. The `BroadPhase` crowd in 256-unit buckets
  ticks in 3.10 ms instead of 6.95, the mixed-size `Grid_RebuildPairsQuery` in 5.51 ms instead of
  7.95, and sparse cells are unchanged. Without the experiment, or on another CPU, the sweep is
  what it was. `make test-simd` and `make bench-simd` build with the experiment.

**Plane**
- `Space.MoveRun(run, deltas, left)` moves the box of every record of a `Run` by its own delta in
  one call, exactly as `Move` would box by box: a box that stays inside the world is written in
  place with no call per box (`Surface.TranslateRun`), the others go through the edge rules; a
  zero delta leaves its box as it is. It returns `left` with the ids of the moved boxes that have
  left by an open edge, panics when the deltas are not one per record, and allocates nothing when
  `left` has room. `Move` is unchanged. `Benchmark_Column_Move`: 5,000 records 32.5 µs box by box,
  28.7 µs a run at a time; gram's world tick of 5,000 boxes 67.1 → 49.6 µs.

**Sight**
- A warm `Scan` in a world that wraps allocates nothing: the rectangles it searches, split at
  each wrapping seam, cost an allocation per wrapping axis and up to three on a torus. What a
  scan sees is unchanged, bit for bit over 648 scans under all nine edge rules.

**Index**
- `Space.Read()` is the space's `SpaceRead` (`Query`, `Scan`, `Bounds`, `WrapAABB`), safe from
  many goroutines at once while nothing writes the space: the first reader after a `Rebuild` or
  an `Invalidate` builds the cells and the ids' positions under a lock, the others waiting for
  it, and after it a reader pays one atomic load. `SpaceRead.Ready()` indexes the space as that
  first reader would, for a caller to call before fanning readers out, so none of them waits for
  another. `Items` and `Item` build the ids' positions alone, not the cells.

**Tests**
- `TestProfile_SeesWhatAFineStepSees` no longer fails one run in forty: its scene drew heights in
  a map's order, so it changed between runs, and in some a sightline grazed a crest closer than
  the fine step resolves. The scene is drawn in id order, and the profile is held to what a fine
  step can promise.

## v1.14.1 — 2026-10-20

**Index**
- After a `Query` several goroutines may `Scan` a space at once again: the ids' positions, which
  v1.14.0 left to the first `Scan`, are built with the cells as the first `Query` after a Rebuild
  builds them, so a reader that queries once before fanning out (gram's vision) reads only.

## v1.14.0 — 2026-10-20

**Index**
- `Space.RebuildRuns(runs []Run, items []Item)` reads the boxes where they lie in the caller's own
  columns, without a copy. A `Run{IDs, Base, Stride, BoxOff, CapsOff}` is a column of records:
  record i at `Base + i·Stride`, its `plane.AABB` at `BoxOff`, its `Capability` at `CapsOff`,
  named by `IDs[i]`; `RunOf(ids, rows, box, caps)` works the offsets out from a slice of rows.
  The items follow the runs, at the positions after theirs. `Query` and `Scan` read the boxes as
  they are and `collide.Engine.Tick` pushes them in place, so `Moved` and `Left` tell of boxes
  already written. Until the last `Query`, `Scan` or `Tick` before the next Rebuild the memory
  under the runs must hold the same records in the same places: a caller that moves records (an
  ECS migrating entities; goke once a table's `ChunkSnapshot.TableVer` changed) gathers its runs
  again. A box may move between Rebuilds, a capability may not. `Rebuild(items)` is
  `RebuildRuns(nil, items)`.
- What the broad phase returns is unchanged: the broad-phase tests pair the same boxes in the
  same order through items, runs, and runs followed by items, and Query and Scan answer alike.
- A Rebuild no longer maps ids to positions; the first `Scan` after it does.
- Over 5,000 records of 104 bytes, as gram keeps them, a rebuild and eight queries cost 18% less
  through runs than through copied Items (248 → 204 µs); with a collision tick 4–5% less, the
  solver's work unchanged. The old entry costs 1–3% more than in v1.13.0 (a box read through the
  entry's pointer). See
  [BENCHMARKS.md](BENCHMARKS.md#records-in-columns--benchmark_column_rebuildquerytick).

## v1.13.0 — 2026-10-19

**Plane**
- `Surface.Translate` keeps a box that stays inside the world where it is without folding it: the
  same result under every edge rule, a fraction of the cost.

## v1.12.0 — 2026-10-11

**Collisions**
- `collide.WallField`: a `SolidField` with walls at any angle besides its boxes,
  `Walls(id, box, visit(FieldWall) bool)`, asked after the boxes. A `FieldWall{A, B, Half, Cell,
  Open}` is a segment `Half` thick either side; a box is pushed out of it square to it — pieces
  joined end to end slide a box along them as one wall, never catching at a joint — and round a free
  end (`Ends`: `EndA`, `EndB`) where that is the shorter way. A sensor is told and not pushed, a
  `FieldHandler` hears of a cell's boxes and walls once a tick, a warm tick allocates nothing.
  Tests hold a wall along an axis against the box of the same rectangle, open along its long sides
  and at its free ends; see [BENCHMARKS.md](BENCHMARKS.md#solid-ground--benchmark_collide_ground).

## v1.11.0 — 2026-10-11

**Sight**
- `Cone.Profile` is a `ProfileField`: `Along(origin, dir, length, visit(d, alt) bool)`, the ground
  along a ray as the points where its slope changes, and its end. Set, a ray reads the ground off
  it instead of sampling `Ground` every `GroundStep`: a ground of flat facets — a terrain of
  triangles over a grid — is read a few points a cell, and what it hides is exact; where the ray
  meets a box, the ground there, straight between the points either side, hides what lies behind
  it. `Ground` still stands a box on the ground. Tests hold a scan over a profile against the oracle
  reading the same points and against the ground sampled a hundred times as finely.

## v1.10.0 — 2026-10-02

**Collisions**
- `collide.Engine.Left` returns `[]collide.Leaver` — the box's id and the box it came to rest at,
  wholly past the open edge — instead of the ids alone. A box pushed out through an open edge is
  still not `Moved`; its owner writes it back from `Left`. Before, it could not: the push was in
  the index alone, the owner's box stayed where it was, and the next Rebuild put it back over what
  had pushed it out. Breaks the callers of `Left`.

## v1.9.0 — 2026-09-25

The ground on a grid, for sight and for collisions, without entities.

**Sight**
- `Cone.Cover` is a `CoverField`: `Walk(origin, dir, length, visit(near, far, bottom, top, tau)
  bool)`. For every ray the scan asks the owner of a grid for the stretches of the ray inside cells
  with cover and treats them as entities of the ray — a blocking stretch is a wall, a see-through
  one spends the budget, with heights their bands cut sight and cast shadows — but never lists them
  as seen. On a plane the walk stops at the first wall and where the budget runs out.
- With cover the cone is sampled every two degrees and between two samples whose reach jumps, halved
  down to a unit at the radius, so an entity seen through a gap is found.
- Tests hold every scan through cover against the same cells as entities: `Depths` and `Shadows`
  equal, flat and with heights, the view from a cliff past a wall included; what a dense fan of rays
  reaches is seen. Scans without cover measure as in v1.8.0; through cover the cost no longer
  depends on how the cover is laid out — see [BENCHMARKS.md](BENCHMARKS.md#ground-cover--benchmark_view_cover).

**Collisions**
- `collide.Config.Field` is a `collide.SolidField`: `Solid(id, box, visit(FieldBox) bool)`, a
  `FieldBox` being a solid box, its cell and its `Open` sides. In every pass, after the pairs, each
  movable box that moved is pushed out of the solid boxes round it through the shallowest open side,
  so a box never catches on the seam between two cells of a wall; a `Sensor` is told, not pushed.
- `collide.FieldHandler` — `TouchField(id, cell, pen)`, `ContactField(id, cell, pen)` — is asked and
  told once per entity and cell a tick when the Handler implements it.
- The solver without a field measures as in v1.8.0 — see
  [BENCHMARKS.md](BENCHMARKS.md#solid-ground--benchmark_collide_ground).

## v1.8.0 — 2026-09-25

Shadows: the ground out of sight, as holes in a view.

**Sight**
- `View.Shadows(k, dst)` lists, at the same k angles as `Depths`, the stretches of ground the
  observer cannot see within the radius (`Shadow{Sample, From, To}`). With heights every run of
  hidden ground — behind a crest, in a wall's shadow, past a cliff — is one shadow, reaching halfway
  to the lit points either side, or to the radius when none follows; ground seen again farther on
  ends it. On a plane a shadow is the stretch from the reach to the radius. One reach per angle
  could not show ground hidden and then seen again; with heights the reach of sight is not the
  reach of the ground, and a view can now be drawn to its full radius with holes.
- `Depths`, `Outline` and `Entities` are unchanged; `Scan` and every read stay at 0 allocs/op.
  `Shadows` costs what `Depths` costs, and the reads without it measure as in v1.7.0 (six
  alternating runs, p ≥ 0.13 on every row) — see
  [BENCHMARKS.md](BENCHMARKS.md#shadows--benchmark_view_shadows).

## v1.7.0 — 2026-09-24

Sight with heights: an eye, entities with a bottom and a top, ground that rises and falls.

**Sight**
- `Cone.Eye`, `Cone.Elevation func(id) (bottom, top float64)`, `Cone.Ground func(p geom.Vec)
  float64` and `Cone.GroundStep` give sight heights. An entity is seen when the straight line from
  the eye to its top stays above every nearer ground sample, passes through no nearer blocking
  entity's band, and the budget lasts — a see-through entity charges `1/τ` per unit only for the
  stretch the line spends inside its band. Entities are seen whether or not they block: a walker
  under a hawk is seen beside it. `Elevation` nil spans every entity over all heights, `Ground` nil
  is flat ground at 0, `GroundStep` 0 is a sixteenth of the radius; with both funcs nil the scan is
  the one from v1.6.0.
- The reach of an angle (`View.Depths`, `View.Outline`) is the farthest lit ground: the last ground
  sample in sight or the foot of a box standing on the ground. A wall taller than the eye cuts the
  reach at its foot as before; a wall lower than the eye is looked over; a hill hides the plain
  behind it from the lowland and not from a hawk. Over uneven ground the whole cone is sampled
  every two degrees.
- **Changed**: a see-through entity the ray enters within its budget is now listed by
  `View.Entities` — a forest looked into is seen. Casts note what they see on the candidates, so
  `Depths` no longer changes what `Entities` reports.
- `Scan` stays at 0 allocs/op; the scan without heights measures as in v1.6.0 (0–6% over six
  alternating runs, one row at p < 0.05). Against the same scan without heights, bands alone cost
  2.5–2.8×, a ground raster sampled every 32 of the radius 800 3.2–5.2×, at the default step
  2.3–4.1× — the price is the angles and ground points cast, see
  [BENCHMARKS.md](BENCHMARKS.md#sight-with-heights--benchmark_view_elevated).

## v1.6.0 — 2026-09-24

Sight through see-through entities.

**Sight**
- `Cone.Transparency func(id uid.UID64) float64` gives each entity a transparency τ. A ray has a
  budget of the cone's radius: an empty stretch costs its length, a stretch through an entity of
  τ in (0, 1] costs its length divided by τ, an entity at τ ≤ 0 blocks where it is met. A forest at
  τ = 0.5 shortens sight by its depth; a wall cuts it as before. Nil (the default) keeps every
  entity blocking, so existing callers see no change.
- See-through entities shorten the reach but are not listed by `View.Entities`; `View.Depths` and
  `View.Outline` show the shortened reach. The outline behind a see-through entity is sampled every
  two degrees; `Depths` is exact per angle.
- A see-through entity around the observer dims the whole cone from where the observer stands.
- `Scan` stays at 0 allocs/op; the opaque-only scan measures the same as in v1.5.0 (no significant
  difference over six alternating runs), a scene with three entities in ten see-through costs about
  8% more — see [BENCHMARKS.md](BENCHMARKS.md#sight--benchmark_view_).

## v1.5.0 — 2026-09-22

The public API settled since v1.4.0 and is documented end to end.

**Collisions**
- One collision engine: `Space.CollideEngine(handler, collide.Config)` returns a `collide.Engine`
  whose `Tick` pairs every two `CanCollide` boxes within `Reach`, asks the `Handler` to confirm each
  overlap (`Touch`), separates confirmed pairs over `Iterations` passes (`Contact`), reports each
  pushed box once (`Moved`) and lists who left through an open edge (`Left`).
- Simpler solver: a pair is measured again only when one of its boxes has moved.

**Spatial index**
- `Rebuild` is lazy: it records the items and the id map; cells are built at the first `Query`.
  Several rebuilds in a tick cost one indexing.
- The engine invalidates the cells after pushing boxes, so `Query` and `Scan` see the pushed boxes
  without another `Rebuild`.
- Pairs are found by position in the item slice; the engine no longer looks pairs up by id
  (collision tick 2–18% faster on the broad-phase scenes).
- Merging the query cells and the pair sweep into one layout was measured and rejected; the
  reasoning is in [BENCHMARKS.md](BENCHMARKS.md#key-takeaways).

**Project**
- Every package has a `doc.go`; the root one carries the concepts, the tick lifecycle and the
  package graph.
- All benchmarks moved to `bench/` (one `bench_test` package); `Makefile` with `test`, `bench`,
  `bench-save`; results and method in `BENCHMARKS.md`.
- README rewritten around what the library is; the boundary-handling section moved under *Edges*.

## v1.4.0

New public API: `Space`, `Item`, `Capability`, `Edges`, `Config`, `View`, `Cone`; the earlier
`Space2D` implementations became the internal surface behind a `Space`.
