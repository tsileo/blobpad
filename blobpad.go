package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dchest/blake2b"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"
	"github.com/nu7hatch/gouuid"
	"github.com/tsileo/blobsnap/clientutil"
	"github.com/tsileo/blobstash/client"
)

var (
	hclient         = &http.Client{}
	defaultAddr     = "localhost:8050"
	kvs             *client.KvStore
	bs              *client.BlobStore
	notebooksSetKey = "blobpad:notebooks"
	notesSetKey     = "blobpad:notes"
)

type Notebook struct {
	UUID      string `json:"id"`
	Name      string `json:"name"`
	CreatedAt int    `json:"created_at"`
}

type Note struct {
	UUID      string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	BodyRef   string `json:"-"`
	CreatedAt int    `json:"created_at,omitempty"`
	UpdatedAt int    `json:"updated_at,omitempty"`
	History   []struct {
		UpdatedAt int    `json:"updated_at"`
		Version   string `json:"version"`
	} `json:"history"`
	Notebook          string      `json:"notebook"`
	AttachmentContent string      `json:"attachment_content,omitempty"` // Only set for indexing
	AttachmentUUID    string      `json:"attachment_id"`                // Pointer to the Attachment uuid
	Attachment        *Attachment `json:"attachment,omitempty"`         // Only set for indexing
}

type Clip struct {
	Note
	URL string `json:"url"`
}

type Attachment struct {
	UUID       string `json:"id"`
	Ref        string `json:"ref"` // Pointer to the meta
	ContentRef string `json:"content_ref,omitempty"`
	Type       string `json:"type,omitempty"`
	Filename   string `json:"filename,omitempty"`
}

func NewNote(con redis.Conn, uuid string) (*Note, error) {
	n := &Note{}
	kv, err := kvs.Get(fmt.Sprintf("blobsnap:note:%v", uuid), -1)
	if err != nil {
		return nil, fmt.Errorf("Error fetching key %v: %v", fmt.Sprintf("blobsnap:note::%v", uuid), err)
	}
	if err := json.Decode(kv.Value, n); err != nil {
		return nil, err
	}
	//attachmentUUID, _ := cl.Get(con, fmt.Sprintf("n:%v:attachment_id", uuid))
	//if err != nil {
	//	return nil, fmt.Errorf("Error fetching AttachmentUUID: %v", err)
	//}
	//bodyRef, updatedAt, _ := cl.LlastWithIndex(con, fmt.Sprintf("n:%v:body", uuid))
	//if err != nil {
	//	return nil, fmt.Errorf("Error fetching history: %v", err)
	//}
	rkv, err := kvs.Get(fmt.Sprintf("blobsnap:noteref:%v", uuid), -1)
	//n.AttachmentUUID = rkv.Value
	return n, nil
}

func (n *Note) LoadAttachment(con redis.Conn) error {
	if n.AttachmentUUID == "" {
		return nil
	}
	n.Attachment = &Attachment{}
	//return cl.HscanStruct(con, fmt.Sprintf("a:%v", n.AttachmentUUID), n.Attachment)
	return nil
}

func (n *Note) Save() error {
	u, _ := uuid.NewV4()
	n.UUID = strings.Replace(u.String(), "-", "", -1)
	created := int(time.Now().UTC().Unix())
	//tx.Set(fmt.Sprintf("n:%v:created", n.UUID), strconv.Itoa(created))
	//tx.Set(fmt.Sprintf("n:%v:attachment_id", n.UUID), n.AttachmentUUID)
	//tx.Set(fmt.Sprintf("n:%v:notebook", n.UUID), n.Notebook)
	//tx.Ladd(fmt.Sprintf("n:%v:title", n.UUID), created, n.Title)
	if n.Body != "" {
		blobHash, err := UploadBlob([]byte(n.Body))
		if err != nil {
			return err
		}
		//tx.Ladd(fmt.Sprintf("n:%v:body", n.UUID), created, blobHash)
		n.BodyRef = blobHash
	}
	n.CreatedAt = int(created)
	n.UpdatedAt = n.CreatedAt
	return nil
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

// UploadBlob upload blobs to BlobDB
func UploadBlob(blob []byte) (blobHash string, err error) {
	blobHash = fmt.Sprintf("%x", blake2b.Sum256(blob))
	exists, err := bs.Stat(blobHash)
	if err != nil {
		return blobHash, err
	}
	if !exists {
		if err := bs.Put(blobHash, blob); err != nil {
			return blobHash, err
		}
	}
	return blobHash, nil
}

// indexHandler servers the index page
func indexHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "app.html")
}

