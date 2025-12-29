package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/redis/go-redis/v9"
)

// Глобальный список участников (инициализируется при запуске)
var participants []string

// Текущая "плашка" для проигравшего
var currentPrize = "ЧМО"

// Структура для хранения ставки
type Bet struct {
	Username        string
	ParticipantName string // Имя участника
	ParticipantHash string // SHA-256 хэш участника
	Amount          int
}

// Переменные для управления игрой
var gameMessageID int
var gameChatID int64
var isGameActive bool
var totalRounds int
var currentRound int

// Переменные для управления ставками
var initialBets = make(map[string]Bet)  // Ставки на начальном этапе (ключ: username игрока)
var finalBets = make(map[string]Bet)    // Ставки на финальном этапе (ключ: username игрока)
var bettingPhase string                 // "initial", "final", "closed"
var bettingParticipants []string        // Участники для ставок (сортированные алфавитно)
var initialBettingParticipants []string // Сохраняем первоначальный список для ставок
var finalBettingNumbers []int           // Номера для финальных ставок
var gameInProgress bool                 // Флаг, что игра в процессе выполнения

// Map для хранения username/ID участников (ключ: имя, значение: user ID)
// ТЕСТОВЫЙ СПИСОК ИЗ 5 УЧАСТНИКОВ
var participantIDs = map[string]string{
	"Алексей Баранов":  "barrrraaa",
	"Глеб Гусев":       "hunnidstooblue",
	"Юля Луцевич":      "iuliia_lutsevich",
	"Василий Гончаров": "BroisHelmut",
	"Никита Шакалов":   "iamnothiding",
}

// Map для хранения хэшей участников (ключ: имя участника, значение: SHA-256 хэш)
var participantHashes = make(map[string]string)

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
	if username != "" {
		return fmt.Sprintf("%s (@%s)", name, username)
	}
	return name
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

	// Выплачиваем выигрыши по начальным ставкам (коэффициент x10)
	if len(initialBets) > 0 {
		log.Printf("payoutWinnings: 🎯 Обрабатываем начальные ставки (x10), количество: %d", len(initialBets))
		resultsText += "💰 *Начальные ставки (x10):*\n"
		log.Printf("payoutWinnings: Начальные ставки найдены, добавляем в resultsText")
		for username, bet := range initialBets {
			log.Printf("payoutWinnings: Проверяем начальную ставку %s: ставка на %s (хэш %s), сумма %d", username, bet.ParticipantName, bet.ParticipantHash[:8]+"...", bet.Amount)
			log.Printf("payoutWinnings: Победитель: %s (хэш %s)", winner, winnerHash[:8]+"...")

			if bet.ParticipantName == winner {
				// Ставка выиграла! Выплачиваем 10 фишек
				winnings := bet.Amount * 10
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

		finalText := fmt.Sprintf("🏆🏆🏆 %s, ПОЗДРАВЛЯЕМ!! Вы выиграли плашку \"%s\"!\n\n🐩 Игра окончена!", formatParticipantNameWithUsername(winner), currentPrize)
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
			finalRoundText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithUsername(participant))
		}
		finalRoundText += "\n⏰ Через 5 секунд начнутся финальные ставки!"

		// Отправляем новое сообщение вместо редактирования старого
		roundMsg := tgbotapi.NewMessage(gameChatID, finalRoundText)
		if _, err := bot.Send(roundMsg); err != nil {
			log.Printf("performGameRound: Ошибка отправки сообщения финального раунда: %v", err)
		}

		log.Printf("performGameRound: Ждем 5 секунд финального раунда...")
		time.Sleep(5 * time.Second)
		log.Printf("performGameRound: Финальный раунд завершен")

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
			finalBetText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithUsername(participant))
		}
		finalBetText += "\n💰 ФИНАЛЬНЫЕ СТАВКИ ОТКРЫТЫ!\n"
		finalBetText += "🎯 Ставьте на победителя: /bet N СУММА\n"
		finalBetText += "💎 Коэффициент: x2\n"
		finalBetText += "⏰ Время на ставки: 30 сек\n"
		finalBetText += "\n❌ ВНИМАНИЕ: Кто уже ставил в начале игры - ставку сделать НЕЛЬЗЯ!"

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

		finalResultText += fmt.Sprintf("🏆🏆🏆 %s, ПОЗДРАВЛЯЕМ!! Вы выиграли плашку \"%s\"!\n", formatParticipantNameWithUsername(winner), currentPrize)

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

		return finalResultText
	} else {
		// Обычный раунд: выбираем случайного участника для удаления
		randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(participants))))
		loserIndex := int(randomIndex.Int64())
		removedParticipant := participants[loserIndex]

		// Удаляем участника из списка
		participants = append(participants[:loserIndex], participants[loserIndex+1:]...)

		roundText := fmt.Sprintf("☹️ К сожалению участник %s не получает плашку в этом туре!\n", formatParticipantName(removedParticipant))
		roundText += "@" + participantIDs[removedParticipant] + ", ничего страшного, повезет в следующей игре 😊🍀!\n"

		remaining := len(participants)
		if remaining > 1 {
			roundText += fmt.Sprintf("\nОсталось участников: %d", remaining)
		} else if remaining == 1 {
			roundText += "\n🏆 Остался последний участник!"
		}

		return roundText
	}
}

