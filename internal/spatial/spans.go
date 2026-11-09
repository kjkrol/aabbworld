package spatial

import "slices"

// sparse lets a layout list the cells it fills; tests turn it off to compare with every cell
// walked.
var sparse = true

// sparseShare is how many cells of a layout's there are to one it lists at most: past it,
// sorting the list costs more than walking every cell.
const sparseShare = 64

// span is where the entries of one cell begin in a layout, and how many there are.
type span struct{ at, n uint32 }

// spans is the cells of a counting sort: each one's span, zero for a cell holding nothing. While
// few cells hold anything they are listed, then sorted, so that a layout of a few pieces over a
// large grid clears, places, walks and asks those cells alone.
type spans struct {
	of      []span
	touched []uint32 // every cell holding anything, while listed
	listed  bool
	limit   int // the most cells listed before every cell is walked instead

	occupied int // the cells holding anything, once placed
}

// reset empties every cell of a grid of cells cells, listing up to one in sparseShare of them.
func (s *spans) reset(cells int) {
	switch {
	case len(s.of) != cells:
		s.of = make([]span, cells)
	case s.listed:
		for _, c := range s.touched {
			s.of[c] = span{}
		}
	default:
		clear(s.of)
	}
	s.touched = s.touched[:0]
	s.limit = cells / sparseShare
	s.listed = sparse && s.limit > 0
}

// count adds one entry to cell.
func (s *spans) count(cell uint32) {
	sp := &s.of[cell]
	if s.listed && sp.n == 0 {
		s.touched = append(s.touched, cell)
		s.listed = len(s.touched) < s.limit
	}
	sp.n++
}

// place lays the cells out one after another, in the order of their index, and returns how many
// entries they hold. Each span is left at its end, for take to count down from.
func (s *spans) place() int {
	sum := uint32(0)
	if !s.listed {
		occupied := 0
		for c := range s.of {
			if s.of[c].n != 0 {
				occupied++
			}
			sum += s.of[c].n
			s.of[c].at = sum
		}
		s.occupied = occupied
		return int(sum)
	}
	slices.Sort(s.touched)
	for _, c := range s.touched {
		sp := &s.of[c]
		sum += sp.n
		sp.at = sum
	}
	s.occupied = len(s.touched)
	return int(sum)
}

// take is the slot of the next entry dealt into cell, the cell's pieces dealt last to first.
func (s *spans) take(cell uint32) uint32 {
	sp := &s.of[cell]
	sp.at--
	return sp.at
}

// listedIn is the listed cells from the first of the rectangle x1..x2, y1..y2 of a grid side
// cells wide to its last, in order, when they are fewer than the rectangle's cells.
func (s *spans) listedIn(x1, y1, x2, y2, side uint32) ([]uint32, bool) {
	if !s.listed || len(s.touched) >= int((x2-x1+1)*(y2-y1+1)) {
		return nil, false
	}
	from, _ := slices.BinarySearch(s.touched, y1*side+x1)
	n, _ := slices.BinarySearch(s.touched[from:], y2*side+x2+1)
	return s.touched[from : from+n], true
}

// entries is where the entries of cell lie.
func (s *spans) entries(cell uint32) (from, to uint32) {
	sp := s.of[cell]
	return sp.at, sp.at + sp.n
}
