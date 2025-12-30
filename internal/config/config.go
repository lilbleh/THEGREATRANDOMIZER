package config

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// Глобальные списки участников (инициализируется при запуске)
var Participants []string

// InitParticipants инициализирует список участников и их хэши
func InitParticipants() {
	Participants = make([]string, 0, len(ParticipantIDs))
	for name := range ParticipantIDs {
		Participants = append(Participants, name)
		hashParticipant(name) // Инициализируем хэши
	}
	// Перемешиваем участников
	shuffleParticipants()
}

// hashParticipant генерирует SHA-256 хэш участника
func hashParticipant(name string) string {
	hash := sha256.Sum256([]byte(name))
	hashed := fmt.Sprintf("%x", hash)
	ParticipantHashes[name] = hashed
	return hashed
}

// shuffleParticipants перемешивает слайс участников
func shuffleParticipants() {
	// Simple shuffle implementation
	for i := len(Participants) - 1; i > 0; i-- {
		j := (int(time.Now().UnixNano()) + i) % (i + 1)
		Participants[i], Participants[j] = Participants[j], Participants[i]
	}
}

// Глобальные переменные для плашек
var Prizes []Prize
var CurrentPrize Prize

// Структура для приза (перемещена из models для избежания циклических зависимостей)
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

// Map для хранения username/ID участников (ключ: имя, значение: user ID)
// ТЕСТОВЫЙ СПИСОК ИЗ 5 УЧАСТНИКОВ
var ParticipantIDs = map[string]string{
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
var ParticipantHashes = make(map[string]string)
