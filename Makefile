up:
	docker compose up -d

down:
	docker compose down
	-pkill -f tg-random-bot

restart:
	docker compose down
	-pkill -f tg-random-bot
	docker compose up -d

rebuild:
	docker compose build --no-cache
	docker compose up -d

stop-local:
	pkill -f tg-random-bot