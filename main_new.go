package main

import (
	"log"
	"os"

	"tg-random-bot/internal/bot"
	"tg-random-bot/internal/config"
	"tg-random-bot/internal/game"
	"tg-random-bot/internal/storage"
	"tg-random-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func main() {
	// Инициализация конфигурации
	config.InitParticipants()

	// Инициализация игры
	game.InitGame()

	// Подключение к Redis
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	redisPassword := os.Getenv("REDIS_PASSWORD")
	redisDB := 0

	err := storage.InitRedis(redisAddr, redisPassword, redisDB)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	// Получение токена бота из переменной окружения
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN environment variable is required")
	}

	// Создание бота
	api, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	log.Printf("Bot authorized on account %s", api.Self.UserName)

	// Настройка обновлений
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := api.GetUpdatesChan(u)

	// Создание обработчика бота
	botHandler := bot.NewBot(api)

	// Основной цикл обработки сообщений
	for update := range updates {
		if update.Message != nil && update.Message.IsCommand() {
			botHandler.HandleCommand(update)
		}
	}
}
