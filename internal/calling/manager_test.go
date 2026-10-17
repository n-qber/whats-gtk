package calling

import (
	"math"
	"sync"
	"testing"
	"time"
)

func TestCallingManagerInitialState(t *testing.T) {
	mgr := NewManager(nil)
	info := mgr.GetInfo()
	if info.State != CallStateIdle {
		t.Fatalf("expected initial state %v, got %v", CallStateIdle, info.State)
	}

	if mgr.IsMuted() {
		t.Fatalf("expected initial mute state to be false")
	}

	// Answering when idle should return ErrNoActiveCall
	err := mgr.AnswerCall()
	if err != ErrNoActiveCall {
		t.Fatalf("expected ErrNoActiveCall, got %v", err)
	}

	// Rejecting when idle should return ErrNoActiveCall
	err = mgr.RejectCall()
	if err != ErrNoActiveCall {
		t.Fatalf("expected ErrNoActiveCall, got %v", err)
	}

	// Hangup when idle should return nil
	err = mgr.Hangup()
	if err != nil {
		t.Fatalf("expected nil on Hangup when idle, got %v", err)
	}
}

func TestMuteToggle(t *testing.T) {
	mgr := NewManager(nil)
	// Without mic initialized, ToggleMute returns false
	if mgr.ToggleMute() {
		t.Fatalf("expected ToggleMute to return false without mic")
	}
}

func TestPCMConversion(t *testing.T) {
	inputPCM := []int16{0, 16384, 32767, -16384, -32768}
	floats := pcmToFloat(inputPCM)
	if len(floats) != len(inputPCM) {
		t.Fatalf("expected %d floats, got %d", len(inputPCM), len(floats))
	}

	// Check clamped values
	if math.Abs(float64(floats[0])) > 0.0001 {
		t.Fatalf("expected ~0, got %f", floats[0])
	}
	if math.Abs(float64(floats[1])-0.5) > 0.001 {
		t.Fatalf("expected ~0.5, got %f", floats[1])
	}

	reconstructed := floatToPCM(floats)
	for i := range inputPCM {
		diff := int(inputPCM[i]) - int(reconstructed[i])
		if diff < -1 || diff > 1 {
			t.Fatalf("mismatch at %d: original %d, reconstructed %d", i, inputPCM[i], reconstructed[i])
		}
	}
}

func TestTonePlayerStop(t *testing.T) {
	tp := &TonePlayer{
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}
	tp.PlayRingback()
	time.Sleep(100 * time.Millisecond)
	tp.Stop()

	select {
	case <-tp.doneCh:
		// Passed, stopped cleanly
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("TonePlayer did not stop within deadline")
	}
}

func TestManagerListenerNotification(t *testing.T) {
	mgr := NewManager(nil)
	var wg sync.WaitGroup
	wg.Add(1)

	var receivedInfo CallInfo
	mgr.Subscribe(func(info CallInfo) {
		receivedInfo = info
		wg.Done()
	})

	mgr.finishCall("test reason")
	wg.Wait()

	if receivedInfo.State != CallStateEnded {
		t.Fatalf("expected state %v, got %v", CallStateEnded, receivedInfo.State)
	}
	if receivedInfo.EndReason != "test reason" {
		t.Fatalf("expected reason 'test reason', got '%s'", receivedInfo.EndReason)
	}
}
