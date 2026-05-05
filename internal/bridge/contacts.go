package bridge

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"whats-gtk/internal/backend"
	"whats-gtk/internal/database"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gdkpixbuf/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

type ContactService struct {
	Backend      *backend.Backend
	DB           *database.AppDB
	ctx          context.Context
	avatarCache  map[string]*gdk.Texture
	avatarQueue  chan string
	pendingFetch map[string]bool
	failedFetch  map[string]time.Time
	onAvatarSet  func(jid string, tex *gdk.Texture)
	mutex        sync.Mutex
}

func NewContactService(b *backend.Backend, db *database.AppDB, ctx context.Context) *ContactService {
	cs := &ContactService{
		Backend:      b,
		DB:           db,
		ctx:          ctx,
		avatarCache:  make(map[string]*gdk.Texture),
		avatarQueue:  make(chan string, 500),
		pendingFetch: make(map[string]bool),
		failedFetch:  make(map[string]time.Time),
	}
	go cs.avatarWorker()
	return cs
}

func (cs *ContactService) SetOnAvatarSet(f func(jid string, tex *gdk.Texture)) {
	cs.onAvatarSet = f
}

func (cs *ContactService) avatarWorker() {
	for jStr := range cs.avatarQueue {
		cs.mutex.Lock()
		if _, exists := cs.avatarCache[jStr]; exists {
			cs.pendingFetch[jStr] = false
			cs.mutex.Unlock()
			continue
		}
		cs.mutex.Unlock()
		
		jid, _ := types.ParseJID(jStr)
		time.Sleep(1 * time.Second)
		
		info, err := cs.Backend.Client.GetProfilePictureInfo(cs.ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true})
		
		cs.mutex.Lock()
		if err != nil {
			cs.failedFetch[jStr] = time.Now()
			cs.pendingFetch[jStr] = false
			cs.mutex.Unlock()
			continue
		}
		cs.mutex.Unlock()

		if info != nil && info.URL != "" {
			resp, err := http.Get(info.URL)
			if err == nil {
				defer resp.Body.Close()
				data, _ := io.ReadAll(resp.Body)
				path := filepath.Join("media", "avatar_"+jid.ToNonAD().String()+".jpg")
				os.WriteFile(path, data, 0644)
				cs.DB.SaveContact(database.Contact{JID: jid.ToNonAD().String(), AvatarPath: sql.NullString{String: path, Valid: true}})
				glib.IdleAdd(func() {
					pixbuf, _ := gdkpixbuf.NewPixbufFromFile(path)
					if pixbuf != nil {
						tex := gdk.NewTextureForPixbuf(pixbuf)
						cs.mutex.Lock()
						cs.avatarCache[jStr] = tex
						cs.mutex.Unlock()
						if cs.onAvatarSet != nil {
							cs.onAvatarSet(jStr, tex)
						}
					}
				})
			}
		}
		cs.mutex.Lock()
		cs.pendingFetch[jStr] = false
		cs.mutex.Unlock()
	}
}

func (cs *ContactService) GetAvatarNoFetch(j string) *gdk.Texture {
	cs.mutex.Lock()
	defer cs.mutex.Unlock()

	if tex, exists := cs.avatarCache[j]; exists {
		return tex
	}

	if c, err := cs.DB.GetContact(j); err == nil && c.AvatarPath.Valid && c.AvatarPath.String != "" {
		if _, err := os.Stat(c.AvatarPath.String); err == nil {
			pixbuf, _ := gdkpixbuf.NewPixbufFromFile(c.AvatarPath.String)
			if pixbuf != nil {
				tex := gdk.NewTextureForPixbuf(pixbuf)
				cs.avatarCache[j] = tex
				return tex
			}
		}
	}
	return nil
}

func (cs *ContactService) GetAvatar(j string) *gdk.Texture {
	if tex := cs.GetAvatarNoFetch(j); tex != nil {
		return tex
	}

	cs.mutex.Lock()
	if !cs.pendingFetch[j] {
		cs.pendingFetch[j] = true
		select {
		case cs.avatarQueue <- j:
		default:
			cs.pendingFetch[j] = false
		}
	}
	cs.mutex.Unlock()
	return nil
}

