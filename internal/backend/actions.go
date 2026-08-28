package backend

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"whats-gtk/internal/database"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

func (b *Backend) GenerateMessageID() string {
	if b.Client != nil {
		return string(b.Client.GenerateMessageID())
	}
	return string(whatsmeow.GenerateMessageID())
}

func (b *Backend) SendText(ctx context.Context, to types.JID, text string, contextInfo *waProto.ContextInfo, extra ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	if contextInfo != nil {
		return b.Client.SendMessage(ctx, to, &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:        proto.String(text),
				ContextInfo: contextInfo,
			},
		}, extra...)
	}
	return b.Client.SendMessage(ctx, to, &waProto.Message{
		Conversation: proto.String(text),
	}, extra...)
}

func (b *Backend) MarkRead(ctx context.Context, jid types.JID, ids []string, sender types.JID, timestamp time.Time) error {
	mids := make([]types.MessageID, len(ids))
	for i, id := range ids { mids[i] = types.MessageID(id) }
	return b.Client.MarkRead(ctx, mids, timestamp, jid, sender)
}

func (b *Backend) GetChats(ctx context.Context) ([]types.JID, error) {
	return nil, nil
}

func (b *Backend) LoadHistory(ctx context.Context, jid types.JID, count int, before types.MessageID) ([]interface{}, error) {
	return nil, nil
}

func (b *Backend) GetAllContacts(ctx context.Context) (map[types.JID]types.ContactInfo, error) {
	return b.Client.Store.Contacts.GetAllContacts(ctx)
}

func (b *Backend) GetJoinedGroups(ctx context.Context) ([]*types.GroupInfo, error) {
	return b.Client.GetJoinedGroups(ctx)
}

func (b *Backend) SendReaction(ctx context.Context, chat types.JID, msgID types.MessageID, fromMe bool, reaction string) (whatsmeow.SendResponse, error) {
	return b.Client.SendMessage(ctx, chat, &waProto.Message{
		ReactionMessage: &waProto.ReactionMessage{
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chat.ToNonAD().String()),
				FromMe:    proto.Bool(fromMe),
				ID:        proto.String(msgID),
			},
			Text:              proto.String(reaction),
			GroupingKey:       proto.String(reaction),
			SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	})
}

func (b *Backend) PinMessage(ctx context.Context, chat types.JID, msgID types.MessageID, fromMe bool, pin bool, duration uint32) (whatsmeow.SendResponse, error) {
	pinType := waProto.PinInChatMessage_PIN_FOR_ALL
	if !pin {
		pinType = waProto.PinInChatMessage_UNPIN_FOR_ALL
	}

	pinMsg := &waProto.PinInChatMessage{
		Key: &waProto.MessageKey{
			RemoteJID: proto.String(chat.ToNonAD().String()),
			FromMe:    proto.Bool(fromMe),
			ID:        proto.String(msgID),
		},
		Type:              &pinType,
		SenderTimestampMS: proto.Int64(time.Now().UnixMilli()),
	}

	msg := &waProto.Message{
		ProtocolMessage: &waProto.ProtocolMessage{
			Type: waProto.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key: &waProto.MessageKey{
				RemoteJID: proto.String(chat.ToNonAD().String()),
				FromMe:    proto.Bool(true),
				ID:        proto.String(msgID),
			},
			EditedMessage: &waProto.Message{
				PinInChatMessage: pinMsg,
			},
			TimestampMS: proto.Int64(time.Now().UnixMilli()),
		},
	}

	if pin && duration > 0 {
		msg.ProtocolMessage.EditedMessage.MessageContextInfo = &waProto.MessageContextInfo{
			MessageAddOnDurationInSecs: proto.Uint32(duration),
		}
	}

	return b.Client.SendMessage(ctx, chat, msg)
}


func (b *Backend) SendImage(ctx context.Context, to types.JID, data []byte, mimetype string) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
}

