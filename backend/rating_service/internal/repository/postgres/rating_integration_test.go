package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"libriary_system/rating_service/internal/domain"
)

func TestRatingPGRepo(t *testing.T) {
	databaseURL := os.Getenv("RATING_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set RATING_TEST_DATABASE_URL to run PostgreSQL integration test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	defer pool.Close()
	repository := NewRatingPGRepo(pool)
	username := "test-" + uuid.NewString()
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DELETE FROM rating WHERE username = $1", username); err != nil {
			t.Errorf("delete test rating: %v", err)
		}
	}()

	if _, err := repository.GetByUsername(ctx, username); !errors.Is(err, domain.ErrRatingNotFound) {
		t.Fatalf("GetByUsername() error = %v, want ErrRatingNotFound", err)
	}
	if err := repository.Create(ctx, domain.Rating{Username: username, StarsCount: 50}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := repository.Create(ctx, domain.Rating{Username: username, StarsCount: 50}); !errors.Is(err, domain.ErrRatingAlreadyExists) {
		t.Fatalf("duplicate Create() error = %v, want ErrRatingAlreadyExists", err)
	}

	updated, err := repository.UpdateStars(ctx, domain.Rating{Username: username, StarsCount: 60})
	if err != nil || updated.StarsCount != 60 {
		t.Fatalf("UpdateStars() = (%+v, %v), want 60", updated, err)
	}

	const workers = 20
	var group sync.WaitGroup
	errorsCh := make(chan error, workers)
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := repository.AddStars(ctx, username, 1, 1, 100)
			errorsCh <- err
		}()
	}
	group.Wait()
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("concurrent AddStars() error = %v", err)
		}
	}

	got, err := repository.GetByUsername(ctx, username)
	if err != nil || got.StarsCount != 80 {
		t.Fatalf("GetByUsername() after concurrent updates = (%+v, %v), want 80", got, err)
	}
	got, err = repository.AddStars(ctx, username, 100, 1, 100)
	if err != nil || got.StarsCount != 100 {
		t.Fatalf("AddStars() upper bound = (%+v, %v), want 100", got, err)
	}
	got, err = repository.AddStars(ctx, username, -100, 1, 100)
	if err != nil || got.StarsCount != 1 {
		t.Fatalf("AddStars() lower bound = (%+v, %v), want 1", got, err)
	}
}
