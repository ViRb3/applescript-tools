package macroman

import "testing"

func TestRoundTripEveryByte(t *testing.T) {
	for value := 0; value <= 0xff; value++ {
		character := DecodeByte(byte(value))
		encoded, ok := EncodeRune(character)
		if !ok || encoded != byte(value) {
			t.Errorf("byte 0x%02x decoded to %U and encoded to 0x%02x, %v", value, character, encoded, ok)
		}
	}
}

func TestDecode(t *testing.T) {
	if got := Decode([]byte{'c', 'a', 'f', 0x8e, ' ', 0xb2}); got != "café ≤" {
		t.Fatalf("Decode = %q", got)
	}
}

func TestEncodeRuneRejectsUnavailableCharacter(t *testing.T) {
	if _, ok := EncodeRune('✓'); ok {
		t.Fatal("EncodeRune accepted a character outside MacRoman")
	}
}