func (cs *ContactService) ResolveSenderName(j string) string {
	if strings.HasSuffix(j, "@lid") {
		cs.ResolveLIDMapping(j)
	}
	c, err := cs.DB.GetContact(j)
	if err == nil {
		if c.SavedName.Valid && c.SavedName.String != "" {
			return c.SavedName.String
		}
		if c.PushName.Valid && c.PushName.String != "" {
			return cs.formatNonAddedName(j, c.PushName.String)
		}
	}
	jidObj, _ := types.ParseJID(j)
	if info, err := cs.Backend.Client.Store.Contacts.GetContact(cs.ctx, jidObj); err == nil && info.Found {
		if info.FullName != "" {
			cs.DB.SaveContact(database.Contact{JID: j, SavedName: sql.NullString{String: info.FullName, Valid: true}})
			return info.FullName
		}
		if info.PushName != "" {
			cs.DB.SaveContact(database.Contact{JID: j, PushName: sql.NullString{String: info.PushName, Valid: true}})
			return cs.formatNonAddedName(j, info.PushName)
		}
	}
	return j
}

func (cs *ContactService) ResolveLIDMapping(lid string) {
	jidObj, _ := types.ParseJID(lid)
	info, err := cs.Backend.Client.Store.Contacts.GetContact(cs.ctx, jidObj)
	if err == nil && info.Found && info.FullName != "" {
		all, _ := cs.Backend.Client.Store.Contacts.GetAllContacts(cs.ctx)
		for pnJID, pnInfo := range all {
			if !strings.HasSuffix(pnJID.String(), "@lid") && pnInfo.FullName == info.FullName {
				cs.DB.MergeLID(pnJID.ToNonAD().String(), lid)
				break
			}
		}
	}
}

func (cs *ContactService) formatNonAddedName(j, p string) string {
	parts := strings.Split(j, "@")
	n := parts[0]
	if !strings.HasSuffix(j, "@lid") {
		n = cs.formatPhoneNumber(n)
	}
	return fmt.Sprintf("%s <span foreground='#8696a0' size='small'>%s</span>", p, n)
}

func (cs *ContactService) formatPhoneNumber(n string) string {
	if len(n) < 10 {
		return n
	}
	if strings.HasPrefix(n, "55") && len(n) >= 12 {
		return fmt.Sprintf("+55 %s %s-%s", n[2:4], n[4:9], n[9:])
	}
	return "+" + n
}

func (cs *ContactService) Sync(ctx context.Context) {
	groups, err := cs.Backend.GetJoinedGroups(ctx)
	if err == nil {
		for _, g := range groups {
			cs.DB.SaveContact(database.Contact{JID: g.JID.ToNonAD().String(), SavedName: sql.NullString{String: g.Name, Valid: g.Name != ""}, IsGroup: sql.NullBool{Bool: true, Valid: true}})
		}
	}
	contacts, err := cs.Backend.GetAllContacts(ctx)
	if err == nil {
		for j, i := range contacts {
			cs.DB.SaveContact(database.Contact{JID: j.ToNonAD().String(), SavedName: sql.NullString{String: i.FullName, Valid: i.FullName != ""}, PushName: sql.NullString{String: i.PushName, Valid: i.PushName != ""}, IsGroup: sql.NullBool{Bool: j.Server == types.GroupServer, Valid: true}})
		}
	}
}

func (cs *ContactService) HealContacts(ctx context.Context) {
	contacts, err := cs.DB.GetUnresolvedPNs(100)
	if err != nil || len(contacts) == 0 {
		return
	}
	for i := 0; i < len(contacts); i += 10 {
		end := i + 10
		if end > len(contacts) { end = len(contacts) }
		batch := contacts[i:end]
		jids := make([]types.JID, 0)
		for _, c := range batch {
			j, _ := types.ParseJID(c.JID); jids = append(jids, j)
		}
		resp, err := cs.Backend.Client.GetUserInfo(cs.ctx, jids)
		if err == nil {
			for jid, info := range resp {
				if !info.LID.IsEmpty() { cs.DB.MergeLID(jid.ToNonAD().String(), info.LID.ToNonAD().String()) }
			}
		}
		time.Sleep(10 * time.Second)
	}
}
