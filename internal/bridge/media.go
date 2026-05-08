package bridge

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
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
	fmt.Printf("MediaService: Download requested for %s (type %s)\n", task.ID, task.MsgType)
	ms.mediaQueue <- task
}

func (ms *MediaService) mediaWorker() {
	for task := range ms.mediaQueue {
		ext := ".bin"
		if task.Metadata != nil && task.Metadata.Mimetype != "" {
			mtype := strings.Split(task.Metadata.Mimetype, ";")[0]
			if exts, _ := mime.ExtensionsByType(mtype); len(exts) > 0 {
				ext = exts[len(exts)-1]
			}
		}

		// Try to find the original filename if it's a document
		filename := task.ID + ext
		msg, err := ms.DB.GetMessage(task.ID)
		if err == nil && msg.Type == "document" && strings.HasPrefix(msg.Content, "[Document: ") {
			origName := strings.TrimSuffix(strings.TrimPrefix(msg.Content, "[Document: "), "]")
			if origName != "" {
				// Clean filename for OS safety but keep the real name
				safeName := strings.Map(func(r rune) rune {
					if strings.ContainsRune("\\/:*?\"<>|", r) {
						return -1
					}
					return r
				}, origName)

				// Ensure it has the correct extension if missing
				if !strings.HasSuffix(strings.ToLower(safeName), strings.ToLower(ext)) {
					safeName += ext
				}
				filename = safeName
			}
		}

		path := filepath.Join("media", filename)
		
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
			if strings.Contains(err.Error(), "403") {
				fmt.Printf("MediaService: Attempting Media Retry Request for %s\n", task.ID)
				chatJID, _ := types.ParseJID(task.ChatJID)
				senderJID, _ := types.ParseJID(task.SenderJID)
				
				if len(task.Metadata.MediaKey) == 0 {
					fmt.Printf("MediaService: CANNOT RETRY %s - MediaKey is missing!\n", task.ID)
					return
				}

				info := &types.MessageInfo{
					ID: task.ID,
					MessageSource: types.MessageSource{
						Chat:     chatJID,
						Sender:   senderJID,
						IsGroup:  chatJID.Server == types.GroupServer,
						IsFromMe: false,
					},
				}
				// If it's from me, we need to adjust
				if msg, err := ms.DB.GetMessage(task.ID); err == nil {
					info.IsFromMe = msg.IsFromMe
					if info.IsFromMe && senderJID.IsEmpty() {
						info.Sender = ms.Backend.Device.ID.ToNonAD()
					}
				}

				err := ms.Backend.Client.SendMediaRetryReceipt(ms.ctx, info, task.Metadata.MediaKey)
				if err != nil {
					fmt.Printf("MediaService: Failed to send retry receipt for %s: %v\n", task.ID, err)
				}
			}
		}
	}
}

func (ms *MediaService) persistMediaMessage(task DownloadTask, path string) {
	ms.DB.UpdateMessageContent(task.ID, task.ChatJID, path)
}
