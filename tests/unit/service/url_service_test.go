package service_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emsbt/url-shortener/internal/model"
	"github.com/emsbt/url-shortener/internal/repository"
	"github.com/emsbt/url-shortener/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dbCounter atomic.Int64

func newTestService(t *testing.T) service.URLService {
	t.Helper()
	n := dbCounter.Add(1)
	dsn := fmt.Sprintf("file:svcdb%d?mode=memory&cache=shared", n)
	repo, err := repository.NewSQLiteRepository(dsn)
	require.NoError(t, err)
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return service.NewURLService(repo, "http://localhost:8080", logger)
}

func TestCreate_Success(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL: "https://example.com/path",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.ID)
	assert.Equal(t, "http://localhost:8080/"+resp.ID, resp.ShortURL)
	assert.Equal(t, "https://example.com/path", resp.OriginalURL)
}

func TestCreate_CustomAlias(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL: "https://example.com",
		CustomAlias: "my-alias",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-alias", resp.ID)
}

func TestCreate_CustomAlias_Conflict(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL: "https://example.com/primeira",
		CustomAlias: "taken",
	})
	require.NoError(t, err)

	// URL diferente, mesmo alias — deve retornar conflito de alias
	_, err = svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL: "https://example.com/segunda",
		CustomAlias: "taken",
	})
	assert.ErrorIs(t, err, service.ErrAliasConflict)
}

func TestCreate_InvalidURL_Empty(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: ""})
	assert.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestCreate_InvalidURL_NoScheme(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: "example.com"})
	assert.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestCreate_InvalidURL_FTPScheme(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: "ftp://example.com"})
	assert.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestGetByID_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.GetByID(context.Background(), "nope")
	assert.ErrorIs(t, err, service.ErrURLNotFound)
}

func TestRedirect_Success(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com"})
	require.NoError(t, err)

	target, err := svc.Redirect(ctx, resp.ID)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", target)
}

func TestRedirect_NotFound(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Redirect(context.Background(), "missing")
	assert.ErrorIs(t, err, service.ErrURLNotFound)
}

func TestRedirect_Expired(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	past := time.Now().Add(-1 * time.Hour)
	resp, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL:    "https://example.com",
		ExpirationDate: &past,
	})
	require.NoError(t, err)

	_, err = svc.Redirect(ctx, resp.ID)
	assert.ErrorIs(t, err, service.ErrURLExpired)
}

func TestRedirect_ClickCountIncremented(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com"})
	require.NoError(t, err)

	for range 3 {
		_, err = svc.Redirect(ctx, resp.ID)
		require.NoError(t, err)
	}

	details, err := svc.GetByID(ctx, resp.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3), details.ClickCount)
}

func TestList_Pagination(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := range 7 {
		_, err := svc.Create(ctx, &model.CreateURLRequest{
			OriginalURL: "https://example.com/" + string(rune('a'+i)),
		})
		require.NoError(t, err)
	}

	result, err := svc.List(ctx, 1, 5)
	require.NoError(t, err)
	assert.Equal(t, int64(7), result.Total)
	assert.Len(t, result.Data, 5)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 5, result.Size)

	result2, err := svc.List(ctx, 2, 5)
	require.NoError(t, err)
	assert.Len(t, result2.Data, 2)
}

func TestCreate_WithExpiration(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	future := time.Now().Add(24 * time.Hour)
	resp, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL:    "https://example.com",
		ExpirationDate: &future,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.ExpirationDate)

	// Deve redirecionar normalmente (ainda não expirado)
	_, err = svc.Redirect(ctx, resp.ID)
	require.NoError(t, err)
}

func TestCreate_IDNotEmpty(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	for i := range 20 {
		resp, err := svc.Create(ctx, &model.CreateURLRequest{
			OriginalURL: fmt.Sprintf("https://example.com/page/%d", i),
		})
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
		assert.GreaterOrEqual(t, len(resp.ID), 6)
		assert.LessOrEqual(t, len(resp.ID), 8)
	}
}

func TestCreate_InvalidURL_MalformedURL(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: "://bad-url"})
	assert.True(t, errors.Is(err, service.ErrInvalidURL))
}

func TestCreate_DuplicateURL(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	req := &model.CreateURLRequest{OriginalURL: "https://example.com/duplicada"}
	_, err := svc.Create(ctx, req)
	require.NoError(t, err)

	_, err = svc.Create(ctx, req)
	assert.ErrorIs(t, err, service.ErrDuplicateURL)
}

func TestCreate_InvalidURL_SingleLabelHost(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: "https://semPonto"})
	assert.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestCreate_InvalidURL_NumericTLD(t *testing.T) {
	svc := newTestService(t)
	_, err := svc.Create(context.Background(), &model.CreateURLRequest{OriginalURL: "https://exemplo.123"})
	assert.ErrorIs(t, err, service.ErrInvalidURL)
}

func TestDelete_Success(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com/delete-me"})
	require.NoError(t, err)

	require.NoError(t, svc.Delete(ctx, resp.ID))

	_, err = svc.GetByID(ctx, resp.ID)
	assert.ErrorIs(t, err, service.ErrURLNotFound)
}

func TestDelete_NotFound(t *testing.T) {
	svc := newTestService(t)
	err := svc.Delete(context.Background(), "naoexiste")
	assert.ErrorIs(t, err, service.ErrURLNotFound)
}

func TestUpdate_ChangeOriginalURL(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	resp, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com/antes"})
	require.NoError(t, err)

	newURL := "https://example.com/depois"
	updated, err := svc.Update(ctx, resp.ID, &model.UpdateURLRequest{OriginalURL: &newURL})
	require.NoError(t, err)
	assert.Equal(t, newURL, updated.OriginalURL)
}

func TestUpdate_NotFound(t *testing.T) {
	svc := newTestService(t)
	newURL := "https://example.com"
	_, err := svc.Update(context.Background(), "naoexiste", &model.UpdateURLRequest{OriginalURL: &newURL})
	assert.ErrorIs(t, err, service.ErrURLNotFound)
}

func TestUpdate_DuplicateURL(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com/a"})
	require.NoError(t, err)
	resp2, err := svc.Create(ctx, &model.CreateURLRequest{OriginalURL: "https://example.com/b"})
	require.NoError(t, err)

	// Tenta atualizar a URL de resp2 para a mesma de resp1
	urlA := "https://example.com/a"
	_, err = svc.Update(ctx, resp2.ID, &model.UpdateURLRequest{OriginalURL: &urlA})
	assert.ErrorIs(t, err, service.ErrDuplicateURL)
}

func TestUpdate_ClearExpiration(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	future := time.Now().Add(24 * time.Hour)
	resp, err := svc.Create(ctx, &model.CreateURLRequest{
		OriginalURL:    "https://example.com/expira",
		ExpirationDate: &future,
	})
	require.NoError(t, err)
	require.NotNil(t, resp.ExpirationDate)

	updated, err := svc.Update(ctx, resp.ID, &model.UpdateURLRequest{ClearExpiration: true})
	require.NoError(t, err)
	assert.Nil(t, updated.ExpirationDate)
}
