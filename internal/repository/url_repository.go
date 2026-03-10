package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/emsbt/url-shortener/internal/model"
	_ "modernc.org/sqlite"
)

// ErrNotFound é retornado quando uma URL não é encontrada no banco de dados.
var ErrNotFound = errors.New("url not found")

// ErrDuplicateID é retornado quando o ID fornecido já existe.
var ErrDuplicateID = errors.New("id already exists")

// URLRepository define o contrato de persistência para URLs encurtadas.
type URLRepository interface {
	Create(ctx context.Context, url *model.URL) error
	GetByID(ctx context.Context, id string) (*model.URL, error)
	FindByOriginalURL(ctx context.Context, originalURL string) (*model.URL, error)
	Update(ctx context.Context, url *model.URL) error
	Delete(ctx context.Context, id string) error
	IncrementClickCount(ctx context.Context, id string) error
	List(ctx context.Context, page, size int) ([]model.URL, int64, error)
	ExistsID(ctx context.Context, id string) (bool, error)
}

// sqliteRepository é a implementação de URLRepository com suporte a SQLite.
type sqliteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository abre (ou cria) o banco de dados SQLite no caminho fornecido
// e executa a migração do schema.
func NewSQLiteRepository(dsn string) (URLRepository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &sqliteRepository{db: db}, nil
}

// migrate cria a tabela urls se ela ainda não existir.
func migrate(db *sql.DB) error {
	const schema = `
	CREATE TABLE IF NOT EXISTS urls (
		id              TEXT PRIMARY KEY,
		original_url    TEXT NOT NULL,
		short_url       TEXT NOT NULL,
		created_at      DATETIME NOT NULL,
		expiration_date DATETIME,
		click_count     INTEGER NOT NULL DEFAULT 0
	);`

	_, err := db.Exec(schema)
	return err
}

// Create insere um novo registro de URL. Retorna ErrDuplicateID em conflito de chave primária.
func (r *sqliteRepository) Create(ctx context.Context, url *model.URL) error {
	const q = `
	INSERT INTO urls (id, original_url, short_url, created_at, expiration_date, click_count)
	VALUES (?, ?, ?, ?, ?, ?)`

	var expirationDate interface{}
	if url.ExpirationDate != nil {
		expirationDate = url.ExpirationDate.UTC().Format(time.RFC3339)
	}

	_, err := r.db.ExecContext(ctx, q,
		url.ID,
		url.OriginalURL,
		url.ShortURL,
		url.CreatedAt.UTC().Format(time.RFC3339),
		expirationDate,
		url.ClickCount,
	)
	if err != nil {
		// modernc sqlite expõe erros de restrição como erros genéricos contendo
		// "UNIQUE constraint failed"
		if isDuplicateError(err) {
			return ErrDuplicateID
		}
		return fmt.Errorf("insert url: %w", err)
	}
	return nil
}

// GetByID recupera uma URL pelo seu ID curto. Retorna ErrNotFound quando não encontrada.
func (r *sqliteRepository) GetByID(ctx context.Context, id string) (*model.URL, error) {
	const q = `
	SELECT id, original_url, short_url, created_at, expiration_date, click_count
	FROM urls
	WHERE id = ?`

	row := r.db.QueryRowContext(ctx, q, id)
	return scanURL(row)
}

// FindByOriginalURL busca uma URL pelo endereço original. Retorna ErrNotFound quando não encontrada.
func (r *sqliteRepository) FindByOriginalURL(ctx context.Context, originalURL string) (*model.URL, error) {
	const q = `
	SELECT id, original_url, short_url, created_at, expiration_date, click_count
	FROM urls
	WHERE original_url = ?
	LIMIT 1`

	row := r.db.QueryRowContext(ctx, q, originalURL)
	return scanURL(row)
}

