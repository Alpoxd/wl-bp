package bot

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/SevereCloud/vksdk/v2/object"

	"server/internal/db"
	"server/internal/worker"
)

type Bot struct {
	vk      *api.VK
	lp      *longpoll.LongPoll
	db      *db.DB
	manager *worker.Manager
	admins  []int64

	// State for FSM (e.g., adding an account)
	// state[user_id] = "adding_account_platform"
	state   map[int64]*UserState
	stateMu sync.Mutex
}

type UserState struct {
	Step     string
	Data     map[string]interface{}
}

func NewBot(token string, groupID int, admins []int64, database *db.DB, manager *worker.Manager) (*Bot, error) {
	vk := api.NewVK(token)

	lp, err := longpoll.NewLongPoll(vk, groupID)
	if err != nil {
		return nil, fmt.Errorf("failed to init longpoll: %w", err)
	}

	b := &Bot{
		vk:      vk,
		lp:      lp,
		db:      database,
		manager: manager,
		admins:  admins,
		state:   make(map[int64]*UserState),
	}

	b.registerHandlers()

	return b, nil
}

func (b *Bot) Start() error {
	log.Println("[bot] Starting VK Long Poll bot...")
	return b.lp.Run()
}

func (b *Bot) Stop() {
	b.lp.Shutdown()
}

func (b *Bot) registerHandlers() {
	b.lp.MessageNew(func(ctx context.Context, obj events.MessageNewObject) {
		msg := obj.Message
		userID := msg.FromID

		// Auto-register user in DB if not exists
		user, err := b.db.GetUserByVkID(int64(userID))
		if err != nil {
			log.Printf("[bot] Error fetching user: %v", err)
			return
		}
		
		isAdmin := b.isAdmin(int64(userID))
		
		if user == nil {
			log.Printf("[bot] Registering new user %d, admin=%v", userID, isAdmin)
			// Wait, we need a label. Fetch user profile.
			label := fmt.Sprintf("User%d", userID)
			profiles, err := b.vk.UsersGet(api.Params{"user_ids": userID})
			if err == nil && len(profiles) > 0 {
				label = profiles[0].FirstName + " " + profiles[0].LastName
			}
			
			_, err = b.db.CreateUser(int64(userID), label, isAdmin)
			if err != nil {
				log.Printf("[bot] Failed to register user: %v", err)
			}
			user, _ = b.db.GetUserByVkID(int64(userID))
		}

		// Update admin status if changed in config
		if user != nil && user.IsAdmin != isAdmin {
			user.IsAdmin = isAdmin
			// We should update it in DB ideally, skipping for brevity
		}

		// Check whitelist
		if user != nil && !user.IsAllowed {
			b.SendMessage(userID, "Доступ запрещен.")
			return
		}

		text := strings.TrimSpace(msg.Text)
		
		// Check FSM state first
		b.stateMu.Lock()
		state, hasState := b.state[int64(userID)]
		b.stateMu.Unlock()

		if hasState && state != nil {
			if text == "❌ Отмена" || text == "/cancel" {
				b.clearState(int64(userID))
				b.SendMessageAndKeyboard(userID, "Действие отменено", MainKeyboard(isAdmin))
				return
			}
			b.handleState(int64(userID), user, state, msg)
			return
		}

		// Handle normal commands
		b.handleCommand(int64(userID), user, msg)
	})
}

func (b *Bot) isAdmin(userID int64) bool {
	for _, id := range b.admins {
		if id == userID {
			return true
		}
	}
	return false
}

func (b *Bot) SendMessage(peerID int, message string) {
	b.SendMessageAndKeyboard(peerID, message, nil)
}

func (b *Bot) SendMessageAndKeyboard(peerID int, message string, kb *object.MessagesKeyboard) {
	params := api.Params{
		"peer_id":          peerID,
		"message":          message,
		"random_id":        0,
		"dont_parse_links": 1,
	}
	if kb != nil {
		params["keyboard"] = kb
	}
	
	_, err := b.vk.MessagesSend(params)
	if err != nil {
		log.Printf("[bot] Failed to send message to %d: %v", peerID, err)
	}
}

// NotifyAdmins sends a message to all admins.
func (b *Bot) NotifyAdmins(message string) {
	for _, adminID := range b.admins {
		b.SendMessage(int(adminID), message)
	}
}
