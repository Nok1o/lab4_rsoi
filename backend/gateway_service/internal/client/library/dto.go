package library

import (
	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
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

func toLibrary(source libraryResponse) domain.Library {
	return domain.Library{Id: source.UID, Name: source.Name, City: source.City, Address: source.Address}
}

func toLibraryBook(source libraryBookResponse) domain.LibraryBook {
	return domain.LibraryBook{
		Book: domain.Book{
			Id: source.UID, Name: source.Name, Author: source.Author, Genre: source.Genre,
			Condition: source.Condition,
		},
		AvailableCount: source.AvailableCount,
	}
}
