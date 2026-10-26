# ⏱️ aabbworld Benchmarks

[← Back to README](./README.md)

> What each benchmark measures, the numbers, and what they taught us about the design.

## Environment

- **CPU:** Intel(R) Core(TM) i5-8265U CPU @ 1.60GHz (4 cores, 8 threads)
- **Go version:** 1.27.1
- **OS:** Linux

Every table below is the float32 tree of v1.17.0, the **median of 10 rounds** of the whole suite,
run alternately with the v1.16.0 tree, 2026-10-24, unless its section says otherwise. This machine
drifts by up to ~10% between runs, so a change is judged by running the old and the new tree
**alternately**, never one run each; see [How to benchmark](#how-to-benchmark).

All benchmarks live in [`bench/`](bench/) and drive the library through its importable API only.
"0 allocs/op" throughout means a steady tick allocates nothing; the non-zero B/op on the crowd
benchmarks is the amortised growth of buffers the grid, the solver and the view keep between calls.

## Broad phase — `Benchmark_BroadPhase`

Two ways of finding who is near whom in a crowd of `CanCollide` boxes on a 2048×2048 torus:

- **probe-per-entity**: one `Space.Query` per entity over its box grown by the reach; the
  straightforward way to do a broad phase from outside.
- **tick**: one `Engine.Tick` whose `Touch` refuses every pair, so it measures pairing plus one
  overlap test per pair and nothing moves.

The boxes are scattered at random, so some overlap. "Covering 20%" means the boxes' total area is
20% of the world's: 8,388 boxes of 10×10 cover 838,800 of the torus's 4,194,304 square units. The
scenes are sized to hold that coverage constant across box sizes, so the same crowding is measured
with different boxes. Bucket size is what a caller sizing buckets from its largest entity would pick.

| Scene | bucket | probe-per-entity | pairs found | tick | overlaps found |
|:---|---:|---:|---:|---:|---:|
| 8,388 boxes, 10×10 each, covering 20% of the world | 32 | 1.99 ms | 13,471 | **1.69 ms** | 3,342 |
| 16,777 boxes, 10×10 each, covering 40% | 32 | 6.11 ms | 53,723 | **5.66 ms** | 13,398 |
| 33,554 boxes, 5×5 each, covering 20% | 16 | 8.47 ms | 53,867 | **7.52 ms** | 13,372 |
| 12,000 boxes 8×8 plus 20 boxes 100×100 | 256 | 13.40 ms | 18,571 | **7.30 ms** | 5,005 |

The tick wins everywhere, and by most in the mixed-size crowd: its sweep places each box by its own
grown size, while a per-entity probe over 256-unit buckets scans every neighbour of every 100-box.

## Spatial grid — `Benchmark_Grid_RebuildPairsQuery`

One tick of the index alone, on a 1024×1024 torus with 16-unit cells: every box moves a little,
`Rebuild`, `Pairs` at reach 0.5 over the two thirds of items that collide, and eight 100×100
`Query` probes.

| Scene | per tick |
|:---|---:|
| 2,000 boxes, 5×5 each | 222 µs |
| 8,388 boxes, 5×5 each | 1.12 ms |
| 8,388 boxes of mixed sizes: 4×4, 5×5, 16×16 and 60×60 | 7.92 ms |

Profile of the 8,388 boxes 5×5 case, taken with float64 boxes: building the query cells ≈31%,
building the pair sweep ≈24%, testing pairs ≈32%, the eight queries ≈2%.

## Wide pair sweep — `GOEXPERIMENT=simd`

This is v1.16.0, with float64 boxes four at a time; since v1.17.0 the sweep takes eight float32
boxes at a time, see [float32](#float32--v1170).

Built with `GOEXPERIMENT=simd` on amd64, and on a CPU with AVX2, the pair sweep tests a cell of
eight entries or more four boxes at a time (`simd/archsimd`). Median of 10 runs of each build, run
alternately, 2026-10-23; this session's machine drifted ±7%, so "~" marks a difference within it.

| Scene | plain build | `GOEXPERIMENT=simd` |
|:---|---:|---:|
| `BroadPhase` tick, 12,000 boxes 8×8 plus 20 boxes 100×100, bucket 256 | 6.95 ms | **3.10 ms** (−55%) |
| `Grid_RebuildPairsQuery`, 8,388 boxes of mixed sizes | 7.95 ms | **5.51 ms** (−31%) |
| `BroadPhase` tick, 16,777 boxes 10×10 covering 40%, bucket 32 | 5.90 ms | **4.77 ms** (−19%) |
| `BroadPhase` tick, 8,388 boxes 10×10 covering 20%, bucket 32 | 1.73 ms | 1.64 ms (~) |
| `BroadPhase` tick, 33,554 boxes 5×5 covering 20%, bucket 16 | 7.95 ms | 7.62 ms (~) |
| `Grid_RebuildPairsQuery`, 8,388 boxes 5×5 | 1.15 ms | 1.18 ms (~) |
| `Column_RebuildQueryTick`, `Collide_Ground` | | ~ |

In the first scene the pair tests were 83% of the tick: a 256-unit cell holds about 190 entries,
so a tick makes some 1.2 million box tests. With 16- or 32-unit cells of two to four entries the
time goes to building the cells, which SIMD does not touch.

What the work taught us:

* **One pair at a time gains nothing.** A box's four numbers fill one 256-bit register, but
  comparing two boxes needs a lane shuffle and a mask extraction that cost what the four scalar
  comparisons do. The gain is one box against the next four: the four boxes' edges load from four
  columns, four compares and one `VMOVMSKPD` give four bits, with no branch to mispredict.
* **Columns only where they pay.** Laying every cell out in columns cost the plain build +7…10% on
  the sparse scenes: six scattered stores per piece instead of one. The cells stay rows of
  structs; a crowded cell is copied into columns just before its wide sweep, O(n) against its
  O(n²) tests.
* **The cell's claim goes into the same pass.** A pair belongs to the cell holding the top-left
  corner of its overlap. In the mixed scene that scalar check was 16% of the tick, since a 60×60
  box meets its neighbours in many cells. Both entries reach the cell, so the corner cannot lie past
  it: one lower bound per axis is the whole check.
* **Eight entries is the threshold.** Two and four did as well on the crowd; sixteen lost half the
  gain in the mixed scene.
* **The compiler does not clear the upper halves.** After 256-bit code every SSE instruction of the
  scalar code pays a false dependency until a `VZEROUPPER`; the sweep issues one before calling
  back and on return.
* **AVX2, not AVX.** Joining the masks compiles to `VPAND` on 256 bits.
* **The corners of a box in one register did not pay.** Finding a piece's four cells with one
  convert and shift instead of four was +4% on the sparse scenes: building the cells is bound by
  memory, and the trip through a vector costs more than it saves.

## float32 — v1.17.0

Since v1.17.0 every coordinate is a float32, so a `plane.AABB` is 32 bytes instead of 64. The
float64 tree (v1.16.0) and the float32 tree, each built plain and with `GOEXPERIMENT=simd`, were run
alternately over the whole suite, 10 rounds, 2026-10-24.

| Scene | build | float64 (v1.16.0) | float32 (v1.17.0) | change |
|:---|:---|---:|---:|:---|
| `BroadPhase` tick, 12,000 boxes 8×8 plus 20 boxes 100×100, bucket 256 | plain | 6.97 ms | 7.30 ms | 5% slower |
| | simd | 2.99 ms | **2.32 ms** | 22% faster |
| `BroadPhase` tick, 16,777 boxes 10×10 covering 40%, bucket 32 | plain | 5.86 ms | 5.66 ms | 3.5% faster |
| | simd | 4.60 ms | **4.09 ms** | 11% faster |
| `BroadPhase` tick, 33,554 boxes 5×5 covering 20%, bucket 16 | plain | 7.95 ms | 7.52 ms | 5.5% faster |
| | simd | 7.50 ms | **6.65 ms** | 11% faster |
| `BroadPhase` tick, 8,388 boxes 10×10 covering 20%, bucket 32 | plain | 1.69 ms | 1.69 ms | ~ |
| | simd | 1.57 ms | 1.51 ms | 4% faster |
| `Grid_RebuildPairsQuery`, 8,388 boxes of mixed sizes | plain | 7.94 ms | 7.92 ms | ~ |
| | simd | 5.24 ms | **4.61 ms** | 12% faster |
| `Grid_RebuildPairsQuery`, 8,388 boxes 5×5 | plain | 1.12 ms | 1.12 ms | ~ |
| | simd | 1.11 ms | 1.08 ms | 3% faster |
| `Solver_Dense`, 13,547 pairs, 3,763 contacts | plain | 3.54 ms | **2.78 ms** | 21% faster |
| | simd | 3.53 ms | **2.79 ms** | 21% faster |
| `Solver_Field`, 9,850 neighbour pairs | plain | 223 µs | 210 µs | 6% faster |
| | simd | 251 µs | 236 µs | 6% faster |
| `Column_RebuildQueryTick`, crowd of 5,000, runs | plain | 3.81 ms | **3.22 ms** | 15.5% faster |
| | simd | 3.75 ms | **3.17 ms** | 15.5% faster |
| `Column_RebuildQueryTick`, spread of 5,000, runs | plain | 1.08 ms | 1.09 ms | 1% slower |
| | simd | 1.02 ms | 0.97 ms | 5% faster |
| `Column_Move`, 5,000 records a run at a time | plain | 28.8 µs | 27.3 µs | 5% faster |
| | simd | 28.8 µs | 27.1 µs | 6% faster |
| `Collide_Ground`, a field of cells | plain | 80.4 µs | 79.9 µs | ~ |
| | simd | 79.3 µs | 77.3 µs | 2.5% faster |
| `Surface_WrapAABB`, torus | plain | 28.7 ns | **12.9 ns** | 55% faster |
| `Vec_Wrap`, just below zero | plain | 6.61 ns | 7.16 ns | 8% slower |
| `View_Visible`, 1,000 entities | plain | 25.5 µs | 26.4 µs | 3% slower |
| `View_Both`, 1,000 entities | plain | 28.9 µs | 30.3 µs | 5% slower |
| `View_Cover`, a forest of cover on 30% of the cells | plain | 14.4 µs | 15.1 µs | 5% slower |
| `View_Elevated`, 1,000 entities, no heights | plain | 31.2 µs | 32.6 µs | 5% slower |
| `View_Elevated`, 1,000 entities, a raster ground | plain | 94.9 µs | 101.7 µs | 7% slower |
| `View_Shadows`, 1,000 entities, a raster ground | plain | 119.3 µs | 130.6 µs | 9.5% slower |

Sight and the primitives do not use SIMD, so their rows are the plain build alone; "~" is a
difference within the noise (p > 0.05). The ground computed as a sine on every call, which no caller
of ours uses, was left out.

Every scene of fixed boxes finds the same pairs and overlaps. Two drift by rounding, as they must:
the 33,554 boxes scattered at random find 53,867 pairs instead of 53,866, one box rounded onto its
neighbour; the crowd of 5,000 moving for a hundred ticks before it is measured meets 2,648 contacts
a tick instead of 2,659.

The buffers the grid, the solver and the view keep are smaller too: B/op is lower in every
benchmark where it is not zero, by up to 38%. What the work taught us:

* **Half the bytes pays where boxes are copied and read.** The solver of a dense crowd is 21%
  faster, folding a box onto the torus 55%, a crowded column's tick 15.5%, as much in the plain
  build as with SIMD. Where the work is the branches of a scan, the size of a number does not
  matter.
* **Eight boxes a register, eight entries a cell.** With float32 a 256-bit register holds eight
  edges, so the wide sweep tests one box against the next eight. Starting it at sixteen entries
  instead of eight lost 11–36% on the scenes of small buckets and 31% on the mixed grid.
* **A 24-byte entry cost the plain sweep a third.** amd64 has no float32 load from an index scaled
  by 8, so with 24-byte entries the compiler put an address computation before each of the eight
  loads of the scalar pair test: the 256-bucket crowd ticked 34% slower than with float64. Padded
  to 32 bytes, the index is shifted and every load takes it directly, as the float64 ones did.
* **Sight keeps float64 inside and pays at its edges.** The eye, the rays and every candidate's box
  are float64 while a scan runs, read once from the float32 corners. What is left is the
  conversion of every ground point handed to `Cone.Ground` and of every height it returns: 3–5%
  on a plane, 7–9.5% over a raster ground. Folding a point just below zero pays a test more, so it
  never lands on the far edge.

## Collision solver — `Benchmark_Solver_*`

The solver alone, fed pre-computed pairs.

| Benchmark | Scene | per solve |
|:---|:---|---:|
| `Solver_Field` | a 100×50 lattice of 10×10 boxes, 9,850 neighbour pairs, 1 box in 25 nudged into its neighbour | 210 µs |
| `Solver_Dense` | 8,388 boxes, 5×5 each, crowded on a 1024×1024 torus: 13,547 candidate pairs, 3,763 real contacts | 2.78 ms |

## Sight — `Benchmark_View_*`

An observer on a 4000×4000 plane with 256-unit buckets scans a 90° cone of radius 800.

| Read | 100 entities | 1,000 entities |
|:---|---:|---:|
| `Scan` + `Entities` (nearest first) | 3.04 µs | 26.4 µs |
| `Scan` + `Outline` (lit region as a fan) | 4.49 µs | 29.4 µs |
| `Scan` + both | 4.56 µs | 30.3 µs |
| `Scan` + `Depths` (63 angles) | 5.39 µs | 30.4 µs |
| `Scan` + both, 3 entities in 10 see-through at τ = 0.5 (`View_Translucent`) | 4.60 µs | 32.8 µs |
| `Scan` + both with heights: eye 6 up, 3 in 10 see-through, 1 in 5 flying at 30, ground a 32-unit heightfield sampled every 32 (`View_Elevated`) | 28.2 µs | 102 µs |

The scan dominates; each read of the samples adds a microsecond or two. Sight through see-through
entities (v1.6.0) costs the opaque-only scan nothing measurable — six alternating runs against the
v1.5.0 tree differ by 1–3% with p > 0.3 on every row — and adds about 8% when three entities in ten
are see-through: the extra samples across their spans and the budget walk behind them.

**Sight in SIMD** (2026-10-26, v1.18.0 against `perf/sight-simd`, alternately on an idle machine,
the lower of six runs). The scan asks the grid for the boxes its cone may reach (`Grid.QueryCone`:
the radius, the wedge and the owner cell tested eight pieces at a time from columns of the cells'
edges with `GOEXPERIMENT=simd`, one at a time otherwise), lays before each sweep where every ray
enters and leaves each candidate's box, four rays at a time in float64, and over a `Cone.Heights`
field walks the ground of four rays at a time, a ray to a lane. With 1,000 entities, SIMD build:
`View_Visible` 25.4 → 15.7 µs, `View_Both` 29.1 → 19.3, `View_Depths` 30.0 → 18.8,
`View_Translucent` 32.2 → 21.4, `View_Elevated` `none` 32.2 → 21.4, `entities` 92.7 → 77.3, `raster`
99.2 → 86.8, `raster-step50` 80.6 → 68.1, `View_Shadows` over a raster 122 → 110, `View_Cover` with
entities on 30% forest 46.1 → 32.1; with 100 entities 10–17% less. The plain build gains nearly as
much from the tight cone query alone (`View_Visible` 1,000: 26.1 → 16.8 µs, 100: 3.0 → 2.3).
`BroadPhase` ticks within 0.4% and `Grid_RebuildPairsQuery` within 1%. Eight rays in float32 against
four in float64: within 1% on every row, so the float64 slabs stay (they see the same entities to
the bit; float32 flipped ties on boxes with a face on a cell's line). New: `View_Tick` (a rebuild
and eight scans a tick) and `View_Seam`; `View_Elevated` rungs `profile` and `field`, the same
lattice through a profile's calls and as the cone's own `Heights`: 294 → 242 µs with the field
walked ray by ray, 173 µs walked four rays at a time (the `profile` rung, the control, 294 either
way; 100 entities 84 → 59 µs).

### Sight with heights — `Benchmark_View_Elevated`

The `View_Translucent` scene and cone (90°, radius 800, three entities in ten see-through at τ = 0.5,
`Entities` + `Outline`) climbed rung by rung into heights, every rung measured in the same run as the
scan without them. The eye is 6 up; every fifth entity flies at 30–34, every other stands to 2, the
rest to 12. The ground is a sine 0–10 high, sampled every 32 units along a ray.

| Rung | What is added | 100 entities | × none | 1,000 entities | × none |
|:---|:---|---:|---:|---:|---:|
| `none` | the v1.6.0 cone, the reference | 4.61 µs | 1.0 | 32.6 µs | 1.0 |
| `entities` | `Eye` + `Elevation`, ground flat: bands, each box's span sampled every 2° | 11.9 µs | 2.6 | 95.0 µs | 2.9 |
| `flat` | + `Ground` returning a constant, `GroundStep` 32: the whole cone every 2°, 25 ground points per angle | 30.2 µs | 6.6 | 120 µs | 3.7 |
| `raster` | `Ground` as a lookup in a 32-unit heightfield — what an engine pays | 28.2 µs | 6.1 | 102 µs | 3.1 |
| `raster-step50` | the raster at the default step, radius/16: 16 ground points per angle | 21.7 µs | 4.7 | 81.8 µs | 2.5 |
| `sine` ¹ | `Ground` computed with sin·cos on the spot | 79.7 µs | 17 | 239 µs | 7.2 |

¹ left out of the full run, as no caller of ours computes its ground so; measured in a shorter run
of 200 ms rounds against the float64 tree, its × none against that run's `none`.

Every rung is 0 allocs/op. The cost is what gets sampled: bands alone nearly triple the scan because
every box's span is now cast every two degrees, as a see-through box's was; ground adds the casts
between boxes — 45 angles for 90° — and a horizon and line test per ground point, about 17 ns each
with a cheap `Ground`; the step sets how many points there are (25 at 32, 16 at 50); and the
`Ground` function itself is paid once per point, some 2,500 times a scan here, which is why the sine
rung more than doubles the raster one. The scan without heights measures as in v1.6.0: six
alternating runs against that tree differ by 0–6% and only `Depths` at 1,000 entities clears
p < 0.05 (+6%).

### Shadows — `Benchmark_View_Shadows`

The same scan read two ways at 63 angles: the reach per angle (`Depths`) and the stretches of ground
out of sight (`Shadows`, v1.8.0), on a plane and over the raster rung above.

| Scene | 100 entities | 1,000 entities |
|:---|---:|---:|
| plane, `Depths` | 5.50 µs | 33.9 µs |
| plane, `Shadows` | 5.48 µs | 33.9 µs |
| raster, `Depths` | 49.5 µs | 127 µs |
| raster, `Shadows` | 50.0 µs | 131 µs |

Shadows costs what Depths costs: it is the same walk, the hidden runs noted as the ground points go
by. Keeping that noting out of the way matters — a first cut that split the ground test into two
calls made the height rungs 8–13% slower even with no shadows read; folded back into one call, six
alternating runs against the v1.7.0 tree differ by 1–10% on those rungs with p ≥ 0.13, and by less
on the rest.

### Ground cover — `Benchmark_View_Cover`

A 90° cone of radius 220 scanned into a forest of τ 0.5 on a grid of 20-unit cells, 40 cells a
side, just ahead of the observer — the whole square, or three cells in ten scattered over it — held
three ways: as `Cone.Cover` (v1.9.0), as one entity per cell, and, whole, as one merged entity. The
scan lists what it sees and traces the outline.

| Forest | cover | one entity per cell | one merged entity |
|:---|---:|---:|---:|
| whole, 1,600 cells | 14.9 µs | 321 µs | 5.1 µs |
| three in ten, 480 cells | 15.1 µs | 47 µs | — |

Through cover the scan costs the same whichever way the forest is laid out: each ray reads the cells
it crosses until its budget runs out (on a plane) or its radius (with heights). One entity per cell
costs what its candidates cost and grows with the forest; a merged entity is the cheapest but exists
only where the forest is a rectangle, and has to be rebuilt when a cell of it changes. A first cut
kept a full candidate per stretch of cover and walked every ray to the radius: 24 µs whole, 15.6 µs
scattered. Scans without cover measure as in v1.8.0: six alternating runs, geomean +1%, no row past
+2.7%.

## Solid ground — `Benchmark_Collide_Ground`

One collision tick over 400 boxes resting 2 units into walls from above — a wall along every tenth
row of a 100×100 grid of 20-unit cells, with a gap every tenth cell — the walls held as
`collide.Config.Field` (v1.9.0), as one static entity per run of wall, or as one per cell.

| Ground | per tick |
|:---|---:|
| field | 79.9 µs |
| one static entity per run | 70.4 µs |
| one static entity per cell | 187 µs |

The same walls as `collide.FieldWall`s (v1.12.0), a wall a cell along the row, half a cell thick,
joined to the next, cost a sixth more than the boxes — the test of a wall at any angle is four axes
to the box's two: 93.6 µs a tick against 79.9 µs. The field without walls measures as in v1.11.0.

The field costs a little more than walls merged into runs, and nothing depends on how the walls are
laid out or on rebuilding them when one changes. A wall of one entity per cell also catches: a box
straddling the seam between two of them is pushed sideways both ways and stays in the wall, where the
field pushes it out through the only open side. The solver without a field measures as in v1.8.0.

## Primitives — `Benchmark_AABB_*`, `Benchmark_Surface_*`, `Benchmark_Vec_*`

| Operation | euclidean | toroidal |
|:---|---:|---:|
| `Surface.WrapAABB` (box across the far corner) | 7.7 ns | 12.9 ns |
| `Surface.Expand` (grow by 5 near the far corner) | 10.9 ns | 15.8 ns |
| `Surface.Translate` (across the far edge and back) | 10.9 ns | 13.0 ns |
| `Surface.Translate` (staying inside) | 8.2 ns | 8.2 ns |

`Translate` is measured on 2026-10-19 (v1.13.0), when a box that lands inside the world stopped
being folded: `Translate` keeps it where it is before calling `move`, so inside costs the same
under every edge rule, and across the torus still folds one axis. Before, inside cost 13 ns
euclidean and 16 ns toroidal, across 13 ns and 17 ns; medians of 16 runs, the v1.12.0 tree and
the new one alternately, on an idle machine.

| Operation | ns |
|:---|---:|
| `AABB.Contains`, `AABB.Intersects` | 1.33 |
| `Clamp` a point | 1.43 |
| `Wrap` a point: already inside / one step over / just below zero | 5.3 / 6.7 / 7.2 |
| `Wrap` a point from far out (99,999) | 69 |

## Records in columns — `Benchmark_Column_RebuildQueryTick`

One tick of a space over 5,000 records of 72 bytes, as gram keeps `Base` in a goke column (the
box at offset 0, the capability at 68), handed over two ways: **items** copies every record
into an `Item` and writes the pushed boxes back from `Moved`, as gram did before v1.14.0;
**runs** hands them over as `Run`s of 128 records, about a goke chunk, and the tick pushes them
in place. Every tick moves each record, rebuilds, answers eight 100×100 queries and, but in the
first scene, runs a collision tick that confirms every pair.

| Scene | items | runs |
|:---|---:|---:|
| 5,000 boxes 20×20, 30 apart on a 4000×4000 torus, all moving one way; no tick | 240 µs | 204 µs |
| the same with a collision tick: every box paired with its neighbours, none touching | 1.12 ms | 1.09 ms |
| 5,000 boxes 6×6 over a sixth of a 1024×1024 torus, each its own way; ~2,600 contacts a tick | 3.24 ms | 3.22 ms |

Runs drop the copy and the id map, not the work: indexing reads the 72-byte records and writes
32-byte cells (48 with float64 boxes), and with contacts the solver dominates. The other
benchmarks, through the old entry, are within 1–3% of v1.13.0 by their minima (a box read
through the entry's pointer); their medians drift more under the machine's noise.

## Key takeaways

* **`Rebuild` is lazy, and that is free.** It records the slice and the id map and leaves cell
  building to the first `Query`. The check is one predictable branch per query; the engine
  invalidates the cells after pushing boxes, so queries see the pushes without another rebuild.
* **The grid keeps two layouts on purpose.** Query cells hold every item at its actual size; the
  pair sweep holds only the items the engine wants, grown by their reach. Merging them into one
  reach-grown layout was implemented and measured against a baseline copy, alternately:
  +10…+30% on the grid tick, +4…+13% on the collision tick, +12…+39% on per-entity probes.
  The shared part (a counting sort) is cheap; what costs is per-piece geometry, which a merge does
  not remove, while both readers then scan a superset (pair entries 45k → 90k in the mixed-size
  scene). Two specialised layouts beat one general one.
* **Pairs are reported by position, not id.** The engine used to look each pair's two ids up in the
  grid; reporting positions in the item slice removed that and shrank a sweep entry from 48 to 40
  bytes, for a 2–18% faster collision tick across the broad-phase scenes.
* **Zero allocations on the tick.** Rebuild, Pairs, Solve, Query and Scan all report 0 allocs/op once
  their buffers have grown to the scene.
* **SIMD pays where the tests are many to a cell.** Eight boxes at a time tick the crowd in a third
  of the plain sweep's time (2.32 against 7.30 ms) and do nothing for sparse cells; see
  [Wide pair sweep](#wide-pair-sweep--goexperimentsimd) and [float32](#float32--v1170).
* **float32 halves the boxes.** A `plane.AABB` of 32 bytes instead of 64 makes the solver, folding
  and the crowded column tick faster in every build; sight, which keeps float64 inside, pays 3–9.5%
  for the conversions at its edges.

## How to benchmark

```bash
make bench             # the whole suite once, with allocations
make bench-save        # 5 repeats, raw output under bench_results/ (ignored by git)
make bench-simd        # the suite once, built with GOEXPERIMENT=simd
make bench-save-simd   # bench-save so built, into a file marked -simd
```

`make test-simd` runs the tests built so, comparing the wide sweep with the plain one.

To judge a change on this or any drifting machine, keep a copy of the baseline tree and run the two
alternately, then compare medians:

```bash
cp -r . /tmp/before          # before the change
for i in 1 2; do
  (cd /tmp/before && go test -run xxx -bench . -count 4 ./bench/... | sed 's/^/BEFORE /')
  go test -run xxx -bench . -count 4 ./bench/... | sed 's/^/AFTER  /'
done
```
