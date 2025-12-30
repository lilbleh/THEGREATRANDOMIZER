package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"tg-random-bot/internal/config"
	"tg-random-bot/internal/models"

	"github.com/redis/go-redis/v9"
)

// RedisClient глобальный клиент Redis
var RedisClient *redis.Client

// InitRedis инициализирует подключение к Redis
func InitRedis(addr, password string, db int) error {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Println("Redis connected successfully")
	return nil
}

// SaveBalance сохраняет баланс игрока в Redis
func SaveBalance(username string, balance int) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("balance:%s", username)
	return RedisClient.Set(ctx, key, balance, 0).Err()
}

// GetBalance получает баланс игрока из Redis
func GetBalance(username string) (int, error) {
	if RedisClient == nil {
		return 0, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := fmt.Sprintf("balance:%s", username)
	balance, err := RedisClient.Get(ctx, key).Int()
	if err != nil {
		return 0, err // Возвращаем ошибку если баланс не найден
	}
	return balance, nil
}

// ChangeBalance изменяет баланс игрока и сохраняет в Redis
func ChangeBalance(username string, amount int) bool {
	if RedisClient == nil {
		return false
	}

	ctx := context.Background()
	key := fmt.Sprintf("balance:%s", username)

	// Получаем текущий баланс
	currentBalance, err := RedisClient.Get(ctx, key).Int()
	if err != nil {
		currentBalance = 0 // Если баланс не найден, считаем что он 0
	}

	// Вычисляем новый баланс
	newBalance := currentBalance + amount

	// Сохраняем новый баланс
	err = RedisClient.Set(ctx, key, newBalance, 0).Err()
	if err != nil {
		log.Printf("Error saving balance for %s: %v", username, err)
		return false
	}

	// Обновляем глобальную переменную (для обратной совместимости)
	playerBalances[username] = newBalance

	return true
}

// Глобальные переменные для обратной совместимости (пока что)
var playerBalances = make(map[string]int)

// GetAllBalances получает все балансы из Redis
func GetAllBalances() (map[string]int, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	keys, err := RedisClient.Keys(ctx, "balance:*").Result()
	if err != nil {
		return nil, err
	}

	balances := make(map[string]int)
	for _, key := range keys {
		username := key[8:] // Убираем префикс "balance:"
		balance, err := RedisClient.Get(ctx, key).Int()
		if err != nil {
			continue // Пропускаем если не можем прочитать
		}
		balances[username] = balance
		playerBalances[username] = balance // Обновляем глобальную переменную
	}

	return balances, nil
}

// SaveFines сохраняет штрафы в Redis
func SaveFines(fines map[string]int, fineDates map[string]time.Time) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Сохраняем штрафы
	finesData := make(map[string]interface{})
	for username, fine := range fines {
		finesData[username] = fine
	}

	finesKey := "fines"
	finesJSON, err := json.Marshal(finesData)
	if err != nil {
		return err
	}

	err = RedisClient.Set(ctx, finesKey, finesJSON, 0).Err()
	if err != nil {
		return err
	}

	// Сохраняем даты штрафов
	fineDatesData := make(map[string]interface{})
	for username, date := range fineDates {
		fineDatesData[username] = date.Format(time.RFC3339)
	}

	fineDatesKey := "fine_dates"
	fineDatesJSON, err := json.Marshal(fineDatesData)
	if err != nil {
		return err
	}

	return RedisClient.Set(ctx, fineDatesKey, fineDatesJSON, 0).Err()
}

// LoadFines загружает штрафы из Redis
func LoadFines() (map[string]int, map[string]time.Time, error) {
	if RedisClient == nil {
		return nil, nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	fines := make(map[string]int)
	fineDates := make(map[string]time.Time)

	// Загружаем штрафы
	finesKey := "fines"
	finesJSON, err := RedisClient.Get(ctx, finesKey).Result()
	if err == nil {
		var finesData map[string]int
		err = json.Unmarshal([]byte(finesJSON), &finesData)
		if err != nil {
			return nil, nil, err
		}
		fines = finesData
	}

	// Загружаем даты штрафов
	fineDatesKey := "fine_dates"
	fineDatesJSON, err := RedisClient.Get(ctx, fineDatesKey).Result()
	if err == nil {
		var fineDatesData map[string]string
		err = json.Unmarshal([]byte(fineDatesJSON), &fineDatesData)
		if err != nil {
			return nil, nil, err
		}

		for username, dateStr := range fineDatesData {
			date, err := time.Parse(time.RFC3339, dateStr)
			if err != nil {
				continue
			}
			fineDates[username] = date
		}
	}

	return fines, fineDates, nil
}

// AddItemToInventory добавляет предмет в инвентарь игрока
func AddItemToInventory(username, itemName string, cost int) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Генерируем хэш для предмета
	hash := generateItemHash(username, itemName)

	// Создаем предмет инвентаря
	item := models.InventoryItem{
		PrizeName: itemName,
		Rarity:    "shop", // Предметы из магазина имеют rarity "shop"
		Cost:      cost,
		Count:     1,
		Hash:      hash,
	}

	// Сохраняем предмет
	itemKey := fmt.Sprintf("inventory:%s:%s", username, hash)
	itemData, err := json.Marshal(item)
	if err != nil {
		return err
	}

	return RedisClient.Set(ctx, itemKey, itemData, 0).Err()
}

