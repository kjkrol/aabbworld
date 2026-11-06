package simdx

// Pad32 is how far past its data a slice of float32 loaded a vector at a time runs: the widest
// vector, 512 bits, holds sixteen.
const Pad32 = 15

// Pad64 is Pad32 for float64, eight to the widest vector.
const Pad64 = 7
