package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/emsbt/url-shortener/internal/model"
	"github.com/emsbt/url-shortener/internal/repository"
)

const (
	base62Chars  = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	defaultIDLen = 7
	maxRetries   = 10
)

// ErrInvalidURL é retornado quando a URL original falha na validação.
var ErrInvalidURL = errors.New("invalid url")

// ErrURLNotFound é retornado quando o ID curto não existe.
var ErrURLNotFound = errors.New("url not found")

// ErrURLExpired é retornado quando a URL encurtada passou da data de expiração.
var ErrURLExpired = errors.New("url expired")

// ErrAliasConflict é retornado quando um alias customizado solicitado já está em uso.
var ErrAliasConflict = errors.New("alias already in use")

// ErrDuplicateURL é retornado quando a URL original já foi cadastrada anteriormente.
var ErrDuplicateURL = errors.New("url already registered")

// URLService define o contrato de lógica de negócio para encurtamento de URLs.
type URLService interface {
	Create(ctx context.Context, req *model.CreateURLRequest) (*model.CreateURLResponse, error)
	GetByID(ctx context.Context, id string) (*model.URLDetailsResponse, error)
	Update(ctx context.Context, id string, req *model.UpdateURLRequest) (*model.URLDetailsResponse, error)
	Delete(ctx context.Context, id string) error
	Redirect(ctx context.Context, id string) (string, error)
	List(ctx context.Context, page, size int) (*model.ListURLsResponse, error)
}

type urlService struct {
	repo    repository.URLRepository
	baseURL string
	logger  *slog.Logger
}

// NewURLService constrói um URLService com suporte ao repositório fornecido.
func NewURLService(repo repository.URLRepository, baseURL string, logger *slog.Logger) URLService {
	return &urlService{
		repo:    repo,
		baseURL: strings.TrimRight(baseURL, "/"),
		logger:  logger,
	}
}

// Create valida a requisição, gera (ou usa) um ID curto e persiste
// o novo registro de URL.
func (s *urlService) Create(ctx context.Context, req *model.CreateURLRequest) (*model.CreateURLResponse, error) {
	if err := validateURL(req.OriginalURL); err != nil {
		return nil, err
	}

	existing, err := s.repo.FindByOriginalURL(ctx, req.OriginalURL)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return nil, fmt.Errorf("check duplicate url: %w", err)
	}
	if existing != nil {
		return nil, ErrDuplicateURL
	}

	var id string
	if req.CustomAlias != "" {
		exists, err := s.repo.ExistsID(ctx, req.CustomAlias)
		if err != nil {
			return nil, fmt.Errorf("check alias: %w", err)
		}
		if exists {
			return nil, ErrAliasConflict
		}
		id = req.CustomAlias
	} else {
		var err error
		id, err = s.generateUniqueID(ctx)
		if err != nil {
			return nil, err
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	u := &model.URL{
		ID:             id,
		ShortURL:       s.baseURL + "/" + id,
		OriginalURL:    req.OriginalURL,
		CreatedAt:      now,
		ExpirationDate: req.ExpirationDate,
		ClickCount:     0,
	}

	if err := s.repo.Create(ctx, u); err != nil {
		if errors.Is(err, repository.ErrDuplicateID) {
			return nil, ErrAliasConflict
		}
		return nil, fmt.Errorf("create url: %w", err)
	}

	s.logger.InfoContext(ctx, "url created",
		slog.String("id", id),
		slog.String("original_url", req.OriginalURL),
	)

	return &model.CreateURLResponse{
		ID:             u.ID,
		ShortURL:       u.ShortURL,
		OriginalURL:    u.OriginalURL,
		CreatedAt:      u.CreatedAt,
		ExpirationDate: u.ExpirationDate,
	}, nil
}

// GetByID busca os detalhes de uma URL pelo ID curto.
func (s *urlService) GetByID(ctx context.Context, id string) (*model.URLDetailsResponse, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrURLNotFound
		}
		return nil, fmt.Errorf("get url: %w", err)
	}

	return &model.URLDetailsResponse{
		ID:             u.ID,
		ShortURL:       u.ShortURL,
		OriginalURL:    u.OriginalURL,
		CreatedAt:      u.CreatedAt,
		ExpirationDate: u.ExpirationDate,
		ClickCount:     u.ClickCount,
	}, nil
}

