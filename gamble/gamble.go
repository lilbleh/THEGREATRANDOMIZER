// Package gamble предоставляет функции для работы с вероятностями и случайными выборами
package gamble

import (
	"crypto/rand"
	"math/big"
)

// Rarity представляет редкость предмета
type Rarity string

const (
	Common    Rarity = "common"
	Rare      Rarity = "rare"
	Legendary Rarity = "legendary"
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
		return Common
	}

	randomNum := int(randomBig.Int64())

	// Определяем редкость по диапазонам
	switch {
	case randomNum <= 59:
		return Common
	case randomNum <= 84:
		return Rare
	default:
		return Legendary
	}
}

// GenerateRandomNumber генерирует случайное число от 0 до 100
func GenerateRandomNumber() int {
	max := big.NewInt(101)
	randomBig, err := rand.Int(rand.Reader, max)
	if err != nil {
		return 0 // fallback
	}
	return int(randomBig.Int64())
}

// CoinResult представляет результат броска монеты
type CoinResult string

const (
	Heads CoinResult = "1" // орел, 49% шанс
	Tails CoinResult = "2" // решка, 49% шанс
	Edge  CoinResult = "3" // ребро, 2% шанс
)

// TossCoin бросает монету с заданными вероятностями:
// - 1 (орел): 49% (0-48)
// - 2 (решка): 49% (49-97)
// - 3 (ребро): 2% (98-99)
func TossCoin() CoinResult {
	// Генерируем число от 0 до 99 (100 вариантов)
	max := big.NewInt(100)
	randomBig, err := rand.Int(rand.Reader, max)
	if err != nil {
		return Tails // fallback
	}

	randomNum := int(randomBig.Int64())

	switch {
	case randomNum <= 48: // 49% (0-48)
		return Heads
	case randomNum <= 97: // 49% (49-97)
		return Tails
	default: // 2% (98-99)
		return Edge
	}
}

// GetCoinMultiplier возвращает коэффициент выплаты для результата монеты
func GetCoinMultiplier(result CoinResult) int {
	switch result {
	case Tails, Heads:
		return 2
	case Edge:
		return 100
	default:
		return 1
	}
}
