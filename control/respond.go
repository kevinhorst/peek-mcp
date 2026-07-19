package control

import "net/http"

func respondBadRequest(message string, w http.ResponseWriter) {
	http.Error(w, message, http.StatusBadRequest)
}

func respondNotFound(message string, w http.ResponseWriter) {
	http.Error(w, message, http.StatusNotFound)
}

func respondInternalServerError(err error, w http.ResponseWriter) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
