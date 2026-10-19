package calling

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os/exec"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/godbus/dbus/v5"
)

var (
	ErrScreenCastCanceled = errors.New("screencast request canceled by user")
	ErrPortalUnavailable = errors.New("xdg-desktop-portal not available")
	ErrGStreamerNotFound  = errors.New("gst-launch-1.0 not found in PATH")
)

// ScreenCaster manages capturing the screen via XDG Desktop Portal (PipeWire)
// and encoding frames to H.264 Annex-B access units using GStreamer.
type ScreenCaster struct {
	mu           sync.Mutex
	conn         *dbus.Conn
	sessionToken string
	sessionPath  dbus.ObjectPath
	cmd          *exec.Cmd
	stdout       io.ReadCloser
	onAccessUnit func([]byte)
	closed       atomic.Bool
}

// NewScreenCaster creates a new ScreenCaster instance.
func NewScreenCaster(onAccessUnit func([]byte)) *ScreenCaster {
	return &ScreenCaster{
		onAccessUnit: onAccessUnit,
	}
}

// Start requests screen sharing via XDG Desktop Portal, waits for user selection,
// and starts the GStreamer encoder pipeline.
func (sc *ScreenCaster) Start(ctx context.Context) error {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	gstPath, err := exec.LookPath("gst-launch-1.0")
	if err != nil {
		return ErrGStreamerNotFound
	}

	conn, err := dbus.SessionBus()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPortalUnavailable, err)
	}
	sc.conn = conn

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	sessionToken := fmt.Sprintf("whats_gtk_sess_%d", r.Uint32())
	sc.sessionToken = sessionToken

	// Subscribe to Portal Request Response signals
	if err := conn.AddMatchSignal(
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		sc.cleanupDBus()
		return err
	}

	sigChan := make(chan *dbus.Signal, 10)
	conn.Signal(sigChan)
	defer conn.RemoveSignal(sigChan)

	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")

	// 1. CreateSession
	reqToken1 := fmt.Sprintf("req_%d", r.Uint32())
	var reqPath1 dbus.ObjectPath
	err = portal.CallWithContext(
		ctx,
		"org.freedesktop.portal.ScreenCast.CreateSession",
		0,
		map[string]dbus.Variant{
			"session_handle_token": dbus.MakeVariant(sessionToken),
			"handle_token":         dbus.MakeVariant(reqToken1),
		},
	).Store(&reqPath1)
	if err != nil {
		sc.cleanupDBus()
		return fmt.Errorf("portal CreateSession failed: %w", err)
	}

	sessionHandle, err := sc.waitForResponse(ctx, sigChan, reqPath1)
	if err != nil {
		sc.cleanupDBus()
		return err
	}
	if sh, ok := sessionHandle["session_handle"].Value().(string); ok {
		sc.sessionPath = dbus.ObjectPath(sh)
	} else {
		sc.cleanupDBus()
		return fmt.Errorf("no session_handle returned by portal")
	}

	// 2. SelectSources (Monitor and Window allowed)
	reqToken2 := fmt.Sprintf("req_%d", r.Uint32())
	var reqPath2 dbus.ObjectPath
	err = portal.CallWithContext(
		ctx,
		"org.freedesktop.portal.ScreenCast.SelectSources",
		0,
		sc.sessionPath,
		map[string]dbus.Variant{
			"types":        dbus.MakeVariant(uint32(1 | 2)), // Monitor | Window
			"multiple":     dbus.MakeVariant(false),
			"cursor_mode":  dbus.MakeVariant(uint32(2)), // Embedded cursor
			"handle_token": dbus.MakeVariant(reqToken2),
		},
	).Store(&reqPath2)
	if err != nil {
		sc.cleanupDBus()
		return fmt.Errorf("portal SelectSources failed: %w", err)
	}

	if _, err := sc.waitForResponse(ctx, sigChan, reqPath2); err != nil {
		sc.cleanupDBus()
		return err
	}

	// 3. Start (Presents the system screen picker dialog)
	reqToken3 := fmt.Sprintf("req_%d", r.Uint32())
	var reqPath3 dbus.ObjectPath
	err = portal.CallWithContext(
		ctx,
		"org.freedesktop.portal.ScreenCast.Start",
		0,
		sc.sessionPath,
		"", // Parent window ID
		map[string]dbus.Variant{
			"handle_token": dbus.MakeVariant(reqToken3),
		},
	).Store(&reqPath3)
	if err != nil {
		sc.cleanupDBus()
		return fmt.Errorf("portal Start failed: %w", err)
	}

	startResults, err := sc.waitForResponse(ctx, sigChan, reqPath3)
	if err != nil {
		sc.cleanupDBus()
		return err
	}

	// Extract PipeWire Node ID from results
	streamsVal, exists := startResults["streams"]
	if !exists {
		sc.cleanupDBus()
		return fmt.Errorf("portal did not return streams")
	}

	nodeID, ok := extractNodeID(streamsVal.Value())
	if !ok || nodeID == 0 {
		sc.cleanupDBus()
		return fmt.Errorf("could not determine PipeWire node ID from portal response")
	}

	// 4. Start GStreamer Pipeline reading from pipewiresrc path=<nodeID>
	// and encoding to H.264 Annex-B
	pipelineArgs := []string{
		"-q",
		"pipewiresrc",
		"path=" + strconv.FormatUint(uint64(nodeID), 10),
		"keepalive-time=1000",
		"do-timestamp=true",
		"!",
		"videoconvert",
		"!",
		"x264enc",
		"tune=zerolatency",
		"speed-preset=ultrafast",
		"bitrate=1500",
		"key-int-max=30",
		"byte-stream=true",
		"!",
		"video/x-h264,stream-format=byte-stream,alignment=au",
		"!",
		"fdsink",
		"fd=1",
	}

	cmd := exec.Command(gstPath, pipelineArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		sc.cleanupDBus()
		return fmt.Errorf("failed to pipe gstreamer stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		sc.cleanupDBus()
		return fmt.Errorf("failed to start gstreamer pipeline: %w", err)
	}

	sc.cmd = cmd
	sc.stdout = stdout
	sc.closed.Store(false)

	go sc.readAccessUnitsLoop()

	return nil
}

