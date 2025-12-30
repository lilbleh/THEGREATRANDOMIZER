package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	crand "math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/redis/go-redis/v9"
)

// Глобальный список участников (инициализируется при запуске)
var participants []string

// Глобальные переменные для плашек
var prizes []Prize
var currentPrize Prize

// Структура для хранения ставки
type Bet struct {
	Username        string
	ParticipantName string // Имя участника
	ParticipantHash string // SHA-256 хэш участника
	Amount          int
}

// Структура для приза
type Prize struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Emoji       string `json:"emoji,omitempty"`
	Rarity      string `json:"rarity"`
	Cost        int    `json:"cost,omitempty"`
}

// Структура для конфига призов
type PrizeConfig struct {
	Prizes []Prize `json:"prizes"`
}

// Структура для элемента инвентаря
type InventoryItem struct {
	PrizeName string `json:"prizeName"`
	Rarity    string `json:"rarity"`
	Cost      int    `json:"cost"`
	Count     int    `json:"count"`
	Hash      string `json:"hash"` // Уникальный хэш предмета для продажи
}

// Rarity представляет редкость предмета
type Rarity string

const (
	CommonRarity    Rarity = "common"
	RareRarity      Rarity = "rare"
	LegendaryRarity Rarity = "legendary"
)

// GenerateRandomRarity генерирует случайную редкость на основе вероятностей:
// - Common: 0-79 (80% шанс)
// - Rare: 80-94 (15% шанс)
// - Legendary: 95-100 (6% шанс)
func GenerateRandomRarity() Rarity {
	// Генерируем криптографически безопасное случайное число от 0 до 100
	max := big.NewInt(101) // 0-100 включительно
	randomBig, err := rand.Int(rand.Reader, max)
	if err != nil {
		// В случае ошибки возвращаем common как fallback
		return CommonRarity
	}

	randomNum := int(randomBig.Int64())

	// Определяем редкость по диапазонам
	switch {
	case randomNum <= 79:
		return CommonRarity
	case randomNum <= 94:
		return RareRarity
	default:
		return LegendaryRarity
	}
}

// Переменные для управления игрой
var gameMessageID int
var gameChatID int64
var isGameActive bool
var gameInProgress bool  // Флаг, что идет процесс игры (чтобы предотвратить запуск нескольких игр)
var gameCancel chan bool // Канал для отмены активной игры
var totalRounds int
var currentRound int
var bettingPhase string

// Переменные для управления ставками
var initialBets = make(map[string]Bet)  // Ставки на начальном этапе (ключ: username игрока)
var finalBets = make(map[string]Bet)    // Ставки на финальном этапе (ключ: username игрока)
var bettingParticipants []string        // Участники для ставок (сортированные алфавитно)
var initialBettingParticipants []string // Сохраняем первоначальный список для ставок
var finalBettingNumbers []int           // Номера для финальных ставок

// Map для хранения username/ID участников (ключ: имя, значение: user ID)
// ТЕСТОВЫЙ СПИСОК ИЗ 5 УЧАСТНИКОВ
var participantIDs = map[string]string{
	"Арсений Квятковский": "Arsenkwait",
	"Василий Гончаров":    "BroisHelmut",
	"Виктория Григорьева": "sweerty_yv",
	"Владислав Рыбаков":   "mbr3unk",
	"Глеб Сушкевич":       "glbmsk",
	"Дарья Шилина":        "quasarqs0",
	"Екатерина Гнедова":   "Katharina_gn",
	"Игнат Пикта":         "LilakGnatius",
	"Максим Хваль":        "Whereisthesenses",
	"Мария Князькова":     "tomazzeto",
	"Назар Закревский":    "Zakrevski_05",
	"Настя Павлюченко":    "kuvillin",
	"Никита Янович":       "nktstrltz",
	"Ольга Легостаева":    "legostaevaa",
	"Ольга Васильева":     "olgavas8",
	"Рома Болдырев":       "woistmeinemutter",
	"Софья Цыбукова":      "Stelul003",
	"Вероника Войтех":     "veronikavoiteh",
	"Юля Луцевич":         "iuliia_lutsevich",
	"Глеб Гусев":          "hunnidstooblue",
	"Никита Шакалов":      "iamnothiding",
	"Алексей Баранов":     "barrrraaa",
}

// Map для хранения хэшей участников (ключ: имя участника, значение: SHA-256 хэш)
var participantHashes = make(map[string]string)
var eliminatedParticipants []string // Выбывшие участники

// Map для хранения балансов игроков (ключ: username, значение: баланс)
var playerBalances = make(map[string]int)

// Redis клиент для персистентного хранения балансов
var redisClient *redis.Client

// Функция для перемешивания слайса с использованием crypto/rand
func shuffleParticipants() {
	for i := len(participants) - 1; i > 0; i-- {
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		j := int(randomIndex.Int64())
		participants[i], participants[j] = participants[j], participants[i]
	}
}

// Функция для генерации SHA-256 хэша участника
func hashParticipant(name string) string {
	hash := sha256.Sum256([]byte(name))
	return fmt.Sprintf("%x", hash)
}

// Функция для генерации хэша предмета инвентаря
func generateItemHash(username, prizeName string) string {
	data := fmt.Sprintf("%s:%s:%d", username, prizeName, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:6] // Берем первые 6 символов для короткого хэша
}

// Функция для инициализации хэшей всех участников
func initParticipantHashes() {
	for name, username := range participantIDs {
		participantHashes[name] = hashParticipant(username)
	}
}

// Функция для форматирования имени участника
func formatParticipantName(name string) string {
	return name
}

// Функция для форматирования имени участника с @username
func formatParticipantNameWithUsername(name string) string {
	username := participantIDs[name]
	baseName := name

	if username != "" {
		baseName = fmt.Sprintf("%s (@%s)", name, username)
	}

	// Проверяем, есть ли надетая плашка
	if username != "" {
		wornData, err := getWornItem(username)
		if err == nil && wornData != nil {
			// Добавляем плашку к имени
			itemName := wornData["name"]
			baseName = fmt.Sprintf("%s %s", baseName, itemName)
		}
	}

	return baseName
}

// Функция для форматирования имени участника только с плашкой (без username)
func formatParticipantNameWithItem(name string) string {
	username := participantIDs[name]
	baseName := name

	// Проверяем, есть ли надетая плашка
	if username != "" {
		wornData, err := getWornItem(username)
		if err == nil && wornData != nil {
			// Добавляем плашку к имени
			itemName := wornData["name"]
			baseName = fmt.Sprintf("%s %s", baseName, itemName)
		}
	}

	return baseName
}

