package calling

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestFindStartCode(t *testing.T) {
	// 4-byte start code
	data4 := []byte{0x12, 0x34, 0x00, 0x00, 0x00, 0x01, 0x67, 0x42}
	idx, length := findStartCode(data4)
	if idx != 2 || length != 4 {
		t.Fatalf("expected 4-byte start code at idx 2 len 4, got idx %d len %d", idx, length)
	}

	// 3-byte start code
	data3 := []byte{0xaa, 0xbb, 0x00, 0x00, 0x01, 0x68, 0xce}
	idx, length = findStartCode(data3)
	if idx != 2 || length != 3 {
		t.Fatalf("expected 3-byte start code at idx 2 len 3, got idx %d len %d", idx, length)
	}

	// No start code
	dataNone := []byte{0x00, 0x00, 0x02, 0x03}
	idx, length = findStartCode(dataNone)
	if idx != -1 {
		t.Fatalf("expected no start code (-1), got idx %d", idx)
	}
}

func TestExtractNodeID(t *testing.T) {
	// Direct uint32
	if id, ok := extractNodeID(uint32(42)); !ok || id != 42 {
		t.Fatalf("failed to extract direct uint32: got %d, %v", id, ok)
	}

	// Struct / slice hierarchy as returned by portal
	type Stream struct {
		NodeID     uint32
		Properties map[string]dbus.Variant
	}
	streams := []Stream{
		{NodeID: 108, Properties: nil},
	}

	if id, ok := extractNodeID(streams); !ok || id != 108 {
		t.Fatalf("failed to extract node ID from stream slice: got %d, %v", id, ok)
	}

	// Generic interface slice
	genericSlice := []interface{}{
		[]interface{}{uint32(256), map[string]interface{}{}},
	}
	if id, ok := extractNodeID(genericSlice); !ok || id != 256 {
		t.Fatalf("failed to extract node ID from generic slice: got %d, %v", id, ok)
	}
}
