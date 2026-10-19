package calling

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/purpshell/meowcaller"
)

// CallState defines the lifecycle state of a call.
type CallState string

const (
	CallStateIdle       CallState = "idle"
	CallStateOutgoing   CallState = "outgoing"
	CallStateIncoming   CallState = "incoming"
	CallStateConnecting CallState = "connecting"
	CallStateActive     CallState = "active"
	CallStateEnded      CallState = "ended"
)

// CallDirection indicates who initiated the call.
type CallDirection string

const (
	DirectionIncoming CallDirection = "incoming"
	DirectionOutgoing CallDirection = "outgoing"
)

// CallInfo contains the full state and metadata of a call.
type CallInfo struct {
	CallID              string
	RemoteJID           string
	Direction           CallDirection
	IsVideo             bool
	State               CallState
	StartTime           time.Time
	Duration            time.Duration
	IsMuted             bool
	IsScreenSharing     bool
	RemoteScreenSharing bool
	RemoteVideoActive   bool
	EndReason           string
}

var (
	ErrCallAlreadyActive = errors.New("a call is already in progress")
	ErrNoActiveCall      = errors.New("no active call")
)

// Manager coordinates VoIP calls, audio hardware, screen sharing, and event dispatch.
type Manager struct {
	mu            sync.RWMutex
	client        *meowcaller.Client
	activeCall    *meowcaller.Call
	currentInfo   CallInfo
	mic           *MicSource
	speaker       *SpeakerSink
	tones         *TonePlayer
	videoReceiver *VideoReceiver
	screenCaster  *ScreenCaster
	videoCallback func(*gdk.Texture)
	ticker        *time.Ticker
	stopTicker    chan struct{}
	listeners     []func(CallInfo)
}

// NewManager creates a calling manager wrapping a meowcaller Client.
func NewManager(client *meowcaller.Client) *Manager {
	m := &Manager{
		client: client,
		currentInfo: CallInfo{
			State: CallStateIdle,
		},
		listeners: make([]func(CallInfo), 0),
	}

	if client != nil {
		client.OnIncomingCall(func(call *meowcaller.Call) {
			m.handleIncomingCall(call)
		})
	}

	return m
}

// Subscribe registers a listener for call state changes.
func (m *Manager) Subscribe(fn func(CallInfo)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, fn)
}

func (m *Manager) notifyListeners(info CallInfo) {
	m.mu.RLock()
	listeners := make([]func(CallInfo), len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.RUnlock()

	for _, fn := range listeners {
		if fn != nil {
			go fn(info)
		}
	}
}

// StartCall places a 1:1 outbound audio or video call.
func (m *Manager) StartCall(ctx context.Context, target string, isVideo bool) (*CallInfo, error) {
	m.mu.Lock()
	if m.currentInfo.State != CallStateIdle && m.currentInfo.State != CallStateEnded {
		m.mu.Unlock()
		return nil, ErrCallAlreadyActive
	}

	// Prepare audio speaker and ringback tone
	speaker, err := NewSpeakerSink()
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("failed to initialize speaker: %w", err)
	}

	tones := NewTonePlayer(speaker)
	tones.PlayRingback()

	m.speaker = speaker
	m.tones = tones
	m.currentInfo = CallInfo{
		RemoteJID: target,
		Direction: DirectionOutgoing,
		IsVideo:   isVideo,
		State:     CallStateOutgoing,
	}
	m.mu.Unlock()

	m.notifyListeners(m.GetInfo())

	// Place call through meowcaller
	opts := meowcaller.CallOptions{Video: isVideo}
	call, err := m.client.CallWithOptions(ctx, target, opts)
	if err != nil {
		m.teardownAudio()
		m.mu.Lock()
		m.currentInfo.State = CallStateEnded
		m.currentInfo.EndReason = err.Error()
		info := m.currentInfo
		m.mu.Unlock()
		m.notifyListeners(info)
		return nil, err
	}

	m.mu.Lock()
	m.activeCall = call
	m.currentInfo.CallID = call.ID()
	m.currentInfo.RemoteJID = call.Peer().String()
	m.wireCallEvents(call)
	info := m.currentInfo
	m.mu.Unlock()

	m.notifyListeners(info)
	return &info, nil
}

// AnswerCall accepts an incoming call.
func (m *Manager) AnswerCall() error {
	m.mu.Lock()
	call := m.activeCall
	if call == nil || m.currentInfo.State != CallStateIncoming {
		m.mu.Unlock()
		return ErrNoActiveCall
	}

	if m.tones != nil {
		m.tones.Stop()
		m.tones = nil
	}
	m.currentInfo.State = CallStateConnecting
	info := m.currentInfo
	m.mu.Unlock()

	m.notifyListeners(info)

	if err := call.Answer(); err != nil {
		m.Hangup()
		return fmt.Errorf("failed to answer call: %w", err)
	}
	return nil
}

