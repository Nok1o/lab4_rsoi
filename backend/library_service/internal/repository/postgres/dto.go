package postgres

import (
	"github.com/google/uuid"

	"libriary_system/library_service/internal/domain"
)

// libraryDTO represents the columns returned by library queries.
type libraryDTO struct {
	UID     uuid.UUID
	Name    string
	City    string
	Address string
}

// libraryBookDTO represents a book together with its availability in a library.
// Nullable fields mirror nullable columns in the books table.
type libraryBookDTO struct {
	UID            uuid.UUID
	Name           string
	Author         *string
	Genre          *string
	Condition      *string
	AvailableCount int
}

func (dto libraryDTO) toDomain() domain.Library {
	return domain.Library{
		Id:      dto.UID,
		Name:    dto.Name,
		City:    dto.City,
		Address: dto.Address,
	}
}

func libraryDTOsToDomain(dtos []libraryDTO) []domain.Library {
	if dtos == nil {
		return nil
	}

	libraries := make([]domain.Library, len(dtos))
	for index, dto := range dtos {
		libraries[index] = dto.toDomain()
	}

	return libraries
}

func (dto libraryBookDTO) toDomain() domain.LibraryBook {
	return domain.LibraryBook{
		Book: domain.Book{
			Id:        dto.UID,
			Name:      dto.Name,
			Author:    nullableString(dto.Author),
			Genre:     nullableString(dto.Genre),
			Condition: nullableBookCondition(dto.Condition),
		},
		AvailableCount: dto.AvailableCount,
	}
}

func libraryBookDTOsToDomain(dtos []libraryBookDTO) []domain.LibraryBook {
	if dtos == nil {
		return nil
	}

	books := make([]domain.LibraryBook, len(dtos))
	for index, dto := range dtos {
		books[index] = dto.toDomain()
	}

	return books
}

func nullableString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func nullableBookCondition(value *string) domain.BookCondition {
	if value == nil {
		return ""
	}

	return domain.BookCondition(*value)
}
