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

func TestBitfieldSetPiece(t *testing.T) {
	// Bytes are read from left to right
	// Bits from right to left
	//
	//     55        156
	// [00110111, 10011100]
	bitfield := Bitfield{55, 156}

	// From [*0*0110111, 10011100] == { 55, 156 }
	// To [*1*0110111, 10011100] == { 183, 156 }
	idx := 7
	bitfield.SetPiece(idx)
	if bitfield[0] != 183 {
		t.Errorf("After set, the byte should be equal to 183")
	}

	// From [00110111, 1*0*011100] == { 55, 156 }
	// To [00110111, 1*1*011100] == { 55, 220 }
	idx = 14
	bitfield.SetPiece(idx)
	if bitfield[1] != 220 {
		t.Errorf("After set, the byte should be equal to 220")
	}
}