// Update substitui os campos original_url, short_url e expiration_date de um registro existente.
func (r *sqliteRepository) Update(ctx context.Context, url *model.URL) error {
	const q = `
	UPDATE urls
	SET original_url = ?, short_url = ?, expiration_date = ?
	WHERE id = ?`

	var expirationDate interface{}
	if url.ExpirationDate != nil {
		expirationDate = url.ExpirationDate.UTC().Format(time.RFC3339)
	}

	res, err := r.db.ExecContext(ctx, q, url.OriginalURL, url.ShortURL, expirationDate, url.ID)
	if err != nil {
		return fmt.Errorf("update url: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete remove um registro de URL pelo seu ID curto. Retorna ErrNotFound quando não encontrado.
func (r *sqliteRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM urls WHERE id = ?`

	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete url: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// IncrementClickCount incrementa atomicamente o contador de cliques de uma URL.
func (r *sqliteRepository) IncrementClickCount(ctx context.Context, id string) error {
	const q = `UPDATE urls SET click_count = click_count + 1 WHERE id = ?`
	res, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("increment click count: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// List retorna uma fatia paginada de URLs junto com o total.
func (r *sqliteRepository) List(ctx context.Context, page, size int) ([]model.URL, int64, error) {
	const countQ = `SELECT COUNT(*) FROM urls`
	var total int64
	if err := r.db.QueryRowContext(ctx, countQ).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count urls: %w", err)
	}

	const q = `
	SELECT id, original_url, short_url, created_at, expiration_date, click_count
	FROM urls
	ORDER BY created_at DESC
	LIMIT ? OFFSET ?`

	offset := (page - 1) * size
	rows, err := r.db.QueryContext(ctx, q, size, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list urls: %w", err)
	}
	defer rows.Close()

	var urls []model.URL
	for rows.Next() {
		u, err := scanURLRow(rows)
		if err != nil {
			return nil, 0, err
		}
		urls = append(urls, *u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return urls, total, nil
}

// ExistsID informa se o ID fornecido já está em uso.
func (r *sqliteRepository) ExistsID(ctx context.Context, id string) (bool, error) {
	const q = `SELECT 1 FROM urls WHERE id = ? LIMIT 1`
	var dummy int
	err := r.db.QueryRowContext(ctx, q, id).Scan(&dummy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("exists id: %w", err)
	}
	return true, nil
}

// ---- funções auxiliares ----

type scanner interface {
	Scan(dest ...any) error
}

func scanURL(s scanner) (*model.URL, error) {
	var (
		u            model.URL
		createdAtStr string
		expDateStr   sql.NullString
	)
	err := s.Scan(
		&u.ID,
		&u.OriginalURL,
		&u.ShortURL,
		&createdAtStr,
		&expDateStr,
		&u.ClickCount,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan url: %w", err)
	}
	return parseURLTimes(&u, createdAtStr, expDateStr)
}

func scanURLRow(rows *sql.Rows) (*model.URL, error) {
	var (
		u            model.URL
		createdAtStr string
		expDateStr   sql.NullString
	)
	err := rows.Scan(
		&u.ID,
		&u.OriginalURL,
		&u.ShortURL,
		&createdAtStr,
		&expDateStr,
		&u.ClickCount,
	)
	if err != nil {
		return nil, fmt.Errorf("scan url row: %w", err)
	}
	return parseURLTimes(&u, createdAtStr, expDateStr)
}

func parseURLTimes(u *model.URL, createdAtStr string, expDateStr sql.NullString) (*model.URL, error) {
	t, err := time.Parse(time.RFC3339, createdAtStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	u.CreatedAt = t

	if expDateStr.Valid && expDateStr.String != "" {
		exp, err := time.Parse(time.RFC3339, expDateStr.String)
		if err != nil {
			return nil, fmt.Errorf("parse expiration_date: %w", err)
		}
		u.ExpirationDate = &exp
	}
	return u, nil
}

func isDuplicateError(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "UNIQUE constraint failed")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
