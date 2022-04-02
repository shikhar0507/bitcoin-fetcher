package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func SendResponseToClient(w http.ResponseWriter, staus int, body interface{}) {
	w.Header().Add("Content-Type", "application/json")
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", "OPTIONS,GET")
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")

	byteData, err := json.Marshal(&body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "Try again later")
		return
	}
	w.WriteHeader(staus)
	fmt.Fprintf(w, string(byteData))
}

func SendErrorResponseToClient(w http.ResponseWriter, status int, message string) {

	resp := map[string]interface{}{
		"message": message,
		"status":  status,
	}

	SendResponseToClient(w, status, resp)
}

func HandleOptions(w http.ResponseWriter, methods string) {
	w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Methods", methods)
	w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
}
