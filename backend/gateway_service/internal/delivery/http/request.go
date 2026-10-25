package http

import (
	"encoding/json"
	"errors"
	"io"
	stdhttp "net/http"
	"strconv"

	"libriary_system/shared/pagination"
)

const (
	defaultPageSize    = 10
	maxPageSize        = 100
	maxRequestBodySize = 1 << 20
)

func parsePage(w stdhttp.ResponseWriter, request *stdhttp.Request) (int, pagination.PageToken, bool) {
	page := 1
	if raw := request.URL.Query().Get("page"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			validationResponse(map[string]string{"page": "must be a positive integer"}).writeResponse(w, stdhttp.StatusBadRequest)
			return 0, pagination.PageToken{}, false
		}
		page = parsed
	}
	size := defaultPageSize
	if raw := request.URL.Query().Get("size"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maxPageSize {
			validationResponse(map[string]string{"size": "must be between 1 and 100"}).writeResponse(w, stdhttp.StatusBadRequest)
			return 0, pagination.PageToken{}, false
		}
		size = parsed
	}
	maxInt := int(^uint(0) >> 1)
	if page-1 > maxInt/size {
		validationResponse(map[string]string{"page": "is too large"}).writeResponse(w, stdhttp.StatusBadRequest)
		return 0, pagination.PageToken{}, false
	}
	return page, pagination.PageToken{Limit: size, Offset: (page - 1) * size}, true
}

func parseShowAll(w stdhttp.ResponseWriter, request *stdhttp.Request) (bool, bool) {
	raw := request.URL.Query().Get("showAll")
	if raw == "" {
		return false, true
	}
	showAll, err := strconv.ParseBool(raw)
	if err != nil {
		validationResponse(map[string]string{"showAll": "must be a boolean"}).writeResponse(w, stdhttp.StatusBadRequest)
		return false, false
	}
	return showAll, true
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
