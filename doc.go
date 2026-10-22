// Package aabbworld is a 2D world of axis-aligned boxes: the boxes themselves, a plane with its
// own edge rules, a spatial index over the boxes placed in it, and the questions a simulation asks
// of them each tick — who is near whom, who is pushing whom apart, who sees what.
//
// It began as a library of operations on the boxes (packages geom and plane) and grew upward from
// there: a plane the boxes move on (stop, wrap or open at each edge), a grid that indexes them, a
// collision engine that separates the ones that overlap, and a line-of-sight scan. [Space] is the
// whole of it; geom and plane are its vocabulary. It knows nothing of any entity system: it is told
// the world as a slice and answers questions about it.
//
// # Space and Item
//
// A [Space] is built once by [NewSpace] from a [Config]: the world's size, what happens at its
// edges, and the side of one grid cell. It is then told the world by [Space.Rebuild], as a slice of
// [Item] — whose box it is (a [uid.UID64]), where it lies (a [plane.AABB]), and what may be done with
// it (a [Capability]).
//
// The items are a snapshot the caller owns. A Space keeps the slice it was last handed and reads
// the boxes in it as they are now, so the caller may move them between Rebuilds; what it may not do
// is add, drop or reorder items without a Rebuild. Rebuild itself is cheap: it records the slice,
// and leaves indexing to the first query that needs it, so several Rebuilds in a tick cost one
// indexing.
//
// A caller that keeps its entities in columns of its own hands them over without copying, by
// [Space.RebuildRuns]: a [Run] is the ids of a column and where in each of its records lie the box
// and the [Capability] ([RunOf] works the offsets out from a slice of rows). The Space reads the
// boxes where they lie and a collision engine pushes them there, so the column is the only copy;
// the records must not move between slots until the last question before the next Rebuild.
//
// # Edges
//
// [Edges] says what a box does at each edge of the world, per axis. The zero value stops it whole:
// a box is clamped so it never leaves. [WrapX] and [WrapY] make the axis periodic — a box leaving
// by one edge comes back through the opposite one, and while it straddles the seam it is held as a
// main box plus up to three wrapped pieces (see [plane.AABB.VisitFragments]). [OpenX] and [OpenY]
// let a box leave; once wholly outside it is reported gone by [Space.Place] and [Space.Move] and
// dropped from the index. [Torus] wraps on both axes. An axis cannot both wrap and be open.
//
// [Space.Place], [Space.Move] and [Space.MoveTo] apply those rules to a box; [Space.WrapAABB] folds
// any rectangle into the space the same way.
//
// # Capability
//
// A [Capability] is a set of up to eight bits that says what may be done with an item. Every
// question names the bits it wants and is answered only with items sharing at least one of them;
// [AnyCapability] accepts everything, [Plain] is geometry and nothing more. [CanCollide] is what a
// collision engine pairs up; [Static] marks a side a contact never shifts; [Sensor] marks one that
// is detected but separates nothing.
//
// # Tick lifecycle
//
// One tick of a simulation reads:
//
//  1. Move the boxes: [Space.Move] on each, under the edge rules.
//  2. [Space.Rebuild] with the items as they now stand, or [Space.RebuildRuns] with the runs.
//  3. [collide.Engine.Tick] on the engine built by [Space.CollideEngine]: every two CanCollide boxes
//     within reach of each other are paired, each overlapping pair is put to the handler's Touch,
//     confirmed pairs are separated over a few passes and reported by Contact, and every box that
//     was pushed is reported once by Moved — unless the push took it out through an open edge, in
//     which case it is listed by [collide.Engine.Left] instead, with the box it came to rest at. With collide.Config.Field the
//     engine also pushes every movable box out of the solid ground of a grid, in the same passes.
//  4. Ask: [Space.Query] and [Space.Scan] see the pushed boxes as they are now; no further Rebuild is
//     owed for that.
//
// # Queries and sight
//
// [Space.Query] calls back for every indexed piece intersecting a rectangle, seams included — a box
// straddling a seam may be reported once per piece. [Space.Scan] fills a [View] with what one
// observer sees through a [Cone]: the entities in sight, nearest first ([View.Entities]), the reach
// of sight at evenly spaced angles ([View.Depths]), or the lit region as a fan of points
// ([View.Outline]). A View is a reusable buffer for one goroutine. [Space.Read] queries and scans
// from many goroutines at once while nothing writes the Space.
//
// Sight has a budget of the cone's radius along every angle. An empty stretch costs its length; a
// stretch through an entity of transparency τ costs its length divided by τ; an entity at τ ≤ 0
// blocks sight where it is met. A forest at τ = 0.5 is thus looked through at half the reach, a
// wall is not looked through at all. Cone.Transparency gives τ per entity and is nil by default,
// when everything blocks. Every entity sight reaches is seen, a forest looked into as much as a
// wall looked at.
//
// Sight has heights once Cone.Elevation, Cone.Ground or Cone.Profile is set: the eye is Cone.Eye
// up, an entity spans its bottom to its top, the ground lies at Cone.Ground of a point and is
// sampled every Cone.GroundStep along a ray — or, for a ground of flat facets, Cone.Profile hands
// each ray the points where its slope changes, a few a cell in place of a sample every step, and
// what the ground hides is then exact. An entity is seen when the straight line from the eye to its top
// clears the ground and every nearer blocking entity's band, within the budget — a see-through
// entity charges only the stretch the line spends inside its band. The reach of an angle is the
// farthest lit ground, so a hawk looks over a wall the walker beside it is stopped by, and a hill
// hides the plain behind it from the lowland alone. [View.Shadows] lists the stretches of ground an
// observer cannot see within the radius, so a view with heights can be drawn to its full reach with
// holes where the ground is out of sight. Details are in internal/raycast.
//
// # Ground on a grid
//
// Walls, forests and rock laid out on a grid need not be entities. Cone.Cover is a [CoverField]:
// for every ray the scan asks its owner for the stretches of the ray inside cells with cover —
// where the ray enters and leaves each cell, the band the cover spans, how see-through it is — and
// treats them as it treats entities, budget, walls and shadows alike; they are never listed as
// seen. The owner walks its cells exactly, so on a plane nothing behind a wall is read, and the cost
// of a scan follows the cells the rays cross, not how much cover the world holds or how it is laid
// out. collide.Config.Field is a collide.SolidField: for every movable box the engine asks for the
// solid boxes of the ground around it, with the sides of each that face open ground, and pushes the
// box out only through those — a box sliding along a wall of many cells never catches on a seam.
// One that is a collide.WallField hands walls at any angle besides — a railing along a slanted
// bridge — each a collide.FieldWall a box is pushed out of square to it, round a free end where
// that is shorter. A collide.FieldHandler hears of each contact with the ground once per entity
// and cell a tick.
//
// # Package dependencies
//
// The packages form a strict acyclic graph. Each layer imports only layers below it:
//
//	Layer 0   geom              — the vocabulary: Vec, AABB
//	Layer 1   plane             — AABB with a Size and an Overhang, wrapped pieces, Overlap        (→ geom)
//	Layer 2   internal/plane    — Surface: the edge rules; Translate, Expand, WrapAABB, Left      (→ geom, plane)
//	Layer 3   internal/spatial  — Grid: the items, lazy cells for Query, a sweep for Pairs;
//	                              Capability, Resolution                                          (→ geom, plane, internal/plane)
//	          internal/raycast  — View: a cone scan over any queryable index, shadows and depths  (→ geom)
//	          collide           — the public contract of an engine: Config, Handler, Engine       (→ geom, plane)
//	Layer 4   internal/collide  — Engine over a Grid: Pairs → Solver → Handler; Left               (→ geom, plane, internal/plane, internal/spatial)
//	Layer 5   aabbworld         — Space, Item, Capability, Edges, Config, View, Cone: the package
//	                              you import, wiring the rest together                           (→ all of the above)
//
// Expressed as a directed graph (arrow = "is imported by"):
//
//	geom ──► plane ──► internal/plane ──► internal/spatial ──► internal/collide ──┐
//	  │        │                                                                  ├─► aabbworld
//	  │        └──► collide ──────────────────────────────────────────────────────┤
//	  └───────────► internal/raycast ─────────────────────────────────────────────┘
//
// [github.com/kjkrol/uid] is an external module used throughout for 64-bit generational entity
// identifiers; a Space never mints one, it only names what it was told.
package aabbworld
