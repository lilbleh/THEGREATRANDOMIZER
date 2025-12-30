module tg-random-bot

go 1.24.5

require (
	github.com/go-telegram-bot-api/telegram-bot-api/v5 v5.5.1
	github.com/redis/go-redis/v9 v9.7.0
	tg-random-bot/gamble v0.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
)

replace tg-random-bot/gamble => ./gamble
