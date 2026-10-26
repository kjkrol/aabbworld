//go:build goexperiment.simd && amd64

package raycast

import (
	"math"
	"simd/archsimd"
	"slices"
	"unsafe"
)

// maxLaneCells caps the cells a ray of a block may cross; past it the rays are walked as cast.
const maxLaneCells = 1 << 12

// walkWide walks the ground along the rays of the block's angles from origin for length side by
// side, cell by cell as walk does along each, and reports whether it could. A round takes every
// ray one cell on, four rays to a vector, each lane in walk's operations and order, so that every
// ray's points are walk's to the bit; the corners are read, and the points gathered, lane by lane.
func (b *profileBlock) walkWide(l *lattice, origin vec, dirs []vec, length float64) bool {
	if len(l.f.Corners) > math.MaxInt32 {
		return false
	}
	k := &b.lanes
	size := float64(l.f.Cell)
	fx, fy := origin.x/size, origin.y/size
	cx, cy := math.Floor(fx), math.Floor(fy)
	ix, iy := wrapCell(int64(cx), l.cellsX, l.wrapX), wrapCell(int64(cy), l.cellsY, l.wrapY)
	fix, fiy := float64(ix), float64(iy)
	most := 0.0
	for j := range 4 {
		dir := dirs[b.at[min(j, b.n-1)]] // a lane past the block's walks the last ray, unrecorded
		bx, by := dir.x/size, dir.y/size
		k.dir[j], k.bx[j], k.by[j] = dir, bx, by
		k.sx[j], k.nx[j], k.dx[j] = crossings(fx, bx)
		k.sy[j], k.ny[j], k.dy[j] = crossings(fy, by)
		k.fx[j], k.fy[j], k.end[j], k.one[j], k.bend[j] = fx, fy, length, 1, l.bend
		k.cols[j], k.rows[j] = float64(l.f.Cols), float64(l.f.Rows)
		k.cellsX[j], k.cellsY[j] = float64(l.cellsX), float64(l.cellsY)
		k.lastX[j], k.lastY[j] = float64(l.cellsX-1), float64(l.cellsY-1)
		// a ray crosses no more edges than it spans along either axis, three points a cell at most
		edges := length*(math.Abs(bx)+math.Abs(by)) + 8
		if !(edges < maxLaneCells) {
			return false
		}
		most = max(most, edges)
		if j < b.n {
			m, w := 3*int(edges)+4, &b.walks[j]
			w.d, w.alt = slices.Grow(w.d[:0], m)[:m], slices.Grow(w.alt[:0], m)[:m]
			w.length = length
			k.entries[j] = k.entries[j][:0]
		}
	}
	k.origin = origin
	k.rounds = slices.Grow(k.rounds[:0], int(most))[:int(most)]
	k.kept = slices.Grow(k.kept[:0], int(most))[:int(most)]
	// The corners are read as integers: an SSE load among the vectors pays for their upper halves.
	c := unsafe.Slice((*uint32)(unsafe.Pointer(unsafe.SliceData(l.f.Corners))), len(l.f.Corners))
	now, next := &k.cells[0], &k.cells[1]
	on := uint8(0)
	if l.on(ix, iy) {
		h0, h1, h2, h3 := cornerBits(c, int64(l.f.Cols), int64(l.f.Rows), ix, iy)
		now.corners = [4][4]uint32{{h0, h0, h0, h0}, {h1, h1, h1, h1}, {h2, h2, h2, h2},
			{h3, h3, h3, h3}}
		on = 0b1111
	}
	now.on = [4]int64{-int64(on & 1), -int64(on & 1), -int64(on & 1), -int64(on & 1)}
	k.ix, k.iy, k.t = [4]float64{fix, fix, fix, fix}, [4]float64{fiy, fiy, fiy, fiy}, [4]float64{}
	k.cx, k.cy = [4]float64{cx, cx, cx, cx}, [4]float64{cy, cy, cy, cy}
	k.bent = l.bend != 0
	alive, ended, rounds := uint8(1)<<b.n-1, uint8(0), 0
	for ; alive != 0; rounds++ {
		var zero archsimd.Float64x4
		one := archsimd.LoadFloat64x4Array(&k.one)
		sx, sy := archsimd.LoadFloat64x4Array(&k.sx), archsimd.LoadFloat64x4Array(&k.sy)
		nx, ny := archsimd.LoadFloat64x4Array(&k.nx), archsimd.LoadFloat64x4Array(&k.ny)
		acrossX := nx.LessEqual(ny)
		across := acrossX.ToBits()
		exit := nx.IfElse(acrossX, ny)
		nx.Add(archsimd.LoadFloat64x4Array(&k.dx)).IfElse(acrossX, nx).StoreArray(&k.nx)
		ny.IfElse(acrossX, ny.Add(archsimd.LoadFloat64x4Array(&k.dy))).StoreArray(&k.ny)

		// The cells the rays step into are read now for the next round, so that no vector loads
		// what was stored a moment before: where in the field each corner lies (the first corner
		// off the lattice), then the corners lane by lane.
		ixs, iys := archsimd.LoadFloat64x4Array(&k.ix), archsimd.LoadFloat64x4Array(&k.iy)
		ixs = ixs.Add(sx).IfElse(acrossX, ixs)
		iys = iys.IfElse(acrossX, iys.Add(sy))
		if l.wrapX {
			ixs = wrapLanes(ixs, &k.cellsX, &k.lastX)
		}
		if l.wrapY {
			iys = wrapLanes(iys, &k.cellsY, &k.lastY)
		}
		ixs.StoreArray(&k.ix)
		iys.StoreArray(&k.iy)
		cols := archsimd.LoadFloat64x4Array(&k.cols)
		lies := ixs.GreaterEqual(zero).And(ixs.Less(archsimd.LoadFloat64x4Array(&k.cellsX))).
			And(iys.GreaterEqual(zero)).And(iys.Less(archsimd.LoadFloat64x4Array(&k.cellsY)))
		x1, y1 := ixs.Add(one), iys.Add(one)
		x1 = zero.IfElse(x1.Equal(cols), x1)
		y1 = zero.IfElse(y1.Equal(archsimd.LoadFloat64x4Array(&k.rows)), y1)
		top, bottom := iys.Mul(cols), y1.Mul(cols)
		top.Add(ixs).Masked(lies).ConvertToInt32().StoreArray(&k.index[0])
		top.Add(x1).Masked(lies).ConvertToInt32().StoreArray(&k.index[1])
		bottom.Add(ixs).Masked(lies).ConvertToInt32().StoreArray(&k.index[2])
		bottom.Add(x1).Masked(lies).ConvertToInt32().StoreArray(&k.index[3])
		lies.ToInt64x4().StoreArray(&next.on)
		was := on
		on = lies.ToBits()
		at, hn := &k.index, &next.corners
		hn[0][0], hn[0][1], hn[0][2], hn[0][3] = c[at[0][0]], c[at[0][1]], c[at[0][2]], c[at[0][3]]
		hn[1][0], hn[1][1], hn[1][2], hn[1][3] = c[at[1][0]], c[at[1][1]], c[at[1][2]], c[at[1][3]]
		hn[2][0], hn[2][1], hn[2][2], hn[2][3] = c[at[2][0]], c[at[2][1]], c[at[2][2]], c[at[2][3]]
		hn[3][0], hn[3][1], hn[3][2], hn[3][3] = c[at[3][0]], c[at[3][1]], c[at[3][2]], c[at[3][3]]

		cxs, cys := archsimd.LoadFloat64x4Array(&k.cx), archsimd.LoadFloat64x4Array(&k.cy)
		ax := archsimd.LoadFloat64x4Array(&k.fx).Sub(cxs)
		ay := archsimd.LoadFloat64x4Array(&k.fy).Sub(cys)
		cxs.Add(sx).IfElse(acrossX, cxs).StoreArray(&k.cx)
		cys.IfElse(acrossX, cys.Add(sy)).StoreArray(&k.cy)
		bx, by := archsimd.LoadFloat64x4Array(&k.bx), archsimd.LoadFloat64x4Array(&k.by)
		lattice := archsimd.LoadInt64x4Array(&now.on)
		h0 := cornerLanes(&now.corners[0], lattice)
		h1 := cornerLanes(&now.corners[1], lattice)
		h2 := cornerLanes(&now.corners[2], lattice)
		h3 := cornerLanes(&now.corners[3], lattice)

		s := clamp01Wide(mulAdd(ay.IfElse(acrossX, ax), by.IfElse(acrossX, bx), exit), zero, one)
		east, south := sx.Greater(zero), sy.Greater(zero)
		ea := h1.IfElse(east, h0).IfElse(acrossX, h2.IfElse(south, h0))
		eb := h3.IfElse(east, h2).IfElse(acrossX, h3.IfElse(south, h1))
		alt := mulAdd(ea, s, eb.Sub(ea))
		flips := was ^ on
		if flips != 0 {
			s.StoreArray(&k.s)
			alt.StoreArray(&k.alt)
			exit.StoreArray(&k.exit)
		}
		end := archsimd.LoadFloat64x4Array(&k.end)
		r := &k.rounds[rounds]
		xd, xalt := k.point(exit, alt)
		xd.StoreArray((*[4]float64)(r[8:12]))
		xalt.StoreArray((*[4]float64)(r[12:16]))
		skipX, goX := xd.LessEqual(zero).ToBits(), xd.Less(end).ToBits()
		ends := exit.GreaterEqual(end).ToBits()

		limit := end.IfElse(end.Less(exit), exit)
		along03 := h0.Sub(h3).Abs().Less(h1.Sub(h2).Abs())
		// Along the fold's diagonal the divisor is 0, the quotient no finite fold: none in range.
		fold := ay.Sub(ax).IfElse(along03, one.Sub(ax).Sub(ay)).
			Div(bx.Sub(by).IfElse(along03, bx.Add(by)))
		folds := fold.Greater(archsimd.LoadFloat64x4Array(&k.t)).And(fold.Less(limit)).ToBits()
		exit.StoreArray(&k.t)
		u := clamp01Wide(mulAdd(ax.IfElse(along03, ay), bx.IfElse(along03, by), fold), zero, one)
		fa, fb := h0.IfElse(along03, h1), h3.IfElse(along03, h2)
		fd, falt := k.point(fold, mulAdd(fa, u, fb.Sub(fa)))
		fd.StoreArray((*[4]float64)(r[0:4]))
		falt.StoreArray((*[4]float64)(r[4:8]))
		skipF, goF := fd.LessEqual(zero).ToBits(), fd.Less(end).ToBits()

		// take's rules a lane at a time: a point at 0 or before is passed over, one at the end ends
		// the walk; a fold lies before the limit and an exit recorded before the end, so neither
		// past the end.
		f := alive & folds &^ skipF
		alive &^= f &^ goF
		e := alive & ends
		alive &^= e
		ended |= e
		x := alive &^ skipX
		alive &^= x &^ goX
		kept := uint16(f) | uint16(x)<<4
		if flips &= alive; flips != 0 {
			kept |= uint16(b.enter(l, flips, across, next)) << 8
		}
		k.kept[rounds] = kept
		now, next = next, now
	}
	archsimd.ClearAVXUpperBits()
	b.record(l, rounds, ended)
	return true
}

