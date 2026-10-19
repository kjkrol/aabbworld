# ⏱️ aabbworld Benchmarks

[← Back to README](./README.md)

> What each benchmark measures, the numbers, and what they taught us about the design.

## Environment

- **CPU:** Intel(R) Core(TM) i5-8265U CPU @ 1.60GHz (4 cores, 8 threads)
- **Go version:** 1.27.1
- **OS:** Linux

Every number below is the **median of 5 runs** (`make bench-save`, `-count=5`) at commit `d6d935d`,
2026-09-22. This machine drifts by up to ~10% between runs, so a change is judged by running the
old and the new tree **alternately**, never one run each; see [How to benchmark](#how-to-benchmark).

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
| 8,388 boxes, 10×10 each, covering 20% of the world | 32 | 2.27 ms | 13,471 | **1.78 ms** | 3,342 |
| 16,777 boxes, 10×10 each, covering 40% | 32 | 7.25 ms | 53,723 | **6.20 ms** | 13,398 |
| 33,554 boxes, 5×5 each, covering 20% | 16 | 11.46 ms | 53,866 | **8.61 ms** | 13,372 |
| 12,000 boxes 8×8 plus 20 boxes 100×100 | 256 | 14.69 ms | 18,571 | **7.42 ms** | 5,005 |

The tick wins everywhere, and by most in the mixed-size crowd: its sweep places each box by its own
grown size, while a per-entity probe over 256-unit buckets scans every neighbour of every 100-box.

## Spatial grid — `Benchmark_Grid_RebuildPairsQuery`

One tick of the index alone, on a 1024×1024 torus with 16-unit cells: every box moves a little,
`Rebuild`, `Pairs` at reach 0.5 over the two thirds of items that collide, and eight 100×100
`Query` probes.

| Scene | per tick |
|:---|---:|
| 2,000 boxes, 5×5 each | 242 µs |
| 8,388 boxes, 5×5 each | 1.23 ms |
| 8,388 boxes of mixed sizes: 4×4, 5×5, 16×16 and 60×60 | 8.48 ms |

Profile of the 8,388 boxes 5×5 case: building the query cells ≈31%, building the pair sweep ≈24%,
testing pairs ≈32%, the eight queries ≈2%.

## Collision solver — `Benchmark_Solver_*`

The solver alone, fed pre-computed pairs.

| Benchmark | Scene | per solve |
|:---|:---|---:|
| `Solver_Field` | a 100×50 lattice of 10×10 boxes, 9,850 neighbour pairs, 1 box in 25 nudged into its neighbour | 221 µs |
| `Solver_Dense` | 8,388 boxes, 5×5 each, crowded on a 1024×1024 torus: 13,547 candidate pairs, 3,763 real contacts | 4.25 ms |

## Sight — `Benchmark_View_*`

An observer on a 4000×4000 plane with 256-unit buckets scans a 90° cone of radius 800.

| Read | 100 entities | 1,000 entities |
|:---|---:|---:|
| `Scan` + `Entities` (nearest first) | 3.08 µs | 26.7 µs |
| `Scan` + `Outline` (lit region as a fan) | 4.45 µs | 28.3 µs |
| `Scan` + both | 4.55 µs | 30.1 µs |
| `Scan` + `Depths` (63 angles) | 5.31 µs | 29.3 µs |
| `Scan` + both, 3 entities in 10 see-through at τ = 0.5 (`View_Translucent`) | 4.52 µs | 32.4 µs |
| `Scan` + both with heights: eye 6 up, 3 in 10 see-through, 1 in 5 flying at 30, ground a sine sampled every 32 (`View_Elevated`) | 74.6 µs | 220 µs |

The scan dominates; each read of the samples adds a microsecond or two. Sight through see-through
entities (v1.6.0) costs the opaque-only scan nothing measurable — six alternating runs against the
v1.5.0 tree differ by 1–3% with p > 0.3 on every row — and adds about 8% when three entities in ten
are see-through: the extra samples across their spans and the budget walk behind them.

### Sight with heights — `Benchmark_View_Elevated`

The `View_Translucent` scene and cone (90°, radius 800, three entities in ten see-through at τ = 0.5,
`Entities` + `Outline`) climbed rung by rung into heights, every rung measured in the same run as the
scan without them. The eye is 6 up; every fifth entity flies at 30–34, every other stands to 2, the
rest to 12. The ground is a sine 0–10 high, sampled every 32 units along a ray.

| Rung | What is added | 100 entities | × none | 1,000 entities | × none |
|:---|:---|---:|---:|---:|---:|
| `none` | the v1.6.0 cone, the reference | 4.86 µs | 1.0 | 34.6 µs | 1.0 |
| `entities` | `Eye` + `Elevation`, ground flat: bands, each box's span sampled every 2° | 12.1 µs | 2.5 | 96.0 µs | 2.8 |
| `flat` | + `Ground` returning a constant, `GroundStep` 32: the whole cone every 2°, 25 ground points per angle | 28.8 µs | 5.9 | 119 µs | 3.4 |
| `raster` | `Ground` as a lookup in a 32-unit heightfield — what an engine pays | 25.4 µs | 5.2 | 112 µs | 3.2 |
| `raster-step50` | the raster at the default step, radius/16: 16 ground points per angle | 20.0 µs | 4.1 | 79.7 µs | 2.3 |
| `sine` | `Ground` computed with sin·cos on the spot | 77.8 µs | 16 | 233 µs | 6.7 |

Every rung is 0 allocs/op. The cost is what gets sampled: bands alone triple the scan because every
box's span is now cast every two degrees, as a see-through box's was; ground adds the casts between
boxes — 45 angles for 90° — and a horizon and line test per ground point, about 17 ns each with a
cheap `Ground`; the step sets how many points there are (25 at 32, 16 at 50); and the `Ground`
function itself is paid once per point, some 2,500 times a scan here, which is why the sine rung
doubles the raster one. The scan without heights measures as in v1.6.0: six alternating runs against
that tree differ by 0–6% and only `Depths` at 1,000 entities clears p < 0.05 (+6%).

### Shadows — `Benchmark_View_Shadows`

The same scan read two ways at 63 angles: the reach per angle (`Depths`) and the stretches of ground
out of sight (`Shadows`, v1.8.0), on a plane and over the raster rung above.

| Scene | 100 entities | 1,000 entities |
|:---|---:|---:|
| plane, `Depths` | 6.1 µs | 37.4 µs |
| plane, `Shadows` | 6.8 µs | 38.8 µs |
| raster, `Depths` | 53.6 µs | 136 µs |
| raster, `Shadows` | 52.5 µs | 138 µs |

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
| whole, 1,600 cells | 14.2 µs | 327 µs | 5.2 µs |
| three in ten, 480 cells | 14.5 µs | 47 µs | — |

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
| field | 83 µs |
| one static entity per run | 72 µs |
| one static entity per cell | 184 µs |

The same walls as `collide.FieldWall`s (v1.12.0), a wall a cell along the row, half a cell thick,
joined to the next, cost a tenth more than the boxes — the test of a wall at any angle is four axes
to the box's two (measured on 2026-10-11: 188 µs boxes, 200 µs walls, on a slower day of the
machine). The field without walls measures as in v1.11.0.

