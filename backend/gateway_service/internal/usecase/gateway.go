package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/shared/log"
	"libriary_system/shared/pagination"
)

const initialStars = 75

var ErrRentalLimitReached = errors.New("rental limit reached")

type LibraryService interface {
	ListLibrariesByCity(ctx context.Context, city string, pageToken pagination.PageToken) (pagination.Page[domain.Library], error)
	ListBooksByLibrary(ctx context.Context, libraryUID uuid.UUID, showAll bool, pageToken pagination.PageToken) (pagination.Page[domain.LibraryBook], error)
	GetLibraryByUID(ctx context.Context, libraryUID uuid.UUID) (domain.Library, error)
	GetBookByUID(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReserveBook(ctx context.Context, libraryUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReturnBook(ctx context.Context, libraryUID, bookUID uuid.UUID, condition domain.BookCondition) error
}

type RatingService interface {
	GetByUsername(ctx context.Context, username string) (domain.Rating, error)
	Create(ctx context.Context, rating domain.Rating) error
	AddStars(ctx context.Context, username string, delta int) (domain.Rating, error)
}

type ReservationService interface {
	ListByUsername(ctx context.Context, username string) ([]domain.Reservation, error)
	CountByUsernameAndStatus(ctx context.Context, username string, status domain.ReservationStatus) (int, error)
	GetByUIDAndUsername(ctx context.Context, reservationUID uuid.UUID, username string) (domain.Reservation, error)
	Rent(ctx context.Context, username string, libraryUID, bookUID uuid.UUID, tillDate time.Time) (domain.Reservation, error)
	Cancel(ctx context.Context, reservationUID uuid.UUID, username string) error
	Return(ctx context.Context, reservationUID uuid.UUID, username string, returnDate time.Time) (domain.Reservation, error)
}

type ReturnQueue interface {
	Enqueue(ctx context.Context, job ReturnJob) error
}

type ReservationDetails struct {
	Reservation domain.Reservation
	Library     domain.Library
	Book        domain.LibraryBook
	Rating      *domain.Rating
}

type GatewayUseCase struct {
	libraries    LibraryService
	ratings      RatingService
	reservations ReservationService
	returnQueue  ReturnQueue
}

func NewGatewayUseCase(
	libraries LibraryService,
	ratings RatingService,
	reservations ReservationService,
	returnQueue ReturnQueue,
) *GatewayUseCase {
	return &GatewayUseCase{
		libraries:    libraries,
		ratings:      ratings,
		reservations: reservations,
		returnQueue:  returnQueue,
	}
}

func (useCase *GatewayUseCase) ListLibraries(
	ctx context.Context,
	city string,
	pageToken pagination.PageToken,
) (pagination.Page[domain.Library], error) {
	city = strings.TrimSpace(city)
	fields := pagination.ValidatePageToken(pageToken)
	if city == "" {
		fields["city"] = "must not be empty"
	}
	if err := validationError(fields); err != nil {
		return pagination.Page[domain.Library]{}, err
	}
	return useCase.libraries.ListLibrariesByCity(ctx, city, pageToken)
}

func (useCase *GatewayUseCase) ListBooks(
	ctx context.Context,
	libraryUID uuid.UUID,
	showAll bool,
	pageToken pagination.PageToken,
) (pagination.Page[domain.LibraryBook], error) {
	fields := pagination.ValidatePageToken(pageToken)
	if libraryUID == uuid.Nil {
		fields["libraryUid"] = "must be a valid UUID"
	}
	if err := validationError(fields); err != nil {
		return pagination.Page[domain.LibraryBook]{}, err
	}
	return useCase.libraries.ListBooksByLibrary(ctx, libraryUID, showAll, pageToken)
}

func (useCase *GatewayUseCase) GetRating(ctx context.Context, username string) (domain.Rating, error) {
	if err := validateUsername(username); err != nil {
		return domain.Rating{}, err
	}
	rating, err := useCase.ratings.GetByUsername(ctx, username)
	if err == nil {
		return rating, nil
	}
	if !errors.Is(err, domain.ErrRatingNotFound) {
		return domain.Rating{}, err
	}
	return domain.Rating{Username: username, StarsCount: initialStars}, nil
}

func (useCase *GatewayUseCase) ensureRating(ctx context.Context, username string) (domain.Rating, error) {
	rating, err := useCase.ratings.GetByUsername(ctx, username)
	if err == nil {
		return rating, nil
	}
	if !errors.Is(err, domain.ErrRatingNotFound) {
		return domain.Rating{}, err
	}
	rating = domain.Rating{Username: username, StarsCount: initialStars}
	if err := useCase.ratings.Create(ctx, rating); err != nil {
		if errors.Is(err, domain.ErrRatingAlreadyExists) {
			return useCase.ratings.GetByUsername(ctx, username)
		}
		return domain.Rating{}, err
	}
	return rating, nil
}

func (useCase *GatewayUseCase) ListReservations(ctx context.Context, username string) ([]ReservationDetails, error) {
	if err := validateUsername(username); err != nil {
		return nil, err
	}
	reservations, err := useCase.reservations.ListByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	details := make([]ReservationDetails, 0, len(reservations))
	for _, reservation := range reservations {
		library, err := useCase.libraries.GetLibraryByUID(ctx, reservation.LibraryId)
		if err != nil {
			if !IsRetryable(err) {
				return nil, fmt.Errorf("get library for reservation %s: %w", reservation.Id, err)
			}
			library = domain.Library{Id: reservation.LibraryId}
		}
		book, err := useCase.libraries.GetBookByUID(ctx, reservation.LibraryId, reservation.BookId)
		if err != nil {
			if !IsRetryable(err) {
				return nil, fmt.Errorf("get book for reservation %s: %w", reservation.Id, err)
			}
			book = domain.LibraryBook{Book: domain.Book{Id: reservation.BookId}}
		}
		details = append(details, ReservationDetails{Reservation: reservation, Library: library, Book: book})
	}
	return details, nil
}

func (useCase *GatewayUseCase) Rent(
	ctx context.Context,
	username string,
	libraryUID, bookUID uuid.UUID,
	tillDate time.Time,
) (ReservationDetails, error) {
	if err := validateUsername(username); err != nil {
		return ReservationDetails{}, err
	}
	fields := make(map[string]string)
	if libraryUID == uuid.Nil {
		fields["libraryUid"] = "must be a valid UUID"
	}
	if bookUID == uuid.Nil {
		fields["bookUid"] = "must be a valid UUID"
	}
	if tillDate.IsZero() {
		fields["tillDate"] = "must be YYYY-MM-DD"
	}
	if err := validationError(fields); err != nil {
		return ReservationDetails{}, err
	}

	activeCount, err := useCase.reservations.CountByUsernameAndStatus(ctx, username, domain.StatusRented)
	if err != nil {
		return ReservationDetails{}, err
	}
	rating, err := useCase.ensureRating(ctx, username)
	if err != nil {
		return ReservationDetails{}, err
	}
	if activeCount >= rating.StarsCount {
		return ReservationDetails{}, ErrRentalLimitReached
	}
	library, err := useCase.libraries.GetLibraryByUID(ctx, libraryUID)
	if err != nil {
		return ReservationDetails{}, err
	}
	book, err := useCase.libraries.GetBookByUID(ctx, libraryUID, bookUID)
	if err != nil {
		return ReservationDetails{}, err
	}
	reservation, err := useCase.reservations.Rent(ctx, username, libraryUID, bookUID, tillDate)
	if err != nil {
		return ReservationDetails{}, err
	}
	if _, err := useCase.libraries.ReserveBook(ctx, libraryUID, bookUID); err != nil {
		// Remove the new reservation when the library cannot lend the book.
		// Compensation must still run if the original HTTP client disconnects.
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if rollbackErr := useCase.reservations.Cancel(rollbackCtx, reservation.Id, username); rollbackErr != nil {
			log.Error("failed to cancel reservation", "reservation_uid", reservation.Id, "rent_error", err, "rollback_error", rollbackErr)
			return ReservationDetails{}, fmt.Errorf("reserve book failed (%v) and reservation compensation failed: %v", err, rollbackErr)
		}
		return ReservationDetails{}, err
	}
	return ReservationDetails{Reservation: reservation, Library: library, Book: book, Rating: &rating}, nil
}

func (useCase *GatewayUseCase) Return(
	ctx context.Context,
	reservationUID uuid.UUID,
	username string,
	condition domain.BookCondition,
	returnDate time.Time,
) error {
	if err := validateUsername(username); err != nil {
		return err
	}
	fields := make(map[string]string)
	if reservationUID == uuid.Nil {
		fields["reservationUid"] = "must be a valid UUID"
	}
	if condition != domain.ConditionExcellent && condition != domain.ConditionGood && condition != domain.ConditionBad {
		fields["condition"] = "must be one of EXCELLENT, GOOD or BAD"
	}
	if returnDate.IsZero() {
		fields["date"] = "must be YYYY-MM-DD"
	}
	if err := validationError(fields); err != nil {
		return err
	}
	reservation, err := useCase.reservations.GetByUIDAndUsername(ctx, reservationUID, username)
	if err != nil {
		return err
	}
	if reservation.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	completed, err := useCase.reservations.Return(ctx, reservationUID, username, returnDate)
	if err != nil {
		return err
	}
	// Once the reservation has been closed, continue the remaining steps even
	// if the original HTTP client disconnects.
	completionCtx := context.WithoutCancel(ctx)
	job := ReturnJob{
		ReservationUID:    reservationUID,
		LibraryUID:        reservation.LibraryId,
		BookUID:           reservation.BookId,
		Username:          username,
		ReturnedCondition: condition,
		Expired:           completed.Status == domain.StatusExpired,
	}
	libraryCtx, cancelLibrary := context.WithTimeout(completionCtx, 10*time.Second)
	book, err := useCase.libraries.GetBookByUID(libraryCtx, reservation.LibraryId, reservation.BookId)
	cancelLibrary()
	if err != nil {
		job.Stage = ReturnStageRestoreBook
		return useCase.enqueueReturnJob(completionCtx, job, err)
	}
	job.OriginalCondition = book.Condition

	restoreCtx, cancelRestore := context.WithTimeout(completionCtx, 10*time.Second)
	err = useCase.libraries.ReturnBook(restoreCtx, reservation.LibraryId, reservation.BookId, condition)
	cancelRestore()
	if err != nil {
		job.Stage = ReturnStageRestoreBook
		return useCase.enqueueReturnJob(completionCtx, job, err)
	}
	job.Stage = ReturnStageUpdateRating
	ratingCtx, cancelRating := context.WithTimeout(completionCtx, 10*time.Second)
	_, err = useCase.ratings.AddStars(ratingCtx, username, job.RatingDelta())
	cancelRating()
	if err != nil {
		return useCase.enqueueReturnJob(completionCtx, job, err)
	}
	return nil
}

func (useCase *GatewayUseCase) enqueueReturnJob(ctx context.Context, job ReturnJob, cause error) error {
	if !IsRetryable(cause) {
		return cause
	}
	if useCase.returnQueue == nil {
		return fmt.Errorf("return operation cannot be deferred: queue is not configured: %w", cause)
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := useCase.returnQueue.Enqueue(enqueueCtx, job); err != nil {
		return fmt.Errorf("enqueue deferred return after %v: %w", cause, err)
	}
	log.Warn(
		"return operation deferred",
		"reservation_uid", job.ReservationUID,
		"stage", job.Stage,
		"error", cause,
	)
	return nil
}
