package main

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "app2.html")
}

func notebooksHandler(w http.ResponseWriter, r *http.Request) {
	notebooks := []string{"notebook1"}
	js, _ := json.Marshal(notebooks)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/api/notebooks", notebooksHandler)
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