func (b *Backend) SendVideo(ctx context.Context, to types.JID, data []byte, mimetype string) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaVideo)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		VideoMessage: &waProto.VideoMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
}

func (b *Backend) SendAudio(ctx context.Context, to types.JID, data []byte, mimetype string) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaAudio)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		AudioMessage: &waProto.AudioMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
		},
	})
}

func (b *Backend) SendDocument(ctx context.Context, to types.JID, data []byte, mimetype, filename string) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaDocument)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		DocumentMessage: &waProto.DocumentMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String(mimetype),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			FileName:      proto.String(filename),
		},
	})
}

func (b *Backend) SendSticker(ctx context.Context, to types.JID, data []byte, isAnimated bool) (whatsmeow.SendResponse, error) {
	resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return whatsmeow.SendResponse{}, err
	}

	return b.Client.SendMessage(ctx, to, &waProto.Message{
		StickerMessage: &waProto.StickerMessage{
			URL:           proto.String(resp.URL),
			DirectPath:    proto.String(resp.DirectPath),
			MediaKey:      resp.MediaKey,
			Mimetype:      proto.String("image/webp"),
			FileEncSHA256: resp.FileEncSHA256,
			FileSHA256:    resp.FileSHA256,
			FileLength:    proto.Uint64(uint64(len(data))),
			IsAnimated:    proto.Bool(isAnimated),
		},
	})
}

func (b *Backend) FetchFavoriteStickers(ctx context.Context) error {
	if b.Client == nil {
		return fmt.Errorf("client not connected")
	}
	return b.Client.FetchAppState(ctx, appstate.WAPatchRegularLow, true, false)
}

func (b *Backend) SetFavoriteStickerAppState(ctx context.Context, item database.StickerItem, isFavorite bool) error {
	if b.Client == nil {
		return fmt.Errorf("client not connected")
	}
	syncKey := item.ID
	if len(item.MediaSHA256) > 0 {
		syncKey = hex.EncodeToString(item.MediaSHA256)
	}
	patch := appstate.PatchInfo{
		Type: appstate.WAPatchRegularLow,
		Mutations: []appstate.MutationInfo{
			{
				Index:   []string{appstate.IndexFavoriteSticker, syncKey},
				Version: 1,
				Value: &waSyncAction.SyncActionValue{
					StickerAction: &waSyncAction.StickerAction{
						URL:           proto.String(item.MediaURL),
						DirectPath:    proto.String(item.MediaDirectPath),
						MediaKey:      item.MediaKey,
						FileEncSHA256: item.MediaEncSHA256,
						Mimetype:      proto.String("image/webp"),
						IsFavorite:    proto.Bool(isFavorite),
					},
				},
			},
		},
	}
	return b.Client.SendAppState(ctx, patch)
}

func (b *Backend) SendPollVote(ctx context.Context, chat types.JID, msgID string, sender types.JID, isFromMe bool, optionNames []string) error {
	// For LID groups, we must ensure the Participant in the MessageKey matches the real sender
	// the poll was created with (LID or PN). We can get this from the message secrets store.
	if _, realSender, err := b.Client.Store.MsgSecrets.GetMessageSecret(ctx, chat, sender, msgID); err == nil && !realSender.IsEmpty() {
		sender = realSender
	}

	info := &types.MessageInfo{
		ID: msgID,
		MessageSource: types.MessageSource{
			Chat:     chat,
			Sender:   sender,
			IsFromMe: isFromMe,
			IsGroup:  chat.Server == types.GroupServer,
		},
	}
	
	voteMsg, err := b.Client.BuildPollVote(ctx, info, optionNames)
	if err != nil {
		return err
	}
	
	_, err = b.Client.SendMessage(ctx, chat, voteMsg)
	return err
}