// record gathers each lane's points out of the log of its rounds, and then the end of every lane
// in ended.
func (b *profileBlock) record(l *lattice, rounds int, ended uint8) {
	k := &b.lanes
	kept := k.kept[:rounds]
	log := k.rounds[:len(kept)]
	for j := range len(b.at) {
		if j >= b.n {
			break
		}
		w, entries := &b.walks[j], k.entries[j]
		d, n := w.d, 0
		alt := w.alt[:len(d)]
		for i, bits := range kept {
			r := &log[i]
			bits >>= j
			d[n], alt[n] = r[j], r[4+j]
			n += int(bits & 1)
			d[n], alt[n] = r[8+j], r[12+j]
			n += int(bits >> 4 & 1)
			if bits>>8&1 != 0 {
				d[n], alt[n] = entries[0], entries[1]
				n, entries = n+1, entries[2:]
			}
		}
		w.d, w.alt = d[:n], alt[:n]
		if ended>>j&1 != 0 {
			l.end(w, k.origin, k.dir[j])
		}
	}
}

// cornerBits is corners of a cell on a lattice of cols by rows corners as float32 bits, read off
// c, the corners as integers.
func cornerBits(c []uint32, cols, rows, x, y int64) (h0, h1, h2, h3 uint32) {
	x1, y1 := x+1, y+1
	if x1 == cols {
		x1 = 0
	}
	if y1 == rows {
		y1 = 0
	}
	top, bottom := y*cols, y1*cols
	return c[top+x], c[top+x1], c[bottom+x], c[bottom+x1]
}

