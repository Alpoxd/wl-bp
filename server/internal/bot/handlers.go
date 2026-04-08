package bot

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SevereCloud/vksdk/v2/object"
	
	"server/internal/accounts"
	"server/internal/db"
)

func (b *Bot) handleCommand(userID int64, user *db.User, msg object.MessagesMessage) {
	text := strings.TrimSpace(msg.Text)
	lowerText := strings.ToLower(text)

	// User Commands
	switch {
	case lowerText == "начать", lowerText == "/start":
		b.SendMessageAndKeyboard(int(userID), "👋 Добро пожаловать.\nВыберите действие:", MainKeyboard(user.IsAdmin))
		return

	case lowerText == "📞 vk dc":
		b.startTunnel(int(userID), user, "vk", "dc")
		return
	case lowerText == "📹 vk video":
		b.startTunnel(int(userID), user, "vk", "video")
		return
	case lowerText == "📞 tm dc":
		b.startTunnel(int(userID), user, "yandex", "dc")
		return
	case lowerText == "📹 tm video":
		b.startTunnel(int(userID), user, "yandex", "video")
		return

	case lowerText == "📋 сессии", lowerText == "/list":
		b.listSessions(int(userID), user)
		return
	}

	// Admin Commands
	if !user.IsAdmin {
		b.SendMessage(int(userID), "Неизвестная команда.")
		return
	}

	switch {
	case lowerText == "👤 аккаунты", lowerText == "/accounts":
		b.listAccounts(int(userID))
		return
	case lowerText == "📊 статус", lowerText == "/status":
		b.showStatus(int(userID))
		return
	case lowerText == "/add":
		b.startAddAccountFSM(userID)
		return
	}

	if strings.HasPrefix(lowerText, "/remove ") {
		idStr := strings.TrimSpace(text[len("/remove "):])
		b.removeAccount(int(userID), idStr)
		return
	}
	
	if strings.HasPrefix(lowerText, "/kill ") {
		idStr := strings.TrimSpace(text[len("/kill "):])
		b.killSession(int(userID), idStr)
		return
	}

	b.SendMessage(int(userID), "Неизвестная команда.")
}

func (b *Bot) startTunnel(peerID int, user *db.User, platform, mode string) {
	b.SendMessage(peerID, fmt.Sprintf("⏳ Создаю туннель %s %s...", platform, mode))
	
	// Delegate to manager
	sessionID, link, err := b.manager.StartSession(user.ID, platform, mode)
	if err != nil {
		b.SendMessageAndKeyboard(peerID, fmt.Sprintf("❌ Ошибка: %v", err), MainKeyboard(user.IsAdmin))
		return
	}

	reply := fmt.Sprintf("✅ Туннель создан (сессия #%d)!\n🔗 Линка: %s", sessionID, link)
	b.SendMessageAndKeyboard(peerID, reply, MainKeyboard(user.IsAdmin))
}

func (b *Bot) listSessions(peerID int, user *db.User) {
	sessList, err := b.db.GetActiveSessions()
	if err != nil {
		b.SendMessage(peerID, fmt.Sprintf("Ошибка: %v", err))
		return
	}

	if len(sessList) == 0 {
		b.SendMessage(peerID, "Нет активных сессий.")
		return
	}

	var sb strings.Builder
	sb.WriteString("📋 Активные сессии:\n\n")
	for _, s := range sessList {
		if !user.IsAdmin && s.UserID != user.ID {
			continue // Users see only their own, admins see all
		}
		
		stats := b.manager.GetSessionStats(s.ID)
		uptime := "-"
		bytesText := ""
		if stats != nil {
			uptime = stats.Uptime.Round(time.Second).String()
			bytesText = fmt.Sprintf(" (TX: %d, RX: %d)", stats.BytesTX, stats.BytesRX)
		}
		
		sb.WriteString(fmt.Sprintf("🔹 #%d: %s (%s) — %s\n⏳ Uptime: %s%s\n🔗 %s\n\n", 
			s.ID, s.Platform, s.Mode, s.Status, uptime, bytesText, s.JoinLink))
	}
	b.SendMessage(peerID, sb.String())
}

