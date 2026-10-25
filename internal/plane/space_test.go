package plane

import (
	"github.com/kjkrol/aabbworld/plane"
	"testing"

	"github.com/kjkrol/aabbworld/geom"
)

func TestToroidal2DNormalizeVec(t *testing.T) {
	toroidal := NewToroidal2D(5, 5)
	for _, test := range []struct {
		arg1     geom.Vec
		arg2     geom.Vec
		expected geom.Vec
	}{
		{vec(2, 3), vec(4, 4), vec(1, 2)},
		{vec(1, 2), vec(0, 0), vec(1, 2)},
		{vec(0, 0), vec(6, 6), vec(1, 1)},
		{vec(4, 0), vec(3, 0), vec(2, 0)},
		{vec(3, 4), vec(7, 1), vec(0, 0)},
	} {
		result := test.arg1
		result.AddMutable(test.arg2)
		result = toroidal.normalizeVec(result)
		if !result.Equals(test.expected) {
			t.Errorf("result %v not equal to expected %v", result, test.expected)
		}
	}
}

func TestEuclidean2DNormalizeVec(t *testing.T) {
	euclidean := NewEuclidean2D(9, 9)
	for _, test := range []struct {
		arg1     geom.Vec
		arg2     geom.Vec
		expected geom.Vec
	}{
		{vec(2, 3), vec(4, 4), vec(6, 7)},
		{vec(1, 2), vec(0, 0), vec(1, 2)},
		{vec(0, 0), vec(15, 15), vec(9, 9)},
		{vec(4, 0), vec(9, 0), vec(9, 0)},
		{vec(6, 1), vec(3, 10), vec(9, 9)},
	} {
		result := test.arg1
		result.AddMutable(test.arg2)
		result = euclidean.normalizeVec(result)
		if !result.Equals(test.expected) {
			t.Errorf("result %v not equal to expected %v", result, test.expected)
		}
	}
}

func TestSpace2dNormalizeVec(t *testing.T) {
	toroidal := NewToroidal2D(5, 5)
	v := toroidal.normalizeVec(vec(7, 13))
	expected := vec(2, 3)
	if v != expected {
		t.Errorf("expected normalized vector %v, got %v", expected, v)
	}
}

func vec(x, y float32) geom.Vec {
	return geom.NewVec(x, y)
}

func TestEuclidean2DSpace_TransformBackAndForth(t *testing.T) {
	euclidean := NewEuclidean2D(10, 10)

	box := plane.NewAABB(geom.NewVec(0, 0), 2, 2)

	shift := geom.NewVec(2, 2)
	euclidean.Translate(&box, shift)
	expectAABBState(t, box, geom.NewVec(2, 2), geom.NewVec(4, 4), map[plane.FragPosition][2]geom.Vec{})

	euclidean.Expand(&box, 2)
	expectAABBState(t, box, geom.NewVec(0, 0), geom.NewVec(6, 6), map[plane.FragPosition][2]geom.Vec{})

	euclidean.Expand(&box, -2)
	expectAABBState(t, box, geom.NewVec(2, 2), geom.NewVec(4, 4), map[plane.FragPosition][2]geom.Vec{})

	shift = geom.NewVec(-shift.X, -shift.Y)
	euclidean.Translate(&box, shift)
	expectAABBState(t, box, geom.NewVec(0, 0), geom.NewVec(2, 2), map[plane.FragPosition][2]geom.Vec{})
}

func TestToroidal2DSpace_TransformBackAndForth(t *testing.T) {
	toroidal := NewToroidal2D(10, 10)

	box := plane.NewAABB(geom.NewVec(0, 0), 2, 2)

	shift := geom.NewVec(-1, -1)
	toroidal.Translate(&box, shift)
	expectAABBState(t, box, geom.NewVec(9, 9), geom.NewVec(10, 10), map[plane.FragPosition][2]geom.Vec{
		plane.FRAG_RIGHT:        {geom.NewVec(0, 9), geom.NewVec(1, 10)},
		plane.FRAG_BOTTOM:       {geom.NewVec(9, 0), geom.NewVec(10, 1)},
		plane.FRAG_BOTTOM_RIGHT: {geom.NewVec(0, 0), geom.NewVec(1, 1)},
	})

	toroidal.Expand(&box, 2)
	expectAABBState(t, box, geom.NewVec(7, 7), geom.NewVec(10, 10), map[plane.FragPosition][2]geom.Vec{
		plane.FRAG_RIGHT:        {geom.NewVec(0, 7), geom.NewVec(3, 10)},
		plane.FRAG_BOTTOM:       {geom.NewVec(7, 0), geom.NewVec(10, 3)},
		plane.FRAG_BOTTOM_RIGHT: {geom.NewVec(0, 0), geom.NewVec(3, 3)},
	})

	toroidal.Expand(&box, -2)
	expectAABBState(t, box, geom.NewVec(9, 9), geom.NewVec(10, 10), map[plane.FragPosition][2]geom.Vec{
		plane.FRAG_RIGHT:        {geom.NewVec(0, 9), geom.NewVec(1, 10)},
		plane.FRAG_BOTTOM:       {geom.NewVec(9, 0), geom.NewVec(10, 1)},
		plane.FRAG_BOTTOM_RIGHT: {geom.NewVec(0, 0), geom.NewVec(1, 1)},
	})

	shift = geom.NewVec(-shift.X, -shift.Y)
	toroidal.Translate(&box, shift)
	expectAABBState(t, box, geom.NewVec(0, 0), geom.NewVec(2, 2), map[plane.FragPosition][2]geom.Vec{})
}