// RejectCall declines an incoming call.
func (m *Manager) RejectCall() error {
	m.mu.Lock()
	call := m.activeCall
	if call == nil || m.currentInfo.State != CallStateIncoming {
		m.mu.Unlock()
		return ErrNoActiveCall
	}
	m.mu.Unlock()

	_ = call.Reject()
	m.finishCall("Call declined")
	return nil
}

// Hangup ends the currently active call.
func (m *Manager) Hangup() error {
	m.mu.Lock()
	call := m.activeCall
	if call == nil {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	_ = call.Hangup()
	m.finishCall("Call ended")
	return nil
}

// ToggleMute toggles microphone mute state.
func (m *Manager) ToggleMute() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.mic != nil {
		newMute := !m.mic.IsMuted()
		m.mic.SetMute(newMute)
		m.currentInfo.IsMuted = newMute
		go m.notifyListeners(m.currentInfo)
		return newMute
	}
	return false
}

// IsMuted returns true if the microphone is currently muted.
func (m *Manager) IsMuted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentInfo.IsMuted
}

// GetInfo returns a snapshot of the current call information.
func (m *Manager) GetInfo() CallInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	info := m.currentInfo
	if !info.StartTime.IsZero() && info.State == CallStateActive {
		info.Duration = time.Since(info.StartTime)
	}
	return info
}

// StartScreenShare starts capturing the screen via XDG Desktop Portal and announces it on the active call.
func (m *Manager) StartScreenShare(ctx context.Context) error {
	m.mu.Lock()
	call := m.activeCall
	if call == nil || m.currentInfo.State != CallStateActive {
		m.mu.Unlock()
		return ErrNoActiveCall
	}
	if m.screenCaster != nil {
		m.mu.Unlock()
		return nil // Already sharing
	}
	m.mu.Unlock()

	// 1. Announce screen share signaling
	if err := call.StartScreenShare(nil); err != nil {
		return fmt.Errorf("failed to signal screen share: %w", err)
	}

	// 2. Start desktop portal capture and GStreamer H.264 encoder
	caster := NewScreenCaster(func(accessUnit []byte) {
		m.mu.RLock()
		active := m.activeCall
		m.mu.RUnlock()
		if active != nil {
			_ = active.SendVideo(accessUnit)
		}
	})

	if err := caster.Start(ctx); err != nil {
		_ = call.StopScreenShare()
		return err
	}

	m.mu.Lock()
	m.screenCaster = caster
	m.currentInfo.IsScreenSharing = true
	info := m.currentInfo
	m.mu.Unlock()

	m.notifyListeners(info)
	return nil
}

// StopScreenShare stops outbound screen sharing.
func (m *Manager) StopScreenShare() error {
	m.mu.Lock()
	call := m.activeCall
	caster := m.screenCaster
	m.screenCaster = nil
	m.currentInfo.IsScreenSharing = false
	info := m.currentInfo
	m.mu.Unlock()

	if caster != nil {
		_ = caster.Stop()
	}

	if call != nil {
		_ = call.StopScreenShare()
	}

	m.notifyListeners(info)
	return nil
}

// ToggleScreenShare toggles outbound screen share on/off.
func (m *Manager) ToggleScreenShare(ctx context.Context) error {
	m.mu.RLock()
	isSharing := m.currentInfo.IsScreenSharing
	m.mu.RUnlock()

	if isSharing {
		return m.StopScreenShare()
	}
	return m.StartScreenShare(ctx)
}

// IsSharingScreen returns true if local screen sharing is active.
func (m *Manager) IsSharingScreen() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentInfo.IsScreenSharing
}

// SetVideoFrameCallback sets the callback for received video or screen share textures.
func (m *Manager) SetVideoFrameCallback(fn func(*gdk.Texture)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.videoCallback = fn
}

func (m *Manager) handleIncomingCall(call *meowcaller.Call) {
	m.mu.Lock()
	if m.currentInfo.State != CallStateIdle && m.currentInfo.State != CallStateEnded {
		m.mu.Unlock()
		_ = call.Reject()
		return
	}

	speaker, _ := NewSpeakerSink()
	var tones *TonePlayer
	if speaker != nil {
		tones = NewTonePlayer(speaker)
		tones.PlayRingtone()
	}

	m.speaker = speaker
	m.tones = tones
	m.activeCall = call
	m.currentInfo = CallInfo{
		CallID:    call.ID(),
		RemoteJID: call.Peer().String(),
		Direction: DirectionIncoming,
		IsVideo:   call.IsVideo(),
		State:     CallStateIncoming,
	}

	m.wireCallEvents(call)
	info := m.currentInfo
	m.mu.Unlock()

	m.notifyListeners(info)
}

