package queue

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
)

// Serialization belongs to the Redis adapter; usecase models have no JSON tags.
type jobDTO struct {
	ReservationUID    uuid.UUID            `json:"reservationUid"`
	LibraryUID        uuid.UUID            `json:"libraryUid"`
	BookUID           uuid.UUID            `json:"bookUid"`
	Username          string               `json:"username"`
	OriginalCondition domain.BookCondition `json:"originalCondition"`
	ReturnedCondition domain.BookCondition `json:"returnedCondition"`
	Expired           bool                 `json:"expired"`
	Stage             usecase.ReturnStage  `json:"stage"`
}

func encodeJob(job usecase.ReturnJob) (string, error) {
	dto := jobDTO{job.ReservationUID, job.LibraryUID, job.BookUID, job.Username,
		job.OriginalCondition, job.ReturnedCondition, job.Expired, job.Stage}
	if err := dto.validate(); err != nil {
		return "", err
	}
	payload, err := json.Marshal(dto)
	return string(payload), err
}

func decodeJob(value any) (usecase.ReturnJob, error) {
	payload, ok := value.(string)
	if !ok {
		return usecase.ReturnJob{}, errors.New("missing return job payload")
	}
	var dto jobDTO
	if err := json.Unmarshal([]byte(payload), &dto); err != nil {
		return usecase.ReturnJob{}, fmt.Errorf("decode return job: %w", err)
	}
	if err := dto.validate(); err != nil {
		return usecase.ReturnJob{}, err
	}
	return usecase.ReturnJob{ReservationUID: dto.ReservationUID, LibraryUID: dto.LibraryUID,
		BookUID: dto.BookUID, Username: dto.Username, OriginalCondition: dto.OriginalCondition,
		ReturnedCondition: dto.ReturnedCondition, Expired: dto.Expired, Stage: dto.Stage}, nil
}

func (dto jobDTO) validate() error {
	validCondition := func(value domain.BookCondition) bool {
		return value == domain.ConditionExcellent || value == domain.ConditionGood || value == domain.ConditionBad
	}
	if dto.ReservationUID == uuid.Nil || dto.LibraryUID == uuid.Nil || dto.BookUID == uuid.Nil || dto.Username == "" || !validCondition(dto.ReturnedCondition) {
		return errors.New("invalid return job fields")
	}
	if dto.Stage != usecase.ReturnStageRestoreBook && dto.Stage != usecase.ReturnStageUpdateRating {
		return fmt.Errorf("unknown return stage %q", dto.Stage)
	}
	if dto.Stage == usecase.ReturnStageUpdateRating && !validCondition(dto.OriginalCondition) {
		return errors.New("rating job must contain original book condition")
	}
	return nil
}
