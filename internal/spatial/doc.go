// Package spatial is the index behind a Space: a cell grid over a slice of items, answering range
// queries and finding every two items close enough to matter.
//
// # Item, Run and Capability
//
// An [Item] is what the grid is told of: an entity id, its box as the plane holds it, and its
// [Capability], a set of bits a query matches against — any shared bit is a match, [AnyCapability]
// matches all. A [Run] is many of them in the caller's own records: ids beside a column of records
// of one stride, with the box and the capability at offsets in each. The public aabbworld.Item,
// aabbworld.Run and aabbworld.Capability have this exact layout.
//
// # Grid
//
// A [Grid] is told the world by [Grid.RebuildRuns] — runs, then items, at positions counted on
// across them — or [Grid.Rebuild], items alone; either records where the records lie and nothing
// more. The cells are built at the first [Grid.Query] after it, from the boxes as they are then.
// The records must stay put until the next Rebuild, but their boxes may move; [Grid.Invalidate]
// says they have, so the next Query rebuilds the cells. [Grid.Items] lists each position as an
// [Entry], pointers to its box and capability, at the first call after a Rebuild; [Grid.Item] and
// [Grid.EntryAABB] answer by id from it, mapping the ids at the first call too.
//
// [Grid.Pairs] finds every two items sharing a capability whose boxes, grown by a reach of their
// shorter side, touch. It does not read the cells: it sweeps its own counting-sorted layout of the
// grown boxes, listing only the items the caller wants, and reports pairs as positions in the slice.
// The two layouts are kept apart on purpose — each holds only what its reader needs, and one shared
// layout was measured to cost more than it saves (see BENCHMARKS.md).
//
// Both layouts are counting sorts into cells: count the pieces per cell, prefix-sum the counts into
// offsets, deal the pieces out. A box straddling a wrapping seam is listed once per piece, and a
// query or a pair met through more than one piece is reported once, by the cell that owns the
// top-left corner of the overlap.
//
// # Resolution
//
// A [Resolution] names a square grid by its side, a power of two, so a coordinate finds its cell by
// a shift. [ResolutionFrom] rounds a size up to the next power of two; [Resolution.Side],
// [Resolution.MaxCoord] and [Resolution.Cells] read it back.
package spatial