// Функция для правильного склонения слова "фишка"
func getChipsWord(count int) string {
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

// Функция для надевания плашки пользователем
func wearItem(username, itemHash string) error {
	log.Printf("wearItem: Пользователь %s надевает плашку с хэшем %s", username, itemHash)

	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Проверяем, есть ли такой предмет у пользователя
	itemKey := fmt.Sprintf("inventory:%s:%s", username, itemHash)
	itemData, err := redisClient.Get(ctx, itemKey).Result()
	if err != nil {
		log.Printf("wearItem: Предмет с хэшем %s не найден у пользователя %s", itemHash, username)
		return fmt.Errorf("предмет не найден в инвентаре")
	}

	// Парсим данные предмета
	var item InventoryItem
	err = json.Unmarshal([]byte(itemData), &item)
	if err != nil {
		log.Printf("wearItem: Ошибка парсинга предмета %s: %v", itemHash, err)
		return fmt.Errorf("ошибка обработки предмета")
	}

	// Сохраняем информацию о надетой плашке
	profileKey := fmt.Sprintf("profile:%s:worn_item", username)
	wornData := map[string]string{
		"hash":      itemHash,
		"name":      item.PrizeName,
		"rarity":    item.Rarity,
		"timestamp": fmt.Sprintf("%d", time.Now().Unix()),
	}

	data, err := json.Marshal(wornData)
	if err != nil {
		log.Printf("wearItem: Ошибка маршалинга данных плашки: %v", err)
		return fmt.Errorf("ошибка сохранения")
	}

	err = redisClient.Set(ctx, profileKey, data, 0).Err()
	if err != nil {
		log.Printf("wearItem: Ошибка сохранения надетой плашки: %v", err)
		return fmt.Errorf("ошибка сохранения профиля")
	}

	log.Printf("wearItem: Плашка %s успешно надета пользователем %s", item.PrizeName, username)
	return nil
}

// Функция для снятия плашки пользователем
func unwearItem(username string) error {
	log.Printf("unwearItem: Пользователь %s снимает плашку", username)

	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	profileKey := fmt.Sprintf("profile:%s:worn_item", username)

	// Проверяем, есть ли надетая плашка
	exists, err := redisClient.Exists(ctx, profileKey).Result()
	if err != nil {
		log.Printf("unwearItem: Ошибка проверки профиля: %v", err)
		return fmt.Errorf("ошибка проверки профиля")
	}

	if exists == 0 {
		log.Printf("unwearItem: У пользователя %s нет надетой плашки", username)
		return fmt.Errorf("нет надетой плашки")
	}

	// Удаляем информацию о надетой плашке
	err = redisClient.Del(ctx, profileKey).Err()
	if err != nil {
		log.Printf("unwearItem: Ошибка удаления надетой плашки: %v", err)
		return fmt.Errorf("ошибка снятия плашки")
	}

	log.Printf("unwearItem: Плашка успешно снята у пользователя %s", username)
	return nil
}

// Функция для получения информации о надетой плашке
func getWornItem(username string) (map[string]string, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	profileKey := fmt.Sprintf("profile:%s:worn_item", username)

	data, err := redisClient.Get(ctx, profileKey).Result()
	if err != nil {
		return nil, err // Возвращаем ошибку, если плашка не надета
	}

	var wornData map[string]string
	err = json.Unmarshal([]byte(data), &wornData)
	if err != nil {
		log.Printf("getWornItem: Ошибка парсинга данных плашки для %s: %v", username, err)
		return nil, err
	}

	return wornData, nil
}

// Функция для получения имени участника по username
func getParticipantNameByUsername(username string) string {
	for name, uname := range participantIDs {
		if uname == username {
			return name
		}
	}
	return username // Если не найдено, возвращаем username
}

// Функция для выплаты выигрышей по ставкам и формирования текста результатов
func payoutWinnings(bot *tgbotapi.BotAPI, winner string, loser string) string {
	log.Printf("💰 payoutWinnings: === НАЧАЛО ВЫПЛАТЫ ВЫИГРЫШЕЙ ===")
	log.Printf("payoutWinnings: Функция ВЫЗВАНА! Победитель: %s, Проигравший: %s", winner, loser)
	log.Printf("payoutWinnings: isGameActive=%t", isGameActive)
	log.Printf("payoutWinnings: Количество ставок - initial: %d, final: %d", len(initialBets), len(finalBets))

	// DEBUG: Показать все ставки
	log.Printf("payoutWinnings: DEBUG: Initial ставки:")
	for username, bet := range initialBets {
		log.Printf("payoutWinnings:   %s -> %s (хэш: %s)", username, bet.ParticipantName, bet.ParticipantHash[:8]+"...")
	}
	log.Printf("payoutWinnings: DEBUG: Final ставки:")
	for username, bet := range finalBets {
		log.Printf("payoutWinnings:   %s -> %s (хэш: %s)", username, bet.ParticipantName, bet.ParticipantHash[:8]+"...")
	}

	// DEBUG: Показать все хэши участников
	log.Printf("payoutWinnings: DEBUG: Все хэши участников:")
	for name, hash := range participantHashes {
		log.Printf("payoutWinnings:   %s -> %s (username: %s)", name, hash[:8]+"...", participantIDs[name])
	}

	// Всегда формируем сообщение с результатами ставок
	resultsText := "🏆 РЕЗУЛЬТАТЫ СТАВОК:\n\n"

	// Если нет ставок, все равно показываем сообщение с результатами
	if len(initialBets) == 0 && len(finalBets) == 0 {
		log.Printf("payoutWinnings: ❌ Нет ставок для обработки")
		resultsText += "❌ В этом раунде ставок не было сделано.\n"
		log.Printf("payoutWinnings: Возвращаем сообщение без ставок: '%s'", resultsText)
		return resultsText
	}

	log.Printf("payoutWinnings: ✅ Есть ставки для обработки")

	// Получаем хэш победителя
	winnerHash := participantHashes[winner]
	if winnerHash == "" {
		log.Printf("payoutWinnings: Ошибка: хэш победителя %s не найден", winner)
		log.Printf("payoutWinnings: DEBUG: доступные хэши участников:")
		for name, hash := range participantHashes {
			log.Printf("payoutWinnings:   %s -> %s (первые 5: %s)", name, hash, hash[:5])
		}
		return resultsText
	}

	log.Printf("payoutWinnings: Победитель %s имеет хэш %s (первые 5: %s)", winner, winnerHash, winnerHash[:5])

	// Выплачиваем выигрыши по начальным ставкам (коэффициент x30)
	if len(initialBets) > 0 {
		log.Printf("payoutWinnings: 🎯 Обрабатываем начальные ставки (x30), количество: %d", len(initialBets))
		resultsText += "💰 *Начальные ставки (x30):*\n"
		log.Printf("payoutWinnings: Начальные ставки найдены, добавляем в resultsText")
		for username, bet := range initialBets {
			log.Printf("payoutWinnings: Проверяем начальную ставку %s: ставка на %s (хэш %s), сумма %d", username, bet.ParticipantName, bet.ParticipantHash[:8]+"...", bet.Amount)
			log.Printf("payoutWinnings: Победитель: %s (хэш %s)", winner, winnerHash[:8]+"...")

			if bet.ParticipantName == winner {
				// Ставка выиграла! Выплачиваем 30 фишек
				winnings := bet.Amount * 30
				log.Printf("payoutWinnings: Начальная ставка выиграла! %s ставил на %s, выигрыш %d фишек", username, bet.ParticipantName, winnings)
				oldBalance := playerBalances[username]
				changeBalance(username, winnings)
				log.Printf("payoutWinnings: ✅ ВЫИГРЫШ! Баланс %s изменен с %d на %d (выигрыш %d фишек)", username, oldBalance, playerBalances[username], winnings)

				resultsText += fmt.Sprintf("✅ @%s: +%d фишек (ставка %d на %s)\n",
					username, winnings, bet.Amount, formatParticipantNameWithUsername(bet.ParticipantName))

				log.Printf("payoutWinnings: Выплачен выигрыш по начальной ставке: %s выиграл %d фишек (ставка %d)", username, winnings, bet.Amount)
			} else {
				log.Printf("payoutWinnings: ❌ ПРОИГРЫШ: ставка %s на %s (не победитель)", username, bet.ParticipantName)
				resultsText += fmt.Sprintf("❌ @%s: проигрыш (ставка %d на %s)\n",
					username, bet.Amount, formatParticipantNameWithUsername(bet.ParticipantName))

				log.Printf("payoutWinnings: Проиграна начальная ставка: %s (ставка %d)", username, bet.Amount)
			}
		}
		resultsText += "\n"
	} else {
		log.Printf("payoutWinnings: Начальных ставок нет")
	}

	// Выплачиваем выигрыши по финальным ставкам (коэффициент x2)
	if len(finalBets) > 0 {
		log.Printf("payoutWinnings: 🎯 Обрабатываем финальные ставки (x2), количество: %d", len(finalBets))
		resultsText += "💰 *Финальные ставки (x2):*\n"
		log.Printf("payoutWinnings: Финальные ставки найдены, добавляем в resultsText")
		for username, bet := range finalBets {
			log.Printf("payoutWinnings: Проверяем финальную ставку %s: ставка на %s (хэш %s), сумма %d", username, bet.ParticipantName, bet.ParticipantHash[:8]+"...", bet.Amount)
			log.Printf("payoutWinnings: Победитель: %s (хэш %s)", winner, winnerHash[:8]+"...")

			if bet.ParticipantName == winner {
				// Ставка выиграла! Выплачиваем 2 фишки
				winnings := bet.Amount * 2
				log.Printf("payoutWinnings: Финальная ставка выиграла! %s ставил на %s, выигрыш %d фишек", username, bet.ParticipantName, winnings)
				oldBalance := playerBalances[username]
				changeBalance(username, winnings)
				log.Printf("payoutWinnings: ✅ ВЫИГРЫШ! Баланс %s изменен с %d на %d (выигрыш %d фишек)", username, oldBalance, playerBalances[username], winnings)

				resultsText += fmt.Sprintf("✅ @%s: +%d фишек (ставка %d на %s)\n",
					username, winnings, bet.Amount, formatParticipantNameWithUsername(bet.ParticipantName))

				log.Printf("payoutWinnings: Выплачен выигрыш по финальной ставке: %s выиграл %d фишек (ставка %d)", username, winnings, bet.Amount)
			} else {
				log.Printf("payoutWinnings: ❌ ПРОИГРЫШ: финальная ставка %s на %s (не победитель)", username, bet.ParticipantName)
				resultsText += fmt.Sprintf("❌ @%s: проигрыш (ставка %d на %s)\n",
					username, bet.Amount, formatParticipantNameWithUsername(bet.ParticipantName))

				log.Printf("payoutWinnings: Проиграна финальная ставка: %s (ставка %d)", username, bet.Amount)
			}
		}
	} else {
		log.Printf("payoutWinnings: Финальных ставок нет")
	}

	// Очищаем ставки после выплаты
	log.Printf("payoutWinnings: Очищаем ставки после выплаты")
	initialBets = make(map[string]Bet)
	finalBets = make(map[string]Bet)
	log.Printf("payoutWinnings: Ставки очищены в памяти")

	// Очищаем Redis
	if redisClient != nil {
		ctx := context.Background()
		err := redisClient.Del(ctx, "game:initialBets", "game:finalBets").Err()
		if err != nil {
			log.Printf("payoutWinnings: ❌ Ошибка очистки ставок в Redis: %v", err)
		} else {
			log.Printf("payoutWinnings: ✅ Ставки очищены в Redis")
		}
	} else {
		log.Printf("payoutWinnings: Redis клиент недоступен, ставки очищены только в памяти")
	}

	// Выдаем приз победителю - используем плашку, выбранную в начале игры
	log.Printf("payoutWinnings: Выдаем приз победителю %s", winner)
	log.Printf("payoutWinnings: Используем плашку из игры: %s (%s)", currentPrize.Name, currentPrize.Rarity)

	if currentPrize.Name == "" {
		log.Printf("payoutWinnings: ОШИБКА: currentPrize пустой!")
		resultsText += fmt.Sprintf("\n\n🎁 Ошибка: плашка не была выбрана!")
	} else {
		// Находим username победителя
		winnerUsername := participantIDs[winner]
		log.Printf("payoutWinnings: participantIDs содержит %d записей", len(participantIDs))
		for name, uname := range participantIDs {
			log.Printf("payoutWinnings: participantIDs[%s] = %s", name, uname)
		}
		log.Printf("payoutWinnings: Ищем username для winner='%s'", winner)
		winnerUsername = participantIDs[winner]
		log.Printf("payoutWinnings: Победитель %s, username: %s", winner, winnerUsername)

		if winnerUsername == "" {
			log.Printf("payoutWinnings: ОШИБКА: username победителя пустой!")
			resultsText += fmt.Sprintf("\n\n🎁 Ошибка определения победителя!")
		} else {
			err := givePrizeToWinner(winnerUsername, currentPrize)
			if err != nil {
				log.Printf("payoutWinnings: Ошибка выдачи приза: %v", err)
				resultsText += fmt.Sprintf("\n\n🎁 Ошибка выдачи приза!")
			} else {
				log.Printf("payoutWinnings: Приз %s успешно выдан победителю %s", currentPrize.Name, winnerUsername)
				resultsText += fmt.Sprintf("\n\n🎁 Победитель получает плашку: **%s**!", currentPrize.Name)
			}
		}
	}

	log.Printf("payoutWinnings: === ВЫПЛАТА ВЫИГРЫШЕЙ ЗАВЕРШЕНА ===")
	previewLen := 100
	if len(resultsText) < previewLen {
		previewLen = len(resultsText)
	}
	log.Printf("payoutWinnings: Финальный resultsText длина = %d, первые %d символов: '%s...'", len(resultsText), previewLen, resultsText[:previewLen])
	log.Printf("payoutWinnings: Возвращаемый текст результатов: '%s'", resultsText)
	return resultsText
}

// Функция для выполнения раунда игры
func performGameRound(bot *tgbotapi.BotAPI, roundNumber int) string {
	log.Printf("performGameRound: Вызвана с roundNumber=%d, len(participants)=%d, totalRounds=%d, isGameActive=%t", roundNumber, len(participants), totalRounds, isGameActive)
	log.Printf("performGameRound: Участники: %v", participants)
	if len(participants) == 0 {
		isGameActive = false
		return "Игра уже окончена!"
	} else if len(participants) == 1 {
		// Финальный раунд: последний участник выигрывает
		winner := participants[0]

		// Показываем полную информацию о выигранной плашке
		rarityText := ""
		switch currentPrize.Rarity {
		case "common":
			rarityText = "ОБЫЧНАЯ"
		case "rare":
			rarityText = "РЕДКАЯ"
		case "legendary":
			rarityText = "ЛЕГЕНДАРНАЯ"
		default:
			rarityText = "НЕИЗВЕСТНАЯ"
		}

		finalText := fmt.Sprintf("🏆🏆🏆 %s, ПОЗДРАВЛЯЕМ!! Вы выиграли плашку \"%s\" (%s)!\n\n🐩 Игра окончена!", formatParticipantNameWithUsername(winner), currentPrize.Name, rarityText)
		participants = []string{} // Полностью очищаем список
		isGameActive = false
		return finalText
	} else if len(participants) == 2 {
		log.Printf("🎯 performGameRound: === НАЧАЛО ФИНАЛЬНОЙ ИГРЫ ===")
		log.Printf("performGameRound: Осталось 2 участника, начинаем финальную последовательность")
		log.Printf("performGameRound: Участники финала: %v", participants)

		// ФАЗА 1: Финальный раунд (30 секунд)
		log.Printf("performGameRound: ФАЗА 1 - Показываем финальный раунд на 30 секунд")
		finalRoundText := "🎯 ФИНАЛЬНЫЙ РАУНД!\n\n"
		finalRoundText += "🏆 ФИНАЛИСТЫ:\n"
		for i, participant := range participants {
			finalRoundText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithItem(participant))
		}
		finalRoundText += "\n⏰ Через 5 секунд начнутся финальные ставки!"

		// Отправляем новое сообщение вместо редактирования старого
		roundMsg := tgbotapi.NewMessage(gameChatID, finalRoundText)
		if _, err := bot.Send(roundMsg); err != nil {
			log.Printf("performGameRound: Ошибка отправки сообщения финального раунда: %v", err)
		}

		log.Printf("performGameRound: Ждем 5 секунд финального раунда...")
		select {
		case <-time.After(5 * time.Second):
			log.Printf("performGameRound: Финальный раунд завершен")
		case <-gameCancel:
			log.Printf("performGameRound: Финальный раунд отменен")
			return "Игра была отменена"
		}

		// ФАЗА 2: Финальные ставки (30 секунд)
		log.Printf("performGameRound: ФАЗА 2 - Запускаем финальные ставки на 30 секунд")
		bettingPhase = "final"

		// Для финальных ставок используем простые номера 1 и 2
		bettingParticipants = make([]string, len(participants))
		copy(bettingParticipants, participants)
		finalBettingNumbers = []int{1, 2}

		finalBetText := "🎯 ФИНАЛЬНЫЕ СТАВКИ!\n\n"
		finalBetText += "🏆 ОСТАЛИСЬ ДВА УЧАСТНИКА:\n"
		for i, participant := range bettingParticipants {
			finalBetText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithItem(participant))
		}
		finalBetText += "\n💰 ФИНАЛЬНЫЕ СТАВКИ ОТКРЫТЫ!\n"
		finalBetText += "🎯 Ставьте на победителя: /bet N СУММА\n"
		finalBetText += "💎 Коэффициент: x2\n"
		finalBetText += "⏰ Время на ставки: 30 сек\n"

		// Отправляем новое сообщение вместо редактирования старого
		betMsg := tgbotapi.NewMessage(gameChatID, finalBetText)
		if _, err := bot.Send(betMsg); err != nil {
			log.Printf("performGameRound: Ошибка отправки сообщения финальных ставок: %v", err)
		}

		log.Printf("performGameRound: Ждем 30 секунд финальных ставок...")
		startTime := time.Now()
		time.Sleep(30 * time.Second)
		elapsed := time.Since(startTime)
		log.Printf("performGameRound: Финальные ставки завершены, прошло времени: %.2f секунд", elapsed.Seconds())

		bettingPhase = "closed"
		log.Printf("performGameRound: Финальные ставки завершены, переходим к определению победителя")

		// Проводим финальную игру
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(2))
		winnerIndex := int(randomIndex.Int64())
		winner := participants[winnerIndex]
		loser := participants[1-winnerIndex]

		log.Printf("performGameRound: 🎲 Рандомизация завершена:")
		log.Printf("performGameRound:   randomIndex = %d", randomIndex)
		log.Printf("performGameRound:   winnerIndex = %d", winnerIndex)
		log.Printf("performGameRound:   winner = %s, loser = %s", winner, loser)
		log.Printf("performGameRound:   winner hash = %s", participantHashes[winner])

		winnerUsername := participantIDs[winner]
		loserUsername := participantIDs[loser]

		log.Printf("performGameRound: Username финалистов:")
		log.Printf("performGameRound:   winnerUsername: %s", winnerUsername)
		log.Printf("performGameRound:   loserUsername: %s", loserUsername)

		finalResultText := fmt.Sprintf("☹️ К сожалению! %s не получает плашку в финале!\n", formatParticipantNameWithUsername(loser))
		finalResultText += "ничего страшного, повезет в следующей игре 🍀!\n\n"

		// Показываем полную информацию о выигранной плашке
		rarityText := ""
		switch currentPrize.Rarity {
		case "common":
			rarityText = "ОБЫЧНАЯ"
		case "rare":
			rarityText = "РЕДКАЯ"
		case "legendary":
			rarityText = "ЛЕГЕНДАРНАЯ"
		default:
			rarityText = "НЕИЗВЕСТНАЯ"
		}

		finalResultText += fmt.Sprintf("🏆🏆🏆 %s, ПОЗДРАВЛЯЕМ!! Вы выиграли плашку \"%s\" (%s)!\n", formatParticipantNameWithUsername(winner), currentPrize.Name, rarityText)

		finalResultText += "\n\n🐩 Игра окончена!"

		log.Printf("performGameRound: Финальное сообщение сформировано")
		log.Printf("performGameRound: Очищаем список участников и завершаем игру")

		participants = []string{} // Полностью очищаем список
		isGameActive = false

		// Отправляем НОВОЕ сообщение с результатами игры (не редактируем старое)
		log.Printf("performGameRound: Отправляем новое сообщение с финальными результатами")
		gameResultMsg := tgbotapi.NewMessage(gameChatID, finalResultText)
		if _, err := bot.Send(gameResultMsg); err != nil {
			log.Printf("performGameRound: Ошибка отправки сообщения с результатами игры: %v", err)
		} else {
			log.Printf("performGameRound: Новое сообщение с результатами игры отправлено успешно")
		}

		log.Printf("performGameRound: Игра окончена, вызываем payoutWinnings")
		log.Printf("performGameRound: Финальные ставки для выплат: %d ставок", len(finalBets))

		// Выплачиваем выигрыши и получаем текст результатов ставок
		log.Printf("performGameRound: Начинаем выплаты. Победитель: %s, Проигравший: %s", winner, loser)
		betsResultsText := payoutWinnings(bot, winner, loser)
		log.Printf("performGameRound: betsResultsText длина = %d, пустой = %t", len(betsResultsText), betsResultsText == "")
		log.Printf("performGameRound: betsResultsText = '%s'", betsResultsText)

		// Отправляем сообщение с результатами ставок только если есть ставки
		if betsResultsText != "" {
			log.Printf("performGameRound: Отправляем сообщение с результатами ставок в чат %d", gameChatID)
			betsMsg := tgbotapi.NewMessage(gameChatID, betsResultsText)
			betsMsg.ParseMode = "Markdown"
			sentMsg, err := bot.Send(betsMsg)
			if err != nil {
				log.Printf("performGameRound: Ошибка отправки сообщения с результатами ставок: %v", err)
				log.Printf("performGameRound: Текст сообщения: %s", betsResultsText)
			} else {
				log.Printf("performGameRound: Сообщение с результатами ставок отправлено успешно, messageID: %d", sentMsg.MessageID)
			}
		} else {
			log.Printf("performGameRound: Нет ставок, сообщение с результатами не отправляется")
		}

		log.Printf("performGameRound: payoutWinnings завершен")
		log.Printf("performGameRound: === ФИНАЛЬНАЯ ИГРА ЗАВЕРШЕНА ===")

		return ""
	} else {
		// Обычный раунд: выбираем случайного участника для удаления
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(participants))))
		loserIndex := int(randomIndex.Int64())
		removedParticipant := participants[loserIndex]

		// Добавляем в список выбывших и удаляем из активных участников
		eliminatedParticipants = append(eliminatedParticipants, removedParticipant)
		participants = append(participants[:loserIndex], participants[loserIndex+1:]...)

		// Формируем полное обновляемое сообщение
		gameText := "🎮 ИГРА ИДЁТ!\n\n"

		// Показываем редкость будущей плашки
		rarityText := ""
		switch currentPrize.Rarity {
		case "common":
			rarityText = "ОБЫЧНАЯ"
		case "rare":
			rarityText = "РЕДКАЯ"
		case "legendary":
			rarityText = "ЛЕГЕНДАРНАЯ"
		}
		gameText += fmt.Sprintf("🎁 БУДЕТ РАЗЫГРАНА %s ПЛАШКА!\n\n", rarityText)

		// Текущие участники
		if len(participants) > 0 {
			gameText += "🏆 ТЕКУЩИЕ УЧАСТНИКИ:\n"
			for i, participant := range participants {
				gameText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithItem(participant))
			}
		}

		// Выбывшие участники
		if len(eliminatedParticipants) > 0 {
			gameText += "\n💀 ВЫБЫВШИЕ УЧАСТНИКИ:\n"
			for _, participant := range eliminatedParticipants {
				gameText += fmt.Sprintf("❌ %s\n", formatParticipantNameWithItem(participant))
			}
		}

		// Сообщение о выбывшем участнике
		gameText += fmt.Sprintf("\n☹️ В этом раунде выбывает: %s\n", formatParticipantName(removedParticipant))
		gameText += "@" + participantIDs[removedParticipant] + ", ничего страшного, повезет в следующей игре 😊🍀!\n"

		remaining := len(participants)
		if remaining > 1 {
			gameText += fmt.Sprintf("\nОсталось участников: %d", remaining)
		} else if remaining == 1 {
			gameText += "\n🏆 Остался последний участник!"
		}

		return gameText
	}
}

