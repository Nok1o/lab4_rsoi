package http

import (
	stdhttp "net/http"

	"github.com/gorilla/mux"
)

func NewRouter(handler *Handler) stdhttp.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/libraries", handler.ListLibrariesByCity).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/libraries/{libraryUid}/books", handler.ListBooksByLibrary).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/reservations", handler.ListReservations).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/reservations", handler.TakeBook).Methods(stdhttp.MethodPost)
	router.HandleFunc("/api/v1/reservations/{reservationUid}/return", handler.ReturnBook).Methods(stdhttp.MethodPost)
	router.HandleFunc("/api/v1/rating", handler.GetRating).Methods(stdhttp.MethodGet)
	router.HandleFunc("/manage/health", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writeJSON(w, stdhttp.StatusOK, map[string]string{"status": "ok"})
	}).Methods(stdhttp.MethodGet)
	return router
}
