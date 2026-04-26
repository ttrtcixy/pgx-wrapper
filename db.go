package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
)

type Config struct {
	Dsn             string        `env:"DB_URL,required,notEmpty"`
	ConnectTimeout  time.Duration `env:"DB_CONNECT_TIMEOUT,required,notEmpty"`
	MaxConnIdleTime time.Duration `env:"DB_MAX_IDLE_TIMEOUT,required,notEmpty"`
	OtelTracer      bool          `env:"DB_OTEL_TRACER"`
}

type Postgres struct {
	log  *slog.Logger
	pool *pgxpool.Pool
}

func New(ctx context.Context, log *slog.Logger, cfg *Config) (*Postgres, error) {
	const op = "storage.New"
	var pg = &Postgres{
		log: log,
		//log: log.WithGroup("postgres_storage"),
	}

	if err := pg.createPool(ctx, cfg); err != nil {
		return nil, fmt.Errorf("%s - error pool creation -> %w", op, err)
	}

	if err := pg.ping(ctx); err != nil {
		return nil, fmt.Errorf("%s - ping error -> %w", op, err)
	}

	return pg, nil
}

// createPool init database connection, but not connect
func (db *Postgres) createPool(ctx context.Context, cfg *Config) (err error) {
	const op = "storage.createPool"

	poolCfg, err := pgxpool.ParseConfig(cfg.Dsn)
	if err != nil {
		return fmt.Errorf("%s - parse config error -> %w", op, err)
	}

	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	if cfg.OtelTracer {
		poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()
	}

	db.pool, err = pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return fmt.Errorf("%s - create pool error -> %w", op, err)
	}

	return nil
}

func (db *Postgres) ping(ctx context.Context) error {
	const op = "storage.ping"
	if err := db.pool.Ping(ctx); err != nil {
		return fmt.Errorf("%s - error connect to database -> %s", op, err.Error())
	}
	return nil
}
func (db *Postgres) Close(_ context.Context) error {
	db.pool.Close()
	return nil
}

type txCtxKey struct {
}

var txKey = txCtxKey{}

// RunInTx before starting a transaction, it checks whether the user has canceled the request, and then initiates the transaction.
func (db *Postgres) RunInTx(ctx context.Context, txOptions pgx.TxOptions, fn func(context.Context) error) (err error) {
	const op = "storage.RunInTx"
	// if user close connection
	if err = ctx.Err(); err != nil {
		return fmt.Errorf("%s -> %w", op, err)
	}

	// if already use RunInTx()
	// todo It might be worth enabling, allowing different usecases to use the same transaction.
	if _, ok := value(ctx); ok {
		//return fn(ctx)
		return fmt.Errorf("%s - transaction already start", op)
	}

	// start tx
	tx, err := db.pool.BeginTx(ctx, txOptions)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil { // no ctx
				db.log.LogAttrs(
					ctx,
					slog.LevelError,
					"Transaction panic",
					slog.String("error", rollbackErr.Error()),
				)
			}
			panic(p)
		}

		if err != nil {
			if rollbackErr := tx.Rollback(context.Background()); rollbackErr != nil { // no ctx
				db.log.LogAttrs(
					ctx,
					slog.LevelError,
					"Transaction error",
					slog.String("error", rollbackErr.Error()),
				)
				err = fmt.Errorf("%s - transaction error - %w -> %w", op, rollbackErr, err)
			}
		}
	}()

	// Adding a transaction to the ctx, requests to the database will go through this transaction.
	ctx = context.WithValue(ctx, txKey, tx)

	if err = fn(ctx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		db.log.LogAttrs(ctx, slog.LevelError, "Transaction commit error", slog.String("error", err.Error()))
		return fmt.Errorf("%s - transaction commit error -> %w", op, err)
	}
	return nil
}

func value(ctx context.Context) (pgx.Tx, bool) {
	tx, ok := ctx.Value(txKey).(pgx.Tx)
	return tx, ok
}

func (db *Postgres) Exec(ctx context.Context, query Query) (pgconn.CommandTag, error) {
	db.logQuery(ctx, query)

	if tx, ok := value(ctx); ok {
		return tx.Exec(ctx, query.Query(), query.Args()...)
	}

	return db.pool.Exec(ctx, query.Query(), query.Args()...)
}

func (db *Postgres) Query(ctx context.Context, query Query) (pgx.Rows, error) {
	db.logQuery(ctx, query)
	if tx, ok := value(ctx); ok {
		return tx.Query(ctx, query.Query(), query.Args()...)
	}

	return db.pool.Query(ctx, query.Query(), query.Args()...)
}
func (db *Postgres) QueryRow(ctx context.Context, query Query) pgx.Row {
	db.logQuery(ctx, query)
	if tx, ok := value(ctx); ok {
		return tx.QueryRow(ctx, query.Query(), query.Args()...)
	}

	return db.pool.QueryRow(ctx, query.Query(), query.Args()...)
}

func (db *Postgres) logQuery(ctx context.Context, query Query) {
	db.log.LogAttrs(
		ctx, slog.LevelDebug,
		"Postgres request",
		slog.String("query_name", query.QueryName()),
		slog.String("query", query.String()),
	)
}

func ErrorCode(err error) string {
	if e, ok := errors.AsType[*pgconn.PgError](err); ok {
		return e.Code
	}
	return ""
}

func (db *Postgres) Ping(ctx context.Context) error {
	return db.pool.Ping(ctx)
}

// SqlDB - closure does not affect the life of the main pool.
func (db *Postgres) SqlDB() *sql.DB {
	return stdlib.OpenDB(*db.pool.Config().ConnConfig)
}
