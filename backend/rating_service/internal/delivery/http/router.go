package http

import (
	stdhttp "net/http"

	"github.com/gorilla/mux"
)

func NewRouter(handler *Handler) stdhttp.Handler {
	router := mux.NewRouter()
	router.HandleFunc("/api/v1/rating", handler.GetRating).Methods(stdhttp.MethodGet)
	router.HandleFunc("/api/v1/rating", handler.CreateRating).Methods(stdhttp.MethodPost)
	router.HandleFunc("/api/v1/rating", handler.UpdateRating).Methods(stdhttp.MethodPut)
	router.HandleFunc("/api/v1/rating", handler.AddStars).Methods(stdhttp.MethodPatch)
	router.HandleFunc("/manage/health", func(w stdhttp.ResponseWriter, _ *stdhttp.Request) {
		writeJSON(w, stdhttp.StatusOK, map[string]string{"status": "ok"})
	}).Methods(stdhttp.MethodGet)
	return router
}
