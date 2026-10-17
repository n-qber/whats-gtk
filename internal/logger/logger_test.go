package logger

import (
	"fmt"
	"sync"
	"testing"
)

func TestRingBufferAppendAndLines(t *testing.T) {
	rb := NewRingBuffer(5)

	for i := 1; i <= 7; i++ {
		rb.Append(fmt.Sprintf("line %d", i))
	}

	lines := rb.Lines()
	if len(lines) != 5 {
		t.Fatalf("expected 5 lines, got %d", len(lines))
	}

	expectedFirst := "line 3"
	if lines[0] != expectedFirst {
		t.Fatalf("expected first line to be %q, got %q", expectedFirst, lines[0])
	}

	expectedLast := "line 7"
	if lines[4] != expectedLast {
		t.Fatalf("expected last line to be %q, got %q", expectedLast, lines[4])
	}
}

func TestRingBufferListeners(t *testing.T) {
	rb := NewRingBuffer(10)

	var received []string
	var mu sync.Mutex

	unregister := rb.RegisterListener(func(line string) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, line)
	})

	rb.Append("hello")
	rb.Append("world")

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected 2 received lines, got %d", len(received))
	}
	mu.Unlock()

	unregister()

	rb.Append("after unregister")

	mu.Lock()
	if len(received) != 2 {
		t.Fatalf("expected still 2 received lines after unregister, got %d", len(received))
	}
	mu.Unlock()
}

func TestRingBufferClear(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Append("a")
	rb.Append("b")

	if len(rb.Lines()) != 2 {
		t.Fatalf("expected 2 lines before clear")
	}

	rb.Clear()

	if len(rb.Lines()) != 0 {
		t.Fatalf("expected 0 lines after clear")
	}
}
