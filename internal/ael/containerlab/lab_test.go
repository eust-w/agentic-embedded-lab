package containerlab

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeIntelHexCanonicalizesOverlappingErasedFlash(t *testing.T) {
	root := t.TempDir()
	erased := filepath.Join(root, "erased.hex")
	firmware := filepath.Join(root, "firmware.hex")
	merged := filepath.Join(root, "merged.hex")
	// Model an erased baseline. Keeping this separate from the firmware used
	// to create the historical overlapping-record corruption in Renode.
	erasedBytes := make([]byte, 1024)
	for index := range erasedBytes {
		erasedBytes[index] = 0xff
	}
	if err := bytesToIntelHex(erasedBytes, erased, 0x08000000); err != nil {
		t.Fatal(err)
	}
	want := []byte{0xf8, 0x53, 0x40, 0x20}
	if err := bytesToIntelHex(want, firmware, 0x080001ff); err != nil {
		t.Fatal(err)
	}
	if err := mergeIntelHex(merged, erased, firmware); err != nil {
		t.Fatal(err)
	}
	image := readIntelHexForTest(t, merged)
	for index, value := range want {
		if got := image[0x080001ff+uint64(index)]; got != value {
			t.Fatalf("byte %#x was %#x, want %#x", 0x080001ff+index, got, value)
		}
	}
}

func readIntelHexForTest(t *testing.T, path string) map[uint64]byte {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result := map[uint64]byte{}
	var upper uint64
	for _, line := range strings.Split(strings.TrimSpace(string(payload)), "\n") {
		record, err := hex.DecodeString(strings.TrimPrefix(line, ":"))
		if err != nil {
			t.Fatal(err)
		}
		switch record[3] {
		case 0:
			address := upper + (uint64(record[1]) << 8) + uint64(record[2])
			for index, value := range record[4 : 4+int(record[0])] {
				if _, duplicate := result[address+uint64(index)]; duplicate {
					t.Fatalf("canonical HEX contains overlapping address %#x", address+uint64(index))
				}
				result[address+uint64(index)] = value
			}
		case 4:
			upper = (uint64(record[4])<<8 | uint64(record[5])) << 16
		}
	}
	return result
}
