package torrents

import "testing"

func TestBitfieldHasPiece(t *testing.T) {
	// Bytes are read from left to right
	// Bits from right to left
	//
	//     55        156
	// [00110111, 10011100]
	bitfield := Bitfield{55, 156} 

	// [00110*1*11, 10011100]
	idx := 2
	if !bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d IS present in the bitfield", idx)
	}

	// [*0*0110111, 10011100]
	idx = 7
	if bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d is NOT present in the bitfield", idx)
	}

	// [00110111, 1001*1*100]
	idx = 11
	if !bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d IS present in the bitfield", idx)
	}

	idx = 14 // [00110111, 1*0*011100]
	if bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d is NOT present in the bitfield", idx)
	}
}
