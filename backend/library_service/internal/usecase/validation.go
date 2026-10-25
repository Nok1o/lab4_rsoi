package usecase

import (
	"libriary_system/library_service/internal/domain"
	"libriary_system/shared/pagination"

	"github.com/google/uuid"
)

const maxPageSize = 100

type ValidationError struct {
	Fields map[string]string
}

func (err *ValidationError) Error() string {
	return "validation failed"
}

func validateListLibraries(city string, pageToken pagination.PageToken) error {
	fields := pagination.ValidatePageToken(pageToken)
	if city == "" {
		fields["city"] = "must not be empty"
	}

	return validationError(fields)
}

func validateListBooks(libraryUID uuid.UUID, pageToken pagination.PageToken) error {
	fields := pagination.ValidatePageToken(pageToken)
	if libraryUID == uuid.Nil {
		fields["libraryUid"] = "must not be empty"
	}

	return validationError(fields)
}

func validateUIDs(libraryUID, bookUID uuid.UUID) error {
	return validationError(validateUIDFields(libraryUID, bookUID))
}

func validateLibraryUID(libraryUID uuid.UUID) error {
	fields := make(map[string]string)
	if libraryUID == uuid.Nil {
		fields["libraryUid"] = "must not be empty"
	}

	return validationError(fields)
}

func validateUIDFields(libraryUID, bookUID uuid.UUID) map[string]string {
	fields := make(map[string]string)
	if libraryUID == uuid.Nil {
		fields["libraryUid"] = "must not be empty"
	}
	if bookUID == uuid.Nil {
		fields["bookUid"] = "must not be empty"
	}

	return fields
}

func validateReturnBook(
	libraryUID uuid.UUID,
	bookUID uuid.UUID,
	condition domain.BookCondition,
) error {
	fields := validateUIDFields(libraryUID, bookUID)
	if !isBookConditionValid(condition) {
		fields["condition"] = "must be one of EXCELLENT, GOOD or BAD"
	}

	return validationError(fields)
}

func isBookConditionValid(condition domain.BookCondition) bool {
	switch condition {
	case domain.ConditionExcellent, domain.ConditionGood, domain.ConditionBad:
		return true
	default:
		return false
	}
}

func validationError(fields map[string]string) error {
	if len(fields) == 0 {
		return nil
	}

	return &ValidationError{Fields: fields}
}
