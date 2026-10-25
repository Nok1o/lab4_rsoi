package usecase

import (
	"context"
	"strings"

	"libriary_system/shared/pagination"

	"github.com/google/uuid"

	"libriary_system/library_service/internal/domain"
)

type LibraryRepository interface {
	ListLibrariesByCity(ctx context.Context, city string, pageToken pagination.PageToken) (pagination.Page[domain.Library], error)
	ListBooksByLibrary(ctx context.Context, libraryUID uuid.UUID, showAll bool, pageToken pagination.PageToken) (pagination.Page[domain.LibraryBook], error)
	GetLibraryByUID(ctx context.Context, libraryUID uuid.UUID) (domain.Library, error)
	GetBookByUID(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReserveBook(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReturnBook(ctx context.Context, libraryUID, bookUID uuid.UUID, condition domain.BookCondition) error
}

type LibraryUseCase struct {
	repository LibraryRepository
}

func NewLibraryUseCase(repository LibraryRepository) *LibraryUseCase {
	return &LibraryUseCase{repository: repository}
}

func (useCase *LibraryUseCase) ListLibrariesByCity(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	city = strings.TrimSpace(city)
	if err := validateListLibraries(city, pageToken); err != nil {
		return pagination.Page[domain.Library]{}, err
	}

	return useCase.repository.ListLibrariesByCity(ctx, city, pageToken)
}

func (useCase *LibraryUseCase) ListBooksByLibrary(
	ctx context.Context,
	libraryUID uuid.UUID,
	showAll bool,
	pageToken pagination.PageToken,
) (pagination.Page[domain.LibraryBook], error) {
	if err := validateListBooks(libraryUID, pageToken); err != nil {
		return pagination.Page[domain.LibraryBook]{}, err
	}

	return useCase.repository.ListBooksByLibrary(ctx, libraryUID, showAll, pageToken)
}

func (useCase *LibraryUseCase) GetLibraryByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
) (domain.Library, error) {
	if err := validateLibraryUID(libraryUID); err != nil {
		return domain.Library{}, err
	}

	return useCase.repository.GetLibraryByUID(ctx, libraryUID)
}

func (useCase *LibraryUseCase) GetBookByUID(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	if err := validateUIDs(libraryUID, bookUID); err != nil {
		return domain.LibraryBook{}, err
	}

	return useCase.repository.GetBookByUID(ctx, libraryUID, bookUID)
}

func (useCase *LibraryUseCase) ReserveBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
) (domain.LibraryBook, error) {
	if err := validateUIDs(libraryUID, bookUID); err != nil {
		return domain.LibraryBook{}, err
	}

	return useCase.repository.ReserveBook(ctx, libraryUID, bookUID)
}

func (useCase *LibraryUseCase) ReturnBook(
	ctx context.Context,
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
	condition domain.BookCondition,
) error {
	if err := validateReturnBook(libraryUID, bookUID, condition); err != nil {
		return err
	}

	return useCase.repository.ReturnBook(ctx, libraryUID, bookUID, condition)
}
