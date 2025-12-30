package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"math/big"
	crand "math/rand"
	"strings"

	"tg-random-bot/gamble"
	"tg-random-bot/internal/config"
)

// ShuffleParticipants перемешивает слайс участников с использованием crypto/rand
func ShuffleParticipants(participants []string) {
	for i := len(participants) - 1; i > 0; i-- {
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(randomIndex.Int64())
		participants[i], participants[j] = participants[j], participants[i]
	}
}

// HashParticipant генерирует SHA-256 хэш участника
func HashParticipant(name string) string {
	hash := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", hash)
}

// InitParticipantHashes удалена - теперь в config.InitParticipants

// FormatParticipantName форматирует имя участника
func FormatParticipantName(name string) string {
	return name
}

// FormatParticipantNameWithUsername форматирует имя участника с @username
func FormatParticipantNameWithUsername(name string) string {
	username := config.ParticipantIDs[name]
	baseName := name

	if username != "" {
		baseName = fmt.Sprintf("%s (@%s)", name, username)
	}

	// TODO: Add worn item logic when storage package is properly set up
	// For now, just return the base name

	return baseName
}

// FormatParticipantNameWithItem форматирует имя участника только с плашкой (без username)
func FormatParticipantNameWithItem(name string) string {
	username := config.ParticipantIDs[name]
	baseName := name

	// TODO: Add worn item logic when storage package is properly set up
	// For now, just return the base name

	return baseName
}

// GetChipsWord возвращает правильное склонение слова "фишка"
func GetChipsWord(count int) string {
	lastDigit := count % 10
	lastTwoDigits := count % 100

	// Исключения для чисел 11-14
	if lastTwoDigits >= 11 && lastTwoDigits <= 14 {
		return "фишек"
	}

	// Основные правила
	switch lastDigit {
	case 1:
		return "фишка"
	case 2, 3, 4:
		return "фишки"
	default:
		return "фишек"
	}
}

// GetCoinResultText возвращает текстовое описание результата броска монеты
func GetCoinResultText(result gamble.CoinResult) string {
	switch result {
	case gamble.Heads:
		return "выпал орел (1)"
	case gamble.Tails:
		return "выпала решка (2)"
	case gamble.Edge:
		return "выпало ребро (3)"
	default:
		return fmt.Sprintf("выпала %s", result)
	}
}

// GetCoinSideName преобразует цифру монеты в название
func GetCoinSideName(side string) string {
	switch side {
	case "1":
		return "1 (орел)"
	case "2":
		return "2 (решка)"
	case "3":
		return "3 (ребро)"
	default:
		return side
	}
}

// GetParticipantNameByUsername получает имя участника по username
func GetParticipantNameByUsername(username string) string {
	for name, uname := range config.ParticipantIDs {
		if uname == username {
			return name
		}
	}
	return username // Если не найдено, возвращаем username
}

// GetRandomGiveplateQuote возвращает случайную цитату для /giveplate
func GetRandomGiveplateQuote() string {
	randomIndex := crand.Intn(len(config.GiveplateQuotes))
	return config.GiveplateQuotes[randomIndex]
}

// GetRandomDebtQuote возвращает случайную цитату про долг
func GetRandomDebtQuote() string {
	if len(config.DebtQuotes) == 0 {
		return "⚖️ Оплати свои долги перед законом!"
	}
	randomIndex := crand.Intn(len(config.DebtQuotes))
	return config.DebtQuotes[randomIndex]
}

// GetRandomDebtRobQuote возвращает случайную агрессивную цитату для должника при грабеже
func GetRandomDebtRobQuote() string {
	if len(config.DebtRobQuotes) == 0 {
		return "🚨 ПОЗОР! Должник взялся за грабёж вместо оплаты долгов!"
	}
	randomIndex := crand.Intn(len(config.DebtRobQuotes))
	return config.DebtRobQuotes[randomIndex]
}

// GetRandomShameQuote возвращает случайную цитату позора для доски позора
func GetRandomShameQuote() string {
	if len(config.ShameBoardQuotes) == 0 {
		return "🚨 ПОЗОР! Должник в списке неплательщиков!"
	}
	randomIndex := crand.Intn(len(config.ShameBoardQuotes))
	return config.ShameBoardQuotes[randomIndex]
}

// CheckLargeDebt проверяет большой долг (>10000)
func CheckLargeDebt(userName string) (hasLargeDebt bool, debtAmount int) {
	// TODO: This needs to be moved to a proper fines management system
	// For now, returning placeholder values
	return false, 0
}

// AddDebtNotificationToMessage добавляет уведомление о долге к сообщению
func AddDebtNotificationToMessage(userName string, messageText string) string {
	if hasLargeDebt, debtAmount := CheckLargeDebt(userName); hasLargeDebt {
		return fmt.Sprintf("⚠️ ДОЛГ ПО ШТРАФУ: %d %s\n💸 Выплатить: /payfine\n\n%s\n\n%s",
			debtAmount, GetChipsWord(debtAmount), GetRandomDebtQuote(), messageText)
	}
	return messageText
}

// GetPrizeCostByName получает стоимость приза по имени
func GetPrizeCostByName(prizeName string) (int, error) {
	for _, prize := range config.Prizes {
		if prize.Name == prizeName {
			return prize.Cost, nil
		}
	}
	return 0, fmt.Errorf("prize not found")
}

// LoadPrizesFromFileToRedis загружает призы из файла в Redis
func LoadPrizesFromFileToRedis() error {
	// TODO: Implement file loading
	return fmt.Errorf("not implemented yet")
}

// PromoteUserToAdmin повышает пользователя до администратора
func PromoteUserToAdmin(bot interface{}, chatID int64, userID int64) {
	// TODO: Implement admin promotion
}