// IndexDrop drops the elasticsearch blobpad index
func IndexDrop() error {
	request, err := http.NewRequest("DELETE", "http://localhost:9200/blobpad/", nil)
	if err != nil {
		return err
	}
	resp, err := hclient.Do(request)
	if err != nil {
		return err
	}
	if resp.StatusCode != 200 {
		var body *bytes.Buffer
		body.ReadFrom(resp.Body)
		resp.Body.Close()
		return fmt.Errorf("failed to drop index %v", body)
	}
	return nil
}

// IndexNote index the note in elasticsearch
func IndexNote(uuid string, note interface{}) error {
	js, _ := json.Marshal(note)
	var body bytes.Buffer
	body.Write(js)
	request, err := http.NewRequest("PUT", "http://localhost:9200/blobpad/notes/"+uuid, &body)
	if err != nil {
		return err
	}
	resp, err := hclient.Do(request)
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
	resp, err := hclient.Do(request)
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
	resp, err := hclient.Do(request)
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
	switch {
	case r.Method == "GET":
		notebooks := []*Notebook{}
		res, err := kvs.Keys("blobsnap:notebook:", "blobsnap:notebook:\xff", 0)
		if err != nil {
			panic(err)
		}
		nb := &Notebook{}
		for _, kv := range res {
			if err := json.Decode([]byte(kv.Value), nb); err != nil {
				panic(err)
			}
			notebooks = append(notebooks, nb)
		}
		WriteJSON(w, notebooks)
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
		var nb Notebook
		err := decoder.Decode(&nb)
		if err != nil {
			panic(err)
		}
		u, _ := uuid.NewV4()
		nb.UUID = strings.Replace(u.String(), "-", "", -1)
		fmt.Printf("%+v\n", nb)
		js, err := json.Encode(nb)
		if err != nil {
			panic(err)
		}
		if err := kvs.Put(fmt.Sprintf("blobsnap:notebook:%v", nb.UUID), js); err != nil {
			panic(err)
		}
		WriteJSON(w, nb)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func notesHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		notes := []*Note{}
		//notesUUIDs, _ := redis.Strings(con.Do("SMEMBERS", "nstest1"))
		q := QueryMatchAll
		if r.FormValue("query") != "" {
			q = map[string]interface{}{"query_string": map[string]interface{}{"query": r.FormValue("query")}}
		}
		if r.FormValue("notebook") != "" {
			q = map[string]interface{}{"filtered": map[string]interface{}{
				"filter": map[string]interface{}{"term": map[string]interface{}{"notebook": r.FormValue("notebook")}},
			}}
		}
		fmt.Printf("%+v", q)
		notesUUIDs, err := IndexQueryNote(q)
		fmt.Printf("%+v", notesUUIDs)
		if err != nil {
			fmt.Printf("err search %v", err)
		}
		for _, uuid := range notesUUIDs {
			n, _ := NewNote(uuid)
			notes = append(notes, n)
		}
		WriteJSON(w, notes)
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
		var n Note
		err := decoder.Decode(&n)
		if err != nil {
			panic(err)
		}
		n.Save()
		IndexNote(n.UUID, &n)
		WriteJSON(w, n)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func noteHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	switch {
	case r.Method == "GET":
		n, err := NewNote(vars["id"])
		if err != nil {
			panic(err)
		}
		if n.BodyRef != "" {
			blob, err := bs.Get(n.BodyRef)
			if err != nil {
				panic(err)
			}
			n.Body = string(blob)
		}
		if err := n.LoadAttachment(); err != nil {
			panic(err)
		}
		//if err := cl.LriterScanSlice(con, fmt.Sprintf("n:%v:body", vars["id"]), &n.History); err != nil {
		//	panic(err)
		//}
		// TODO handle history
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
		now := time.Now().UTC().Unix()
		//if n.Title != "" {
		//	con.Do("LADD", fmt.Sprintf("n:%v:title", vars["id"]), now, n.Title)
		//}
		// TODO handle title update
		if n.Body != "" {
			blobHash, err := UploadBlob([]byte(n.Body))
			if err != nil {
				panic(err)
			}
			if err := kvs.Put(fmt.Sprintf("blobsnap:noteref:%v", vars["id"]), blobHash, -1); err != nil {
				panic(err)
			}
			n.UpdatedAt = int(now)
		}
		IndexUpdateNote(&n)
		WriteJSON(w, &n)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func clipHandler(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST":
		decoder := json.NewDecoder(r.Body)
		var n Clip
		err := decoder.Decode(&n)
		if err != nil {
			panic(err)
		}
		u, _ := uuid.NewV4()
		n.UUID = strings.Replace(u.String(), "-", "", -1)
		ua, _ := uuid.NewV4()
		attachment := &Attachment{
			UUID:     strings.Replace(ua.String(), "-", "", -1),
			Ref:      n.URL,
			Filename: n.Title,
			Type:     "url",
		}
		n.AttachmentUUID = attachment.UUID
		n.Attachment = attachment
		// TODO save clip
		log.Printf("Clip: %+v", n)
		//IndexNote(n.UUID, &n)
		WriteJSON(w, n)
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func noteVersionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	switch {
	case r.Method == "GET":
		blob, err := bs.Get(vars["hash"])
		if err != nil {
			panic(err)
		}
		WriteJSON(w, map[string]string{"body": string(blob)})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
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
			n := Note{}
			tmp, _ := ioutil.TempFile("", "blobpad")
			if _, err := io.Copy(tmp, part); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			tmp.Close()

			fmt.Printf("received file %v", filename)
			cmd := exec.Command("pdftotext", tmp.Name(), tmp.Name()+".txt")
			defer os.Remove(tmp.Name())
			defer os.Remove(tmp.Name() + ".txt")
			_, err = cmd.CombinedOutput()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			text, err := ioutil.ReadFile(tmp.Name() + ".txt")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			textHash, err := UploadBlob(text)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			uploader := clientutil.NewUploader(bs, kvs)

			meta, wr, err := uploader.PutFile(tmp.Name())
			fmt.Printf("upload meta: %+v, wr: %v", meta, wr)
			if err != nil {
				fmt.Printf("Error put file %v", err)
			}
			fmt.Printf("upload notebook: %v", vars["notebook"])
			u, _ := uuid.NewV4()
			n.UUID = strings.Replace(u.String(), "-", "", -1)
			n.Notebook = vars["notebook"]
			n.Title = filepath.Base(filename)
			ua, _ := uuid.NewV4()
			attachment := &Attachment{
				UUID:       strings.Replace(ua.String(), "-", "", -1),
				Ref:        meta.Hash,
				Filename:   filepath.Base(filename),
				Type:       "pdf",
				ContentRef: textHash,
			}
			n.AttachmentContent = textHash
			n.AttachmentUUID = attachment.UUID
			n.Attachment = attachment
			// TODO save attachment
			if err := n.Save(); err != nil {
				panic(err)
			}
			IndexNote(n.UUID, &n)
			WriteJSON(w, &n)
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func pdfHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	note, err := NewNote(vars["id"])
	if err != nil {
		panic(err)
	}
	if err := note.LoadAttachment(); err != nil {
		panic(err)
	}
	//pdfRef, _ := redis.String(con.Do("GET", fmt.Sprintf("n:%v:pdf_ref", vars["id"])))
	meta, err := clientutil.NewMetaFromBlobStore(bs, note.Attchment.Ref)
	if err != nil {
		panic(err)
	}
	ffile := clientutil.NewFakeFile(bs, meta)
	defer ffile.Close()
	var buf bytes.Buffer
	io.Copy(&buf, ffile)
	if r.FormValue("dl") != "" {
		w.Header().Set("Content-Disposition", "attachment; filename="+note.Attachment.Filename)
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Write(buf.Bytes())
}

func reindexHandler(w http.ResponseWriter, r *http.Request) {
	con := cl.ConnWithCtx(blobPadCtx)
	defer con.Close()
	log.Printf("Reindexing...")
	if err := IndexDrop(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cnt := 0
	notesUUIDs, err := cl.Smembers(con, notesSetKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, uuid := range notesUUIDs {
		n, _ := NewNote(con, uuid)
		if n.BodyRef != "" {
			blob, err := cl.BlobStore.Get(blobPadCtx, n.BodyRef)
			if err != nil {
				panic(err)
			}
			n.Body = string(blob)
		}
		if err := n.LoadAttachment(con); err != nil {
			panic(err)
		}
		if n.AttachmentUUID != "" {
			contentBlob, err := cl.BlobStore.Get(blobPadCtx, n.Attachment.ContentRef)
			if err != nil {
				panic(err)
			}
			n.AttachmentContent = string(contentBlob)
		}
		IndexNote(n.UUID, n)
		cnt++
		log.Printf("Note %v indexed", uuid)
	}
	log.Printf("Reindexing done, %v notes indexed", cnt)
}

func main() {
	bs = client.NewBlobStore(defaultAddr)
	kvs = client.NewKvStore(defaultAddr)
	r := mux.NewRouter()
	r.HandleFunc("/", indexHandler)
	r.HandleFunc("/api/notebook", notebooksHandler)
	r.HandleFunc("/api/note", notesHandler)
	r.HandleFunc("/api/note/version/{hash}", noteVersionHandler)
	r.HandleFunc("/api/note/{id}", noteHandler)
	r.HandleFunc("/api/note/{id}/pdf", pdfHandler)
	r.HandleFunc("/api/upload/{notebook}", uploadHandler)
	r.HandleFunc("/api/clip", clipHandler)
	r.HandleFunc("/_reindex", reindexHandler)
	http.Handle("/public/", http.StripPrefix("/public", http.FileServer(http.Dir("public"))))
	http.Handle("/", r)
	log.Fatalf("%v", http.ListenAndServe(":8005", nil))
}