The field costs a little more than walls merged into runs, and nothing depends on how the walls are
laid out or on rebuilding them when one changes. A wall of one entity per cell also catches: a box
straddling the seam between two of them is pushed sideways both ways and stays in the wall, where the
field pushes it out through the only open side. The solver without a field measures as in v1.8.0.

## Primitives — `Benchmark_AABB_*`, `Benchmark_Surface_*`, `Benchmark_Vec_*`

| Operation | euclidean | toroidal |
|:---|---:|---:|
| `Surface.WrapAABB` (box across the far corner) | 25.7 ns | 30.6 ns |
| `Surface.Expand` (grow by 5 near the far corner) | 11.2 ns | 16.6 ns |
| `Surface.Translate` (across the far edge and back) | 12 ns | 14 ns |
| `Surface.Translate` (staying inside) | 8 ns | 9 ns |

`Translate` is measured on 2026-10-19 (v1.13.0), when a box that lands inside the world stopped
being folded: `Translate` keeps it where it is before calling `move`, so inside costs the same
under every edge rule, and across the torus still folds one axis. Before, inside cost 13 ns
euclidean and 16 ns toroidal, across 13 ns and 17 ns; medians of 16 runs, the v1.12.0 tree and
the new one alternately, on an idle machine.

| Operation | ns |
|:---|---:|
| `AABB.Contains`, `AABB.Intersects` | 1.43 |
| `Clamp` a point | 1.50 |
| `Wrap` a point: already inside / one step over / just below zero | 5.5 / 6.6 / 7.1 |
| `Wrap` a point from far out (99,999) | 73 |

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

## How to benchmark

```bash
make bench        # the whole suite once, with allocations
make bench-save   # 5 repeats, raw output under bench_results/ (ignored by git)
```

To judge a change on this or any drifting machine, keep a copy of the baseline tree and run the two
alternately, then compare medians:

```bash
cp -r . /tmp/before          # before the change
for i in 1 2; do
  (cd /tmp/before && go test -run xxx -bench . -count 4 ./bench/... | sed 's/^/BEFORE /')
  go test -run xxx -bench . -count 4 ./bench/... | sed 's/^/AFTER  /'
done
```