// Функция для управления сессией игры
func runGameSession(bot *tgbotapi.BotAPI) {
	log.Printf("runGameSession: Начало игры, totalRounds=%d, currentRound=%d, len(participants)=%d", totalRounds, currentRound, len(participants))

	// Цикл для всех раундов
	for isGameActive && currentRound <= totalRounds {
		// Проверяем, не была ли игра отменена
		select {
		case <-gameCancel:
			log.Printf("runGameSession: Игра отменена во время выполнения")
			return
		default:
			// Продолжаем игру
		}

		log.Printf("runGameSession: НАЧАЛО РАУНДА %d (%d-й по порядку), isGameActive=%t, len(participants)=%d", currentRound, currentRound+1, isGameActive, len(participants))

		// Выполняем раунд
		roundResult := performGameRound(bot, currentRound)
		log.Printf("runGameSession: Раунд %d выполнен, isGameActive=%t, roundResult содержит 'ПОДГОТОВКА': %t", currentRound, isGameActive, strings.Contains(roundResult, "ПОДГОТОВКА"))

		// Если игра закончилась, показываем финальный результат
		if !isGameActive {
			log.Printf("Игра закончилась после раунда %d", currentRound)
			// Для финальной игры результаты уже отправлены отдельными сообщениями
			if roundResult != "" {
				log.Printf("runGameSession: Отправляем финальное сообщение: %s", roundResult)
				editMsg := tgbotapi.NewEditMessageText(gameChatID, gameMessageID, roundResult)
				_, err := bot.Send(editMsg)
				if err != nil {
					log.Printf("runGameSession: Ошибка отправки финального сообщения: %v", err)
				} else {
					log.Printf("runGameSession: Финальное сообщение отправлено успешно")
				}
			}
			log.Printf("runGameSession: игра завершена")
			return
		}

		// Проверяем, последний ли это раунд
		if currentRound >= totalRounds {
			// Последний раунд - показываем результат и завершаем
			log.Printf("runGameSession: Последний раунд %d завершен", currentRound)
			if roundResult != "" {
				editMsg := tgbotapi.NewEditMessageText(gameChatID, gameMessageID, roundResult)
				if _, err := bot.Send(editMsg); err != nil {
					log.Printf("runGameSession: Ошибка отправки сообщения последнего раунда: %v", err)
				}
			}
			currentRound++
			break
		}

		// Есть следующий раунд - показываем результат + отсчёт до следующего раунда
		nextRoundText := fmt.Sprintf("%s\n\n🎮 РАУНД %d/%d\n⏰ До следующего раунда: 5 сек",
			roundResult, currentRound+1, totalRounds)

		log.Printf("Показываем результат раунда %d с отсчётом до раунда %d", currentRound, currentRound+1)
		editMsg := tgbotapi.NewEditMessageText(gameChatID, gameMessageID, nextRoundText)
		if _, err := bot.Send(editMsg); err != nil {
			log.Printf("Ошибка редактирования сообщения: %v", err)
			isGameActive = false
			break
		}

		// Ждём 5 секунд до следующего раунда с проверкой отмены
		select {
		case <-time.After(5 * time.Second):
			// Время вышло, продолжаем
		case <-gameCancel:
			log.Printf("runGameSession: Игра отменена во время паузы между раундами")
			return
		}

		currentRound++
		log.Printf("runGameSession: Переходим к раунду %d", currentRound)

		// Небольшая пауза между раундами
		if isGameActive && len(participants) > 1 {
			time.Sleep(500 * time.Millisecond)
		}
	}

	log.Printf("runGameSession: Цикл завершен, isGameActive=%t, currentRound=%d, totalRounds=%d", isGameActive, currentRound, totalRounds)

	// Сбрасываем состояние после завершения игры
	if !isGameActive {
		log.Printf("runGameSession: Игра завершена, сбрасываем состояние")
		bettingPhase = "closed"
		currentRound = 0
		initialBets = make(map[string]Bet)
		finalBets = make(map[string]Bet)
		finalBettingNumbers = []int{}
		currentPrize = Prize{}
		gameInProgress = false // Сбрасываем флаг процесса игры
	}
}

// Функция для инициализации Redis клиента
func initRedis() {
	// Получаем адрес Redis из переменной окружения или используем по умолчанию
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379" // По умолчанию localhost
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     redisAddr, // Redis сервер
		Password: "",        // Пароль (пустой по умолчанию)
		DB:       0,         // База данных (0 по умолчанию)
	})

	// Проверяем подключение
	ctx := context.Background()
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Printf("Ошибка подключения к Redis (%s): %v", redisAddr, err)
		log.Printf("Балансы будут храниться только в памяти!")
	} else {
		log.Printf("Redis подключен успешно (%s)", redisAddr)
	}
}

// Функция для сохранения баланса в Redis
func saveBalanceToRedis(username string, balance int) {
	if redisClient == nil {
		return
	}

	ctx := context.Background()
	key := fmt.Sprintf("balance:%s", username)
	err := redisClient.Set(ctx, key, balance, 0).Err()
	if err != nil {
		log.Printf("Ошибка сохранения баланса для %s: %v", username, err)
	}
}

// Функция для сохранения всех балансов в Redis
func saveBalancesToRedis() error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	for username, balance := range playerBalances {
		key := fmt.Sprintf("balance:%s", username)
		err := redisClient.Set(ctx, key, balance, 0).Err()
		if err != nil {
			log.Printf("Ошибка сохранения баланса для %s: %v", username, err)
			return fmt.Errorf("failed to save balance for %s: %v", username, err)
		}
	}

	log.Printf("saveBalancesToRedis: Сохранено %d балансов в Redis", len(playerBalances))
	return nil
}

// Функция для загрузки баланса из Redis
func loadBalanceFromRedis(username string) (int, bool) {
	if redisClient == nil {
		return 0, false
	}

	ctx := context.Background()
	key := fmt.Sprintf("balance:%s", username)
	val, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return 0, false
	}

	balance, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}

	return balance, true
}

// Функция для загрузки всех балансов из Redis
func loadAllBalancesFromRedis() {
	if redisClient == nil {
		return
	}

	ctx := context.Background()
	keys, err := redisClient.Keys(ctx, "balance:*").Result()
	if err != nil {
		log.Printf("Ошибка загрузки балансов из Redis: %v", err)
		return
	}

	for _, key := range keys {
		username := strings.TrimPrefix(key, "balance:")
		if balance, ok := loadBalanceFromRedis(username); ok {
			playerBalances[username] = balance
		}
	}

	log.Printf("Загружено %d балансов из Redis", len(playerBalances))
}

