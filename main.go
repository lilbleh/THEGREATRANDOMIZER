package main

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Глобальный список участников (инициализируется при запуске)
var participants []string

// Текущая "плашка" для проигравшего
var currentPrize = "ЧМО"

// Map для хранения username/ID участников (ключ: имя, значение: user ID)
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

// Функция для перемешивания слайса
func shuffleParticipants() {
	rand.Shuffle(len(participants), func(i, j int) {
		participants[i], participants[j] = participants[j], participants[i]
	})
}

func main() {
	// Получаем токен бота из переменной окружения
	token := "8278983491:AAHxFOFBxndgwq2T_zpWBuNZTV9KG70LlLU"

	// Создаем бота
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	// Устанавливаем время для генерации случайных чисел
	rand.Seed(time.Now().UnixNano())

	// Инициализируем список участников из основного списка
	participants = make([]string, 0, len(participantIDs))
	for name := range participantIDs {
		participants = append(participants, name)
	}

	// Перемешиваем список участников при запуске
	shuffleParticipants()

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
				// Проверяем доступ пользователя
				userName := update.Message.From.UserName
				if userName != "hunnidstooblue" && userName != "iamnothiding" {
					msg := tgbotapi.NewMessage(update.Message.Chat.ID, "🚫 Доступ запрещен! Только избранные могут использовать этого бота.")
					if _, err := bot.Send(msg); err != nil {
						log.Panic(err)
					}
					continue // Пропускаем обработку команды
				}

				msg := tgbotapi.NewMessage(update.Message.Chat.ID, "")

				switch update.Message.Command() {
				case "random":
					if len(participants) == 0 {
						msg.Text = "Игра уже окончена!"
					} else if len(participants) == 2 {
						// Финальный раунд: случайный выбор победителя
						winnerIndex := rand.Intn(2)
						winner := participants[winnerIndex]
						loser := participants[1-winnerIndex]

						winnerUsername := participantIDs[winner]
						loserUsername := participantIDs[loser]

						finalText := fmt.Sprintf("☹️ К сожалению! %s не получает плашку в финале!\n", loser)
						if loserUsername != "" {
							finalText += fmt.Sprintf("@%s ничего страшного, повезет в следующей игре 🍀!\n\n", loserUsername)
						}

						finalText += fmt.Sprintf("🏆🏆🏆 %s, ПОЗДРАВЛЯЕМ!! Вы выиграли плашку \"%s\"!\n", winner, currentPrize)
						if winnerUsername != "" {
							finalText += fmt.Sprintf("@%s", winnerUsername)
						}

						finalText += "\n\n🐩 Игра окончена!"
						participants = []string{} // Полностью очищаем список
						msg.Text = finalText
					} else {
						// Обычный раунд: выбираем случайного участника для удаления
						loserIndex := rand.Intn(len(participants))
						removedParticipant := participants[loserIndex]

						// Удаляем участника из списка
						participants = append(participants[:loserIndex], participants[loserIndex+1:]...)

						loserUsername := participantIDs[removedParticipant]

						roundText := fmt.Sprintf("☹️ К сожалению участник %s не получает плашку в этом туре!\n", removedParticipant)
						if loserUsername != "" {
							roundText += fmt.Sprintf("@%s ничего страшного, повезет в следующей игре 😊🍀!\n", loserUsername)
						}
						roundText += fmt.Sprintf("\n✅ Удалено из списка. Осталось участников: %d", len(participants))

						msg.Text = roundText
					}

				case "start":
					msg.Text = fmt.Sprintf("привет долбоебы! сейчас будем решать кого удалить нахуй\nВсего участников: %d\n", len(participants))

				case "reset":
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
								// Также добавляем в текущий активный список, если он не пустой
								if len(participants) > 0 {
									participants = append(participants, fullName)
								}
								msg.Text = fmt.Sprintf("✅ Участник %s (@%s) добавлен в основной список!\nТеперь в списке %d участников.", fullName, username, len(participantIDs))
							}
						}
					}

				case "remove":
					// Получаем аргументы команды
					args := update.Message.CommandArguments()
					if args == "" {
						msg.Text = "🚫 Укажите имя участника! Пример: /remove Арсений Квятковский"
					} else {
						participantName := strings.TrimSpace(args)

						// Удаляем из основного списка participantIDs
						if _, exists := participantIDs[participantName]; exists {
							delete(participantIDs, participantName)

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
						poll := tgbotapi.SendPollConfig{
							BaseChat: tgbotapi.BaseChat{
								ChatID: update.Message.Chat.ID,
							},
							Question:    question,
							Options:     participants,
							IsAnonymous: false, // Не анонимный poll
						}

						if _, err := bot.Send(poll); err != nil {
							msg.Text = "🚫 Ошибка создания poll: " + err.Error()
						} else {
							msg.Text = "📊 Poll создан! Голосование активно."
						}
					}

				case "prize":
					msg.Text = fmt.Sprintf("🎁 Текущая плашка для проигравшего: \"%s\"", currentPrize)

				case "debug":
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
							username := participantIDs[participant]
							if username != "" {
								msg.Text += fmt.Sprintf("\n%d. %s (@%s)", i+1, participant, username)
							} else {
								msg.Text += fmt.Sprintf("\n%d. %s", i+1, participant)
							}
						}
					}

				case "help":
					msg.Text = "ты совсем долбоеб? ты не знаешь команд???\n" +
						"/reset - сбросить раунд\n" +
						"/random - следующий раунд\n" +
						"/add (Имя Фамилия username) - добавить участника в основной список\n" +
						"/remove (Имя Фамилия) - удалить участника из основного списка\n" +
						"/list - список активных участников\n" +
						"/setprize (текст) - изменить плашку\n" +
						"/prize - показать плашку\n" +
						"/poll - голосование\n" +
						"/debug - отладочная информация\n" +
						"это все что тебе надо"

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
