# Changelog

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