// Функция для сохранения количества туров в Redis
func saveTotalRoundsToRedis(rounds int) {
	if redisClient == nil {
		return
	}

	ctx := context.Background()
	err := redisClient.Set(ctx, "game:totalRounds", rounds, 0).Err()
	if err != nil {
		log.Printf("Ошибка сохранения количества туров: %v", err)
	}
}

// Функция для загрузки количества туров из Redis
func loadTotalRoundsFromRedis() (int, bool) {
	if redisClient == nil {
		return 0, false
	}

	ctx := context.Background()
	val, err := redisClient.Get(ctx, "game:totalRounds").Result()
	if err != nil {
		return 0, false
	}

	rounds, err := strconv.Atoi(val)
	if err != nil {
		return 0, false
	}

	return rounds, true
}

// Функция для сохранения ставок в Redis
func saveBetsToRedis(bets map[string]Bet, key string) {
	if redisClient == nil {
		return
	}

	ctx := context.Background()
	data, err := json.Marshal(bets)
	if err != nil {
		log.Printf("Ошибка сериализации ставок: %v", err)
		return
	}

	err = redisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		log.Printf("Ошибка сохранения ставок: %v", err)
	}
}

// Функция для загрузки ставок из Redis
func loadBetsFromRedis(key string) (map[string]Bet, error) {
	if redisClient == nil {
		return make(map[string]Bet), nil
	}

	ctx := context.Background()
	val, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return make(map[string]Bet), err
	}

	var bets map[string]Bet
	err = json.Unmarshal([]byte(val), &bets)
	if err != nil {
		return make(map[string]Bet), err
	}

	return bets, nil
}

// Функция для сохранения приза в Redis
func savePrizeToRedis(prize Prize) error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("prize:%s", prize.Name)

	data, err := json.Marshal(prize)
	if err != nil {
		return fmt.Errorf("failed to marshal prize: %v", err)
	}

	err = redisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to save prize to Redis: %v", err)
	}

	return nil
}

// Функция для загрузки приза из Redis
func loadPrizeFromRedis(name string) (Prize, error) {
	if redisClient == nil {
		return Prize{}, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("prize:%s", name)

	val, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return Prize{}, fmt.Errorf("failed to get prize from Redis: %v", err)
	}

	var prize Prize
	err = json.Unmarshal([]byte(val), &prize)
	if err != nil {
		return Prize{}, fmt.Errorf("failed to unmarshal prize: %v", err)
	}

	return prize, nil
}

// Функция для загрузки всех призов из Redis
func loadAllPrizesFromRedis() ([]Prize, error) {
	if redisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	keys, err := redisClient.Keys(ctx, "prize:*").Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get prize keys: %v", err)
	}

	var prizes []Prize
	for _, key := range keys {
		val, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			log.Printf("Warning: failed to get prize %s: %v", key, err)
			continue
		}

		var prize Prize
		err = json.Unmarshal([]byte(val), &prize)
		if err != nil {
			log.Printf("Warning: failed to unmarshal prize %s: %v", key, err)
			continue
		}

		prizes = append(prizes, prize)
	}

	return prizes, nil
}

// Функция для удаления всех призов из Redis
func removeAllPrizesFromRedis() error {
	if redisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	keys, err := redisClient.Keys(ctx, "prize:*").Result()
	if err != nil {
		return fmt.Errorf("failed to get prize keys: %v", err)
	}

	if len(keys) == 0 {
		return nil // Нет ключей для удаления
	}

	err = redisClient.Del(ctx, keys...).Err()
	if err != nil {
		return fmt.Errorf("failed to delete prizes from Redis: %v", err)
	}

	log.Printf("Удалено %d призов из Redis", len(keys))
	return nil
}

// Функция для загрузки призов из JSON файла в Redis
func loadPrizesFromFileToRedis() error {
	// Загружаем призы из файла
	data, err := os.ReadFile("prizes.json")
	if err != nil {
		return fmt.Errorf("failed to read prizes.json: %v", err)
	}

	var config PrizeConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse prizes.json: %v", err)
	}

	// Сохраняем каждый приз в Redis
	for _, prize := range config.Prizes {
		if err := savePrizeToRedis(prize); err != nil {
			log.Printf("Warning: failed to save prize %s: %v", prize.Name, err)
		}
	}

	log.Printf("Загружено %d призов в Redis из prizes.json", len(config.Prizes))
	return nil
}

// Функция для выдачи приза победителю
func givePrizeToWinner(winnerUsername string, prize Prize) error {
	log.Printf("givePrizeToWinner: Начинаем выдачу приза %s игроку %s", prize.Name, winnerUsername)

	if redisClient == nil {
		log.Printf("givePrizeToWinner: Redis client not available")
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Генерируем уникальный хэш для этого предмета
	itemHash := generateItemHash(winnerUsername, prize.Name)
	key := fmt.Sprintf("inventory:%s:%s", winnerUsername, itemHash)
	log.Printf("givePrizeToWinner: Используем ключ %s для нового предмета", key)

	// Создаем новый элемент инвентаря
	item := InventoryItem{
		PrizeName: prize.Name,
		Rarity:    prize.Rarity,
		Cost:      prize.Cost,
		Count:     1, // Каждый предмет хранится отдельно
		Hash:      itemHash,
	}

	// Сохраняем в Redis
	data, err := json.Marshal(item)
	if err != nil {
		log.Printf("givePrizeToWinner: Ошибка маршалинга: %v", err)
		return fmt.Errorf("failed to marshal inventory item: %v", err)
	}

	err = redisClient.Set(ctx, key, data, 0).Err()
	if err != nil {
		log.Printf("givePrizeToWinner: Ошибка сохранения в Redis: %v", err)
		return fmt.Errorf("failed to save inventory item: %v", err)
	}

	log.Printf("givePrizeToWinner: Приз %s успешно выдан игроку %s (хэш: %s)", prize.Name, winnerUsername, itemHash)

	// Проверяем, что предмет действительно сохранен
	_, testErr := redisClient.Get(ctx, key).Result()
	if testErr != nil {
		log.Printf("givePrizeToWinner: ОШИБКА: не удалось проверить сохраненный предмет: %v", testErr)
	} else {
		log.Printf("givePrizeToWinner: Проверка пройдена - предмет сохранен под ключом %s", key)
	}

	return nil
}

// Функция для получения инвентаря игрока
func getPlayerInventory(username string) ([]InventoryItem, error) {
	log.Printf("getPlayerInventory: Получаем инвентарь для пользователя %s", username)

	if redisClient == nil {
		log.Printf("getPlayerInventory: Redis client not available")
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("inventory:%s:*", username)
	log.Printf("getPlayerInventory: Ищем ключи по паттерну %s", pattern)

	keys, err := redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		log.Printf("getPlayerInventory: Ошибка получения ключей: %v", err)
		return nil, fmt.Errorf("failed to get inventory keys: %v", err)
	}

	log.Printf("getPlayerInventory: Найдено %d ключей: %v", len(keys), keys)

	var inventory []InventoryItem
	for _, key := range keys {
		log.Printf("getPlayerInventory: Обрабатываем ключ %s", key)
		val, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			log.Printf("getPlayerInventory: Ошибка получения значения для ключа %s: %v", key, err)
			continue
		}

		log.Printf("getPlayerInventory: Значение для ключа %s: %s", key, val)

		var item InventoryItem
		err = json.Unmarshal([]byte(val), &item)
		if err != nil {
			log.Printf("getPlayerInventory: Ошибка распаковки для ключа %s: %v", key, err)
			continue
		}

		log.Printf("getPlayerInventory: Добавляем предмет: %s (хэш: %s)", item.PrizeName, item.Hash)
		inventory = append(inventory, item)
	}

	log.Printf("getPlayerInventory: Возвращаем %d предметов", len(inventory))
	return inventory, nil
}

// Функция для получения всех экземпляров предмета игрока (для продажи)
func getPlayerItemInstances(username, prizeName string) ([]InventoryItem, error) {
	log.Printf("getPlayerItemInstances: Получаем все экземпляры %s для пользователя %s", prizeName, username)

	if redisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("inventory:%s:*", username)

	keys, err := redisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get inventory keys: %v", err)
	}

	var instances []InventoryItem
	for _, key := range keys {
		val, err := redisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var item InventoryItem
		err = json.Unmarshal([]byte(val), &item)
		if err != nil {
			continue
		}

		// Ищем предметы с нужным именем
		if item.PrizeName == prizeName {
			instances = append(instances, item)
		}
	}

	log.Printf("getPlayerItemInstances: Найдено %d экземпляров предмета %s", len(instances), prizeName)
	return instances, nil
}

// Функция для выбора случайного приза по редкости
func selectRandomPrizeByRarity(rarity Rarity) (Prize, error) {
	log.Printf("selectRandomPrizeByRarity: Выбираем приз для редкости %s", rarity)

	// Загружаем все призы из Redis
	prizes, err := loadAllPrizesFromRedis()
	if err != nil {
		log.Printf("selectRandomPrizeByRarity: Ошибка загрузки призов: %v", err)
		return Prize{}, fmt.Errorf("failed to load prizes: %v", err)
	}

	log.Printf("selectRandomPrizeByRarity: Загружено %d призов из Redis", len(prizes))

	// Фильтруем призы по редкости
	var filteredPrizes []Prize
	for _, prize := range prizes {
		if prize.Rarity == string(rarity) {
			filteredPrizes = append(filteredPrizes, prize)
		}
	}

	log.Printf("selectRandomPrizeByRarity: Найдено %d призов для редкости %s", len(filteredPrizes), rarity)

	if len(filteredPrizes) == 0 {
		log.Printf("selectRandomPrizeByRarity: Не найдено призов для редкости %s", rarity)
		return Prize{}, fmt.Errorf("no prizes found for rarity %s", rarity)
	}

	// Выбираем случайный приз из отфильтрованных
	randomIndex := crand.Intn(len(filteredPrizes))
	selectedPrize := filteredPrizes[randomIndex]

	log.Printf("selectRandomPrizeByRarity: Выбрана плашка '%s' (индекс %d из %d)", selectedPrize.Name, randomIndex, len(filteredPrizes))
	return selectedPrize, nil
}

// Функция для безопасного изменения баланса (гарантирует отсутствие отрицательных значений)
func changeBalance(username string, amount int) bool {
	log.Printf("changeBalance: Попытка изменить баланс %s на %d", username, amount)
	if _, exists := playerBalances[username]; !exists {
		log.Printf("changeBalance: Баланс %s не найден в памяти, проверяем Redis", username)
		// Проверяем, есть ли баланс в Redis
		if balance, ok := loadBalanceFromRedis(username); ok {
			playerBalances[username] = balance
			log.Printf("changeBalance: Загружен баланс из Redis: %d", balance)
		} else {
			log.Printf("changeBalance: Баланс %s не найден в Redis, операция отменена", username)
			return false
		}
	}

	newBalance := playerBalances[username] + amount
	if newBalance < 0 {
		return false // Не позволяем балансу стать отрицательным
	}

	playerBalances[username] = newBalance
	log.Printf("changeBalance: Баланс %s изменен на %d", username, newBalance)

	// Сохраняем в Redis
	saveBalanceToRedis(username, newBalance)

	return true
}

// Функция для инициализации балансов участников
func initializeBalances() {
	// Сначала пытаемся загрузить существующие балансы из Redis
	loadAllBalancesFromRedis()

	// Для новых участников, у которых нет баланса, устанавливаем начальный баланс
	for _, username := range participantIDs {
		if username != "" {
			if _, exists := playerBalances[username]; !exists {
				playerBalances[username] = 1000 // Начальный баланс 1000
				saveBalanceToRedis(username, 1000)
			}
		}
	}
}

func promoteUserToAdmin(bot *tgbotapi.BotAPI, chatID int64, userID int64) {
	promoteConfig := tgbotapi.PromoteChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
		CanChangeInfo:      false,
		CanManageChat:      false,
		CanEditMessages:    false,
		CanDeleteMessages:  false,
		CanInviteUsers:     false,
		CanRestrictMembers: false,
		CanPinMessages:     false,
		CanPromoteMembers:  false,
	}

	_, err := bot.Request(promoteConfig)

	if err != nil {
		log.Printf("Ошибка при повышении пользователя %d до администратора: %v", userID, err)

	} else {
		log.Printf("Пользователь %d успешно повышен до администратора", userID)
	}
}

