package model

import "time"

// URL representa uma entidade de URL encurtada armazenada no banco de dados.
type URL struct {
	ID             string     `json:"id"`
	ShortURL       string     `json:"shortUrl"`
	OriginalURL    string     `json:"originalUrl"`
	CreatedAt      time.Time  `json:"createdAt"`
	ExpirationDate *time.Time `json:"expirationDate,omitempty"`
	ClickCount     int64      `json:"clickCount"`
}

// CreateURLRequest é o corpo da requisição para POST /v1/urls.
type CreateURLRequest struct {
	OriginalURL    string     `json:"originalUrl"`
	ExpirationDate *time.Time `json:"expirationDate,omitempty"`
	CustomAlias    string     `json:"customAlias,omitempty"`
}

// CreateURLResponse é o corpo da resposta para uma URL encurtada criada com sucesso.
type CreateURLResponse struct {
	ID             string     `json:"id"`
	ShortURL       string     `json:"shortUrl"`
	OriginalURL    string     `json:"originalUrl"`
	CreatedAt      time.Time  `json:"createdAt"`
	ExpirationDate *time.Time `json:"expirationDate,omitempty"`
}

// URLDetailsResponse é o corpo da resposta para GET /v1/urls/{id}.
type URLDetailsResponse struct {
	ID             string     `json:"id"`
	ShortURL       string     `json:"shortUrl"`
	OriginalURL    string     `json:"originalUrl"`
	CreatedAt      time.Time  `json:"createdAt"`
	ExpirationDate *time.Time `json:"expirationDate,omitempty"`
	ClickCount     int64      `json:"clickCount"`
}

// ListURLsResponse é o corpo da resposta para GET /v1/urls.
type ListURLsResponse struct {
	Data  []URLDetailsResponse `json:"data"`
	Page  int                  `json:"page"`
	Size  int                  `json:"size"`
	Total int64                `json:"total"`
}

// ErrorResponse é o envelope padrão de erros.
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contém o código legível por máquina e a mensagem legível por humanos.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// UpdateURLRequest é o corpo da requisição para PATCH /v1/urls/{id}.
// Apenas os campos presentes são atualizados.
// Para remover a data de expiração, utilize clearExpiration: true.
type UpdateURLRequest struct {
	OriginalURL     *string    `json:"originalUrl,omitempty"`
	ExpirationDate  *time.Time `json:"expirationDate,omitempty"`
	ClearExpiration bool       `json:"clearExpiration,omitempty"`
}
