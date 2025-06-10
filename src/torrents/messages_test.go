package torrents

import "testing"

func TestBitfieldHasPiece(t *testing.T) {
	// Bytes are read from left to right
	// Bits from left to right (high bit first)
	//
	//     55        156
	// [00110111, 10011100]
	bitfield := Bitfield{55, 156}

	// [00*1*10111, 10011100]
	idx := 2
	if !bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d IS present in the bitfield", idx)
	}

	// [0011011*1*, 10011100]
	idx = 4
	if bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d is NOT present in the bitfield", idx)
	}

	// [00110111, 100*1*1100]
	idx = 11
	if !bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d IS present in the bitfield", idx)
	}

	idx = 14 // [00110111, 100111*0*0]
	if bitfield.HasPiece(idx) {
		t.Errorf("Piece index %d is NOT present in the bitfield", idx)
	}
}

func TestBitfieldSetPiece(t *testing.T) {
	// Bytes are read from left to right
	// Bits from left to right (high bit first)
	//
	//     55        156
	// [00110111, 10011100]
	bitfield := Bitfield{55, 156}

	// From [0011*0*111, 10011100] == { 55, 156 }
	// To [00111111, 10011100] == { 63, 156 }
	idx := 4
	var expectedDecimal byte = 63
	bitfield.SetPiece(idx)
	if bitfield[0] != expectedDecimal {
		t.Errorf("After set, the byte should be equal to %d", expectedDecimal)
	}

	// From [00110111, 1*0*011100] == { 55, 156 }
	// To [00110111, 11011100] == { 55, 220 }
	idx = 9
	expectedDecimal = 220
	bitfield.SetPiece(idx)
	if bitfield[1] != expectedDecimal {
		t.Errorf("After set, the byte should be equal to %d", expectedDecimal)
	}
}