// on reports whether cell (x, y) lies on the lattice.
func (l *lattice) on(x, y int64) bool {
	return uint64(x) < uint64(l.cellsX) && uint64(y) < uint64(l.cellsY)
}

// enter records, for every lane in flips, where its ray enters the cell it stepped into, onto the
// lattice or off it, where that differs from where it left the last, as walk does, and reports
// the lanes that recorded it; next holds the cells. Between two cells on the lattice, or two off
// it, the two never differ.
func (b *profileBlock) enter(l *lattice, flips, across uint8, next *cells) uint8 {
	k := &b.lanes
	var got uint8
	for j := range 4 {
		if flips>>j&1 == 0 {
			continue
		}
		var h [4]float64
		if next.on[j] != 0 {
			for q := range h {
				h[q] = float64(math.Float32frombits(next.corners[q][j]))
			}
		}
		entry := entered(h[0], h[1], h[2], h[3], across>>j&1 != 0, k.sx[j], k.sy[j], k.s[j])
		if entry == k.alt[j] {
			continue
		}
		// as point and take record it: past the exit before it, which went on, it goes on too
		if d32 := float32(k.exit[j]); d32 > 0 {
			k.entries[j] = append(k.entries[j], float64(d32), float64(l.sunk(float32(entry), d32)))
			got |= 1 << j
		}
	}
	return got
}

