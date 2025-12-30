package bot

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"tg-random-bot/internal/config"
	"tg-random-bot/internal/game"
	"tg-random-bot/internal/models"
	"tg-random-bot/internal/storage"
	"tg-random-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot представляет Telegram бота
type Bot struct {
	API *tgbotapi.BotAPI
}

// NewBot создает новый экземпляр бота
func NewBot(api *tgbotapi.BotAPI) *Bot {
	return &Bot{API: api}
}

// HandleCommand обрабатывает команду от пользователя
func (b *Bot) HandleCommand(update tgbotapi.Update) {
	userName := update.Message.From.UserName
	if userName == "" {
		userName = update.Message.From.FirstName
	}

	log.Printf("Команда от пользователя %s: %s", userName, update.Message.Text)

	msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")
	msg.ReplyToMessageID = update.Message.MessageID

	command := update.Message.Command()
	args := update.Message.CommandArguments()

	switch command {
	case "start":
		b.handleStart(&msg, userName)
	case "help":
		b.handleHelp(&msg)
	case "balance", "bal":
		b.handleBalance(&msg, userName)
	case "shop":
		b.handleShop(&msg, userName, args)
	case "inv", "inventory":
		b.handleInventory(&msg, userName)
	case "wear":
		b.handleWear(&msg, userName, args)
	case "unwear":
		b.handleUnwear(&msg, userName)
	case "sell":
		b.handleSell(&msg, userName, args)
	case "give":
		b.handleGive(&msg, userName, args, update.Message.Chat.ID)
	case "payfine":
		b.handlePayFine(&msg, userName)
	case "shameboard":
		b.handleShameBoard(&msg)
	case "rob":
		b.handleRob(&msg, userName, args)
	case "platerob":
		b.handlePlateRob(&msg, userName, args)
	case "loadprizes":
		b.handleLoadPrizes(&msg, userName)
	case "removefromredis":
		b.handleRemoveFromRedis(&msg, userName, args)
	case "promote":
		b.handlePromote(&msg, userName, args, update.Message.Chat.ID)
	default:
		msg.Text = "ты долбоеб? не знаешь команд? пиши /help"
	}

	// Добавляем уведомление о долге к сообщению если нужно
	msg.Text = utils.AddDebtNotificationToMessage(userName, msg.Text)

	// Отправляем сообщение
	if _, err := b.API.Send(msg); err != nil {
		log.Panic(err)
	}
}

// handleStart обрабатывает команду /start
func (b *Bot) handleStart(msg *tgbotapi.MessageConfig, userName string) {
	msg.Text = fmt.Sprintf(`🎉 Привет, %s!

🏆 Добро пожаловать в игру "ГОНКА НА ВЫЖИВАНИЕ"!

🎯 Правила просты:
• Каждый раунд выбывает случайный участник
• Можно ставить на участников
• Выигрываешь, если твой участник остается в игре

💰 Текущий баланс: %d фишек

📋 Доступные команды:
/help - список всех команд
/balance - проверить баланс
/shop - магазин предметов
/inv - инвентарь
/give - подарить предмет

🎮 Для начала игры используйте команду /startgame (только админы)`, userName, getUserBalance(userName))
}

// handleHelp обрабатывает команду /help
func (b *Bot) handleHelp(msg *tgbotapi.MessageConfig) {
	msg.Text = `📋 СПИСОК КОМАНД:

🎮 ИГРА:
/startgame - начать новую игру (админы)
/status - статус текущей игры
/bet <имя> <сумма> - поставить на участника

💰 ЭКОНОМИКА:
/balance - проверить баланс
/shop - магазин предметов
/inv - посмотреть инвентарь
/sell <хэш> - продать предмет
/wear <хэш> - надеть плашку
/unwear - снять плашку
/give <username> <хэш> - подарить предмет

⚔️ АКТИВНОСТИ:
/rob <username> - ограбить игрока
/platerob <username> - украсть плашку
/payfine - оплатить штрафы

📊 ИНФОРМАЦИЯ:
/shameboard - доска позора должников
/help - эта справка

🔧 АДМИНСКИЕ:
/loadprizes - загрузить призы из файла
/removefromredis - удалить все призы
/promote <id> - повысить до админа`
}

