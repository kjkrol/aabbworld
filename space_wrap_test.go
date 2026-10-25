package aabbworld

import (
	"testing"

	"github.com/kjkrol/aabbworld/geom"
	"github.com/kjkrol/aabbworld/plane"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpace_WrapAABB_DelegatesAndFragments(t *testing.T) {
	cfg := Config{
		Width: 100, Height: 100,
		Edges:      Torus,
		BucketSize: 8,
	}
	space, err := NewSpace(cfg)
	require.NoError(t, err)

	overflowing := geom.AABB{TopLeft: geom.NewVec(95, 95), BottomRight: geom.NewVec(105, 105)}

	wrapped := space.WrapAABB(overflowing)

	frags := 0
	wrapped.VisitFragments(func(plane.FragPosition, geom.AABB) bool {
		frags++
		return true
	})
	assert.Equal(t, 3, frags, "expected a box overflowing both edges to wrap into three fragments")
}

func TestSpace_WrapAABB_EuclideanClips(t *testing.T) {
	cfg := Config{
		Width: 100, Height: 100,
		Edges:      0,
		BucketSize: 8,
	}
	space, err := NewSpace(cfg)
	require.NoError(t, err)

	overflowing := geom.AABB{TopLeft: geom.NewVec(95, 95), BottomRight: geom.NewVec(105, 105)}

	wrapped := space.WrapAABB(overflowing)

	assert.Equal(t, float32(100), wrapped.BottomRight.X, "expected BottomRight.X clipped to the world edge")
	assert.Equal(t, float32(100), wrapped.BottomRight.Y, "expected BottomRight.Y clipped to the world edge")
}
