// Package collide is the contract of a Space's collision engine: what it is told ([Config]), whom it
// reports to ([Handler]), and what a caller drives ([Engine]). Space.CollideEngine of package
// aabbworld builds one over a Space's CanCollide boxes.
//
// # Handler
//
// One [Engine.Tick] talks to the Handler in a fixed order. Touch is asked once for every pair of
// boxes that overlap, with the penetration between them — boxes overlapping by no more than
// float32's rounding of where they lie touch, and are no pair; the handler may confirm the pair as
// it is, refine the penetration (a round shape inside a square box touches later than the box does)
// or veto the pair for this tick, and says what part of the push apart moves the first box, the
// rest moving the second: ½ shares it evenly, 0 leaves the first where it is; a Static box never
// moves whatever the share. Contact is told each confirmed pair once, with the penetration it
// is being separated by. Moved is told every box the engine pushed, once, where it came to rest —
// already written where the Space reads it, so only a caller whose items are a copy writes it back
// to whatever owns it. A box pushed out through an open edge is not Moved; it is listed by
// [Engine.Left] until the next Tick, as a [Leaver] with the box it came to rest at, wholly past the
// edge, written where the Space reads it the same way.
//
// # Config
//
// Reach is how far a box may travel in one tick, as a fraction of its shorter side; boxes whose
// reaches do not touch are never paired, so it bounds the work of a tick. Iterations caps the passes
// one Tick spends on chained overlaps: separating one pair may push a box into a third, which the
// next pass resolves.
//
// # Field
//
// Config.Field is a [SolidField], the solid ground of a world on a grid, which need not be boxes
// in the Space. In every pass, after the pairs, the engine asks it for the solid boxes around each
// movable box (CanCollide, not Static) that moved since it last asked, and pushes the box out of
// each it overlaps through the shallowest side marked open in [FieldBox.Open] — the shallowest
// side at all when none is — so a box sliding along a wall of many cells never catches on the
// seams between them. A field that is also a [WallField] hands the engine its walls at any angle
// besides, after the boxes: a [FieldWall] is a segment Half thick either side, and a box is pushed
// out of it square to it — pieces joined end to end slide a box along them as one wall — or round
// a free end ([FieldWall.Open]) where that is shorter. A Sensor is told but not pushed. A pushed
// box is Moved like any other. A Handler that is also a [FieldHandler] is asked TouchField and told
// ContactField once per entity and cell a tick, boxes and walls of one cell alike; any other
// Handler hears nothing of the ground.
//
// # Engine
//
// An Engine serves one goroutine. Tick reads the boxes of its Space as they are now — the Space
// need only have been Rebuilt since the items changed — and leaves them pushed apart; the Space
// answers its next Query with the pushed boxes on its own.
package collide
