package usecase

import (
	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
)

type ReturnStage string

const (
	ReturnStageRestoreBook  ReturnStage = "RESTORE_BOOK"
	ReturnStageUpdateRating ReturnStage = "UPDATE_RATING"
)

// ReturnJob contains everything needed to finish an already accepted book
// return without depending on the original HTTP request context.
type ReturnJob struct {
	ReservationUID    uuid.UUID
	LibraryUID        uuid.UUID
	BookUID           uuid.UUID
	Username          string
	OriginalCondition domain.BookCondition
	ReturnedCondition domain.BookCondition
	Expired           bool
	Stage             ReturnStage
}

func (job ReturnJob) RatingDelta() int {
	delta := 0
	if job.Expired {
		delta -= 10
	}
	if job.ReturnedCondition != job.OriginalCondition {
		delta -= 10
	}
	if delta == 0 {
		return 1
	}
	return delta
}
