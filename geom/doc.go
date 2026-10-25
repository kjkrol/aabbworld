// Package geom holds the 2D vocabulary of aabbworld: [Vec], a point or a displacement, and [AABB],
// an axis-aligned box. Both are plain values with no notion of a world or its edges; the packages
// above build those on top.
//
// # Vec
//
// A [Vec] is two float32 coordinates. [Vec.Add] and [Vec.Sub] return a new Vec; [Vec.AddMutable]
// changes one in place for the hot paths that shift a box every tick.
//
// # AABB
//
// An [AABB] is its top-left and bottom-right corner, built by [NewAABB] from both corners or by
// [NewAABBAt] from a position and a size. [AABB.Intersects] is true when two boxes overlap or merely
// touch; [AABB.Contains] when one lies wholly in the other. [AABB.Penetration] is the shortest
// push that separates two overlapping boxes, zero when they do not overlap, and is what the
// collision solver works from. [AABB.AxisDistanceX] and [AABB.AxisDistanceY] are the gap between two
// boxes along one axis.
package geom