// wrapLanes is stepCell's wrap of cells v onto an axis of n cells, the last of them last.
func wrapLanes(v archsimd.Float64x4, n, last *[4]float64) archsimd.Float64x4 {
	var zero archsimd.Float64x4
	wrapped := archsimd.LoadFloat64x4Array(last).IfElse(v.Less(zero), v)
	return zero.IfElse(v.Equal(archsimd.LoadFloat64x4Array(n)), wrapped)
}

// cornerLanes is a corner of the lanes' cells as float64, 0 where on says a cell lies off the
// lattice.
func cornerLanes(h *[4]uint32, on archsimd.Int64x4) archsimd.Float64x4 {
	corner := archsimd.LoadUint32x4Array(h).AsFloat32x4().ConvertToFloat64()
	return corner.AsInt64x4().And(on).AsFloat64x4()
}

// point is lattice.point's rounding of d and alt, a ray to a lane.
func (k *lanes) point(d, alt archsimd.Float64x4) (archsimd.Float64x4, archsimd.Float64x4) {
	d32, alt32 := d.ConvertToFloat32(), alt.ConvertToFloat32()
	if k.bent {
		alt32 = alt32.Sub(archsimd.LoadFloat32x4Array(&k.bend).Mul(d32).Mul(d32))
	}
	return d32.ConvertToFloat64(), alt32.ConvertToFloat64()
}

// fused says the compiler fuses the walk's a + b*c into one rounding, as it does from GOAMD64=v3
// on, so that the lanes round as walk does.
var fused = fusesMulAdd(1+0x1p-30, 1+0x1p-30, -(1 + 0x1p-29))

// fusesMulAdd reports whether a + b*c, b·c being 2⁻⁶⁰ above -a, below a float64's precision,
// comes out other than 0, as it does only fused.
//
//go:noinline
func fusesMulAdd(b, c, a float64) bool { return a+b*c != 0 }

// mulAdd is a + b·c a lane at a time, rounded as the compiler rounds walk's.
func mulAdd(a, b, c archsimd.Float64x4) archsimd.Float64x4 {
	if fused {
		return b.MulAdd(c, a)
	}
	return a.Add(b.Mul(c))
}

// clamp01Wide is clamp01 a lane at a time.
func clamp01Wide(x, zero, one archsimd.Float64x4) archsimd.Float64x4 {
	return zero.IfElse(x.Less(zero), one.IfElse(x.Greater(one), x))
}
