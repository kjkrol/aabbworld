// Package collide is the engine behind the public collide contract: it finds the overlapping
// items of one grid, separates them and reports as it goes.
//
// # Engine
//
// An [Engine] is built by [New] over a Grid and its Surface, reporting to a [Handler]. Each
// [Engine.Tick] takes the grid's items as they are, asks the grid for every pair within reach, hands
// the pairs to a [Solver], and afterwards reports each pushed item once — as Moved, or as Left, with
// the box it came to rest at, when the push took it wholly out through an open edge. It then tells
// the grid its cells are behind, so
// the next Query sees the pushed boxes.
//
// # Solver
//
// The [Solver] holds a batch of [Pair] values — two positions among the items and flags saying
// which side is [StaticA] or [StaticB] and whether the pair is a [Sensor] pair that is only ever
// reported. [Solver.Solve] sweeps the batch for up to a number of passes: each overlapping pair is
// put to the [Touch] callback once, and a confirmed pair is pushed apart along its penetration, the
// whole way by the non-static side or shared as Touch said, half each without one. A pair is measured again only when one of its
// boxes has moved since, so a settled batch costs one pass. [Solver.VisitMoved] then names every
// item a pass shifted.
//
// # Handler
//
// [Handler] mirrors the public collide.Handler method for method. It is declared again here
// because an internal package does not import the public one; the public Space passes its
// handler straight through.
package collide
