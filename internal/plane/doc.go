// Package plane is the surface behind a Space: the edge rules of a bounded plane or a torus, and
// what they do to the public plane's boxes — folding, moving, growing, and telling when one has left.
//
// # Edges
//
// [Edges] is the rule per axis: nothing (a box stops at the edge, clamped whole), [WrapX] or
// [WrapY] (the axis is periodic), [OpenX] or [OpenY] (a box may leave). [Torus] wraps both.
// [Edges.Valid] refuses an axis that both wraps and is open.
//
// # Surface
//
// A [Surface] is a size and its Edges, built by [NewSurface], or by [NewEuclidean2D] and
// [NewToroidal2D] for the two common cases. [Surface.Translate] moves a box by a delta, axis by
// axis: folded back in on a wrapping axis, let go on an open one, clamped on a closed one;
// [Surface.TranslateRun] does it to every box of a column of records by its own delta.
// [Surface.Expand] grows a box by a margin on every side and folds it the same way. [Surface.WrapAABB]
// folds any rectangle into a box the plane holds. [Surface.Left] reports a box wholly past an open
// edge, which a Space then drops from its index.
//
// # Clamp and Wrap
//
// [Clamp] bounds a point to the plane; [Wrap] folds it back in on both axes. They are the scalar
// rules the Surface applies per axis.
package plane
