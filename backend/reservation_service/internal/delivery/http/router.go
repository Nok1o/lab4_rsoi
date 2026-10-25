package http

import (
	stdhttp "net/http"

	"github.com/gorilla/mux"
)

func NewRouter(handler *Handler) stdhttp.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/reservations", handler.ListReservations).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/reservations", handler.Rent).Methods(stdhttp.MethodPost)
	router.HandleFunc("/api/v1/reservations/count", handler.CountReservations).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/reservations/{reservationUid}", handler.GetReservation).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/reservations/{reservationUid}", handler.Cancel).Methods(stdhttp.MethodDelete)
	router.HandleFunc("/api/v1/reservations/{reservationUid}/return", handler.Return).Methods(stdhttp.MethodPost)
	router.HandleFunc("/manage/health", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writeJSON(w, stdhttp.StatusOK, map[string]string{"status": "ok"})
	}).Methods(stdhttp.MethodGet)
	return router
}
