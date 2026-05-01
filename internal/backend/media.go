package backend

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
)

func (b *Backend) DownloadMedia(ctx context.Context, msg *events.Message) ([]byte, error) {
	// whatsmeow has a Download method that takes the message info
	// We need to find which part of the message is downloadable
	var downloadable whatsmeow.DownloadableMessage

	if img := msg.Message.GetImageMessage(); img != nil {
		downloadable = img
	} else if stkr := msg.Message.GetStickerMessage(); stkr != nil {
		downloadable = stkr
	} else if aud := msg.Message.GetAudioMessage(); aud != nil {
		downloadable = aud
	} else if vid := msg.Message.GetVideoMessage(); vid != nil {
		downloadable = vid
	} else if doc := msg.Message.GetDocumentMessage(); doc != nil {
		downloadable = doc
	}

	if downloadable == nil {
		return nil, fmt.Errorf("message is not downloadable")
	}

	return b.Client.Download(ctx, downloadable)
}