func (b *Bot) listAccounts(peerID int) {
	accs, err := b.db.ListHostAccounts()
	if err != nil {
		b.SendMessage(peerID, fmt.Sprintf("Ошибка: %v", err))
		return
	}

	if len(accs) == 0 {
		b.SendMessage(peerID, "Список аккаунтов пуст. Используйте /add")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("👤 Список аккаунтов (всего %d):\n\n", len(accs)))
	for _, a := range accs {
		sb.WriteString(fmt.Sprintf("🔹 #%d [%s] %s\n", a.ID, a.Platform, a.Label))
		sb.WriteString(fmt.Sprintf("   Статус: %s (workers: %d/%d)\n", a.Status, a.ActiveWorkers, a.MaxConcurrentCalls))
		if a.FailCount > 0 {
			sb.WriteString(fmt.Sprintf("   Ошибок подряд: %d\n", a.FailCount))
		}
		sb.WriteString("\n")
	}
	b.SendMessage(peerID, sb.String())
}

func (b *Bot) showStatus(peerID int) {
	// Simple stub for status, usually we combine stats
	b.SendMessage(peerID, "Статус системы в норме.\nИспользуйте /list и /accounts для деталей.")
}

func (b *Bot) removeAccount(peerID int, idStr string) {
	// DB logic
	b.SendMessage(peerID, fmt.Sprintf("Аккаунт %s удален (stub).", idStr))
}

func (b *Bot) killSession(peerID int, idStr string) {
	var sessionID int
	fmt.Sscanf(idStr, "%d", &sessionID)
	if sessionID > 0 {
		b.manager.StopSession(sessionID)
		b.SendMessage(peerID, fmt.Sprintf("Сессия #%d закрыта.", sessionID))
	}
}

// FSM Logic

func (b *Bot) startAddAccountFSM(userID int64) {
	b.stateMu.Lock()
	b.state[userID] = &UserState{
		Step: "adding_account_platform",
		Data: make(map[string]interface{}),
	}
	b.stateMu.Unlock()
	b.SendMessageAndKeyboard(int(userID), "Добавление аккаунта.\nВведите платформу (vk или yandex):", CancelKeyboard())
}

func (b *Bot) handleState(userID int64, user *db.User, state *UserState, msg object.MessagesMessage) {
	text := strings.TrimSpace(msg.Text)
	
	switch state.Step {
	case "adding_account_platform":
		platform := strings.ToLower(text)
		if platform != "vk" && platform != "yandex" {
			b.SendMessageAndKeyboard(int(userID), "Пожалуйста, введите 'vk' или 'yandex'.", CancelKeyboard())
			return
		}
		state.Data["platform"] = platform
		state.Step = "adding_account_label"
		b.SendMessageAndKeyboard(int(userID), "Введите название (label) аккаунта:", CancelKeyboard())

	case "adding_account_label":
		state.Data["label"] = text
		state.Step = "adding_account_cookies"
		b.SendMessageAndKeyboard(int(userID), "Отправьте cookies текстом, или загрузите файл .json/.txt:", CancelKeyboard())

	case "adding_account_cookies":
		cookies := text
		// Handle document attachment if text is empty
		if cookies == "" && len(msg.Attachments) > 0 {
			doc := msg.Attachments[0].Doc
			if doc.URL != "" {
				// download doc stub
				cookies = "dummy_cookies_from_doc" // we'll implement downloader later
			}
		}

		platform := state.Data["platform"].(string)
		label := state.Data["label"].(string)
		
		log.Printf("[bot] Add account Request: platform=%s, label=%s", platform, label)

		// Validate
		if platform == "vk" {
			err := accounts.ValidateVKCookies(cookies)
			if err != nil {
				b.SendMessageAndKeyboard(int(userID), fmt.Sprintf("❌ Ошибка валидации cookies: %v\nПопробуйте отправить валидные куки.", err), CancelKeyboard())
				return
			}
		}

		maxConcurrent := 1
		if platform == "yandex" {
			maxConcurrent = 0 // unlimited
		}

		id, err := b.db.CreateHostAccount(platform, label, cookies, maxConcurrent)
		if err != nil {
			b.SendMessageAndKeyboard(int(userID), fmt.Sprintf("❌ Ошибка сохранения: %v", err), MainKeyboard(user.IsAdmin))
		} else {
			b.SendMessageAndKeyboard(int(userID), fmt.Sprintf("✅ Аккаунт '%s' успешно добавлен!\nID: %d", label, id), MainKeyboard(user.IsAdmin))
		}
		
		b.clearState(userID)
	}
}

func (b *Bot) clearState(userID int64) {
	b.stateMu.Lock()
	delete(b.state, userID)
	b.stateMu.Unlock()
}
