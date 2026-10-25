package http

import (
	"encoding/json"
	stdhttp "net/http"
	"sort"

	"github.com/google/uuid"

	"libriary_system/gateway_service/internal/domain"
	"libriary_system/gateway_service/internal/usecase"
	"libriary_system/shared/pagination"
)

const dateLayout = "2006-01-02"

type errorResponse struct {
	Message string             `json:"message"`
	Errors  []ErrorDescription `json:"errors,omitempty"`
}

type ErrorDescription struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

func newErrorResponse() *errorResponse {
	return &errorResponse{}
}

func (response *errorResponse) message(message string) *errorResponse {
	response.Message = message
	return response
}

func (response *errorResponse) addField(field, description string) *errorResponse {
	response.Errors = append(response.Errors, ErrorDescription{Field: field, Error: description})
	return response
}

func (response *errorResponse) writeResponse(w stdhttp.ResponseWriter, statusCode int) {
	writeJSON(w, statusCode, response)
}

func validationResponse(fields map[string]string) *errorResponse {
	response := newErrorResponse().message("Bad Request")
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		response.addField(key, fields[key])
	}
	return response
}

type pageResponse[T any] struct {
	Page          int   `json:"page"`
	PageSize      int   `json:"pageSize"`
	TotalElements int64 `json:"totalElements"`
	Items         []T   `json:"items"`
}

type libraryResponse struct {
	UID     uuid.UUID `json:"libraryUid"`
	Name    string    `json:"name,omitempty"`
	Address string    `json:"address,omitempty"`
	City    string    `json:"city,omitempty"`
}

type libraryBookResponse struct {
	UID            uuid.UUID            `json:"bookUid"`
	Name           string               `json:"name"`
	Author         string               `json:"author"`
	Genre          string               `json:"genre"`
	Condition      domain.BookCondition `json:"condition"`
	AvailableCount int                  `json:"availableCount"`
}

type bookInfoResponse struct {
	UID    uuid.UUID `json:"bookUid"`
	Name   string    `json:"name,omitempty"`
	Author string    `json:"author,omitempty"`
	Genre  string    `json:"genre,omitempty"`
}

type ratingResponse struct {
	Stars int `json:"stars"`
}

type reservationResponse struct {
	UID       uuid.UUID                `json:"reservationUid"`
	Status    domain.ReservationStatus `json:"status"`
	StartDate string                   `json:"startDate"`
	TillDate  string                   `json:"tillDate"`
	Book      bookInfoResponse         `json:"book"`
	Library   libraryResponse          `json:"library"`
	Rating    *ratingResponse          `json:"rating,omitempty"`
}

type takeBookRequest struct {
	BookUID    uuid.UUID `json:"bookUid"`
	LibraryUID uuid.UUID `json:"libraryUid"`
	TillDate   string    `json:"tillDate"`
}

type returnBookRequest struct {
	Condition domain.BookCondition `json:"condition"`
	Date      string               `json:"date"`
}

func toLibraryResponse(library domain.Library) libraryResponse {
	return libraryResponse{UID: library.Id, Name: library.Name, Address: library.Address, City: library.City}
}

func toLibraryPage(page int, source pagination.Page[domain.Library]) pageResponse[libraryResponse] {
	items := make([]libraryResponse, 0, len(source.Items))
	for _, library := range source.Items {
		items = append(items, toLibraryResponse(library))
	}
	return pageResponse[libraryResponse]{Page: page, PageSize: len(items), TotalElements: source.Total, Items: items}
}

func toLibraryBookResponse(book domain.LibraryBook) libraryBookResponse {
	return libraryBookResponse{
		UID: book.Id, Name: book.Name, Author: book.Author, Genre: book.Genre,
		Condition: book.Condition, AvailableCount: book.AvailableCount,
	}
}

func toBookPage(page int, source pagination.Page[domain.LibraryBook]) pageResponse[libraryBookResponse] {
	items := make([]libraryBookResponse, 0, len(source.Items))
	for _, book := range source.Items {
		items = append(items, toLibraryBookResponse(book))
	}
	return pageResponse[libraryBookResponse]{Page: page, PageSize: len(items), TotalElements: source.Total, Items: items}
}

func toReservationResponse(details usecase.ReservationDetails) reservationResponse {
	response := reservationResponse{
		UID: details.Reservation.Id, Status: details.Reservation.Status,
		StartDate: details.Reservation.StartDate.Format(dateLayout),
		TillDate:  details.Reservation.TillDate.Format(dateLayout),
		Book: bookInfoResponse{
			UID: details.Book.Id, Name: details.Book.Name,
			Author: details.Book.Author, Genre: details.Book.Genre,
		},
		Library: toLibraryResponse(details.Library),
	}
	if details.Rating != nil {
		response.Rating = &ratingResponse{Stars: details.Rating.StarsCount}
	}
	return response
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