// Update aplica alterações parciais a uma URL existente.
// Apenas os campos presentes no request são modificados.
func (s *urlService) Update(ctx context.Context, id string, req *model.UpdateURLRequest) (*model.URLDetailsResponse, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return nil, ErrURLNotFound
		}
		return nil, fmt.Errorf("get url for update: %w", err)
	}

	if req.OriginalURL != nil {
		newURL := *req.OriginalURL
		if err := validateURL(newURL); err != nil {
			return nil, err
		}
		existing, err := s.repo.FindByOriginalURL(ctx, newURL)
		if err != nil && !errors.Is(err, repository.ErrNotFound) {
			return nil, fmt.Errorf("check duplicate url: %w", err)
		}
		if existing != nil && existing.ID != id {
			return nil, ErrDuplicateURL
		}
		u.OriginalURL = newURL
		u.ShortURL = s.baseURL + "/" + u.ID
	}

	if req.ClearExpiration {
		u.ExpirationDate = nil
	} else if req.ExpirationDate != nil {
		u.ExpirationDate = req.ExpirationDate
	}

	if err := s.repo.Update(ctx, u); err != nil {
		return nil, fmt.Errorf("update url: %w", err)
	}

	s.logger.InfoContext(ctx, "url updated", slog.String("id", id))

	return &model.URLDetailsResponse{
		ID:             u.ID,
		ShortURL:       u.ShortURL,
		OriginalURL:    u.OriginalURL,
		CreatedAt:      u.CreatedAt,
		ExpirationDate: u.ExpirationDate,
		ClickCount:     u.ClickCount,
	}, nil
}

// Delete remove permanentemente uma URL encurtada pelo seu ID.
func (s *urlService) Delete(ctx context.Context, id string) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrURLNotFound
		}
		return fmt.Errorf("delete url: %w", err)
	}
	s.logger.InfoContext(ctx, "url deleted", slog.String("id", id))
	return nil
}

// Redirect resolve a URL original para um redirecionamento e incrementa o
// contador de cliques. Retorna ErrURLNotFound ou ErrURLExpired conforme apropriado.
func (s *urlService) Redirect(ctx context.Context, id string) (string, error) {
	u, err := s.repo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			s.logger.WarnContext(ctx, "redirect: url not found", slog.String("id", id))
			return "", ErrURLNotFound
		}
		return "", fmt.Errorf("get url for redirect: %w", err)
	}

	if u.ExpirationDate != nil && time.Now().UTC().After(*u.ExpirationDate) {
		s.logger.WarnContext(ctx, "redirect: url expired",
			slog.String("id", id),
			slog.Time("expired_at", *u.ExpirationDate),
		)
		return "", ErrURLExpired
	}

	if err := s.repo.IncrementClickCount(ctx, id); err != nil {
		// Não fatal: registra e continua
		s.logger.ErrorContext(ctx, "increment click count failed",
			slog.String("id", id),
			slog.String("error", err.Error()),
		)
	}

	s.logger.InfoContext(ctx, "redirect",
		slog.String("id", id),
		slog.String("original_url", u.OriginalURL),
	)

	return u.OriginalURL, nil
}

// List retorna uma lista paginada de todas as URLs.
func (s *urlService) List(ctx context.Context, page, size int) (*model.ListURLsResponse, error) {
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 10
	}

	urls, total, err := s.repo.List(ctx, page, size)
	if err != nil {
		return nil, fmt.Errorf("list urls: %w", err)
	}

	details := make([]model.URLDetailsResponse, len(urls))
	for i, u := range urls {
		details[i] = model.URLDetailsResponse{
			ID:             u.ID,
			ShortURL:       u.ShortURL,
			OriginalURL:    u.OriginalURL,
			CreatedAt:      u.CreatedAt,
			ExpirationDate: u.ExpirationDate,
			ClickCount:     u.ClickCount,
		}
	}

	return &model.ListURLsResponse{
		Data:  details,
		Page:  page,
		Size:  size,
		Total: total,
	}, nil
}

// ---- funções auxiliares ----

// validateURL verifica que a URL original não está vazia, é bem formada e usa
// o esquema http ou https.
func validateURL(rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return fmt.Errorf("%w: url is required", ErrInvalidURL)
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidURL, err.Error())
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrInvalidURL)
	}

	if parsed.Host == "" {
		return fmt.Errorf("%w: host is required", ErrInvalidURL)
	}

	host := parsed.Hostname()
	if host != "localhost" && net.ParseIP(host) == nil {
		parts := strings.Split(host, ".")
		if len(parts) < 2 {
			return fmt.Errorf("%w: host must be a fully qualified domain name", ErrInvalidURL)
		}
		tld := parts[len(parts)-1]
		if len(tld) < 2 || !isAlpha(tld) {
			return fmt.Errorf("%w: invalid top-level domain", ErrInvalidURL)
		}
	}

	return nil
}

// isAlpha informa se a string contém apenas letras (a-z, A-Z).
func isAlpha(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// generateUniqueID gera um ID Base62 aleatório que ainda não existe no
// repositório. Tenta até maxRetries vezes antes de retornar um erro.
func (s *urlService) generateUniqueID(ctx context.Context) (string, error) {
	for range maxRetries {
		id := randomBase62(defaultIDLen)
		exists, err := s.repo.ExistsID(ctx, id)
		if err != nil {
			return "", fmt.Errorf("check id existence: %w", err)
		}
		if !exists {
			return id, nil
		}
	}
	return "", errors.New("could not generate unique id after max retries")
}

// randomBase62 retorna uma string alfanumérica aleatória do tamanho fornecido.
func randomBase62(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = base62Chars[rand.IntN(len(base62Chars))]
	}
	return string(b)
}
