package usecase

import (
	"context"
	"errors"
	"testing"

	"libriary_system/rating_service/internal/domain"
	"libriary_system/shared/username"
)

func TestAddStarsDelegatesAtomicChange(t *testing.T) {
	for _, test := range []struct {
		name      string
		delta     int
		wantDelta int
	}{
		{name: "increase", delta: 1, wantDelta: 1},
		{name: "decrease", delta: -10, wantDelta: -10},
		{name: "large increase", delta: 1000, wantDelta: 100},
		{name: "large decrease", delta: -1000, wantDelta: -100},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository := &ratingRepositoryStub{
				addStars: func(
					_ context.Context,
					username string,
					delta, minimum, maximum int,
				) (domain.Rating, error) {
					if username != "reader" || delta != test.wantDelta ||
						minimum != minStars || maximum != maxStars {
						t.Fatalf("AddStars args = (%q, %d, %d, %d)", username, delta, minimum, maximum)
					}
					return domain.Rating{Username: username, StarsCount: 75}, nil
				},
			}

			got, err := NewRatingUseCase(repository).AddStars(
				context.Background(), "reader", test.delta,
			)
			if err != nil || got.StarsCount != 75 {
				t.Fatalf("AddStars() = (%+v, %v)", got, err)
			}
		})
	}
}

func TestGetByUsernameRejectsInvalidUsername(t *testing.T) {
	_, err := NewRatingUseCase(&ratingRepositoryStub{}).GetByUsername(context.Background(), " ")
	if !errors.Is(err, username.ErrInvalid) {
		t.Fatalf("GetByUsername() error = %v", err)
	}
}

func TestCreateAndUpdateValidateRating(t *testing.T) {
	useCase := NewRatingUseCase(&ratingRepositoryStub{})
	if err := useCase.Create(context.Background(), domain.Rating{
		Username: "reader", StarsCount: 0,
	}); !errors.Is(err, InvalidStarsCountErr) {
		t.Fatalf("Create() error = %v", err)
	}

	_, err := useCase.UpdateRating(context.Background(), domain.Rating{
		Username: "reader", StarsCount: 101,
	})
	if !errors.Is(err, InvalidStarsCountErr) {
		t.Fatalf("UpdateRating() error = %v", err)
	}
}

type ratingRepositoryStub struct {
	addStars func(context.Context, string, int, int, int) (domain.Rating, error)
}

func (stub *ratingRepositoryStub) GetByUsername(context.Context, string) (domain.Rating, error) {
	panic("unexpected GetByUsername")
}

func (stub *ratingRepositoryStub) Create(context.Context, domain.Rating) error {
	panic("unexpected Create")
}

func (stub *ratingRepositoryStub) UpdateStars(context.Context, domain.Rating) (domain.Rating, error) {
	panic("unexpected UpdateStars")
}

func (stub *ratingRepositoryStub) AddStars(
	ctx context.Context,
	username string,
	delta, minimum, maximum int,
) (domain.Rating, error) {
	return stub.addStars(ctx, username, delta, minimum, maximum)
}
