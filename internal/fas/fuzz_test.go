package fas

import (
	"bytes"
	"os"
	"testing"
)

func FuzzParse(f *testing.F) {
	f.Add([]byte("FasdUAS 1.101.10\x03\x00\x00\x00\x01"))
	for _, path := range []string{"../../testdata/demo.scpt", "../../testdata/seccon.scpt"} {
		if data, err := os.ReadFile(path); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		limits := DefaultLimits()
		limits.MaxInputBytes = 1 << 20
		limits.MaxBlobBytes = 1 << 20
		limits.MaxObjects = 10000
		limits.MaxReferences = 10000
		limits.MaxDepth = 128
		_, _ = Parse(bytes.NewReader(data), Options{Limits: limits})
	})
}
