package calling

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/purpshell/meowcaller"
)

// Ensure VideoReceiver satisfies meowcaller.VideoSink and meowcaller.VideoOrientationSink.
var (
	_ meowcaller.VideoSink            = (*VideoReceiver)(nil)
	_ meowcaller.VideoOrientationSink = (*VideoReceiver)(nil)
)

// VideoReceiver decodes incoming H.264 access units (Annex-B) using FFmpeg,
// converting them to MJPEG frames and producing GdkTextures for GTK4.
type VideoReceiver struct {
	mu          sync.Mutex
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	onFrame     func(*gdk.Texture)
	orientation atomic.Int32
	closed      atomic.Bool
	started     bool
}

// NewVideoReceiver creates a receiver that feeds decoded frames to onFrame.
func NewVideoReceiver(onFrame func(*gdk.Texture)) (*VideoReceiver, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	cmd := exec.Command(
		ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-f", "h264",
		"-i", "pipe:0",
		"-f", "image2pipe",
		"-vcodec", "mjpeg",
		"-q:v", "2",
		"pipe:1",
	)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open ffmpeg stdin: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("failed to open ffmpeg stdout: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("failed to start ffmpeg decoder: %w", err)
	}

	vr := &VideoReceiver{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		onFrame: onFrame,
		started: true,
	}

	go vr.readFramesLoop()

	return vr, nil
}

// SetOrientation implements meowcaller.VideoOrientationSink.
func (vr *VideoReceiver) SetOrientation(orientation int) {
	vr.orientation.Store(int32(orientation))
}

// WriteVideo consumes one Annex-B H.264 access unit from the peer.
func (vr *VideoReceiver) WriteVideo(accessUnit []byte) error {
	if vr.closed.Load() || len(accessUnit) == 0 {
		return nil
	}

	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.stdin == nil {
		return nil
	}

	_, err := vr.stdin.Write(accessUnit)
	return err
}

// readFramesLoop parses JPEG frames from ffmpeg stdout and converts them to GdkTexture.
func (vr *VideoReceiver) readFramesLoop() {
	buf := make([]byte, 64*1024)
	var stream []byte

	for !vr.closed.Load() {
		n, err := vr.stdout.Read(buf)
		if n > 0 {
			stream = append(stream, buf[:n]...)

			for {
				start := bytes.Index(stream, []byte{0xFF, 0xD8})
				if start == -1 {
					if len(stream) > 1 {
						stream = stream[len(stream)-1:]
					}
					break
				}

				end := bytes.Index(stream[start+2:], []byte{0xFF, 0xD9})
				if end == -1 {
					// Incomplete frame, wait for more data
					if start > 0 {
						stream = stream[start:]
					}
					break
				}

				frameEnd := start + 2 + end + 2
				frameBytes := make([]byte, frameEnd-start)
				copy(frameBytes, stream[start:frameEnd])
				stream = stream[frameEnd:]

				vr.dispatchFrame(frameBytes)
			}
		}

		if err != nil {
			break
		}
	}
}

func (vr *VideoReceiver) dispatchFrame(jpegData []byte) {
	if vr.closed.Load() || len(jpegData) == 0 || vr.onFrame == nil {
		return
	}

	gBytes := glib.NewBytes(jpegData)
	tex, err := gdk.NewTextureFromBytes(gBytes)
	if err != nil {
		return
	}

	vr.onFrame(tex)
}

// Close terminates ffmpeg and cleans up pipes.
func (vr *VideoReceiver) Close() error {
	if vr.closed.Swap(true) {
		return nil
	}

	vr.mu.Lock()
	if vr.stdin != nil {
		_ = vr.stdin.Close()
		vr.stdin = nil
	}
	vr.mu.Unlock()

	if vr.stdout != nil {
		_ = vr.stdout.Close()
	}

	if vr.cmd != nil && vr.cmd.Process != nil {
		_ = vr.cmd.Process.Kill()
		_ = vr.cmd.Wait()
	}

	return nil
}
