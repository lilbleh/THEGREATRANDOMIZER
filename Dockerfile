# Используем официальный образ Go для сборки
FROM golang:1.24-alpine AS builder

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем go.mod и go.sum для загрузки зависимостей
COPY go.mod go.sum ./

# Загружаем зависимости
RUN go mod download

# Копируем исходный код
COPY main.go ./

# Собираем приложение
RUN CGO_ENABLED=0 GOOS=linux go build -o tg-random-bot main.go

# Финальный образ на основе Alpine Linux
FROM alpine:latest

# Устанавливаем ca-certificates для HTTPS запросов
RUN apk --no-cache add ca-certificates

# Создаем пользователя для безопасности
RUN adduser -D -s /bin/sh appuser

# Устанавливаем рабочую директорию
WORKDIR /app

# Копируем бинарный файл из builder образа
COPY --from=builder /app/tg-random-bot .

# Меняем владельца файла на appuser
RUN chown appuser:appuser tg-random-bot

# Переключаемся на пользователя appuser
USER appuser

# Запускаем приложение
CMD ["./tg-random-bot"]
