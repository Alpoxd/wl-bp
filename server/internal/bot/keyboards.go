package bot

import (
	"github.com/SevereCloud/vksdk/v2/object"
)

// MainKeyboard returns the default keyboard for allowed users.
func MainKeyboard(isAdmin bool) *object.MessagesKeyboard {
	kb := object.NewMessagesKeyboard(false)
	
	kb.AddRow()
	kb.AddTextButton("📞 VK DC", "", "primary")
	kb.AddTextButton("📹 VK Video", "", "primary")

	kb.AddRow()
	kb.AddTextButton("📞 TM DC", "", "secondary")
	kb.AddTextButton("📹 TM Video", "", "secondary")

	kb.AddRow()
	kb.AddTextButton("📋 Сессии", "", "secondary")

	if isAdmin {
		kb.AddRow()
		kb.AddTextButton("👤 Аккаунты", "", "negative")
		kb.AddTextButton("📊 Статус", "", "negative")
	}

	return kb
}

// CancelKeyboard returns a simple cancel button
func CancelKeyboard() *object.MessagesKeyboard {
	kb := object.NewMessagesKeyboard(false)
	kb.AddRow()
	kb.AddTextButton("❌ Отмена", "", "negative")
	return kb
}