func (sc *ScreenCaster) waitForResponse(ctx context.Context, sigChan chan *dbus.Signal, targetPath dbus.ObjectPath) (map[string]dbus.Variant, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case sig, ok := <-sigChan:
			if !ok {
				return nil, errors.New("dbus signal channel closed")
			}
			if sig.Path == targetPath && len(sig.Body) >= 2 {
				resCode, ok1 := sig.Body[0].(uint32)
				results, ok2 := sig.Body[1].(map[string]dbus.Variant)
				if !ok1 || !ok2 {
					return nil, fmt.Errorf("unexpected signal signature: %v", sig.Body)
				}
				if resCode != 0 {
					return nil, ErrScreenCastCanceled
				}
				return results, nil
			}
		}
	}
}

// readAccessUnitsLoop parses Annex-B access units and passes them to onAccessUnit callback.
func (sc *ScreenCaster) readAccessUnitsLoop() {
	buf := make([]byte, 64*1024)
	var stream []byte

	for !sc.closed.Load() {
		n, err := sc.stdout.Read(buf)
		if n > 0 {
			stream = append(stream, buf[:n]...)

			for {
				start, startLen := findStartCode(stream)
				if start == -1 {
					// No start code yet, keep small tail
					if len(stream) > 4 {
						stream = stream[len(stream)-4:]
					}
					break
				}

				// Find next start code after current
				nextStart, _ := findStartCode(stream[start+startLen:])
				if nextStart == -1 {
					// Incomplete access unit, keep waiting
					if start > 0 {
						stream = stream[start:]
					}
					break
				}

				nextPos := start + startLen + nextStart
				accessUnit := make([]byte, nextPos-start)
				copy(accessUnit, stream[start:nextPos])
				stream = stream[nextPos:]

				if sc.onAccessUnit != nil && len(accessUnit) > 0 {
					sc.onAccessUnit(accessUnit)
				}
			}
		}

		if err != nil {
			break
		}
	}
}

// findStartCode finds the first 0x000001 (3-byte) or 0x00000001 (4-byte) start code.
func findStartCode(b []byte) (int, int) {
	for i := 0; i+2 < len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 {
			if b[i+2] == 1 {
				if i > 0 && b[i-1] == 0 {
					return i - 1, 4
				}
				return i, 3
			}
		}
	}
	return -1, 0
}

// extractNodeID extracts the uint32 PipeWire node ID from arbitrary unmarshaled variants.
func extractNodeID(val interface{}) (uint32, bool) {
	if val == nil {
		return 0, false
	}

	switch v := val.(type) {
	case uint32:
		return v, true
	case int:
		return uint32(v), true
	case []interface{}:
		for _, item := range v {
			if id, ok := extractNodeID(item); ok && id > 0 {
				return id, true
			}
		}
	}

	rv := reflect.ValueOf(val)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if id, ok := extractNodeID(rv.Index(i).Interface()); ok && id > 0 {
				return id, true
			}
		}
	case reflect.Struct:
		for i := 0; i < rv.NumField(); i++ {
			if id, ok := extractNodeID(rv.Field(i).Interface()); ok && id > 0 {
				return id, true
			}
		}
	}

	return 0, false
}

// Stop terminates the encoder pipeline and closes the portal session.
func (sc *ScreenCaster) Stop() error {
	if sc.closed.Swap(true) {
		return nil
	}

	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.stdout != nil {
		_ = sc.stdout.Close()
		sc.stdout = nil
	}

	if sc.cmd != nil && sc.cmd.Process != nil {
		_ = sc.cmd.Process.Kill()
		_ = sc.cmd.Wait()
		sc.cmd = nil
	}

	sc.cleanupDBus()
	return nil
}

func (sc *ScreenCaster) cleanupDBus() {
	if sc.conn != nil {
		if sc.sessionPath != "" {
			sessObj := sc.conn.Object("org.freedesktop.portal.Desktop", sc.sessionPath)
			_ = sessObj.Call("org.freedesktop.portal.Session.Close", 0)
			sc.sessionPath = ""
		}
		_ = sc.conn.Close()
		sc.conn = nil
	}
}