// Функция для управления сессией игры
func runGameSession(bot *tgbotapi.BotAPI) {
	log.Printf("runGameSession: Начало игры, totalRounds=%d, currentRound=%d, len(participants)=%d", totalRounds, currentRound, len(participants))

	// Цикл для всех раундов
	for isGameActive && currentRound <= totalRounds {
		log.Printf("runGameSession: НАЧАЛО РАУНДА %d (%d-й по порядку), isGameActive=%t, len(participants)=%d", currentRound, currentRound+1, isGameActive, len(participants))

		// Выполняем раунд
		roundResult := performGameRound(bot, currentRound)
		log.Printf("runGameSession: Раунд %d выполнен, isGameActive=%t, roundResult содержит 'ПОДГОТОВКА': %t", currentRound, isGameActive, strings.Contains(roundResult, "ПОДГОТОВКА"))

		// Если игра закончилась, показываем финальный результат
		if !isGameActive {
			log.Printf("Игра закончилась после раунда %d", currentRound)
			log.Printf("runGameSession: Отправляем финальное сообщение: %s", roundResult)
			editMsg := tgbotapi.NewEditMessageText(gameChatID, gameMessageID, roundResult)
			_, err := bot.Send(editMsg)
			if err != nil {
				log.Printf("runGameSession: Ошибка отправки финального сообщения: %v", err)
			} else {
				log.Printf("runGameSession: Финальное сообщение отправлено успешно")
			}
			gameInProgress = false
			log.Printf("runGameSession: gameInProgress установлен в false")
			return
		}

		// Показываем результат раунда
		if currentRound >= totalRounds {
			// Последний раунд - просто показываем результат
			editMsg := tgbotapi.NewEditMessageText(gameChatID, gameMessageID, roundResult)
			bot.Send(editMsg)
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

		// Ждём 5 секунд до следующего раунда
		time.Sleep(5 * time.Second)

		currentRound++
		log.Printf("runGameSession: Переходим к раунду %d", currentRound)

		// Небольшая пауза между раундами
		if isGameActive && len(participants) > 1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	log.Printf("runGameSession: Цикл завершен, isGameActive=%t, currentRound=%d, totalRounds=%d", isGameActive, currentRound, totalRounds)
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

	// Инициализируем переменные ставок
	bettingPhase = "closed"
	bettingParticipants = []string{}
	finalBettingNumbers = []int{}
	gameInProgress = false

	bot.Debug = true

	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Настраиваем обновления
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	// Обрабатываем обновления
	for update := range updates {
		if update.Message != nil { // Если это сообщение
			// Проверяем, является ли сообщение командой
			if update.Message.IsCommand() {
				// Проверяем доступ пользователя - теперь проверка идет внутри команд
				userName := update.Message.From.UserName

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

				switch update.Message.Command() {
				case "bet":
					log.Printf("🎯 Команда /bet от %s: isGameActive=%t, bettingPhase=%s, gameInProgress=%t", userName, isGameActive, bettingPhase, gameInProgress)
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
						msg.Text = "🚫 Укажите номер участника и сумму ставки! Пример: /bet 1 100"
						break
					}

					// Парсим аргументы
					parts := strings.Split(strings.TrimSpace(args), " ")
					if len(parts) != 2 {
						msg.Text = "🚫 Укажите номер участника и сумму ставки через пробел! Пример: /bet 1 100"
						break
					}

					// Парсим номер участника
					participantN, err := strconv.Atoi(strings.TrimSpace(parts[0]))
					if err != nil {
						msg.Text = "🚫 Неверный формат номера участника!"
						break
					}

					// Проверяем валидность номера в зависимости от фазы
					var participantName string
					if bettingPhase == "initial" {
						if participantN < 1 || participantN > len(bettingParticipants) {
							msg.Text = fmt.Sprintf("🚫 Неверный номер участника! Доступные номера: 1-%d", len(bettingParticipants))
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
					betAmount, err := strconv.Atoi(strings.TrimSpace(parts[1]))
					if err != nil || betAmount <= 0 {
						msg.Text = "🚫 Укажите корректную положительную сумму ставки!"
						break
					}

					// Проверяем, что пользователь еще не ставил НИКОГДА (ни в начальной, ни в финальной фазе)
					if _, alreadyBetInitial := initialBets[userName]; alreadyBetInitial {
						msg.Text = "🚫 Вы уже сделали ставку в начале игры! Ставку можно сделать только один раз за всю игру."
						break
					}
					if _, alreadyBetFinal := finalBets[userName]; alreadyBetFinal {
						msg.Text = "🚫 Вы уже сделали ставку в финальной фазе! Ставку можно сделать только один раз за всю игру."
						break
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
					log.Printf("🎮 Команда /game получена от %s, chatID=%d", userName, update.Message.Chat.ID)
					log.Printf("game: Состояние игры - isGameActive=%t, gameInProgress=%t, len(participants)=%d", isGameActive, gameInProgress, len(participants))
					// Временно убираем проверку администратора для тестирования
					// if userName != "hunnidstooblue" && userName != "iamnothiding" {
					//     log.Printf("Пользователь %s не является администратором", userName)
					//     msg.Text = "🚫 Только администраторы могут управлять игрой!"
					//     break
					// }

					// Проверяем, не запущена ли уже игра
					if isGameActive || gameInProgress {
						msg.Text = "🎮 Игра уже запущена!"
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
					gameInProgress = true

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
					gameText += "🏆 УЧАСТНИКИ:\n"
					for i, participant := range bettingParticipants {
						gameText += fmt.Sprintf("%d - %s\n", i+1, formatParticipantNameWithUsername(participant))
					}
					gameText += "\n💰 РАУНД СТАВОК!\n"
					gameText += "🎯 Ставьте на победителя: /bet N СУММА\n"
					gameText += "💎 Коэффициент: x10\n"
					gameText += "⏰ Время: 30 секунд\n"

					// Отправляем начальное сообщение со ставками
					gameChatID = update.Message.Chat.ID
					initialMsg := tgbotapi.NewMessage(gameChatID, gameText)
					sentMsg, err := bot.Send(initialMsg)
					if err != nil {
						log.Printf("Ошибка отправки начального сообщения: %v", err)
						msg.Text = "🚫 Ошибка запуска игры!"
						gameInProgress = false
						break
					}

					// Теперь устанавливаем флаги игры
					isGameActive = true      // Устанавливаем после успешной отправки сообщения
					bettingPhase = "initial" // Начинаем фазу ставок

					// Сохраняем ID сообщения для редактирования
					gameMessageID = sentMsg.MessageID
					totalRounds = len(participants) - 1
					log.Printf("Игра запущена: chatID=%d, messageID=%d, totalRounds=%d", gameChatID, gameMessageID, totalRounds)

					// Запускаем горутину для ожидания ставок и запуска игры
					go func() {
						log.Printf("Горутина ставок: ждем 30 секунд для начальных ставок")
						time.Sleep(30 * time.Second)

						// Закрываем фазу ставок
						bettingPhase = "closed"
						log.Printf("Горутина ставок: ставки закрыты, запускаем игру")

						// Создаем новое сообщение для игры
						gameMsg := tgbotapi.NewMessage(gameChatID, "🎮 ИГРА НАЧИНАЕТСЯ!\n⏰ До первого раунда: 3 сек")
						sentGameMsg, err := bot.Send(gameMsg)
						if err != nil {
							log.Printf("Горутина ставок: Ошибка отправки сообщения игры: %v", err)
							isGameActive = false
							gameInProgress = false
							return
						}

						// Сохраняем ID нового сообщения для редактирования результатов раундов
						gameMessageID = sentGameMsg.MessageID
						log.Printf("Горутина ставок: Создано новое сообщение для игры, messageID=%d", gameMessageID)

						time.Sleep(3 * time.Second)

						// Запускаем игру
						runGameSession(bot)
					}()

					// Отправляем подтверждение запуска
					msg.Text = "✅ Игра запущена! Делайте ставки в течение 30 секунд."
					break

				case "stopgame":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять игрой!"
						break
					}

					if !isGameActive {
						msg.Text = "🎮 Игра не запущена!"
						break
					}

					isGameActive = false
					msg.Text = "🛑 Игра остановлена администратором!"

				case "start":
					// Проверяем, является ли пользователь администратором
					if userName != "hunnidstooblue" && userName != "iamnothiding" {
						msg.Text = "🚫 Только администраторы могут управлять игрой!"
						break
					}
					msg.Text = fmt.Sprintf("привет долбоебы! сейчас будем решать кого удалить нахуй\nВсего участников: %d\n", len(participants))

				case "reset":
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
						msg.Text = fmt.Sprintf("🎁 Текущая плашка: \"%s\"\nУкажите новую плашку! Пример: /setprize %s", currentPrize, currentPrize)
					} else {
						oldPrize := currentPrize
						currentPrize = args
						msg.Text = fmt.Sprintf("🎁 Плашка изменена!\nБыло: \"%s\"\nСтало: \"%s\"", oldPrize, currentPrize)
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
							question = fmt.Sprintf("🏆 Кто получит плашку \"%s\"?", currentPrize)
						}

						// Создаем poll
						pollOptions := make([]string, len(participants))
						for i, participant := range participants {
							pollOptions[i] = formatParticipantNameWithUsername(participant)
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
					msg.Text = fmt.Sprintf("🎁 Текущая плашка для проигравшего: \"%s\"", currentPrize)

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
							msg.Text += fmt.Sprintf("\n%d. %s", i+1, formatParticipantNameWithUsername(participant))
						}
					}

				case "help":
					msg.Text = "ты совсем долбоеб? ты не знаешь команд???\n\n" +
						"🎮 ОСНОВНЫЕ КОМАНДЫ:\n" +
						"/reset - сбросить раунд\n" +
						"/game - начать автоматическую игру с таймером\n" +
						"/stopgame - остановить текущую игру\n" +
						"/list - список активных участников\n" +
						"/prize - показать плашку\n\n" +
						"💰 ЭКОНОМИКА:\n" +
						"/balance - посмотреть свой баланс\n" +
						"/bet (хэш сумма) - сделать ставку на участника\n\n" +
						"👑 АДМИНИСТРАТОРСКИЕ КОМАНДЫ:\n" +
						"/add (Имя Фамилия username) - добавить участника\n" +
						"/remove (Имя Фамилия) - удалить участника\n" +
						"/setprize (текст) - изменить плашку\n" +
						"/poll - голосование\n" +
						"/givefunds (@username сумма) - дать деньги игроку\n" +
						"/withdrawfunds (@username сумма) - снять деньги у игрока\n" +
						"/debug - отладочная информация\n" +
						"/promote (ID) - повысить до администратора\n\n" +
						"это все что тебе надо"
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
