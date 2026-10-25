package http

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
)

func TestReservationFallbackContainsOnlyUIDs(t *testing.T) {
	libraryUID, bookUID := uuid.New(), uuid.New()
	response := toReservationResponse(usecase.ReservationDetails{
		Library: domain.Library{Id: libraryUID},
		Book:    domain.LibraryBook{Book: domain.Book{Id: bookUID}},
	})
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Book    map[string]any `json:"book"`
		Library map[string]any `json:"library"`
	}
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Book) != 1 || got.Book["bookUid"] != bookUID.String() || len(got.Library) != 1 || got.Library["libraryUid"] != libraryUID.String() {
		t.Fatalf("fallback contains extra fields or wrong IDs: %s", encoded)
	}
}
