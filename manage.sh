#!/bin/bash

# Скрипт для управления Docker Compose проектом

set -e

COMPOSE_FILE="docker-compose.yml"

case "$1" in
    "start"|"up")
        echo "🚀 Запуск сервисов..."
        docker-compose -f "$COMPOSE_FILE" up -d
        echo "✅ Сервисы запущены!"
        echo ""
        echo "📊 Статус сервисов:"
        docker-compose -f "$COMPOSE_FILE" ps
        ;;

    "stop"|"down")
        echo "🛑 Остановка сервисов..."
        docker-compose -f "$COMPOSE_FILE" down
        echo "✅ Сервисы остановлены!"
        ;;

    "restart")
        echo "🔄 Перезапуск сервисов..."
        docker-compose -f "$COMPOSE_FILE" restart
        echo "✅ Сервисы перезапущены!"
        ;;

    "logs")
        if [ -n "$2" ]; then
            docker-compose -f "$COMPOSE_FILE" logs -f "$2"
        else
            docker-compose -f "$COMPOSE_FILE" logs -f
        fi
        ;;

    "status"|"ps")
        echo "📊 Статус сервисов:"
        docker-compose -f "$COMPOSE_FILE" ps
        ;;

    "build")
        echo "🔨 Сборка образов..."
        docker-compose -f "$COMPOSE_FILE" build --no-cache
        echo "✅ Образы собраны!"
        ;;

    "clean")
        echo "🧹 Очистка (остановка и удаление volumes)..."
        docker-compose -f "$COMPOSE_FILE" down -v
        echo "✅ Сервисы остановлены, volumes удалены!"
        ;;

    "redis-cli")
        echo "🔧 Подключение к Redis CLI..."
        docker-compose -f "$COMPOSE_FILE" exec redis redis-cli
        ;;

    *)
        echo "📋 Использование: $0 {command}"
        echo ""
        echo "Команды:"
        echo "  start|up     - Запустить сервисы"
        echo "  stop|down    - Остановить сервисы"
        echo "  restart      - Перезапустить сервисы"
        echo "  logs [service] - Просмотреть логи (всех или конкретного сервиса)"
        echo "  status|ps    - Показать статус сервисов"
        echo "  build        - Пересобрать образы"
        echo "  clean        - Остановить сервисы и удалить данные"
        echo "  redis-cli    - Подключиться к Redis CLI"
        echo ""
        echo "Примеры:"
        echo "  $0 start          # Запустить все сервисы"
        echo "  $0 logs app       # Просмотреть логи приложения"
        echo "  $0 redis-cli      # Подключиться к Redis"
        exit 1
        ;;
esac
