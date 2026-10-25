package geom

import (
	"fmt"
)

// Vec is a 2D vector in float32 coordinates.
type Vec struct{ X, Y float32 }

func NewVec(X, Y float32) Vec { return Vec{X, Y} }

// Add returns a new vector that is the sum of v and v2.
func (v Vec) Add(v2 Vec) Vec { return Vec{v.X + v2.X, v.Y + v2.Y} }

// Sub returns a new vector that subtracts v2 from v.
func (v Vec) Sub(v2 Vec) Vec { return Vec{v.X - v2.X, v.Y - v2.Y} }

// AddMutable adds v2 to v in place.
func (v *Vec) AddMutable(v2 Vec) { v.X += v2.X; v.Y += v2.Y }

// Equals reports whether v and v2 have the same components.
func (v Vec) Equals(v2 Vec) bool { return v.X == v2.X && v.Y == v2.Y }

// String formats v as "(X,Y)".
func (v Vec) String() string { return fmt.Sprintf("(%v,%v)", v.X, v.Y) }
