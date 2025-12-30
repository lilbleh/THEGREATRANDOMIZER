package models

import (
	"time"

	"tg-random-bot/gamble"
)

// Bet представляет ставку игрока
type Bet struct {
	Username        string
	ParticipantName string // Имя участника
	ParticipantHash string // SHA-256 хэш участника
	Amount          int
}

// Note: Prize and PrizeConfig moved to internal/config package

// InventoryItem представляет элемент инвентаря
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
	InitialBets                map[string]Bet // Ставки на начальном этапе
	FinalBets                  map[string]Bet // Ставки на финальном этапе
	BettingParticipants        []string       // Участники для ставок
	InitialBettingParticipants []string       // Сохраняем первоначальный список для ставок
	FinalBettingNumbers        []int          // Номера для финальных ставок
}

// PlayerData представляет данные игрока
type PlayerData struct {
	Balance         int
	Bank            int
	Fine            int
	FineDate        time.Time
	Inventory       []InventoryItem
	WornItem        map[string]string
}

// ConvertToGambleRarity конвертирует локальную редкость в редкость из пакета gamble
func (r Rarity) ToGambleRarity() gamble.Rarity {
	switch r {
	case RareRarity:
		return gamble.Rare
	case LegendaryRarity:
		return gamble.Legendary
	default:
		return gamble.Common
	}
}

// FromGambleRarity конвертирует редкость из пакета gamble в локальную
func FromGambleRarity(r gamble.Rarity) Rarity {
	switch r {
	case gamble.Rare:
		return RareRarity
	case gamble.Legendary:
		return LegendaryRarity
	default:
		return CommonRarity
	}
}