func (m *Manager) wireCallEvents(call *meowcaller.Call) {
	call.OnReady(func() {
		m.mu.Lock()
		if m.tones != nil {
			m.tones.Stop()
			m.tones = nil
		}

		// Initialize microphone
		mic, err := NewMicSource()
		if err == nil {
			m.mic = mic
			call.Play(mic)
		}

		if m.speaker == nil {
			m.speaker, _ = NewSpeakerSink()
		}
		if m.speaker != nil {
			call.Receive(m.speaker)
		}

		// Initialize video receiver for remote video & screen sharing
		if m.videoReceiver == nil {
			vr, err := NewVideoReceiver(func(tex *gdk.Texture) {
				m.mu.RLock()
				cb := m.videoCallback
				m.mu.RUnlock()
				if cb != nil {
					cb(tex)
				}
			})
			if err == nil {
				m.videoReceiver = vr
				call.ReceiveVideo(vr)
			}
		}

		m.currentInfo.State = CallStateActive
		m.currentInfo.StartTime = time.Now()
		m.startDurationTimer()
		info := m.currentInfo
		m.mu.Unlock()

		m.notifyListeners(info)
	})

	call.OnEnd(func(reason string) {
		m.finishCall(reason)
	})

	call.OnScreenShare(func(state meowcaller.ScreenShareState) {
		m.mu.Lock()
		m.currentInfo.RemoteScreenSharing = state.Active
		info := m.currentInfo
		m.mu.Unlock()
		m.notifyListeners(info)
	})

	call.OnVideoState(func(state meowcaller.VideoState) {
		m.mu.Lock()
		m.currentInfo.RemoteVideoActive = state.Active
		info := m.currentInfo
		m.mu.Unlock()
		m.notifyListeners(info)
	})

	call.OnStateChange(func(phase meowcaller.CallPhase) {
		m.mu.Lock()
		switch phase {
		case meowcaller.CallPhaseCalling:
			m.currentInfo.State = CallStateOutgoing
		case meowcaller.CallPhaseRinging:
			if m.currentInfo.Direction == DirectionOutgoing {
				m.currentInfo.State = CallStateOutgoing
			} else {
				m.currentInfo.State = CallStateIncoming
			}
		case meowcaller.CallPhaseConnecting:
			m.currentInfo.State = CallStateConnecting
		case meowcaller.CallPhaseActive:
			m.currentInfo.State = CallStateActive
		case meowcaller.CallPhaseEnded:
			m.currentInfo.State = CallStateEnded
		}
		info := m.currentInfo
		m.mu.Unlock()

		m.notifyListeners(info)
	})
}

func (m *Manager) startDurationTimer() {
	if m.ticker != nil {
		m.ticker.Stop()
	}
	m.ticker = time.NewTicker(1 * time.Second)
	m.stopTicker = make(chan struct{})

	go func(stop chan struct{}, ticker *time.Ticker) {
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				m.mu.RLock()
				if m.currentInfo.State != CallStateActive {
					m.mu.RUnlock()
					return
				}
				info := m.currentInfo
				info.Duration = time.Since(info.StartTime)
				m.mu.RUnlock()
				m.notifyListeners(info)
			}
		}
	}(m.stopTicker, m.ticker)
}

func (m *Manager) stopDurationTimer() {
	if m.ticker != nil {
		m.ticker.Stop()
		m.ticker = nil
	}
	if m.stopTicker != nil {
		close(m.stopTicker)
		m.stopTicker = nil
	}
}

func (m *Manager) finishCall(reason string) {
	m.mu.Lock()
	m.stopDurationTimer()
	m.teardownMedia()
	m.activeCall = nil
	m.currentInfo.State = CallStateEnded
	m.currentInfo.IsScreenSharing = false
	m.currentInfo.RemoteScreenSharing = false
	m.currentInfo.RemoteVideoActive = false
	if reason != "" {
		m.currentInfo.EndReason = reason
	}
	info := m.currentInfo
	m.mu.Unlock()

	m.notifyListeners(info)
}

func (m *Manager) teardownAudio() {
	m.teardownMedia()
}

func (m *Manager) teardownMedia() {
	if m.screenCaster != nil {
		_ = m.screenCaster.Stop()
		m.screenCaster = nil
	}
	if m.videoReceiver != nil {
		_ = m.videoReceiver.Close()
		m.videoReceiver = nil
	}
	if m.tones != nil {
		m.tones.Stop()
		m.tones = nil
	}
	if m.mic != nil {
		_ = m.mic.Close()
		m.mic = nil
	}
	if m.speaker != nil {
		_ = m.speaker.Close()
		m.speaker = nil
	}
}
