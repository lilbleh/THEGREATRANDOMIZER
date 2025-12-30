package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	math_rand "math/rand"

	"github.com/redis/go-redis/v9"
)

type Rarity string

const (
	CommonRarity    Rarity = "common"
	RareRarity      Rarity = "rare"
	LegendaryRarity Rarity = "legendary"
)

type Prize struct {
	Name   string `json:"name"`
	Rarity string `json:"rarity"`
	Cost   int    `json:"cost"`
	ID     string `json:"id,omitempty"`
}

var redisClient *redis.Client

func initRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
}

func GenerateRandomRarity() Rarity {
	// Генерируем криптографически безопасное случайное число от 0 до 100
	max := big.NewInt(101) // 0-100 включительно
	randomBig, err := rand.Int(rand.Reader, max)
	if err != nil {
		// В случае ошибки возвращаем common как fallback
		log.Printf("GenerateRandomRarity: error generating random number: %v, returning Common", err)
		return CommonRarity
	}

	randomNum := int(randomBig.Int64())
	log.Printf("GenerateRandomRarity: generated random number: %d", randomNum)

	// Определяем редкость по диапазонам
	var rarity Rarity
	switch {
	case randomNum <= 79:
		rarity = CommonRarity
	case randomNum <= 94:
		rarity = RareRarity
	default:
		rarity = LegendaryRarity
	}

	log.Printf("GenerateRandomRarity: selected rarity: %s", string(rarity))
	return rarity
}

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

func selectRandomPrizeByRarity(rarity Rarity) (Prize, error) {
	// Загружаем все призы из Redis
	prizes, err := loadAllPrizesFromRedis()
	if err != nil {
		log.Printf("selectRandomPrizeByRarity: failed to load prizes: %v", err)
		return Prize{}, fmt.Errorf("failed to load prizes: %v", err)
	}

	log.Printf("selectRandomPrizeByRarity: loaded %d prizes from Redis", len(prizes))
	log.Printf("selectRandomPrizeByRarity: requested rarity: %s", string(rarity))

	// Фильтруем призы по редкости
	var filteredPrizes []Prize
	for _, prize := range prizes {
		log.Printf("selectRandomPrizeByRarity: checking prize %s with rarity %s", prize.Name, prize.Rarity)
		if prize.Rarity == string(rarity) {
			filteredPrizes = append(filteredPrizes, prize)
		}
	}

	log.Printf("selectRandomPrizeByRarity: found %d prizes for rarity %s", len(filteredPrizes), string(rarity))

	if len(filteredPrizes) == 0 {
		log.Printf("selectRandomPrizeByRarity: no prizes found for rarity %s", rarity)
		return Prize{}, fmt.Errorf("no prizes found for rarity %s", rarity)
	}

	// Выбираем случайный приз из отфильтрованных
	randomIndex := math_rand.Intn(len(filteredPrizes))
	selectedPrize := filteredPrizes[randomIndex]
	log.Printf("selectRandomPrizeByRarity: selected prize: %s (rarity: %s)", selectedPrize.Name, selectedPrize.Rarity)

	return selectedPrize, nil
}

func main() {
	initRedis()

	// Тестируем 10 раз
	for i := 0; i < 10; i++ {
		fmt.Printf("\n=== Тест %d ===\n", i+1)
		rarity := GenerateRandomRarity()
		prize, err := selectRandomPrizeByRarity(rarity)
		if err != nil {
			fmt.Printf("Ошибка: %v\n", err)
		} else {
			fmt.Printf("Редкость: %s, Плашка: %s\n", rarity, prize.Name)
		}
	}
}
