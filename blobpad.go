package main

import (
	"encoding/json"
	"net/http"
	"log"

	"github.com/gorilla/mux"
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "app2.html")
}

type Notebook struct {
	Name string `json:"name"`
}

func notebooksHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		notebooks := []string{"notebook1"}
		js, _ := json.Marshal(notebooks)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
	    var t Notebook
	    err := decoder.Decode(&t)
	    if err != nil {
	        panic(err)
	    }
	    log.Printf("%v", t)
	    return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/api/notebook", notebooksHandler)
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