// handleBalance обрабатывает команду /balance
func (b *Bot) handleBalance(msg *tgbotapi.MessageConfig, userName string) {
	balance := getUserBalance(userName)
	msg.Text = fmt.Sprintf("💰 Ваш баланс: %d %s", balance, utils.GetChipsWord(balance))
}

// handleShop обрабатывает команду /shop
func (b *Bot) handleShop(msg *tgbotapi.MessageConfig, userName string, args string) {
	if args == "" {
		msg.Text = `🛒 МАГАЗИН

💰 Доступные товары:

1️⃣ Оборудование для грабежа - 1,000 фишек
   Специальное оборудование для проведения грабежей
   📦 Хранится в инвентаре
   🎯 Шанс успеха: 30% (украсть до 50% баланса)
   💸 Штраф: 30% (потерять 10% баланса)
   🏃‍♂️ Бегство: 40% (ничего не происходит)

2️⃣ Оборудование для разведки - 100 фишек
   Позволяет шпионить за балансами и инвентарем других игроков
   📦 Хранится в инвентаре
   👁️ Шанс успеха: 70%

💡 Для покупки используйте:
/shop buy 1 [кол-во] (грабеж)
/shop buy 2 [кол-во] (разведка)`
		msg.ReplyToMessageID = 0
		return
	}

	// TODO: Implement shop buying logic
	msg.Text = "🛒 Функция магазина пока в разработке"
}

// handleInventory обрабатывает команду /inv
func (b *Bot) handleInventory(msg *tgbotapi.MessageConfig, userName string) {
	inventory, err := storage.GetPlayerInventory(userName)
	if err != nil {
		msg.Text = fmt.Sprintf("❌ Ошибка получения инвентаря: %v", err)
		return
	}

	if len(inventory) == 0 {
		msg.Text = "📦 Ваш инвентарь пуст"
		return
	}

	msg.Text = "📦 ВАШ ИНВЕНТАРЬ:\n\n"

	totalValue := 0
	commonItems := []models.InventoryItem{}
	rareItems := []models.InventoryItem{}
	legendaryItems := []models.InventoryItem{}
	shopItems := []models.InventoryItem{}

	for _, item := range inventory {
		totalValue += item.Cost * item.Count

		switch item.Rarity {
		case "common":
			commonItems = append(commonItems, item)
		case "rare":
			rareItems = append(rareItems, item)
		case "legendary":
			legendaryItems = append(legendaryItems, item)
		default:
			shopItems = append(shopItems, item)
		}
	}

	// TODO: Complete inventory display logic
	msg.Text += fmt.Sprintf("\n💰 Общая стоимость инвентаря: %d фишек", totalValue)
	msg.Text += "\n\n💡 Для продажи предмета используйте: /sell <хэш>"
	msg.Text += "\n💡 Для надевания плашки: /wear <хэш>"
	msg.Text += "\n💡 Для снятия плашки: /unwear"
}

// handleWear обрабатывает команду /wear
func (b *Bot) handleWear(msg *tgbotapi.MessageConfig, userName string, args string) {
	if args == "" {
		msg.Text = "🚫 Укажите хэш предмета! Пример: /wear abc123"
		return
	}

	err := storage.WearItem(userName, args)
	if err != nil {
		msg.Text = fmt.Sprintf("❌ Ошибка: %v", err)
		return
	}

	msg.Text = "✅ Плашка успешно надета!"
}

// handleUnwear обрабатывает команду /unwear
func (b *Bot) handleUnwear(msg *tgbotapi.MessageConfig, userName string) {
	err := storage.UnwearItem(userName)
	if err != nil {
		msg.Text = fmt.Sprintf("❌ Ошибка: %v", err)
		return
	}

	msg.Text = "✅ Плашка успешно снята!"
}

