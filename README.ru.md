# pgx-wrapper

Легкая обертка над `pgx/v5` для работы с PostgreSQL:
- единый объект запроса (`Query`)
- логирование SQL-запросов
- маршрутизация запросов в транзакцию через `context.Context`

Основной README на английском: [README.md](./README.md)

## Что делает пакет

`Postgres` оборачивает `pgxpool.Pool` и дает небольшой API:
- `Exec`, `Query`, `QueryRow` для SQL
- `RunInTx` для транзакций
- `Close`, `Ping`, `SqlDB` для управления подключением

Главная идея: если вы вызвали `RunInTx`, то все вызовы БД внутри callback с тем же `context.Context` автоматически выполняются в одной транзакции.

## Быстрый старт

```go
ctx := context.Background()
log := slog.New(slog.NewTextHandler(os.Stdout, nil))

db, err := storage.New(ctx, log, &storage.Config{
	Dsn:             "postgres://user:pass@localhost:5432/app?sslmode=disable",
	ConnectTimeout:  5 * time.Second,
	MaxConnIdleTime: 5 * time.Minute,
})
if err != nil {
	panic(err)
}
defer db.Close(ctx)
```

## Объект Query

```go
q := storage.Query{
	Name:      "insert_user",
	RawQuery:  "insert into users(email, age) values ($1, $2)",
	Arguments: []any{"john@example.com", 30},
}
_, err := db.Exec(ctx, q)
```

- `Name` удобно использовать в логах
- `RawQuery` хранит SQL с placeholders
- `Arguments` содержит значения

## Транзакции (подробно)

`RunInTx`:
- стартует транзакцию
- кладет ее в контекст
- делает `commit`, если callback вернул `nil`
- делает `rollback`, если callback вернул ошибку
- делает `rollback` и пробрасывает `panic` дальше, если внутри callback произошел panic

Сигнатура:

```go
RunInTx(ctx context.Context, txOptions pgx.TxOptions, fn func(context.Context) error) error
```

### 1) Успешный сценарий

```go
err := db.RunInTx(ctx, pgx.TxOptions{}, func(txCtx context.Context) error {
	_, err := db.Exec(txCtx, storage.Query{
		Name:      "debit_balance",
		RawQuery:  "update accounts set balance = balance - $1 where id = $2",
		Arguments: []any{100, fromID},
	})
	if err != nil {
		return err
	}

	_, err = db.Exec(txCtx, storage.Query{
		Name:      "credit_balance",
		RawQuery:  "update accounts set balance = balance + $1 where id = $2",
		Arguments: []any{100, toID},
	})
	return err
})
```

Обе операции фиксируются только вместе.

### 2) Rollback при любой ошибке

```go
err := db.RunInTx(ctx, pgx.TxOptions{}, func(txCtx context.Context) error {
	_, err := db.Exec(txCtx, storage.Query{
		Name:      "reserve_item",
		RawQuery:  "update items set stock = stock - 1 where id = $1 and stock > 0",
		Arguments: []any{itemID},
	})
	if err != nil {
		return err
	}

	if !canCreateOrder {
		return errors.New("order is not allowed")
	}

	_, err = db.Exec(txCtx, storage.Query{
		Name:      "create_order",
		RawQuery:  "insert into orders(user_id, item_id) values ($1, $2)",
		Arguments: []any{userID, itemID},
	})
	return err
})
```

Если callback вернул ошибку, все изменения в рамках этой транзакции откатываются.

Критично: во все вызовы репозиториев передавайте именно `txCtx`.  
Если передать другой контекст, запрос может уйти мимо транзакции.

### 3) Настройка уровня изоляции

```go
err := db.RunInTx(ctx, pgx.TxOptions{
	IsoLevel:   pgx.Serializable,
	AccessMode: pgx.ReadWrite,
}, func(txCtx context.Context) error {
	// SQL операции
	return nil
})
```

## Получение PostgreSQL кода ошибки

```go
if code := storage.ErrorCode(err); code == "23505" {
	// unique_violation
}
```
