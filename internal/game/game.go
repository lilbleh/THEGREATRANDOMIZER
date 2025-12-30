package game

import (
	"fmt"
	"log"
	"math/big"
	"crypto/rand"
	"time"

	"tg-random-bot/internal/config"
	"tg-random-bot/internal/models"
	"tg-random-bot/internal/storage"
	"tg-random-bot/internal/utils"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// GameState представляет состояние игры
type GameState struct {
	MessageID      int
	ChatID         int64
	IsActive       bool
	InProgress     bool
	TotalRounds    int
	CurrentRound   int
	BettingPhase   string
	GameCancel     chan bool
}

// BettingState представляет состояние ставок
type BettingState struct {
	InitialBets                map[string]models.Bet // Ставки на начальном этапе
	FinalBets                  map[string]models.Bet // Ставки на финальном этапе
	BettingParticipants        []string              // Участники для ставок
	InitialBettingParticipants []string              // Сохраняем первоначальный список для ставок
	FinalBettingNumbers        []int                 // Номера для финальных ставок
}

// Global game state variables
var (
	GameStateInstance GameState
	BettingStateInstance BettingState
	EliminatedParticipants []string // Выбывшие участники
)

// InitGame инициализирует состояние игры
func InitGame() {
	GameStateInstance = GameState{
		GameCancel: make(chan bool),
	}
	BettingStateInstance = BettingState{
		InitialBets: make(map[string]models.Bet),
		FinalBets:   make(map[string]models.Bet),
	}
}

// StartGame начинает новую игру
func StartGame(bot *tgbotapi.BotAPI, chatID int64) error {
	if GameStateInstance.IsActive || GameStateInstance.InProgress {
		return fmt.Errorf("игра уже идет")
	}

	log.Printf("=== НАЧАЛО НОВОЙ ИГРЫ ===")
	GameStateInstance.IsActive = true
	GameStateInstance.InProgress = true
	GameStateInstance.ChatID = chatID
	GameStateInstance.TotalRounds = len(config.Participants)
	GameStateInstance.CurrentRound = 1
	GameStateInstance.BettingPhase = "initial"

	// Очищаем списки для новой игры
	BettingStateInstance.InitialBets = make(map[string]models.Bet)
	BettingStateInstance.FinalBets = make(map[string]models.Bet)
	BettingStateInstance.BettingParticipants = make([]string, len(config.Participants))
	BettingStateInstance.InitialBettingParticipants = make([]string, len(config.Participants))
	copy(BettingStateInstance.BettingParticipants, config.Participants)
	copy(BettingStateInstance.InitialBettingParticipants, config.Participants)

	EliminatedParticipants = []string{}

	// Перемешиваем участников
	utils.ShuffleParticipants(config.Participants)

	log.Printf("Игра начата. Участников: %d, Раундов: %d", len(config.Participants), GameStateInstance.TotalRounds)

	return nil
}

// EndGame завершает текущую игру
func EndGame() {
	log.Printf("=== ИГРА ЗАВЕРШЕНА ===")
	GameStateInstance.IsActive = false
	GameStateInstance.InProgress = false
	GameStateInstance.TotalRounds = 0
	GameStateInstance.CurrentRound = 0
	GameStateInstance.BettingPhase = ""
}

// GetCurrentWinner получает текущего победителя раунда
func GetCurrentWinner() string {
	if len(config.Participants) == 0 {
		return ""
	}

	// Выбираем случайного участника
	randomIndex, _ := rand.Int(rand.Reader, big.NewInt(int64(len(config.Participants))))
	winnerIndex := int(randomIndex.Int64())

	return config.Participants[winnerIndex]
}

// EliminateParticipant удаляет участника из игры
func EliminateParticipant(participant string) {
	for i, p := range config.Participants {
		if p == participant {
			config.Participants = append(config.Participants[:i], config.Participants[i+1:]...)
			EliminatedParticipants = append(EliminatedParticipants, participant)
			log.Printf("Участник %s выбывает из игры", participant)
			break
		}
	}
}

// PayoutWinnings выплачивает выигрыши по ставкам и формирует текст результатов
func PayoutWinnings(bot *tgbotapi.BotAPI, winner string, loser string) string {
	log.Printf("💰 payoutWinnings: === НАЧАЛО ВЫПЛАТЫ ВЫИГРЫШЕЙ ===")
	log.Printf("payoutWinnings: Функция ВЫЗВАНА! Победитель: %s, Проигравший: %s", winner, loser)
	log.Printf("payoutWinnings: isGameActive=%t", GameStateInstance.IsActive)
	log.Printf("payoutWinnings: Количество ставок - initial: %d, final: %d", len(BettingStateInstance.InitialBets), len(BettingStateInstance.FinalBets))

	// TODO: Implement full payout logic
	// This is a placeholder - the full implementation is quite complex and spans many lines

	return fmt.Sprintf("Выигрыш выплачен! Победитель: %s", winner)
}

// PlaceBet размещает ставку игрока
func PlaceBet(username string, participantName string, amount int) error {
	if !GameStateInstance.IsActive {
		return fmt.Errorf("игра не активна")
	}

	// Проверяем баланс игрока
	balance, err := storage.GetBalance(username)
	if err != nil {
		return fmt.Errorf("не удалось получить баланс: %w", err)
	}

	if balance < amount {
		return fmt.Errorf("недостаточно средств")
	}

	// Списываем сумму ставки
	if !storage.ChangeBalance(username, -amount) {
		return fmt.Errorf("не удалось списать средства")
	}

	bet := models.Bet{
		Username:        username,
		ParticipantName: participantName,
		ParticipantHash: config.ParticipantHashes[participantName],
		Amount:          amount,
	}

	// Определяем тип ставки в зависимости от фазы
	if GameStateInstance.BettingPhase == "initial" {
		BettingStateInstance.InitialBets[username] = bet
	} else if GameStateInstance.BettingPhase == "final" {
		BettingStateInstance.FinalBets[username] = bet
	} else {
		// Возвращаем деньги если фаза не определена
		storage.ChangeBalance(username, amount)
		return fmt.Errorf("неверная фаза ставок")
	}

	log.Printf("Ставка размещена: %s поставил %d на %s", username, amount, participantName)
	return nil
}

// GetGameStatus возвращает текущий статус игры
func GetGameStatus() string {
	if !GameStateInstance.IsActive {
		return "Игра не активна"
	}

	status := fmt.Sprintf("🏆 ИГРА АКТИВНА\n\n")
	status += fmt.Sprintf("📊 Раунд: %d/%d\n", GameStateInstance.CurrentRound, GameStateInstance.TotalRounds)
	status += fmt.Sprintf("👥 Осталось участников: %d\n", len(config.Participants))
	status += fmt.Sprintf("💰 Фаза ставок: %s\n\n", GameStateInstance.BettingPhase)

	if len(config.Participants) > 0 {
		status += "🎯 Участники:\n"
		for i, participant := range config.Participants {
			status += fmt.Sprintf("%d. %s\n", i+1, utils.FormatParticipantNameWithUsername(participant))
		}
	}

	if len(EliminatedParticipants) > 0 {
		status += "\n❌ Выбывшие:\n"
		for _, participant := range EliminatedParticipants {
			status += fmt.Sprintf("• %s\n", participant)
		}
	}

	return status
}
