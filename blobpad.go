package main

import (
	"encoding/json"
	"net/http"
	"fmt"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"
)

var defaultAddr = ":9735"
var pool *redis.Pool

func getPool(addr string) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     50,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", addr)
			if err != nil {
				return nil, err
			}
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "app2.html")
}

type Notebook struct {
	UUID string `json:"id"`
	Name string `json:"name"`
}

func notebooksHandler(w http.ResponseWriter, r *http.Request) {
	con := pool.Get()
	defer con.Close()	
	switch {
	case r.Method == "GET":
        notebooks := []*Notebook{}
		notebooksUUIDs, _ := redis.Strings(con.Do("SMEMBERS", "nbstest1"))
		for _, UUID := range notebooksUUIDs {
			title, _ := redis.String(con.Do("LLAST", fmt.Sprintf("nb:%v:title", UUID)))
			notebooks = append(notebooks, &Notebook{UUID: UUID, Name: title})
		}
		js, _ := json.Marshal(notebooks)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	case r.Method == "POST":
		// TODO handle PUT request
		decoder := json.NewDecoder(r.Body)
	    var t Notebook
	    err := decoder.Decode(&t)
	    if err != nil {
	        panic(err)
	    }
		u, _ := uuid.NewV4()
	    t.UUID = u.String()
	    con.Do("SADD", "nbstest1", t.UUID)
	    // TODO a mattr cmd
	    // 1 arg => get
	    // 2 arg => set with current timestamp
	    con.Do("LADD", fmt.Sprintf("nb:%v:title", t.UUID), time.Now().UTC().Unix(), t.Name)
	    return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func main() {
	pool = getPool(defaultAddr)
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/api/notebook", notebooksHandler)
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