// GetPlayerInventory получает инвентарь игрока
func GetPlayerInventory(username string) ([]models.InventoryItem, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	pattern := fmt.Sprintf("inventory:%s:*", username)
	keys, err := RedisClient.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	var inventory []models.InventoryItem
	for _, key := range keys {
		itemData, err := RedisClient.Get(ctx, key).Result()
		if err != nil {
			continue
		}

		var item models.InventoryItem
		err = json.Unmarshal([]byte(itemData), &item)
		if err != nil {
			continue
		}

		inventory = append(inventory, item)
	}

	return inventory, nil
}

// UseItemFromInventory использует предмет из инвентаря (уменьшает количество или удаляет)
func UseItemFromInventory(username, itemName string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Находим предмет в инвентаре
	inventory, err := GetPlayerInventory(username)
	if err != nil {
		return err
	}

	for _, item := range inventory {
		if item.PrizeName == itemName && item.Count > 0 {
			// Уменьшаем количество
			item.Count--
			if item.Count <= 0 {
				// Удаляем предмет если количество 0
				itemKey := fmt.Sprintf("inventory:%s:%s", username, item.Hash)
				return RedisClient.Del(ctx, itemKey).Err()
			} else {
				// Обновляем предмет
				itemKey := fmt.Sprintf("inventory:%s:%s", username, item.Hash)
				itemData, err := json.Marshal(item)
				if err != nil {
					return err
				}
				return RedisClient.Set(ctx, itemKey, itemData, 0).Err()
			}
		}
	}

	return fmt.Errorf("предмет не найден в инвентаре")
}

// WearItem надевает плашку игроку
func WearItem(username, itemHash string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()

	// Проверяем, есть ли такой предмет у пользователя
	itemKey := fmt.Sprintf("inventory:%s:%s", username, itemHash)
	itemData, err := RedisClient.Get(ctx, itemKey).Result()
	if err != nil {
		return fmt.Errorf("предмет не найден в инвентаре")
	}

	// Парсим данные предмета
	var item models.InventoryItem
	err = json.Unmarshal([]byte(itemData), &item)
	if err != nil {
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
		return fmt.Errorf("ошибка сохранения")
	}

	err = RedisClient.Set(ctx, profileKey, data, 0).Err()
	if err != nil {
		return fmt.Errorf("ошибка сохранения профиля")
	}

	log.Printf("wearItem: Плашка %s успешно надета пользователем %s", item.PrizeName, username)
	return nil
}

// UnwearItem снимает плашку у игрока
func UnwearItem(username string) error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	profileKey := fmt.Sprintf("profile:%s:worn_item", username)

	// Проверяем, есть ли надетая плашка
	exists, err := RedisClient.Exists(ctx, profileKey).Result()
	if err != nil {
		return fmt.Errorf("ошибка проверки профиля")
	}

	if exists == 0 {
		return fmt.Errorf("нет надетой плашки")
	}

	// Удаляем информацию о надетой плашке
	err = RedisClient.Del(ctx, profileKey).Err()
	if err != nil {
		return fmt.Errorf("ошибка снятия плашки")
	}

	log.Printf("unwearItem: Плашка успешно снята у пользователя %s", username)
	return nil
}

// GetWornItem получает информацию о надетой плашке
func GetWornItem(username string) (map[string]string, error) {
	if RedisClient == nil {
		return nil, fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	profileKey := fmt.Sprintf("profile:%s:worn_item", username)

	data, err := RedisClient.Get(ctx, profileKey).Result()
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

// LoadPrizesFromRedis загружает призы из Redis
func LoadPrizesFromRedis() error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := "prizes"

	data, err := RedisClient.Get(ctx, key).Result()
	if err != nil {
		return err
	}

	var prizeConfig config.PrizeConfig
	err = json.Unmarshal([]byte(data), &prizeConfig)
	if err != nil {
		return err
	}

	config.Prizes = prizeConfig.Prizes
	return nil
}

// SavePrizesToRedis сохраняет призы в Redis
func SavePrizesToRedis() error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	key := "prizes"

	prizeConfig := config.PrizeConfig{Prizes: config.Prizes}
	data, err := json.Marshal(prizeConfig)
	if err != nil {
		return err
	}

	return RedisClient.Set(ctx, key, data, 0).Err()
}

// RemoveAllPrizesFromRedis удаляет все призы из Redis
func RemoveAllPrizesFromRedis() error {
	if RedisClient == nil {
		return fmt.Errorf("Redis client not available")
	}

	ctx := context.Background()
	return RedisClient.Del(ctx, "prizes").Err()
}

// generateItemHash генерирует уникальный хэш для предмета инвентаря
func generateItemHash(username, prizeName string) string {
	data := fmt.Sprintf("%s:%s:%d", username, prizeName, time.Now().UnixNano())
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)[:6] // Берем первые 6 символов для короткого хэша
}
