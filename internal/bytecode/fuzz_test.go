package bytecode

import "testing"

func FuzzDecode(f *testing.F) {
	f.Add([]byte{0xe0, 0x6b, 0x1e, 0x0f})
	f.Add([]byte{0x62, 0, 1, 0, 2, 0x59, 0xff, 0xf9})
	f.Fuzz(func(t *testing.T, code []byte) { _, _ = Decode(2, code, false) })
}
