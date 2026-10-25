package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"libriary_system/reservation_service/internal/domain"
)

type memoryReservationRepo struct {
	items map[uuid.UUID]domain.Reservation
}

func (repo *memoryReservationRepo) ListByUsername(_ context.Context, username string) ([]domain.Reservation, error) {
	items := make([]domain.Reservation, 0)
	for _, reservation := range repo.items {
		if reservation.Username == username {
			items = append(items, reservation)
		}
	}
	return items, nil
}

func (repo *memoryReservationRepo) CountByUsernameAndStatus(_ context.Context, username string, status domain.ReservationStatus) (int, error) {
	count := 0
	for _, reservation := range repo.items {
		if reservation.Username == username && reservation.Status == status {
			count++
		}
	}
	return count, nil
}

func (repo *memoryReservationRepo) GetByUIDAndUsername(_ context.Context, reservationUID uuid.UUID, username string) (domain.Reservation, error) {
	reservation, exists := repo.items[reservationUID]
	if !exists || reservation.Username != username {
		return domain.Reservation{}, domain.ErrReservationNotFound
	}
	return reservation, nil
}

func (repo *memoryReservationRepo) Create(_ context.Context, reservation domain.Reservation) error {
	repo.items[reservation.Id] = reservation
	return nil
}

func (repo *memoryReservationRepo) CompleteRental(_ context.Context, reservationUID uuid.UUID, status domain.ReservationStatus) error {
	reservation, exists := repo.items[reservationUID]
	if !exists || reservation.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	reservation.Status = status
	repo.items[reservationUID] = reservation
	return nil
}

func (repo *memoryReservationRepo) Cancel(_ context.Context, uid uuid.UUID, username string) error {
	item, err := repo.GetByUIDAndUsername(context.Background(), uid, username)
	if err != nil {
		return err
	}
	if item.Status != domain.StatusRented {
		return domain.ErrReservationNotRented
	}
	delete(repo.items, uid)
	return nil
}

func TestCancelChecksOwnerAndStatus(t *testing.T) {
	repo := &memoryReservationRepo{items: make(map[uuid.UUID]domain.Reservation)}
	useCase := NewReservationUseCase(repo)
	ctx := context.Background()
	item, err := useCase.Rent(ctx, "alice", uuid.New(), uuid.New(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := useCase.Cancel(ctx, item.Id, "bob"); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("wrong owner: %v", err)
	}
	if err := useCase.Cancel(ctx, uuid.Nil, "alice"); !errors.Is(err, ErrInvalidReservation) {
		t.Fatalf("invalid UID: %v", err)
	}
	if err := useCase.Cancel(ctx, item.Id, ""); !errors.Is(err, ErrInvalidReservation) {
		t.Fatalf("invalid username: %v", err)
	}
	if err := useCase.Cancel(ctx, item.Id, "alice"); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 0 {
		t.Fatal("cancelled reservation remains")
	}
	item.Status = domain.StatusReturned
	repo.items[item.Id] = item
	if err := useCase.Cancel(ctx, item.Id, "alice"); !errors.Is(err, domain.ErrReservationNotRented) {
		t.Fatalf("completed reservation: %v", err)
	}
	if len(repo.items) != 1 {
		t.Fatal("completed reservation was removed")
	}
}

func TestRentAndReturn(t *testing.T) {
	for _, check := range []struct {
		name       string
		returnDate string
		wantStatus domain.ReservationStatus
	}{
		{"on time", "2026-09-23", domain.StatusReturned},
		{"late", "2026-09-24", domain.StatusExpired},
	} {
		t.Run(check.name, func(t *testing.T) {
			repo := &memoryReservationRepo{items: make(map[uuid.UUID]domain.Reservation)}
			useCase := NewReservationUseCase(repo)
			useCase.now = func() time.Time { return time.Date(2026, 9, 20, 14, 30, 0, 0, time.UTC) }
			tillDate := mustDate(t, "2026-09-23")
			reservation, err := useCase.Rent(context.Background(), "alice", uuid.New(), uuid.New(), tillDate)
			if err != nil || reservation.Status != domain.StatusRented || reservation.Id == uuid.Nil {
				t.Fatalf("Rent() = (%+v, %v)", reservation, err)
			}
			if reservation.StartDate.Day() != 20 || !reservation.TillDate.Equal(tillDate) {
				t.Fatalf("wrong rental dates: %+v", reservation)
			}
			count, err := repo.CountByUsernameAndStatus(context.Background(), "alice", domain.StatusRented)
			if err != nil || count != 1 {
				t.Fatalf("rented count = %d, %v", count, err)
			}

			if _, err := useCase.Return(context.Background(), reservation.Id, "other", mustDate(t, check.returnDate)); !errors.Is(err, domain.ErrReservationNotFound) {
				t.Fatalf("another user Return() error = %v", err)
			}
			returned, err := useCase.Return(context.Background(), reservation.Id, "alice", mustDate(t, check.returnDate))
			if err != nil || returned.Status != check.wantStatus {
				t.Fatalf("Return() = (%+v, %v), want %s", returned, err, check.wantStatus)
			}
			if _, err := useCase.Return(context.Background(), reservation.Id, "alice", mustDate(t, check.returnDate)); !errors.Is(err, domain.ErrReservationNotRented) {
				t.Fatalf("second Return() error = %v", err)
			}
		})
	}
}

func TestRentRejectsInvalidInput(t *testing.T) {
	repo := &memoryReservationRepo{items: make(map[uuid.UUID]domain.Reservation)}
	useCase := NewReservationUseCase(repo)
	useCase.now = func() time.Time { return mustDate(t, "2026-09-20") }
	for _, check := range []struct {
		username   string
		libraryUID uuid.UUID
		bookUID    uuid.UUID
		tillDate   time.Time
	}{
		{"", uuid.New(), uuid.New(), mustDate(t, "2026-09-21")},
		{"alice", uuid.Nil, uuid.New(), mustDate(t, "2026-09-21")},
		{"alice", uuid.New(), uuid.Nil, mustDate(t, "2026-09-21")},
	} {
		if _, err := useCase.Rent(context.Background(), check.username, check.libraryUID, check.bookUID, check.tillDate); !errors.Is(err, ErrInvalidReservation) {
			t.Fatalf("Rent() error = %v, want ErrInvalidReservation", err)
		}
	}
	if len(repo.items) != 0 {
		t.Fatal("invalid rentals must not be saved")
	}
}

func mustDate(t *testing.T, date string) time.Time {
	t.Helper()
	value, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
