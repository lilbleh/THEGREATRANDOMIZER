package gamble

import (
	"fmt"
	"testing"
)

func TestGenerateRandomRarity(t *testing.T) {
	// Тестируем генерацию редкостей
	commonCount := 0
	rareCount := 0
	legendaryCount := 0

	// Генерируем 1000 значений для тестирования вероятностей
	for i := 0; i < 1000; i++ {
		rarity := GenerateRandomRarity()
		switch rarity {
		case Common:
			commonCount++
		case Rare:
			rareCount++
		case Legendary:
			legendaryCount++
		}
	}

	fmt.Printf("Результаты тестирования (1000 генераций):\n")
	fmt.Printf("Common: %d (~80%%)\n", commonCount)
	fmt.Printf("Rare: %d (~15%%)\n", rareCount)
	fmt.Printf("Legendary: %d (~6%%)\n", legendaryCount)

	// Проверяем что все значения сгенерированы
	if commonCount == 0 || rareCount == 0 || legendaryCount == 0 {
		t.Error("Не все редкости были сгенерированы")
	}
}

func ExampleGenerateRandomRarity() {
	rarity := GenerateRandomRarity()
	fmt.Printf("Выпала редкость: %s\n", rarity)
}

func ExampleGenerateRandomNumber() {
	num := GenerateRandomNumber()
	fmt.Printf("Случайное число: %d\n", num)
}
