# pgx-wrapper

Thin PostgreSQL storage wrapper on top of `pgx/v5` with:
- typed query object (`Query`)
- automatic query logging
- context-based transaction routing (`RunInTx`)

[Russian](./README.ru.md)

## What it does

`Postgres` wraps a `pgxpool.Pool` and exposes a small API:
- `Exec`, `Query`, `QueryRow` for SQL operations
- `RunInTx` for transactional execution
- `Close`, `Ping`, `SqlDB` for lifecycle/integration

The key behavior: if a transaction is started with `RunInTx`, all DB calls inside the callback that use the same `context.Context` are executed in that transaction automatically.

## Quick start

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

## Query object

```go
q := storage.Query{
	Name:      "insert_user",
	RawQuery:  "insert into users(email, age) values ($1, $2)",
	Arguments: []any{"john@example.com", 30},
}
_, err := db.Exec(ctx, q)
```

- `Name` is useful in logs
- `RawQuery` stores SQL with placeholders
- `Arguments` contains values

## Transactions (detailed)

`RunInTx`:
- starts a transaction
- puts it into context
- commits if callback returns `nil`
- rolls back if callback returns an error
- rolls back and re-panics if callback panics

Signature:

```go
RunInTx(ctx context.Context, txOptions pgx.TxOptions, fn func(context.Context) error) error
```

### 1) Successful transaction

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

Both operations are committed only together.

### 2) Rollback on any error

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

If callback returns an error, all changes in this transaction are rolled back.

Important: pass `txCtx` to every repository call.  
If you pass a different context, the query may run outside the transaction.

### 3) Isolation level options

```go
err := db.RunInTx(ctx, pgx.TxOptions{
	IsoLevel:   pgx.Serializable,
	AccessMode: pgx.ReadWrite,
}, func(txCtx context.Context) error {
	// SQL operations
	return nil
})
```

Use stricter isolation when you must prevent concurrent anomalies and are ready to handle retry logic on serialization conflicts.

## Error code helper

You can extract PostgreSQL error code:

```go
if code := storage.ErrorCode(err); code == "23505" {
	// unique_violation
}
```
