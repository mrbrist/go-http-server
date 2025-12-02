include .env

psql:
	psql $(DB_URL)

gooseup:
	goose postgres $(DB_URL) -dir "sql/schema" up

goosedown:
	goose postgres $(DB_URL) -dir "sql/schema" down