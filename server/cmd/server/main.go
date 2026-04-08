package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"server/internal/accounts"
	"server/internal/bot"
	"server/internal/browser"
	"server/internal/config"
	"server/internal/db"
	"server/internal/worker"
)

func main() {
	log.Println("[server] Starting WL-BP Server...")
	cfg := config.LoadConfig()

	// 1. Setup DB
	database, err := db.Connect(cfg.DBPath)
	if err != nil {
		log.Fatalf("[server] Failed to connect DB: %v", err)
	}
	defer database.Close()

	// 2. Setup Chromedp
	b := browser.NewBrowser()
	defer b.Close()

	// 3. Setup Account Pool
	pool := accounts.NewAccountPool(database)
	pool.StartRecoveryRoutine(cfg.AccountCooldownMin)

	// 4. Setup Worker Manager
	manager := worker.NewManager(database, pool, b)

	// 5. Setup Bot
	vkBot, err := bot.NewBot(cfg.VKBotToken, cfg.VKBotGroupID, cfg.AdminVKUserIDs, database, manager)
	if err != nil {
		log.Fatalf("[server] Failed to init VK bot: %v", err)
	}

	go func() {
		if err := vkBot.Start(); err != nil {
			log.Fatalf("[server] VK bot error: %v", err)
		}
	}()
	defer vkBot.Stop()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[server] Server started successfully")
	sig := <-sigCh

	log.Printf("[server] Received %v signal, shutting down...", sig)
	manager.StopAll()
	log.Println("[server] Shutdown complete")
}