func (b *Backend) ForwardMessage(ctx context.Context, to types.JID, msg database.Message) (whatsmeow.SendResponse, error) {
	contextInfo := &waProto.ContextInfo{
		IsForwarded: proto.Bool(true),
	}

	var waMsg *waProto.Message

	switch msg.Type {
	case "text":
		waMsg = &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:        proto.String(msg.Content),
				ContextInfo: contextInfo,
			},
		}

	case "image":
		caption := ""
		if msg.Caption.Valid && msg.Caption.String != "" {
			caption = msg.Caption.String
		}
		if len(msg.MediaKey) > 0 && msg.MediaDirectPath.Valid {
			waMsg = &waProto.Message{
				ImageMessage: &waProto.ImageMessage{
					URL:           proto.String(msg.MediaURL.String),
					DirectPath:    proto.String(msg.MediaDirectPath.String),
					MediaKey:      msg.MediaKey,
					Mimetype:      proto.String(msg.MediaMimetype.String),
					FileEncSHA256: msg.MediaEncSHA256,
					FileSHA256:    msg.MediaSHA256,
					FileLength:    proto.Uint64(uint64(msg.MediaLength.Int64)),
					Caption:       proto.String(caption),
					ContextInfo:   contextInfo,
				},
			}
		} else if msg.Content != "" {
			data, err := os.ReadFile(msg.Content)
			if err == nil {
				mtype := msg.MediaMimetype.String
				if mtype == "" {
					mtype = "image/jpeg"
				}
				resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaImage)
				if err == nil {
					waMsg = &waProto.Message{
						ImageMessage: &waProto.ImageMessage{
							URL:           proto.String(resp.URL),
							DirectPath:    proto.String(resp.DirectPath),
							MediaKey:      resp.MediaKey,
							Mimetype:      proto.String(mtype),
							FileEncSHA256: resp.FileEncSHA256,
							FileSHA256:    resp.FileSHA256,
							FileLength:    proto.Uint64(uint64(len(data))),
							Caption:       proto.String(caption),
							ContextInfo:   contextInfo,
						},
					}
				}
			}
		}

	case "video":
		caption := ""
		if msg.Caption.Valid && msg.Caption.String != "" {
			caption = msg.Caption.String
		}
		if len(msg.MediaKey) > 0 && msg.MediaDirectPath.Valid {
			waMsg = &waProto.Message{
				VideoMessage: &waProto.VideoMessage{
					URL:           proto.String(msg.MediaURL.String),
					DirectPath:    proto.String(msg.MediaDirectPath.String),
					MediaKey:      msg.MediaKey,
					Mimetype:      proto.String(msg.MediaMimetype.String),
					FileEncSHA256: msg.MediaEncSHA256,
					FileSHA256:    msg.MediaSHA256,
					FileLength:    proto.Uint64(uint64(msg.MediaLength.Int64)),
					Caption:       proto.String(caption),
					ContextInfo:   contextInfo,
				},
			}
		} else if msg.Content != "" {
			data, err := os.ReadFile(msg.Content)
			if err == nil {
				mtype := msg.MediaMimetype.String
				if mtype == "" {
					mtype = "video/mp4"
				}
				resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaVideo)
				if err == nil {
					waMsg = &waProto.Message{
						VideoMessage: &waProto.VideoMessage{
							URL:           proto.String(resp.URL),
							DirectPath:    proto.String(resp.DirectPath),
							MediaKey:      resp.MediaKey,
							Mimetype:      proto.String(mtype),
							FileEncSHA256: resp.FileEncSHA256,
							FileSHA256:    resp.FileSHA256,
							FileLength:    proto.Uint64(uint64(len(data))),
							Caption:       proto.String(caption),
							ContextInfo:   contextInfo,
						},
					}
				}
			}
		}

	case "audio":
		if len(msg.MediaKey) > 0 && msg.MediaDirectPath.Valid {
			waMsg = &waProto.Message{
				AudioMessage: &waProto.AudioMessage{
					URL:           proto.String(msg.MediaURL.String),
					DirectPath:    proto.String(msg.MediaDirectPath.String),
					MediaKey:      msg.MediaKey,
					Mimetype:      proto.String(msg.MediaMimetype.String),
					FileEncSHA256: msg.MediaEncSHA256,
					FileSHA256:    msg.MediaSHA256,
					FileLength:    proto.Uint64(uint64(msg.MediaLength.Int64)),
					ContextInfo:   contextInfo,
				},
			}
		} else if msg.Content != "" {
			data, err := os.ReadFile(msg.Content)
			if err == nil {
				mtype := msg.MediaMimetype.String
				if mtype == "" {
					mtype = "audio/ogg; codecs=opus"
				}
				resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaAudio)
				if err == nil {
					waMsg = &waProto.Message{
						AudioMessage: &waProto.AudioMessage{
							URL:           proto.String(resp.URL),
							DirectPath:    proto.String(resp.DirectPath),
							MediaKey:      resp.MediaKey,
							Mimetype:      proto.String(mtype),
							FileEncSHA256: resp.FileEncSHA256,
							FileSHA256:    resp.FileSHA256,
							FileLength:    proto.Uint64(uint64(len(data))),
							ContextInfo:   contextInfo,
						},
					}
				}
			}
		}

	case "document":
		filename := filepath.Base(msg.Content)
		if strings.HasPrefix(msg.Content, "[Document: ") {
			filename = strings.TrimSuffix(strings.TrimPrefix(msg.Content, "[Document: "), "]")
		}
		if len(msg.MediaKey) > 0 && msg.MediaDirectPath.Valid {
			waMsg = &waProto.Message{
				DocumentMessage: &waProto.DocumentMessage{
					URL:           proto.String(msg.MediaURL.String),
					DirectPath:    proto.String(msg.MediaDirectPath.String),
					MediaKey:      msg.MediaKey,
					Mimetype:      proto.String(msg.MediaMimetype.String),
					FileEncSHA256: msg.MediaEncSHA256,
					FileSHA256:    msg.MediaSHA256,
					FileLength:    proto.Uint64(uint64(msg.MediaLength.Int64)),
					FileName:      proto.String(filename),
					ContextInfo:   contextInfo,
				},
			}
		} else if msg.Content != "" && !strings.HasPrefix(msg.Content, "[Document:") {
			data, err := os.ReadFile(msg.Content)
			if err == nil {
				mtype := msg.MediaMimetype.String
				if mtype == "" {
					mtype = "application/octet-stream"
				}
				resp, err := b.Client.Upload(ctx, data, whatsmeow.MediaDocument)
				if err == nil {
					waMsg = &waProto.Message{
						DocumentMessage: &waProto.DocumentMessage{
							URL:           proto.String(resp.URL),
							DirectPath:    proto.String(resp.DirectPath),
							MediaKey:      resp.MediaKey,
							Mimetype:      proto.String(mtype),
							FileEncSHA256: resp.FileEncSHA256,
							FileSHA256:    resp.FileSHA256,
							FileLength:    proto.Uint64(uint64(len(data))),
							FileName:      proto.String(filename),
							ContextInfo:   contextInfo,
						},
					}
				}
			}
		}

	case "sticker":
		if len(msg.MediaKey) > 0 && msg.MediaDirectPath.Valid {
			waMsg = &waProto.Message{
				StickerMessage: &waProto.StickerMessage{
					URL:           proto.String(msg.MediaURL.String),
					DirectPath:    proto.String(msg.MediaDirectPath.String),
					MediaKey:      msg.MediaKey,
					Mimetype:      proto.String(msg.MediaMimetype.String),
					FileEncSHA256: msg.MediaEncSHA256,
					FileSHA256:    msg.MediaSHA256,
					FileLength:    proto.Uint64(uint64(msg.MediaLength.Int64)),
					ContextInfo:   contextInfo,
				},
			}
		}
	}

	if waMsg == nil {
		waMsg = &waProto.Message{
			ExtendedTextMessage: &waProto.ExtendedTextMessage{
				Text:        proto.String(msg.Content),
				ContextInfo: contextInfo,
			},
		}
	}

	return b.Client.SendMessage(ctx, to, waMsg)
}
