package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/reservation_service/internal/domain"
)

func TestReservationPGRepo(t *testing.T) {
	databaseURL := os.Getenv("RESERVATION_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set RESERVATION_TEST_DATABASE_URL to run PostgreSQL integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()
	repository := NewReservationPGRepo(pool)
	startDate := time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC)
	reservation := domain.Reservation{
		Id: uuid.New(), Username: "test-" + uuid.NewString(),
		BookId: uuid.New(), LibraryId: uuid.New(),
		Status: domain.StatusRented, StartDate: startDate,
		TillDate: startDate.AddDate(0, 0, 2),
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM reservation WHERE reservation_uid = $1", reservation.Id); err != nil {
			t.Errorf("delete test reservation: %v", err)
		}
	}()

	if err := repository.Create(ctx, reservation); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	items, err := repository.ListByUsername(ctx, reservation.Username)
	if err != nil || len(items) != 1 || items[0].Id != reservation.Id {
		t.Fatalf("ListByUsername() = (%+v, %v)", items, err)
	}
	count, err := repository.CountByUsernameAndStatus(ctx, reservation.Username, domain.StatusRented)
	if err != nil || count != 1 {
		t.Fatalf("CountByUsernameAndStatus() = (%d, %v)", count, err)
	}
	if _, err := repository.GetByUIDAndUsername(ctx, reservation.Id, "another-user"); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("wrong owner error = %v", err)
	}
	got, err := repository.GetByUIDAndUsername(ctx, reservation.Id, reservation.Username)
	if err != nil || got.Status != domain.StatusRented || got.BookId != reservation.BookId {
		t.Fatalf("GetByUIDAndUsername() = (%+v, %v)", got, err)
	}
	if err := repository.CompleteRental(ctx, reservation.Id, domain.StatusReturned); err != nil {
		t.Fatalf("CompleteRental() error = %v", err)
	}
	if err := repository.CompleteRental(ctx, reservation.Id, domain.StatusExpired); !errors.Is(err, domain.ErrReservationNotRented) {
		t.Fatalf("repeated CompleteRental() error = %v", err)
	}
	got, err = repository.GetByUIDAndUsername(ctx, reservation.Id, reservation.Username)
	if err != nil || got.Status != domain.StatusReturned {
		t.Fatalf("GetByUIDAndUsername() after return = (%+v, %v)", got, err)
	}
	if err := repository.Cancel(ctx, reservation.Id, reservation.Username); !errors.Is(err, domain.ErrReservationNotRented) {
		t.Fatalf("completed reservation cancellation = %v", err)
	}
	active := reservation
	active.Id = uuid.New()
	if err := repository.Create(ctx, active); err != nil {
		t.Fatal(err)
	}
	defer pool.Exec(context.Background(), "DELETE FROM reservation WHERE reservation_uid = $1", active.Id)
	if err := repository.Cancel(ctx, active.Id, "other-user"); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("wrong owner cancellation = %v", err)
	}
	if err := repository.Cancel(ctx, active.Id, active.Username); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.GetByUIDAndUsername(ctx, active.Id, active.Username); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("cancelled reservation still exists: %v", err)
	}
	if err := repository.Cancel(ctx, active.Id, active.Username); !errors.Is(err, domain.ErrReservationNotFound) {
		t.Fatalf("missing reservation cancellation = %v", err)
	}
}
