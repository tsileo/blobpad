package main

import (
	"crypto/sha1"
	"encoding/json"
	"net/http"
	"fmt"
	"time"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"bytes"
	"strconv"
	"strings"
	"path/filepath"

	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"
	"github.com/tsileo/blobstash/client/blobstore"
	"github.com/tsileo/blobstash/client"
)

var (
	defaultAddr = ":9735"
	bctx = &blobstore.Ctx{Namespace: "blobpad"}
	ctx = &client.Ctx{Namespace: "blobpad"}
	cl *client.Client
)

func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "app.html")
}

type Notebook struct {
	UUID string `json:"id"`
	Name string `json:"name"`
}

type Note struct {
	UUID string `json:"id"`
	Title string `json:"title"`
	Body string `json:"body"`
	PdfRef string `json:"pdf_ref"`
	PdfContent string `json:"pdf_content,omitempty"`
	PdfContentRef string `json:"pdf_content_ref,omitempty"`
	PdfFilename string `json:"pdf_filename,omitempty"`
	CreatedAt int `json:"created_at"`
	UpdatedAt int `json:"updated_at"`
	History []struct{
		UpdatedAt int `json:"updated_at"`
		Version string `json:"version"`
	} `json:"history"`
	Notebook string `json:"notebook"`
}

// IndexNote index the note in elasticsearch
func IndexNote(note *Note) error {
	js, _ := json.Marshal(note)
	var body bytes.Buffer
	body.Write(js)
	request, err := http.NewRequest("PUT", "http://localhost:9200/blobpad/notes/"+note.UUID, &body)
	if err != nil {
	  	return err
	}
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
	 	return err
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
	  	return fmt.Errorf("failed to index note %+v %v", note, body)
	}
    return nil
}

// IndexUpdateNote update the note in elasticsearch index
func IndexUpdateNote(note *Note) error {
	data := map[string]map[string]interface{}{"doc": map[string]interface{}{}}
	if note.Title != "" {
		data["doc"]["title"] = note.Title
	}
	if note.Body != "" {
		data["doc"]["body"] = note.Body
		data["doc"]["updated_at"] = note.UpdatedAt
	}
	js, _ := json.Marshal(data)
	var body bytes.Buffer
	body.Write(js)
	request, err := http.NewRequest("POST", "http://localhost:9200/blobpad/notes/"+note.UUID+"/_update", &body)
	if err != nil {
	  	return err
	}
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
	 	return err
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
	  	return fmt.Errorf("failed to index note %+v %v", note, body)
	}
    return nil
}

// IndexQueryNote performs an elasticsearch query and returns the list of notes UUID
func IndexQueryNote(query map[string]interface{}) ([]string, error) {
	notes := []string{}
	data := map[string]interface{}{"query": query}
	data["sort"] = []map[string]interface{}{map[string]interface{}{"updated_at": map[string]interface{}{"order": "desc"}}}
	js, _ := json.Marshal(data)
	var body bytes.Buffer
	body.Write(js)
	request, err := http.NewRequest("POST", "http://localhost:9200/blobpad/notes/_search", &body)
	if err != nil {
	  	return nil, err
	}
	client := &http.Client{}
	resp, err := client.Do(request)
	if err != nil {
	 	return nil, err
	}
	body.Reset()
	body.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 {
	  	return nil, fmt.Errorf("query failed %v", body.String())
	}
	var f map[string]interface{}
	if err := json.Unmarshal(body.Bytes(), &f); err != nil {
		return nil, err
	}
	hits := f["hits"].(map[string]interface{})["hits"].([]interface{})
	for _, hit := range hits {
		notes = append(notes, hit.(map[string]interface{})["_id"].(string))
	}
    return notes, nil
}

// Default query
var QueryMatchAll = map[string]interface{}{"match_all": map[string]interface{}{}}

func notebooksHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(ctx)
	defer con.Close()	
	switch {
	case r.Method == "GET":
        notebooks := []*Notebook{}
		notebooksUUIDs, _ := redis.Strings(con.Do("SMEMBERS", "nbstest1151"))
		for _, UUID := range notebooksUUIDs {
			title, _ := redis.String(con.Do("LLAST", fmt.Sprintf("nb:%v:title", UUID)))
			notebooks = append(notebooks, &Notebook{UUID: UUID, Name: title})
		}
		WriteJSON(w, notebooks)
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
	    var t Notebook
	    err := decoder.Decode(&t)
	    if err != nil {
	        panic(err)
	    }
		u, _ := uuid.NewV4()
	    t.UUID = strings.Replace(u.String(), "-", "", -1)
	    fmt.Printf("%+v\n", t)
	    con.Do("TXINIT", "blobpad")
	    con.Do("SADD", "nbstest1151", t.UUID)
	    con.Do("SET", fmt.Sprintf("nb:%v:created", t.UUID), time.Now().UTC().Unix())
	    // TODO a mattr cmd
	    // 1 arg => get
	    // 2 arg => set with current timestamp
	    con.Do("LADD", fmt.Sprintf("nb:%v:title", t.UUID), time.Now().UTC().Unix(), t.Name)
	    con.Do("TXCOMMIT")
		WriteJSON(w, t)
	    return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func WriteJSON(w http.ResponseWriter, data interface{}) {
	js, err := json.Marshal(data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}

func notesHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(ctx)
	defer con.Close()	
	switch {
	case r.Method == "GET":
        notes := []*Note{}
		//notesUUIDs, _ := redis.Strings(con.Do("SMEMBERS", "nstest1"))
		q := QueryMatchAll
		if r.FormValue("notebook") != "" {
			q = map[string]interface{}{"filtered": map[string]interface{}{
				"filter": map[string]interface{}{"term": map[string]interface{}{"notebook": r.FormValue("notebook")}},
			}}
		}
		fmt.Printf("%+v", q)
		notesUUIDs, err := IndexQueryNote(q)
		if err != nil {
			fmt.Printf("err search %v", err)
		}
		for _, UUID := range notesUUIDs {
			title, _ := redis.String(con.Do("LLAST", fmt.Sprintf("n:%v:title", UUID)))
			created, _ := redis.Int(con.Do("GET", fmt.Sprintf("n:%v:created", UUID)))
			notebook, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:notebook", UUID)))
			bodyData, _ := redis.Strings(con.Do("LLAST", fmt.Sprintf("n:%v:body", UUID), "WITH", "INDEX"))
			n := &Note{
				UUID: UUID,
				Title: title,
				CreatedAt: created,
				Notebook: notebook,
			}
			notes = append(notes, n)
			if len(bodyData) == 2 {
				n.UpdatedAt, _ = strconv.Atoi(bodyData[0])
			}
			
		}
		WriteJSON(w, notes)
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
	    var n Note
	    err := decoder.Decode(&n)
	    if err != nil {
	        panic(err)
	    }
		u, _ := uuid.NewV4()
	    n.UUID = strings.Replace(u.String(), "-", "", -1)
	    con.Do("TXINIT", "blobpad")
	    con.Do("SADD", "nstest1", n.UUID)
	    created := time.Now().UTC().Unix()
	    con.Do("SET", fmt.Sprintf("n:%v:created", n.UUID), created)
	    con.Do("SET", fmt.Sprintf("n:%v:notebook", n.UUID), n.Notebook)
	    con.Do("LADD", fmt.Sprintf("n:%v:title", n.UUID), 0, "")
	    con.Do("LADD", fmt.Sprintf("n:%v:title", n.UUID), time.Now().UTC().Unix(), n.Title)
	    con.Do("LADD", fmt.Sprintf("n:%v:body", n.UUID), 0, "")	
	    con.Do("TXCOMMIT")
	    n.CreatedAt = int(created)
	    IndexNote(&n)
	    WriteJSON(w, n)
	    return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func noteHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(ctx)
	defer con.Close()
	vars := mux.Vars(r)
	switch {
	case r.Method == "GET":
		created, _ := redis.Int(con.Do("GET", fmt.Sprintf("n:%v:created", vars["id"])))
		notebook, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:notebook", vars["id"])))
		pdfRef, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_ref", vars["id"])))
		pdfFilename, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_filename", vars["id"])))
		pdfContentRef, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_content_ref", vars["id"])))
		title, _ := redis.String(con.Do("LLAST", fmt.Sprintf("n:%v:title", vars["id"])))
		n := &Note{
			UUID: vars["id"],
			Title: title,
			CreatedAt: created,
			Notebook: notebook,
			PdfFilename: pdfFilename,
			PdfRef: pdfRef,
			PdfContentRef: pdfContentRef,
		}
		bodyData, _ := redis.Strings(con.Do("LLAST", fmt.Sprintf("n:%v:body", vars["id"]), "WITH", "INDEX"))
		if len(bodyData) == 2 {
			n.UpdatedAt, _ = strconv.Atoi(bodyData[0])
			blob, err := blobstore.GetBlob(bctx, bodyData[1])
			if err != nil {
				panic(err)
			}
			n.Body = string(blob)
		}
		values, _ := redis.Values(con.Do("LRITER", fmt.Sprintf("n:%v:body", vars["id"]), "WITH", "RANGE"))
		redis.ScanSlice(values, &n.History)
		WriteJSON(w, n)
	case r.Method == "PUT":
		decoder := json.NewDecoder(r.Body)
	    var n Note
	    err := decoder.Decode(&n)
	    if err != nil {
	        panic(err)
	    }
	    if vars["id"] == "" {
	    	panic("missing note id")
	    }
	    n.UUID = vars["id"]
	    con.Do("TXINIT", "blobpad")
	    if n.Title != "" {
	    	con.Do("LADD", fmt.Sprintf("n:%v:title", vars["id"]), time.Now().UTC().Unix(), n.Title)
	    }
	    if n.Body != "" {
	    	blobHash, err := UploadBlob([]byte(n.Body))
	    	if err != nil {
	    		panic(err)
	    	}
			updatedAt := time.Now().UTC().Unix()
	    	con.Do("LADD", fmt.Sprintf("n:%v:body", vars["id"]), updatedAt, blobHash)
	    	n.UpdatedAt = int(updatedAt)
	    }
	    con.Do("TXCOMMIT")
	    IndexUpdateNote(&n)
	    WriteJSON(w, &n)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func UploadBlob(blob []byte) (blobHash string, err error) {
	h := sha1.New()
	h.Write(blob)
	blobHash = fmt.Sprintf("%x", h.Sum(nil))
	exists, err := blobstore.StatBlob(bctx, blobHash)
	if err != nil {
		return blobHash, err
	}
	if !exists {
		if err := blobstore.PutBlob(bctx, blobHash, blob); err != nil {
			return blobHash, err
		}
	}
	return blobHash, nil
}

func noteVersionHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(ctx)
	defer con.Close()
	vars := mux.Vars(r)
	switch {
	case r.Method == "GET":
		blob, err := blobstore.GetBlob(bctx, vars["hash"])
		if err != nil {
			panic(err)
		}
		WriteJSON(w, map[string]string{"body": string(blob)})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func noteSearchHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(ctx)
	defer con.Close()
	switch {
	case r.Method == "POST":
		js, _ := json.Marshal([]struct{
			Value string  `json:"value"`
			Title string  `json:"title"`
		}{{"OK", "OK"}})
		w.Header().Set("Content-Type", "application/json")
		fmt.Printf("%+v", string(js))
		w.Write(js)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	con := cl.ConnWithCtx(ctx)
	defer con.Close()
	switch r.Method {

	//POST takes the uploaded file(s) and saves it to disk.
	case "POST":
		//parse the multipart form in the request
		mr, err := r.MultipartReader()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			filename := part.FileName()
			n := Note{
				PdfFilename: filepath.Base(filename),
			}

			tmp, _ := ioutil.TempFile("", "blobpad")
			if _, err := io.Copy(tmp, part); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			tmp.Close()
 
			fmt.Printf("received file %v", filename)
			cmd := exec.Command("pdftotext", tmp.Name(), tmp.Name()+".txt")
			defer os.Remove(tmp.Name())
			defer os.Remove(tmp.Name()+".txt")
			_, err = cmd.CombinedOutput()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			text, err := ioutil.ReadFile(tmp.Name()+".txt")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			textHash, err := UploadBlob(text)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			n.PdfContentRef = textHash
			n.PdfContent = string(text)
			snap, _, _, err := cl.Put(&client.Ctx{Namespace: "blobpad", Archive: true}, tmp.Name())
			if err != nil {
				fmt.Printf("Error put file %v", err)
			}
			fmt.Printf("upload notebook: %v", vars["notebook"])
			n.PdfRef = snap.Ref
	   		u, _ := uuid.NewV4()
	   		n.UUID = strings.Replace(u.String(), "-", "", -1)
		    n.Notebook = vars["notebook"]
		    n.Title = filepath.Base(filename)
	    	con.Do("TXINIT", "blobpad")
		    con.Do("SADD", "nstest1", n.UUID)
		    created := time.Now().UTC().Unix()
		    con.Do("SET", fmt.Sprintf("n:%v:created", n.UUID), created)
		    con.Do("SET", fmt.Sprintf("n:%v:notebook", n.UUID), n.Notebook)
		    con.Do("SET", fmt.Sprintf("n:%v:pdf_ref", n.UUID), n.PdfRef)
		    con.Do("SET", fmt.Sprintf("n:%v:pdf_filename", n.UUID), n.PdfFilename)
		    con.Do("SET", fmt.Sprintf("n:%v:pdf_content_ref", n.UUID), n.PdfContentRef)
		    con.Do("LADD", fmt.Sprintf("n:%v:title", n.UUID), 0, "")
		    con.Do("LADD", fmt.Sprintf("n:%v:title", n.UUID), time.Now().UTC().Unix(), n.Title)
		    con.Do("LADD", fmt.Sprintf("n:%v:body", n.UUID), 0, "")	
		    con.Do("TXCOMMIT")
		    n.CreatedAt = int(created)
		    IndexNote(&n)
		    WriteJSON(w, &n)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func pdfHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	con := cl.ConnWithCtx(ctx)
	defer con.Close()
	pdfRef, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_ref", vars["id"])))
	meta, err := client.NewMetaFromDB(con, pdfRef)
	if err != nil {
		panic(err)
	}
	ffile := client.NewFakeFile(cl, ctx, meta.Ref, meta.Size)
	defer ffile.Close()
	var buf bytes.Buffer
	io.Copy(&buf, ffile)
	if r.FormValue("dl") != "" {
		pdfFilename, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_filename", vars["id"])))
		w.Header().Set("Content-Disposition", "attachment; filename="+pdfFilename)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Write(buf.Bytes())
}

func main() {
	client, err := client.NewClient("blobpad", defaultAddr, []string{})
	if err != nil {
		panic(err)
	}
	cl = client
	defer cl.Close()
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/api/notebook", notebooksHandler)
	r.HandleFunc("/api/note", notesHandler)
	r.HandleFunc("/api/note/version/{hash}", noteVersionHandler)
	r.HandleFunc("/api/note/search", noteSearchHandler)
	r.HandleFunc("/api/note/{id}", noteHandler)
	r.HandleFunc("/api/note/{id}/pdf", pdfHandler)
	r.HandleFunc("/api/upload/{notebook}", uploadHandler)
	//r.HandleFunc("/_reindex", reindexHandler)
	http.Handle("/public/", http.StripPrefix("/public", http.FileServer(http.Dir("public"))))
	http.Handle("/", r)
	http.ListenAndServe(":8000", nil)
}
