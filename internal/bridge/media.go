package bridge

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

type MediaMetadata struct {
	URL           string
	DirectPath    string
	MediaKey      []byte
	Mimetype      string
	FileEncSHA256 []byte
	FileSHA256    []byte
	FileLength    uint64
}

func (m *MediaMetadata) GetURL() string           { return m.URL }
func (m *MediaMetadata) GetDirectPath() string    { return m.DirectPath }
func (m *MediaMetadata) GetMediaKey() []byte      { return m.MediaKey }
func (m *MediaMetadata) GetMimetype() string      { return m.Mimetype }
func (m *MediaMetadata) GetFileEncSHA256() []byte { return m.FileEncSHA256 }
func (m *MediaMetadata) GetFileSHA256() []byte    { return m.FileSHA256 }
func (m *MediaMetadata) GetFileLength() uint64    { return m.FileLength }

type DownloadTask struct {
	ID         string
	ChatJID    string
	SenderJID  string
	MsgType    string
	Metadata   *MediaMetadata
	SenderName string
	TimeStr    string
	Avatar     *gdk.Texture
	IsCont     bool
}

type MediaService struct {
	Backend     *backend.Backend
	DB          *database.AppDB
	ctx         context.Context
	mediaQueue  chan DownloadTask
	onMediaDown func(task DownloadTask, data []byte, path string)
}

func NewMediaService(b *backend.Backend, db *database.AppDB, ctx context.Context) *MediaService {
	ms := &MediaService{
		Backend:    b,
		DB:         db,
		ctx:        ctx,
		mediaQueue: make(chan DownloadTask, 100),
	}
	go ms.mediaWorker()
	return ms
}

func (ms *MediaService) SetOnMediaDownloaded(f func(task DownloadTask, data []byte, path string)) {
	ms.onMediaDown = f
}

func (ms *MediaService) Download(task DownloadTask) {
	ms.mediaQueue <- task
}

func (ms *MediaService) mediaWorker() {
	for task := range ms.mediaQueue {
		ext := ".jpg"
		switch task.MsgType {
		case "sticker": ext = ".webp"
		case "video": ext = ".mp4"
		case "audio": ext = ".ogg"
		case "document": ext = ".bin"
		}
		path := filepath.Join("media", task.ID+ext)
		
		if _, err := os.Stat(path); err == nil {
			ms.persistMediaMessage(task, path)
			if ms.onMediaDown != nil { ms.onMediaDown(task, nil, path) }
			continue
		}

		if task.Metadata == nil { continue }

		var downloadable whatsmeow.DownloadableMessage
		switch task.MsgType {
		case "image":
			downloadable = &waProto.ImageMessage{
				URL:           proto.String(task.Metadata.URL),
				DirectPath:    proto.String(task.Metadata.DirectPath),
				MediaKey:      task.Metadata.MediaKey,
				Mimetype:      proto.String(task.Metadata.Mimetype),
				FileEncSHA256: task.Metadata.FileEncSHA256,
				FileSHA256:    task.Metadata.FileSHA256,
				FileLength:    proto.Uint64(task.Metadata.FileLength),
			}
		case "sticker":
			downloadable = &waProto.StickerMessage{
				URL:           proto.String(task.Metadata.URL),
				DirectPath:    proto.String(task.Metadata.DirectPath),
				MediaKey:      task.Metadata.MediaKey,
				Mimetype:      proto.String(task.Metadata.Mimetype),
				FileEncSHA256: task.Metadata.FileEncSHA256,
				FileSHA256:    task.Metadata.FileSHA256,
				FileLength:    proto.Uint64(task.Metadata.FileLength),
			}
		case "video":
			downloadable = &waProto.VideoMessage{
				URL:           proto.String(task.Metadata.URL),
				DirectPath:    proto.String(task.Metadata.DirectPath),
				MediaKey:      task.Metadata.MediaKey,
				Mimetype:      proto.String(task.Metadata.Mimetype),
				FileEncSHA256: task.Metadata.FileEncSHA256,
				FileSHA256:    task.Metadata.FileSHA256,
				FileLength:    proto.Uint64(task.Metadata.FileLength),
			}
		case "audio":
			downloadable = &waProto.AudioMessage{
				URL:           proto.String(task.Metadata.URL),
				DirectPath:    proto.String(task.Metadata.DirectPath),
				MediaKey:      task.Metadata.MediaKey,
				Mimetype:      proto.String(task.Metadata.Mimetype),
				FileEncSHA256: task.Metadata.FileEncSHA256,
				FileSHA256:    task.Metadata.FileSHA256,
				FileLength:    proto.Uint64(task.Metadata.FileLength),
			}
		case "document":
			downloadable = &waProto.DocumentMessage{
				URL:           proto.String(task.Metadata.URL),
				DirectPath:    proto.String(task.Metadata.DirectPath),
				MediaKey:      task.Metadata.MediaKey,
				Mimetype:      proto.String(task.Metadata.Mimetype),
				FileEncSHA256: task.Metadata.FileEncSHA256,
				FileSHA256:    task.Metadata.FileSHA256,
				FileLength:    proto.Uint64(task.Metadata.FileLength),
			}
		}

		if downloadable == nil { continue }

		data, err := ms.Backend.Client.Download(ms.ctx, downloadable)
		if err == nil {
			os.WriteFile(path, data, 0644)
			ms.persistMediaMessage(task, path)
			
			if ms.onMediaDown != nil {
				ms.onMediaDown(task, data, path)
			}
		} else {
			fmt.Printf("MediaService: Download Failed for %s: %v\n", task.ID, err)
		}
	}
}

func (ms *MediaService) persistMediaMessage(task DownloadTask, path string) {
	ms.DB.UpdateMessageContent(task.ID, task.ChatJID, path)
}