func main() {
	log.Printf("🚀 === ЗАПУСК БОТА ===")

	// Инициализируем канал отмены игры
	gameCancel = make(chan bool, 1)

	// Инициализируем Redis клиент
	log.Printf("main: Инициализируем Redis клиент")
	initRedis()

	// Получаем токен бота из переменной окружения
	token := "8278983491:AAHxFOFBxndgwq2T_zpWBuNZTV9KG70LlLU"
	log.Printf("main: Токен бота получен (скрыт для безопасности)")

	// Создаем бота
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	// crypto/rand не нуждается в инициализации seed

	// Инициализируем полный список участников
	log.Printf("main: Инициализируем список участников")
	participants = make([]string, 0, len(participantIDs))
	for name := range participantIDs {
		participants = append(participants, name)
	}
	log.Printf("main: Загружено %d участников: %v", len(participants), participants)

	// Инициализируем хэши участников
	log.Printf("main: Инициализируем хэши участников")
	initParticipantHashes()
	log.Printf("main: Хэши инициализированы для %d участников", len(participantHashes))

	// Инициализируем балансы участников
	log.Printf("main: Инициализируем балансы участников")
	initializeBalances()
	log.Printf("main: Балансы инициализированы, всего игроков с балансами: %d", len(playerBalances))

	// Перемешиваем список участников при запуске
	log.Printf("main: Перемешиваем список участников")
	shuffleParticipants()
	log.Printf("main: Участники после перемешивания: %v", participants)

	// Загружаем призы из файла в Redis при запуске
	log.Printf("main: Загружаем призы из prizes.json в Redis")
	if err := loadPrizesFromFileToRedis(); err != nil {
		log.Printf("Ошибка загрузки призов: %v", err)
		log.Printf("main: Продолжаем без призов, будет использоваться дефолтная плашка")
	} else {
		log.Printf("main: Призы успешно загружены")
	}

	// Инициализируем переменные ставок
	bettingPhase = "closed"
	bettingParticipants = []string{}
	finalBettingNumbers = []int{}

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Настраиваем обновления
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обрабатываем обновления
	for update := range updates {
		log.Printf("Получено обновление: %v", update.UpdateID)
		if update.Message != nil { // Если это сообщение
			log.Printf("Получено сообщение от %s: %s", update.Message.From.UserName, update.Message.Text)
			// Проверяем, является ли сообщение командой
			if update.Message.IsCommand() {
				log.Printf("Обработка команды: %s от %s", update.Message.Command(), update.Message.From.UserName)
				// Проверяем доступ пользователя - теперь проверка идет внутри команд
				userName := update.Message.From.UserName

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

				switch update.Message.Command() {
				case "bet":
					log.Printf("🎯 Команда /bet от %s: isGameActive=%t, bettingPhase=%s", userName, isGameActive, bettingPhase)

					// Проверяем, что игра активна
					if !isGameActive {
						log.Printf("❌ Ставка отклонена: игра не активна (isGameActive=false)")
						msg.Text = "🎮 Игра не запущена! Ставки принимаются только во время игры."
						break
					}

					// Проверяем, что фаза ставок открыта
					if bettingPhase == "closed" {
						log.Printf("❌ Ставка отклонена: ставки закрыты (bettingPhase=closed)")
						msg.Text = "❌ Ставки закрыты! Сейчас нельзя делать ставки."
						break
					}
					log.Printf("✅ Ставка принимается: все проверки пройдены")

					// Получаем аргументы команды
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите номер участника и сумму ставки! Пример: /bet 1 100 или /bet 1 all"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Парсим аргументы
					parts := strings.Split(strings.TrimSpace(args), " ")
					if len(parts) != 2 {
						msg.Text = "🚫 Укажите номер участника и сумму ставки через пробел! Пример: /bet 1 100 или /bet 1 all"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Парсим номер участника
					participantN, err := strconv.Atoi(strings.TrimSpace(parts[0]))
					if err != nil {
						msg.Text = "🚫 Неверный формат номера участника!"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Проверяем валидность номера в зависимости от фазы
					var participantName string
					if bettingPhase == "initial" {
						if participantN < 1 || participantN > len(bettingParticipants) {
							msg.Text = fmt.Sprintf("🚫 Неверный номер участника! Доступные номера: 1-%d", len(bettingParticipants))
							msg.ReplyToMessageID = update.Message.MessageID
							break
						}
						participantName = bettingParticipants[participantN-1]
					} else if bettingPhase == "final" {
						// Для финальных ставок проверяем, что номер в списке допустимых номеров
						validIndex := -1
						for i, num := range finalBettingNumbers {
							if participantN == num {
								validIndex = i
								break
							}
						}
						if validIndex == -1 {
							validNumbersStr := ""
							for i, num := range finalBettingNumbers {
								if i > 0 {
									validNumbersStr += ", "
								}
								validNumbersStr += fmt.Sprintf("%d", num)
							}
							msg.Text = fmt.Sprintf("🚫 Неверный номер участника! Доступные номера: %s", validNumbersStr)
							break
						}
						participantName = bettingParticipants[validIndex]
					} else {
						msg.Text = "🚫 Ставки сейчас не принимаются!"
						break
					}

					// Парсим сумму ставки
					var betAmount int
					amountStr := strings.TrimSpace(parts[1])

					if strings.ToLower(amountStr) == "all" {
						// Ставим все деньги
						if balance, exists := playerBalances[userName]; exists && balance > 0 {
							betAmount = balance
							log.Printf("🎯 Ставка ALL: пользователь %s ставит все деньги (%d фишек)", userName, betAmount)
						} else {
							msg.Text = "🚫 У вас нет денег для ставки!"
							break
						}
					} else {
						// Парсим обычную сумму
						var err error
						betAmount, err = strconv.Atoi(amountStr)
						if err != nil || betAmount <= 0 {
							msg.Text = "🚫 Укажите корректную положительную сумму ставки или 'all'!"
							break
						}
					}

					// Проверяем баланс пользователя
					if balance, exists := playerBalances[userName]; !exists || balance < betAmount {
						msg.Text = fmt.Sprintf("🚫 Недостаточно средств! Ваш баланс: %d %s, требуется: %d %s",
							balance, getChipsWord(balance), betAmount, getChipsWord(betAmount))
						break
					}

					// Получаем хэш участника
					participantHash := participantHashes[participantName]

					// Создаем ставку
					bet := Bet{
						Username:        userName,
						ParticipantName: participantName,
						ParticipantHash: participantHash,
						Amount:          betAmount,
					}

					// Списываем сумму ставки с баланса
					if !changeBalance(userName, -betAmount) {
						msg.Text = "🚫 Ошибка при списании средств!"
						break
					}

					// Сохраняем ставку
					if bettingPhase == "initial" {
						initialBets[userName] = bet
						saveBetsToRedis(initialBets, "game:initialBets")
						log.Printf("bet: Сохранена начальная ставка %s на участника %s (хэш %s)", userName, participantName, participantHash)
					} else {
						finalBets[userName] = bet
						saveBetsToRedis(finalBets, "game:finalBets")
						log.Printf("bet: Сохранена финальная ставка %s на участника %s (хэш %s)", userName, participantName, participantHash)
					}

					msg.Text = fmt.Sprintf("✅ Ставка принята!\n🎯 Вы поставили на №%d: %s\n💰 Списано: %d %s\n💰 Ваш баланс: %d %s",
						participantN, participantName, betAmount, getChipsWord(betAmount), playerBalances[userName], getChipsWord(playerBalances[userName]))
					msg.ReplyToMessageID = update.Message.MessageID

				case "game":
					// Проверяем, не запущена ли уже игра или идет процесс завершения
					log.Printf("Команда /game: isGameActive=%t, gameInProgress=%t", isGameActive, gameInProgress)
					if isGameActive || gameInProgress {
						msg.Text = "Для запуска игры нужно сделать /reset"
						log.Printf("Команда /game: Отклонена - игра уже активна")
						break
					}

					// Проверяем, есть ли участники
					if len(participants) < 2 {
						msg.Text = "🚫 Недостаточно участников для игры! Нужно минимум 2 участника."
						break
					}

					// Очищаем предыдущие ставки
					initialBets = make(map[string]Bet)
					finalBets = make(map[string]Bet)
					finalBettingNumbers = []int{}

					// Устанавливаем фазу ставок
					bettingPhase = "initial"

					// Выбираем плашку для этой игры (всегда новая при каждом запуске)
					rarity := GenerateRandomRarity()
					selectedPrize, err := selectRandomPrizeByRarity(rarity)
					if err != nil {
						log.Printf("Ошибка выбора плашки: %v, используем дефолтную", err)
						currentPrize = Prize{Name: "ЧМО", Rarity: "common", Cost: 300}
					} else {
						currentPrize = selectedPrize
						log.Printf("Выбрана плашка для игры: %s (%s редкость)", currentPrize.Name, currentPrize.Rarity)
					}

					// Создаем отсортированный список участников для ставок (по фамилии)
					bettingParticipants = make([]string, len(participants))
					copy(bettingParticipants, participants)

					// Сортируем по фамилии (предполагаем формат "Имя Фамилия")
					for i := 0; i < len(bettingParticipants)-1; i++ {
						for j := i + 1; j < len(bettingParticipants); j++ {
							namePartsI := strings.Split(bettingParticipants[i], " ")
							namePartsJ := strings.Split(bettingParticipants[j], " ")

							var surnameI, surnameJ string
							if len(namePartsI) >= 2 {
								surnameI = namePartsI[len(namePartsI)-1] // Последнее слово - фамилия
							} else {
								surnameI = bettingParticipants[i]
							}
							if len(namePartsJ) >= 2 {
								surnameJ = namePartsJ[len(namePartsJ)-1] // Последнее слово - фамилия
							} else {
								surnameJ = bettingParticipants[j]
							}

							if surnameI > surnameJ {
								bettingParticipants[i], bettingParticipants[j] = bettingParticipants[j], bettingParticipants[i]
							}
						}
					}

					// Сохраняем первоначальный список для ставок (он не будет меняться)
					initialBettingParticipants = make([]string, len(bettingParticipants))
					copy(initialBettingParticipants, bettingParticipants)

					// Создаем сообщение со списком участников для ставок
					gameText := "🎮 НАЧИНАЕМ ИГРУ!\n\n"

					// Показываем редкость будущей плашки
					rarityText := ""
					switch currentPrize.Rarity {
					case "common":
						rarityText = "ОБЫЧНАЯ"
					case "rare":
						rarityText = "РЕДКАЯ"
					case "legendary":
						rarityText = "ЛЕГЕНДАРНАЯ"
					}
					gameText += fmt.Sprintf("🎁 БУДЕТ РАЗЫГРАНА %s ПЛАШКА!\n\n", rarityText)

					gameText += "🏆 УЧАСТНИКИ:\n"
					for i, participant := range bettingParticipants {
						gameText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithItem(participant))
					}
					gameText += "\n💰 РАУНД СТАВОК!\n"
					gameText += "🎯 Ставьте на победителя: /bet N СУММА\n"
					gameText += "💎 Коэффициент: x30\n"
					gameText += "⏰ Время: 30 секунд\n"

					// Отправляем начальное сообщение со ставками
					gameChatID = update.Message.Chat.ID
					initialMsg := tgbotapi.NewMessage(gameChatID, gameText)
					sentMsg, err := bot.Send(initialMsg)
					if err != nil {
						log.Printf("Ошибка отправки начального сообщения: %v", err)
						msg.Text = "🚫 Ошибка запуска игры!"
						break
					}

					// Очищаем канал отмены от предыдущих сигналов
					select {
					case <-gameCancel:
						log.Printf("Команда /game: Очищен старый сигнал отмены")
					default:
						// Канал пуст
					}

					// Теперь устанавливаем флаги игры
					isGameActive = true   // Устанавливаем после успешной отправки сообщения
					gameInProgress = true // Помечаем, что процесс игры запущен

					// Сохраняем ID сообщения для редактирования
					gameMessageID = sentMsg.MessageID
					totalRounds = len(participants) - 1
					log.Printf("Игра запущена: chatID=%d, messageID=%d, totalRounds=%d", gameChatID, gameMessageID, totalRounds)

					// Запускаем таймер на 30 секунд с возможностью отмены
					go func() {
						select {
						case <-time.After(30 * time.Second):
							// Таймер истек - запускаем игру
							log.Printf("Горутина игры: Таймер истек, запускаем игру")
							bettingPhase = "closed"
							runGameSession(bot)
							log.Printf("Горутина игры: runGameSession завершен")

						case <-gameCancel:
							// Игра была отменена через stopgame
							log.Printf("Горутина игры: Игра отменена через stopgame")
							return
						}
					}()

					// Отправляем подтверждение запуска
					msg.Text = "✅ Игра запущена! У вас 30 секунд на ставки."
					break

				case "status":
					statusText := fmt.Sprintf("📊 Статус бота:\n"+
						"isGameActive: %t\n"+
						"currentRound: %d\n"+
						"bettingPhase: %s\n"+
						"len(participants): %d\n"+
						"len(initialBets): %d\n"+
						"len(finalBets): %d\n"+
						"currentPrize: %s (%s)",
						isGameActive, currentRound, bettingPhase,
						len(participants), len(initialBets), len(finalBets),
						currentPrize.Name, currentPrize.Rarity)
					msg.Text = statusText

				case "reset":
					log.Printf("Команда /reset: Вызвана пользователем %s", userName)

					// Команда для полного сброса состояния и восстановления списка участников (только для администраторов)
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						log.Printf("Команда /reset: Отклонена - пользователь %s не администратор", userName)
						msg.Text = "🚫 Только администраторы могут использовать эту команду!"
						break
					}

					log.Printf("Команда /reset: Администратор %s подтвердил, выполняем сброс", userName)

					// Полностью сбрасываем ВСЕ состояние
					isGameActive = false
					gameInProgress = false
					currentRound = 0
					bettingPhase = "closed"
					currentPrize = Prize{}

					// Очищаем все ставки
					initialBets = make(map[string]Bet)
					finalBets = make(map[string]Bet)
					finalBettingNumbers = []int{}

					// Восстанавливаем список участников из participantIDs
					participants = make([]string, 0, len(participantIDs))
					for name := range participantIDs {
						participants = append(participants, name)
					}
					log.Printf("Команда /reset: Восстановлено %d участников: %v", len(participants), participants)

					// Восстанавливаем хэши участников
					participantHashes = make(map[string]string)
					for name, username := range participantIDs {
						participantHashes[name] = hashParticipant(username)
					}
					log.Printf("Команда /reset: Восстановлено %d хэшей участников", len(participantHashes))

					// Очищаем канал отмены
					select {
					case <-gameCancel:
						log.Printf("Команда /reset: Очищен сигнал отмены")
					default:
						// Канал пуст
					}

					msg.Text = fmt.Sprintf("🔄 Полный сброс состояния выполнен!\n✅ Восстановлено %d участников", len(participants))
					log.Printf("Команда /reset: Успешно выполнена, отправляем сообщение: %s", msg.Text)

				case "clearallinv":
					log.Printf("Команда /clearallinv: Вызвана пользователем %s", userName)

					// Команда для очистки всех инвентарей (только для администраторов)
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						log.Printf("Команда /clearallinv: Отклонена - пользователь %s не администратор", userName)
						msg.Text = "🚫 Только администраторы могут использовать эту команду!"
						break
					}

					log.Printf("Команда /clearallinv: Администратор %s подтвердил, очищаем все инвентари", userName)

					if redisClient == nil {
						log.Printf("Команда /clearallinv: Redis client not available")
						msg.Text = "❌ Ошибка подключения к базе данных!"
						break
					}

					ctx := context.Background()

					// Ищем все ключи инвентаря
					pattern := "inventory:*:*"
					keys, err := redisClient.Keys(ctx, pattern).Result()
					if err != nil {
						log.Printf("Команда /clearallinv: Ошибка получения ключей инвентаря: %v", err)
						msg.Text = "❌ Ошибка получения списка инвентарей!"
						break
					}

					log.Printf("Команда /clearallinv: Найдено %d ключей инвентаря для удаления", len(keys))

					if len(keys) == 0 {
						msg.Text = "🧹 Все инвентари уже пусты!"
						log.Printf("Команда /clearallinv: Инвентари уже пусты")
						break
					}

					// Удаляем все ключи инвентаря
					deletedCount, err := redisClient.Del(ctx, keys...).Result()
					if err != nil {
						log.Printf("Команда /clearallinv: Ошибка удаления инвентарей: %v", err)
						msg.Text = "❌ Ошибка очистки инвентарей!"
						break
					}

					log.Printf("Команда /clearallinv: Успешно удалено %d предметов из инвентарей", deletedCount)
					msg.Text = fmt.Sprintf("🧹 Все инвентари очищены!\n✅ Удалено %d предметов у всех игроков", deletedCount)

				case "setdefaultbalance":
					log.Printf("Команда /setdefaultbalance: Вызвана пользователем %s", userName)

					// Команда для установки баланса 1000 фишек всем игрокам (только для администраторов)
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						log.Printf("Команда /setdefaultbalance: Отклонена - пользователь %s не администратор", userName)
						msg.Text = "🚫 Только администраторы могут использовать эту команду!"
						break
					}

					log.Printf("Команда /setdefaultbalance: Администратор %s подтвердил, устанавливаем баланс 1000 всем игрокам", userName)

					// Устанавливаем баланс 1000 для всех игроков
					setCount := 0
					for username := range participantIDs {
						playerBalances[username] = 1000
						setCount++
						log.Printf("Команда /setdefaultbalance: Установлен баланс 1000 для игрока %s", username)
					}

					// Сохраняем балансы в Redis
					if err := saveBalancesToRedis(); err != nil {
						log.Printf("Команда /setdefaultbalance: Ошибка сохранения балансов в Redis: %v", err)
						msg.Text = "❌ Ошибка сохранения балансов!"
						break
					}

					log.Printf("Команда /setdefaultbalance: Успешно установлено 1000 фишек для %d игроков", setCount)
					msg.Text = fmt.Sprintf("💰 Баланс сброшен!\n✅ Установлено 1000 фишек для %d игроков", setCount)

				case "stopgame":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять игрой!"
						break
					}

					log.Printf("Команда /stopgame: isGameActive=%t, gameInProgress=%t", isGameActive, gameInProgress)
					if !isGameActive {
						msg.Text = "🎮 Игра не запущена!"
						break
					}

					// Отменяем активную горутину игры
					select {
					case gameCancel <- true:
						log.Printf("Команда /stopgame: Отправлен сигнал отмены активной игре")
					default:
						log.Printf("Команда /stopgame: Нет активной горутины для отмены")
					}

					// Сбрасываем состояние игры
					isGameActive = false
					gameInProgress = false // Сбрасываем флаг процесса игры
					bettingPhase = "closed"
					currentRound = 0

					// Очищаем ставки
					initialBets = make(map[string]Bet)
					finalBets = make(map[string]Bet)
					finalBettingNumbers = []int{}

					// Сбрасываем выбранную плашку
					currentPrize = Prize{}

					msg.Text = "🛑 Игра остановлена!"

				case "start":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять игрой!"
						break
					}
					msg.Text = fmt.Sprintf("привет долбоебы! сейчас будем решать кого удалить нахуй\nВсего участников: %d\n", len(participants))

				case "restart":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять игрой!"
						break
					}
					// Копируем всех участников из основного списка participantIDs
					participants = make([]string, 0, len(participantIDs))
					for name := range participantIDs {
						participants = append(participants, name)
					}
					shuffleParticipants() // Перемешиваем список
					msg.Text = fmt.Sprintf("🎲 Новый раунд! участвует %d участника", len(participants))

				case "mention":
					msg.Text = "🚫 К сожалению, Telegram Bot API не позволяет автоматически отмечать всех участников группы.\n\n" +
						"**Варианты решения:**\n" +
						"1️⃣ Сделайте бота администратором группы\n" +
						"2️⃣ Используйте команду @all (если есть такой бот в группе)\n" +
						"3️⃣ Отмечайте участников вручную\n" +
						"4️⃣ Добавьте username участников в код бота для автоматической отметки\n\n" +
						"🎲 Продолжаем игру!"

				case "add":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять списком участников!"
						break
					}
					// Получаем аргументы команды
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите имя, фамилию и username! Пример: /add Иван Иванов ivan_username"
					} else {
						parts := strings.Split(args, " ")
						if len(parts) < 3 {
							msg.Text = "🚫 Укажите имя, фамилию и username через пробел! Пример: /add Иван Иванов ivan_username"
						} else {
							firstName := strings.TrimSpace(parts[0])
							lastName := strings.TrimSpace(parts[1])
							username := strings.TrimSpace(parts[2])

							if firstName == "" || lastName == "" || username == "" {
								msg.Text = "🚫 Имя, фамилия и username не могут быть пустыми!"
							} else {
								fullName := firstName + " " + lastName
								participantIDs[fullName] = username
								// Обновляем хэш нового участника (хэш от username)
								participantHashes[fullName] = hashParticipant(username)
								// Также добавляем в текущий активный список, если он не пустой
								if len(participants) > 0 {
									participants = append(participants, fullName)
								}
								msg.Text = fmt.Sprintf("✅ Участник %s (@%s) добавлен в основной список!\nТеперь в списке %d участников.", fullName, username, len(participantIDs))
							}
						}
					}

				case "remove":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять списком участников!"
						break
					}
					// Получаем аргументы команды
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите имя участника! Пример: /remove Арсений Квятковский"
					} else {
						participantName := strings.TrimSpace(args)

						// Удаляем из основного списка participantIDs
						if _, exists := participantIDs[participantName]; exists {
							delete(participantIDs, participantName)
							// Также удаляем хэш участника
							delete(participantHashes, participantName)

							// Также удаляем из текущего списка participants, если он там есть
							for i, participant := range participants {
								if participant == participantName {
									participants = append(participants[:i], participants[i+1:]...)
									break
								}
							}

							msg.Text = fmt.Sprintf("✅ Участник %s удален из основного списка!\nТеперь в списке %d участников.", participantName, len(participantIDs))
						} else {
							msg.Text = fmt.Sprintf("🚫 Участник '%s' не найден в основном списке!", participantName)
						}
					}
				case "setprize":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут изменять плашку!"
						break
					}
					// Получаем аргументы команды
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = fmt.Sprintf("🎁 Текущая плашка: \"%s\" (%s редкость)\nУкажите ID или название плашки! Пример: /setprize chmo", currentPrize.Name, currentPrize.Rarity)
					} else {
						// Ищем плашку по ID или названию
						found := false
						for _, prize := range prizes {
							if prize.ID == args || prize.Name == args {
								oldPrize := currentPrize
								currentPrize = prize
								msg.Text = fmt.Sprintf("🎁 Плашка изменена!\nБыло: \"%s\" (%s)\nСтало: \"%s\" (%s)", oldPrize.Name, oldPrize.Rarity, currentPrize.Name, currentPrize.Rarity)
								found = true
								break
							}
						}
						if !found {
							availablePrizes := ""
							for i, prize := range prizes {
								if i > 0 {
									availablePrizes += ", "
								}
								availablePrizes += fmt.Sprintf("%s (%s)", prize.ID, prize.Name)
							}
							msg.Text = fmt.Sprintf("🚫 Плашка '%s' не найдена!\nДоступные плашки: %s", args, availablePrizes)
						}
					}

				case "poll":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут создавать голосования!"
						break
					}
					if len(participants) == 0 {
						msg.Text = "📊 Нет участников для голосования!"
					} else if len(participants) > 10 {
						msg.Text = fmt.Sprintf("📊 Слишком много участников (%d). Максимум 10 для poll. Используйте /list", len(participants))
					} else {
						// Определяем вопрос в зависимости от количества участников
						question := "🎯 Кто следующий участник?"
						if len(participants) == 2 {
							question = fmt.Sprintf("🏆 Кто получит плашку \"%s\"?", currentPrize.Name)
						}

						// Создаем poll
						pollOptions := make([]string, len(participants))
						for i, participant := range participants {
							pollOptions[i] = formatParticipantNameWithItem(participant)
						}
						poll := tgbotapi.SendPollConfig{
							BaseChat: tgbotapi.BaseChat{
								ChatID: update.Message.Chat.ID,
							},
							Question:    question,
							Options:     pollOptions,
							IsAnonymous: false, // Не анонимный poll
						}

						if _, err := bot.Send(poll); err != nil {
							msg.Text = "🚫 Ошибка создания poll: " + err.Error()
						}
					}

				case "prize":
					rarityText := ""
					switch currentPrize.Rarity {
					case "common":
						rarityText = "ОБЫЧНАЯ"
					case "rare":
						rarityText = "РЕДКАЯ"
					case "legendary":
						rarityText = "ЛЕГЕНДАРНАЯ"
					}
					msg.Text = fmt.Sprintf("🎁 В этой игре будет разыграна %s плашка для победителя!", rarityText)

				case "balance":
					userName := update.Message.From.UserName
					if balance, exists := playerBalances[userName]; exists {
						// Дополнительная проверка на отрицательный баланс (на всякий случай)
						if balance < 0 {
							playerBalances[userName] = 0 // Исправляем отрицательный баланс
							balance = 0
						}
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, fmt.Sprintf("💰 Ваш баланс: %d %s", balance, getChipsWord(balance)))
						msg.ReplyToMessageID = update.Message.MessageID
						if _, err := bot.Send(msg); err != nil {
							log.Panic(err)
						}
					} else {
						msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Ваш баланс не найден. Обратитесь к администратору.")
						msg.ReplyToMessageID = update.Message.MessageID
						if _, err := bot.Send(msg); err != nil {
							log.Panic(err)
						}
					}
					continue // Пропускаем стандартную отправку сообщения

				case "givefunds":
					// Проверяем, является ли пользователь администратором
					userName := update.Message.From.UserName
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут использовать эту команду!"
					} else {
						args := update.Message.CommandArguments()
						if args == "" {
							msg.Text = "🚫 Укажите username получателя и сумму! Пример: /givefunds @username 500"
						} else {
							parts := strings.Split(args, " ")
							if len(parts) < 2 {
								msg.Text = "🚫 Укажите username получателя и сумму через пробел! Пример: /givefunds @username 500"
							} else {
								recipientUsername := strings.TrimPrefix(strings.TrimSpace(parts[0]), "@")
								amountStr := strings.TrimSpace(parts[1])

								amount, err := strconv.Atoi(amountStr)
								if err != nil || amount <= 0 {
									msg.Text = "🚫 Укажите корректную положительную сумму!"
								} else if !changeBalance(recipientUsername, amount) {
									msg.Text = fmt.Sprintf("🚫 Ошибка при изменении баланса пользователя @%s!", recipientUsername)
								} else {
									msg.Text = fmt.Sprintf("✅ Успешно добавлено %d %s пользователю @%s!\n💰 Новый баланс: %d %s",
										amount, getChipsWord(amount), recipientUsername, playerBalances[recipientUsername], getChipsWord(playerBalances[recipientUsername]))
								}
							}
						}
					}

				case "withdrawfunds":
					// Проверяем, является ли пользователь администратором
					userName := update.Message.From.UserName
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут использовать эту команду!"
					} else {
						args := update.Message.CommandArguments()
						if args == "" {
							msg.Text = "🚫 Укажите username и сумму для снятия! Пример: /withdrawfunds @username 500"
						} else {
							parts := strings.Split(args, " ")
							if len(parts) < 2 {
								msg.Text = "🚫 Укажите username и сумму для снятия через пробел! Пример: /withdrawfunds @username 500"
							} else {
								targetUsername := strings.TrimPrefix(strings.TrimSpace(parts[0]), "@")
								amountStr := strings.TrimSpace(parts[1])

								amount, err := strconv.Atoi(amountStr)
								if err != nil || amount <= 0 {
									msg.Text = "🚫 Укажите корректную положительную сумму!"
								} else if _, exists := playerBalances[targetUsername]; !exists {
									msg.Text = fmt.Sprintf("🚫 Пользователь @%s не найден в списке участников!", targetUsername)
								} else if !changeBalance(targetUsername, -amount) {
									msg.Text = fmt.Sprintf("🚫 Недостаточно средств! Баланс @%s: %d %s",
										targetUsername, playerBalances[targetUsername], getChipsWord(playerBalances[targetUsername]))
								} else {
									msg.Text = fmt.Sprintf("✅ Успешно снято %d %s у пользователя @%s!\n💰 Новый баланс: %d %s",
										amount, getChipsWord(amount), targetUsername, playerBalances[targetUsername], getChipsWord(playerBalances[targetUsername]))
								}
							}
						}
					}

				case "pay":
					log.Printf("Команда /pay от %s", userName)
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите получателя и сумму! Пример: /pay @username 500"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					parts := strings.Split(args, " ")
					if len(parts) < 2 {
						msg.Text = "🚫 Укажите получателя и сумму через пробел! Пример: /pay @username 500"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					recipientUsername := strings.TrimPrefix(strings.TrimSpace(parts[0]), "@")
					amountStr := strings.TrimSpace(parts[1])

					amount, err := strconv.Atoi(amountStr)
					if err != nil || amount <= 0 {
						msg.Text = "🚫 Укажите корректную положительную сумму!"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Проверяем, что получатель существует
					if _, exists := playerBalances[recipientUsername]; !exists {
						msg.Text = fmt.Sprintf("🚫 Пользователь @%s не найден в списке участников!", recipientUsername)
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Проверяем, что не переводим себе
					if recipientUsername == userName {
						msg.Text = "🚫 Нельзя переводить фишки самому себе!"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Проверяем баланс отправителя
					senderBalance, exists := playerBalances[userName]
					if !exists || senderBalance < amount {
						msg.Text = fmt.Sprintf("🚫 Недостаточно средств! Ваш баланс: %d %s",
							senderBalance, getChipsWord(senderBalance))
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Выполняем перевод
					if !changeBalance(userName, -amount) {
						msg.Text = "🚫 Ошибка при списании средств!"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					if !changeBalance(recipientUsername, amount) {
						// Возвращаем фишки отправителю в случае ошибки
						changeBalance(userName, amount)
						msg.Text = "🚫 Ошибка при зачислении средств получателю!"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					log.Printf("Команда /pay: %s перевел %d фишек пользователю %s", userName, amount, recipientUsername)
					msg.Text = fmt.Sprintf("✅ Успешно переведено %d %s пользователю @%s!\n💰 Ваш баланс: %d %s",
						amount, getChipsWord(amount), recipientUsername, playerBalances[userName], getChipsWord(playerBalances[userName]))
					msg.ReplyToMessageID = update.Message.MessageID

				case "debug":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут просматривать отладочную информацию!"
						break
					}
					debugText := "🔍 Отладочная информация:\n"
					debugText += fmt.Sprintf("Всего в participantIDs: %d\n", len(participantIDs))
					debugText += fmt.Sprintf("Активных в participants: %d\n", len(participants))

					// Проверяем консистентность
					validCount := 0
					duplicates := 0
					seen := make(map[string]bool)

					for _, p := range participants {
						if participantIDs[p] != "" {
							if seen[p] {
								duplicates++
							} else {
								seen[p] = true
								validCount++
							}
						}
					}

					debugText += fmt.Sprintf("Валидных участников: %d\n", validCount)
					debugText += fmt.Sprintf("Дубликатов: %d\n", duplicates)

					if len(participants) != validCount {
						debugText += "⚠️ Найдены невалидные данные! Используйте /reset для восстановления.\n"
					}

					msg.Text = debugText

				case "list":
					if len(participants) == 0 {
						msg.Text = "🎮 ИГРА ОКОНЧЕНА - СПИСОК ПУСТ\n\nИспользуйте /reset для начала новой игры со всеми участниками."
					} else {
						msg.Text = fmt.Sprintf("🎮 ТЕКУЩИЕ УЧАСТНИКИ ИГРЫ (%d):\n", len(participants))
						for i, participant := range participants {
							msg.Text += fmt.Sprintf("\n%d. %s", i+1, formatParticipantNameWithItem(participant))
						}
					}

				case "leaderboard":
					log.Printf("Команда /leaderboard от %s", userName)
					log.Printf("Команда /leaderboard: participantIDs содержит %d участников", len(participantIDs))

					// Создаем карту стоимости инвентаря и списка предметов для каждого игрока
					inventoryValues := make(map[string]int)
					inventoryItems := make(map[string][]InventoryItem)

					// Для каждого участника считаем стоимость его инвентаря
					for participantName, username := range participantIDs {
						log.Printf("Команда /leaderboard: обрабатываем участника %s (username: %s)", participantName, username)
						inventory, err := getPlayerInventory(username)
						if err != nil {
							log.Printf("Ошибка получения инвентаря для %s: %v", username, err)
							continue
						}

						totalValue := 0
						for _, item := range inventory {
							totalValue += item.Cost
							log.Printf("Команда /leaderboard: предмет %s стоит %d, итого %d", item.PrizeName, item.Cost, totalValue)
						}
						inventoryValues[username] = totalValue
						inventoryItems[username] = inventory
						log.Printf("Команда /leaderboard: участник %s имеет стоимость инвентаря %d", participantName, totalValue)
					}

					log.Printf("Команда /leaderboard: собрано данных для %d участников", len(inventoryValues))

					// Создаем слайс для сортировки
					type playerValue struct {
						username string
						value    int
					}

					var players []playerValue
					for username, value := range inventoryValues {
						players = append(players, playerValue{username: username, value: value})
					}

					// Фильтруем игроков с нулевой стоимостью инвентаря
					var filteredPlayers []playerValue
					for _, player := range players {
						if player.value > 0 {
							filteredPlayers = append(filteredPlayers, player)
						}
					}

					log.Printf("Команда /leaderboard: после фильтрации осталось %d игроков с инвентарем", len(filteredPlayers))

					// Проверяем, есть ли игроки с инвентарем
					if len(filteredPlayers) == 0 {
						log.Printf("Команда /leaderboard: все игроки бомжи, показываем соответствующее сообщение")
						msg.Text = "🏆 ДОСКA ЛИДЕРОВ ПО СТОИМОСТИ ИНВЕНТАРЯ 🏆\n\n💸 Все бомжи! Никто не имеет ценных плашек."
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					// Сортируем по убыванию стоимости
					for i := 0; i < len(filteredPlayers)-1; i++ {
						for j := i + 1; j < len(filteredPlayers); j++ {
							if filteredPlayers[i].value < filteredPlayers[j].value {
								filteredPlayers[i], filteredPlayers[j] = filteredPlayers[j], filteredPlayers[i]
							}
						}
					}

					log.Printf("Команда /leaderboard: сортировка завершена, топ игрок: %s с %d фишками", filteredPlayers[0].username, filteredPlayers[0].value)

					// Формируем сообщение
					msg.Text = "🏆 ДОСКA ЛИДЕРОВ ПО СТОИМОСТИ ИНВЕНТАРЯ 🏆\n\n"

					for i, player := range filteredPlayers {
						if i >= 10 { // Показываем только топ-10
							break
						}

						// Получаем имя участника по username
						participantName := getParticipantNameByUsername(player.username)

						// Получаем надетую плашку для отображения
						wornItem := ""
						if wornData, err := getWornItem(player.username); err == nil && wornData != nil {
							wornItem = " " + wornData["name"]
						}

						emoji := ""
						switch i {
						case 0:
							emoji = "🥇"
						case 1:
							emoji = "🥈"
						case 2:
							emoji = "🥉"
						default:
							emoji = fmt.Sprintf("%d.", i+1)
						}

						msg.Text += fmt.Sprintf("%s %s%s\n", emoji, participantName, wornItem)

						// Показываем список предметов
						playerItems := inventoryItems[player.username]
						if len(playerItems) > 0 {
							// Группируем предметы по имени для краткости
							itemCounts := make(map[string]int)
							for _, item := range playerItems {
								itemCounts[item.PrizeName]++
							}

							itemList := ""
							for itemName, count := range itemCounts {
								if itemList != "" {
									itemList += ", "
								}
								if count > 1 {
									itemList += fmt.Sprintf("%s x%d", itemName, count)
								} else {
									itemList += itemName
								}
							}

							msg.Text += fmt.Sprintf("   📦 %s\n", itemList)
						} else {
							msg.Text += "   📦 Пусто\n"
						}

						msg.Text += fmt.Sprintf("   💰 Стоимость: %d фишек\n\n", player.value)
					}

					// Добавляем информацию о текущем игроке, если он не в топ-10
					currentPlayerValue := inventoryValues[userName]

					// Ищем позицию текущего игрока среди отфильтрованных игроков
					currentRank := -1
					for i, player := range filteredPlayers {
						if player.username == userName {
							currentRank = i + 1
							break
						}
					}

					// Показываем позицию игрока только если у него есть инвентарь
					if currentPlayerValue > 0 && (currentRank > 10 || currentRank == -1) {
						participantName := getParticipantNameByUsername(userName)
						wornItem := ""
						if wornData, err := getWornItem(userName); err == nil && wornData != nil {
							wornItem = " " + wornData["name"]
						}

						if currentRank == -1 {
							msg.Text += fmt.Sprintf("\n\nТвоя позиция:\n%s%s\n", participantName, wornItem)

							// Показываем список предметов игрока
							playerItems := inventoryItems[userName]
							if len(playerItems) > 0 {
								itemCounts := make(map[string]int)
								for _, item := range playerItems {
									itemCounts[item.PrizeName]++
								}

								itemList := ""
								for itemName, count := range itemCounts {
									if itemList != "" {
										itemList += ", "
									}
									if count > 1 {
										itemList += fmt.Sprintf("%s x%d", itemName, count)
									} else {
										itemList += itemName
									}
								}

								msg.Text += fmt.Sprintf("   📦 %s\n", itemList)
							} else {
								msg.Text += "   📦 Пусто\n"
							}

							msg.Text += fmt.Sprintf("   💰 Стоимость: %d фишек", currentPlayerValue)
						} else {
							msg.Text += fmt.Sprintf("\n\n%d. %s%s\n", currentRank, participantName, wornItem)

							// Показываем список предметов игрока
							playerItems := inventoryItems[userName]
							if len(playerItems) > 0 {
								itemCounts := make(map[string]int)
								for _, item := range playerItems {
									itemCounts[item.PrizeName]++
								}

								itemList := ""
								for itemName, count := range itemCounts {
									if itemList != "" {
										itemList += ", "
									}
									if count > 1 {
										itemList += fmt.Sprintf("%s x%d", itemName, count)
									} else {
										itemList += itemName
									}
								}

								msg.Text += fmt.Sprintf("   📦 %s\n", itemList)
							} else {
								msg.Text += "   📦 Пусто\n"
							}

							msg.Text += fmt.Sprintf("   💰 Стоимость: %d фишек (ты)", currentPlayerValue)
						}
					}

					// Отвечаем на сообщение пользователя
					msg.ReplyToMessageID = update.Message.MessageID

				case "help":
					msg.Text = "ты совсем долбоеб? ты не знаешь команд???\n\n" +
						"🎮 ОСНОВНЫЕ КОМАНДЫ:\n" +
						"/reset - сбросить раунд\n" +
						"/game - начать автоматическую игру с таймером\n" +
						"/stopgame - остановить текущую игру\n" +
						"/list - список активных участников\n" +
						"/prize - показать плашку\n" +
						"/leaderboard - доска лидеров по стоимости инвентаря\n\n" +
						"💰 ЭКОНОМИКА:\n" +
						"/balance - посмотреть свой баланс\n" +
						"/inv - посмотреть свой инвентарь плашек\n" +
						"/sell (хэш) - продать плашку\n" +
						"/wear (хэш) - надеть плашку\n" +
						"/unwear - снять плашку\n" +
						"/pay (@username сумма) - перевести фишки другому игроку\n" +
						"/bet (номер сумма) - сделать ставку на участника\n" +
						"/bet (номер all) - поставить все деньги\n\n" +
						"👑 АДМИНИСТРАТОРСКИЕ КОМАНДЫ:\n" +
						"/add (Имя Фамилия username) - добавить участника\n" +
						"/remove (Имя Фамилия) - удалить участника\n" +
						"/setprize (ID плашки) - установить плашку для игры\n" +
						"/loadfromfile - загрузить призы из prizes.json в Redis\n" +
						"/removefromredis - удалить все призы из Redis\n" +
						"/clearallinv - очистить инвентари всех игроков\n" +
						"/setdefaultbalance - установить всем игрокам баланс 1000 фишек\n" +
						"/poll - голосование\n" +
						"/givefunds (@username сумма) - дать деньги игроку\n" +
						"/withdrawfunds (@username сумма) - снять деньги у игрока\n" +
						"/debug - отладочная информация\n" +
						"/promote (ID) - повысить до администратора\n\n" +
						"это все что тебе надо"
				case "inv":
					log.Printf("Команда /inv: Вызвана пользователем %s", userName)

					// Показать инвентарь игрока
					inventory, err := getPlayerInventory(userName)
					if err != nil {
						log.Printf("Команда /inv: Ошибка загрузки инвентаря: %v", err)
						msg.Text = fmt.Sprintf("❌ Ошибка загрузки инвентаря: %v", err)
					} else if len(inventory) == 0 {
						log.Printf("Команда /inv: Инвентарь пользователя %s пуст", userName)
						msg.Text = fmt.Sprintf("🎒 Инвентарь @%s:\n\n📦 Ваш инвентарь пуст", userName)
					} else {
						log.Printf("Команда /inv: Найдено %d предметов в инвентаре пользователя %s", len(inventory), userName)
						msg.Text = fmt.Sprintf("🎒 Инвентарь @%s:\n", userName)
						totalValue := 0

						// Группируем по редкости для красивого отображения
						commonItems := []InventoryItem{}
						rareItems := []InventoryItem{}
						legendaryItems := []InventoryItem{}

						for _, item := range inventory {
							totalValue += item.Cost * item.Count
							switch item.Rarity {
							case "common":
								commonItems = append(commonItems, item)
							case "rare":
								rareItems = append(rareItems, item)
							case "legendary":
								legendaryItems = append(legendaryItems, item)
							}
						}

						// Показываем по редкостям
						if len(legendaryItems) > 0 {
							msg.Text += "\n🔥 **ЛЕГЕНДАРНЫЕ:**\n"
							for _, item := range legendaryItems {
								msg.Text += fmt.Sprintf("  %s [хэш: %s] (%d фишек) - /sell %s\n",
									item.PrizeName, item.Hash, item.Cost, item.Hash)
							}
						}

						if len(rareItems) > 0 {
							msg.Text += "\n💎 **РЕДКИЕ:**\n"
							for _, item := range rareItems {
								msg.Text += fmt.Sprintf("  %s [хэш: %s] (%d фишек) - /sell %s\n",
									item.PrizeName, item.Hash, item.Cost, item.Hash)
							}
						}

						if len(commonItems) > 0 {
							msg.Text += "\n⚪ **ОБЫЧНЫЕ:**\n"
							for _, item := range commonItems {
								msg.Text += fmt.Sprintf("  %s [хэш: %s] (%d фишек) - /sell %s\n",
									item.PrizeName, item.Hash, item.Cost, item.Hash)
							}
						}

						msg.Text += fmt.Sprintf("\n💰 Общая стоимость инвентаря: %d фишек", totalValue)
						msg.Text += "\n\n💡 Для продажи предмета используйте: /sell <хэш>"
						msg.Text += "\n💡 Для надевания плашки: /wear <хэш>"
						msg.Text += "\n💡 Для снятия плашки: /unwear"
						log.Printf("Команда /inv: Успешно сформирован инвентарь для пользователя %s, длина сообщения: %d", userName, len(msg.Text))
					}

					// Отвечаем на сообщение пользователя
					msg.ReplyToMessageID = update.Message.MessageID

				case "sell":
					log.Printf("Команда /sell от %s", userName)
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите хэш предмета для продажи! Пример: /sell abc123def456"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					itemHash := strings.TrimSpace(args)
					log.Printf("Команда /sell: Попытка продажи предмета с хэшем %s пользователем %s", itemHash, userName)

					// Ищем предмет в инвентаре пользователя
					ctx := context.Background()
					key := fmt.Sprintf("inventory:%s:%s", userName, itemHash)

					val, err := redisClient.Get(ctx, key).Result()
					if err != nil {
						log.Printf("Команда /sell: Предмет с хэшем %s не найден у пользователя %s", itemHash, userName)
						msg.Text = "❌ Предмет с таким хэшем не найден в вашем инвентаре!"
						break
					}

					// Парсим предмет
					var item InventoryItem
					err = json.Unmarshal([]byte(val), &item)
					if err != nil {
						log.Printf("Команда /sell: Ошибка парсинга предмета %s: %v", itemHash, err)
						msg.Text = "❌ Ошибка обработки предмета!"
						break
					}

					// Проверяем, не надет ли этот предмет на игроке
					wornData, wornErr := getWornItem(userName)
					itemWasWorn := false
					if wornErr == nil && wornData != nil && wornData["hash"] == itemHash {
						// Предмет надет - автоматически снимаем
						unwearErr := unwearItem(userName)
						if unwearErr != nil {
							log.Printf("Команда /sell: Ошибка автоматического снятия плашки: %v", unwearErr)
						} else {
							log.Printf("Команда /sell: Плашка %s автоматически снята с игрока %s", item.PrizeName, userName)
							itemWasWorn = true
						}
					}

					// Удаляем предмет из инвентаря
					err = redisClient.Del(ctx, key).Err()
					if err != nil {
						log.Printf("Команда /sell: Ошибка удаления предмета %s: %v", itemHash, err)
						msg.Text = "❌ Ошибка удаления предмета!"
						break
					}

					// Начисляем деньги игроку
					changeBalance(userName, item.Cost)

					log.Printf("Команда /sell: Предмет %s продан за %d фишек пользователем %s", item.PrizeName, item.Cost, userName)

					// Формируем сообщение
					msg.Text = fmt.Sprintf("✅ Предмет \"%s\" продан за %d фишек!", item.PrizeName, item.Cost)
					if itemWasWorn {
						msg.Text += "\n👕 Плашка автоматически снята с вашего имени!"
					}
					msg.Text += fmt.Sprintf("\n💰 Ваш баланс: %d фишек", playerBalances[userName])

					// Отвечаем на сообщение пользователя
					msg.ReplyToMessageID = update.Message.MessageID

				case "wear":
					log.Printf("Команда /wear от %s", userName)
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите хэш предмета для надевания! Пример: /wear abc123"
						msg.ReplyToMessageID = update.Message.MessageID
						break
					}

					itemHash := strings.TrimSpace(args)
					log.Printf("Команда /wear: Попытка надеть предмет с хэшем %s пользователем %s", itemHash, userName)

					// Сначала снимаем текущую плашку, если она есть
					unwearErr := unwearItem(userName)
					if unwearErr != nil && unwearErr.Error() != "нет надетой плашки" {
						log.Printf("Команда /wear: Ошибка снятия предыдущей плашки: %v", unwearErr)
					}

					// Надеваем новую плашку
					err := wearItem(userName, itemHash)
					if err != nil {
						log.Printf("Команда /wear: Ошибка надевания плашки %s: %v", itemHash, err)
						msg.Text = fmt.Sprintf("❌ %s", err.Error())
						break
					}

					// Получаем информацию о надетой плашке для отображения
					wornData, _ := getWornItem(userName)
					if wornData != nil {
						msg.Text = fmt.Sprintf("✅ Плашка \"%s\" надета!\nТеперь ваше имя отображается как: %s",
							wornData["name"], formatParticipantNameWithUsername(getParticipantNameByUsername(userName)))
					} else {
						msg.Text = "✅ Плашка надета!"
					}

					// Отвечаем на сообщение пользователя
					msg.ReplyToMessageID = update.Message.MessageID

				case "unwear":
					log.Printf("Команда /unwear от %s", userName)

					err := unwearItem(userName)
					if err != nil {
						log.Printf("Команда /unwear: Ошибка снятия плашки: %v", err)
						msg.Text = fmt.Sprintf("❌ %s", err.Error())
						break
					}

					msg.Text = "✅ Плашка снята!"

					// Отвечаем на сообщение пользователя
					msg.ReplyToMessageID = update.Message.MessageID

				case "loadfromfile":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут загружать призы!"
						break
					}

					if err := loadPrizesFromFileToRedis(); err != nil {
						msg.Text = fmt.Sprintf("❌ Ошибка загрузки призов: %v", err)
					} else {
						msg.Text = "✅ Призы успешно загружены из prizes.json в Redis!"
					}

				case "removefromredis":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут удалять призы!"
						break
					}

					if err := removeAllPrizesFromRedis(); err != nil {
						msg.Text = fmt.Sprintf("❌ Ошибка удаления призов: %v", err)
					} else {
						msg.Text = "✅ Все призы удалены из Redis!"
					}

				case "promote":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут повышать пользователей!"
						break
					}
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите ID пользователя для повышения до администратора! Пример: /promote 123456789"
					} else {
						userID, err := strconv.ParseInt(strings.TrimSpace(args), 10, 64)
						if err != nil {
							msg.Text = "🚫 Неверный формат ID пользователя! Используйте числовой ID."
						} else {
							promoteUserToAdmin(bot, update.Message.Chat.ID, userID)
							msg.Text = "✅ Попытка повышения пользователя до администратора выполнена."
						}
					}

				default:
					msg.Text = "ты долбоеб? не знаешь команд? пиши /help"
				}

				// Отправляем сообщение
				if _, err := bot.Send(msg); err != nil {
					log.Panic(err)
				}
			}
		}
	}
}
