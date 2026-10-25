package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"

	"github.com/google/uuid"
	"github.com/gorilla/mux"

	"libriary_system/library_service/internal/domain"
	"libriary_system/library_service/internal/usecase"
	"libriary_system/shared/log"
	"libriary_system/shared/pagination"
)

const (
	defaultPageLimit   = 10
	maxRequestBodySize = 1 << 20
)

type LibraryUseCase interface {
	ListLibrariesByCity(ctx context.Context, city string, pageToken pagination.PageToken) (pagination.Page[domain.Library], error)
	ListBooksByLibrary(ctx context.Context, libraryUID uuid.UUID, showAll bool, pageToken pagination.PageToken) (pagination.Page[domain.LibraryBook], error)
	GetLibraryByUID(ctx context.Context, libraryUID uuid.UUID) (domain.Library, error)
	GetBookByUID(ctx context.Context, libraryUID uuid.UUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReserveBook(ctx context.Context, libraryUID uuid.UUID, bookUID uuid.UUID) (domain.LibraryBook, error)
	ReturnBook(ctx context.Context, libraryUID uuid.UUID, bookUID uuid.UUID, condition domain.BookCondition) error
}

type Handler struct {
	libraries LibraryUseCase
}

func NewHandler(libraries LibraryUseCase) *Handler {
	return &Handler{libraries: libraries}
}

func (handler *Handler) ListLibraries(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	pageToken, ok := parsePageToken(w, request)
	if !ok {
		return
	}

	page, err := handler.libraries.ListLibrariesByCity(
		request.Context(),
		request.URL.Query().Get("city"),
		pageToken,
	)
	if err != nil {
		handler.writeError(w, err)
		return
	}

	writeJSON(w, stdhttp.StatusOK, toLibraryPageResponse(page))
}

func (handler *Handler) GetLibrary(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, ok := parseUUIDPathParameter(w, request, "libraryUid")
	if !ok {
		return
	}

	library, err := handler.libraries.GetLibraryByUID(request.Context(), libraryUID)
	if err != nil {
		handler.writeError(w, err)
		return
	}

	writeJSON(w, stdhttp.StatusOK, toLibraryResponse(library))
}

func (handler *Handler) ListBooks(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, ok := parseUUIDPathParameter(w, request, "libraryUid")
	if !ok {
		return
	}
	pageToken, ok := parsePageToken(w, request)
	if !ok {
		return
	}
	showAll, ok := parseShowAll(w, request)
	if !ok {
		return
	}

	page, err := handler.libraries.ListBooksByLibrary(
		request.Context(),
		libraryUID,
		showAll,
		pageToken,
	)
	if err != nil {
		handler.writeError(w, err)
		return
	}

	writeJSON(w, stdhttp.StatusOK, toLibraryBookPageResponse(page))
}

func (handler *Handler) GetBook(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, bookUID, ok := parseBookUIDs(w, request)
	if !ok {
		return
	}

	book, err := handler.libraries.GetBookByUID(request.Context(), libraryUID, bookUID)
	if err != nil {
		handler.writeError(w, err)
		return
	}

	writeJSON(w, stdhttp.StatusOK, toLibraryBookResponse(book))
}

func (handler *Handler) ReserveBook(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, bookUID, ok := parseBookUIDs(w, request)
	if !ok {
		return
	}

	book, err := handler.libraries.ReserveBook(request.Context(), libraryUID, bookUID)
	if err != nil {
		handler.writeError(w, err)
		return
	}

	writeJSON(w, stdhttp.StatusOK, toLibraryBookResponse(book))
}

func (handler *Handler) ReturnBook(w stdhttp.ResponseWriter, request *stdhttp.Request) {
	libraryUID, bookUID, ok := parseBookUIDs(w, request)
	if !ok {
		return
	}

	var body returnBookRequest
	if err := decodeJSON(w, request, &body); err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "invalid request body"})
		return
	}

	if err := handler.libraries.ReturnBook(
		request.Context(),
		libraryUID,
		bookUID,
		body.Condition,
	); err != nil {
		handler.writeError(w, err)
		return
	}

	w.WriteHeader(stdhttp.StatusNoContent)
}

func (handler *Handler) writeError(w stdhttp.ResponseWriter, err error) {
	var validationErr *usecase.ValidationError
	switch {
	case errors.As(err, &validationErr):
		writeJSON(w, stdhttp.StatusBadRequest, validationErrorResponse{
			Message: validationErr.Error(),
			Errors:  validationErr.Fields,
		})
	case errors.Is(err, domain.ErrLibraryNotFound), errors.Is(err, domain.ErrBookNotFound):
		writeJSON(w, stdhttp.StatusNotFound, errorResponse{Message: err.Error()})
	case errors.Is(err, domain.ErrBookUnavailable):
		writeJSON(w, stdhttp.StatusConflict, errorResponse{Message: err.Error()})
	default:
		log.Error("library request failed", "error", err)
		writeJSON(w, stdhttp.StatusInternalServerError, errorResponse{Message: "internal server error"})
	}
}

func parsePageToken(
	w stdhttp.ResponseWriter,
	request *stdhttp.Request,
) (pagination.PageToken, bool) {
	limit, err := parseIntegerQueryParameter(request, "limit", defaultPageLimit)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: err.Error()})
		return pagination.PageToken{}, false
	}
	offset, err := parseIntegerQueryParameter(request, "offset", 0)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: err.Error()})
		return pagination.PageToken{}, false
	}

	return pagination.PageToken{Limit: limit, Offset: offset}, true
}

func parseIntegerQueryParameter(
	request *stdhttp.Request,
	name string,
	defaultValue int,
) (int, error) {
	rawValue := request.URL.Query().Get(name)
	if rawValue == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(rawValue)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer", name)
	}

	return value, nil
}

func parseShowAll(w stdhttp.ResponseWriter, request *stdhttp.Request) (bool, bool) {
	rawValue := request.URL.Query().Get("showAll")
	if rawValue == "" {
		return false, true
	}
	value, err := strconv.ParseBool(rawValue)
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{Message: "showAll must be a boolean"})
		return false, false
	}

	return value, true
}

func parseBookUIDs(
	w stdhttp.ResponseWriter,
	request *stdhttp.Request,
) (uuid.UUID, uuid.UUID, bool) {
	libraryUID, ok := parseUUIDPathParameter(w, request, "libraryUid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	bookUID, ok := parseUUIDPathParameter(w, request, "bookUid")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}

	return libraryUID, bookUID, true
}

func parseUUIDPathParameter(
	w stdhttp.ResponseWriter,
	request *stdhttp.Request,
	name string,
) (uuid.UUID, bool) {
	value, err := uuid.Parse(mux.Vars(request)[name])
	if err != nil {
		writeJSON(w, stdhttp.StatusBadRequest, errorResponse{
			Message: name + " must be a valid UUID",
		})
		return uuid.Nil, false
	}

	return value, true
}

func decodeJSON(w stdhttp.ResponseWriter, request *stdhttp.Request, target any) error {
	request.Body = stdhttp.MaxBytesReader(w, request.Body, maxRequestBodySize)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}

	return nil
}

func writeJSON(w stdhttp.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