// handleSell обрабатывает команду /sell
func (b *Bot) handleSell(msg *tgbotapi.MessageConfig, userName string, args string) {
	// TODO: Implement sell logic
	msg.Text = "💰 Функция продажи пока в разработке"
}

// handleGive обрабатывает команду /give
func (b *Bot) handleGive(msg *tgbotapi.MessageConfig, userName string, args string, chatID int64) {
	// TODO: Implement give logic
	msg.Text = "🎁 Функция дарения пока в разработке"
}

// handlePayFine обрабатывает команду /payfine
func (b *Bot) handlePayFine(msg *tgbotapi.MessageConfig, userName string) {
	// TODO: Implement pay fine logic
	msg.Text = "💸 Функция оплаты штрафов пока в разработке"
}

// handleShameBoard обрабатывает команду /shameboard
func (b *Bot) handleShameBoard(msg *tgbotapi.MessageConfig) {
	// TODO: Implement shame board logic
	msg.Text = "📜 Доска позора пока в разработке"
}

// handleRob обрабатывает команду /rob
func (b *Bot) handleRob(msg *tgbotapi.MessageConfig, userName string, args string) {
	// TODO: Implement rob logic
	msg.Text = "🔫 Функция грабежа пока в разработке"
}

// handlePlateRob обрабатывает команду /platerob
func (b *Bot) handlePlateRob(msg *tgbotapi.MessageConfig, userName string, args string) {
	// TODO: Implement plate rob logic
	msg.Text = "🎯 Функция кражи плашек пока в разработке"
}

// handleLoadPrizes обрабатывает команду /loadprizes
func (b *Bot) handleLoadPrizes(msg *tgbotapi.MessageConfig, userName string) {
	if userName != "hunnidstooblue" && userName != "iamnothiding" {
		msg.Text = "🚫 Только администраторы могут загружать призы!"
		return
	}

	if err := utils.LoadPrizesFromFileToRedis(); err != nil {
		msg.Text = fmt.Sprintf("❌ Ошибка загрузки призов: %v", err)
	} else {
		msg.Text = "✅ Призы успешно загружены из prizes.json в Redis!"
	}
}

// handleRemoveFromRedis обрабатывает команду /removefromredis
func (b *Bot) handleRemoveFromRedis(msg *tgbotapi.MessageConfig, userName string, args string) {
	if userName != "hunnidstooblue" && userName != "iamnothiding" {
		msg.Text = "🚫 Только администраторы могут удалять призы!"
		return
	}

	if args != "confirm" {
		msg.Text = "⚠️ ВНИМАНИЕ!\n\n" +
			"Эта команда удалит ВСЕ ПРИЗЫ из Redis!\n" +
			"Призы будут потеряны без возможности восстановления!\n\n" +
			"Для подтверждения введите:\n" +
			"`/removefromredis confirm`"
		return
	}

	if err := storage.RemoveAllPrizesFromRedis(); err != nil {
		msg.Text = fmt.Sprintf("❌ Ошибка удаления призов: %v", err)
	} else {
		msg.Text = "✅ Все призы удалены из Redis!"
	}
}

// handlePromote обрабатывает команду /promote
func (b *Bot) handlePromote(msg *tgbotapi.MessageConfig, userName string, args string, chatID int64) {
	if userName != "hunnidstooblue" && userName != "iamnothiding" {
		msg.Text = "🚫 Только администраторы могут повышать пользователей!"
		return
	}

	if args == "" {
		msg.Text = "🚫 Укажите ID пользователя для повышения до администратора! Пример: /promote 123456789"
		return
	}

	userID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
	if err != nil {
		msg.Text = "🚫 Неверный формат ID пользователя! Используйте числовой ID."
		return
	}

	utils.PromoteUserToAdmin(b.API, chatID, userID)
	msg.Text = "✅ Попытка повышения пользователя до администратора выполнена."
}

// getUserBalance получает баланс пользователя (вспомогательная функция)
func getUserBalance(username string) int {
	balance, err := storage.GetBalance(username)
	if err != nil {
		log.Printf("Error getting balance for %s: %v", username, err)
		return 0
	}
	return balance
}
