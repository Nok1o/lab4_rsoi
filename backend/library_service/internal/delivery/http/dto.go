package http

import (
	"github.com/google/uuid"

	"libriary_system/library_service/internal/domain"
	"libriary_system/shared/pagination"
)

type libraryResponse struct {
	UID     uuid.UUID `json:"libraryUid"`
	Name    string    `json:"name"`
	City    string    `json:"city"`
	Address string    `json:"address"`
}

type libraryBookResponse struct {
	UID            uuid.UUID            `json:"bookUid"`
	Name           string               `json:"name"`
	Author         string               `json:"author"`
	Genre          string               `json:"genre"`
	Condition      domain.BookCondition `json:"condition"`
	AvailableCount int                  `json:"availableCount"`
}

type pageResponse[T any] struct {
	Items []T   `json:"items"`
	Total int64 `json:"total"`
}

type returnBookRequest struct {
	Condition domain.BookCondition `json:"condition"`
}

type errorResponse struct {
	Message string `json:"message"`
}

type validationErrorResponse struct {
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"`
}

func toLibraryResponse(library domain.Library) libraryResponse {
	return libraryResponse{
		UID:     library.Id,
		Name:    library.Name,
		City:    library.City,
		Address: library.Address,
	}
}

func toLibraryPageResponse(page pagination.Page[domain.Library]) pageResponse[libraryResponse] {
	items := make([]libraryResponse, 0, len(page.Items))
	for _, library := range page.Items {
		items = append(items, toLibraryResponse(library))
	}

	return pageResponse[libraryResponse]{Items: items, Total: page.Total}
}

func toLibraryBookResponse(book domain.LibraryBook) libraryBookResponse {
	return libraryBookResponse{
		UID:            book.Id,
		Name:           book.Name,
		Author:         book.Author,
		Genre:          book.Genre,
		Condition:      book.Condition,
		AvailableCount: book.AvailableCount,
	}
}

func toLibraryBookPageResponse(
	page pagination.Page[domain.LibraryBook],
) pageResponse[libraryBookResponse] {
	items := make([]libraryBookResponse, 0, len(page.Items))
	for _, book := range page.Items {
		items = append(items, toLibraryBookResponse(book))
	}

	return pageResponse[libraryBookResponse]{Items: items, Total: page.Total}
}
