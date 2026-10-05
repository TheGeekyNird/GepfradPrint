package api

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gepfrad/gepfradprint/internal/discovery"
	"github.com/gepfrad/gepfradprint/internal/ipp"
	"github.com/gepfrad/gepfradprint/internal/model"
	"github.com/gepfrad/gepfradprint/internal/printjob"
	"github.com/gepfrad/gepfradprint/internal/render"
	"github.com/gepfrad/gepfradprint/internal/store"
)

type Server struct {
	mux      *http.ServeMux
	printers *store.Printers
	persist  *store.Store
	queue    *printjob.Queue
}

func New(s *store.Store) *Server {
	ps := store.NewPrinters()
	for _, p := range s.Printers() {
		ps.Put(p)
	}
	srv := &Server{mux: http.NewServeMux(), printers: ps, persist: s}
	srv.queue = printjob.New(ps)
	srv.routes()
	return srv
}

func (s *Server) routes() {
	s.mux.HandleFunc("/api/printers", s.printersHandler)
	s.mux.HandleFunc("/api/discover", s.discover)
	s.mux.HandleFunc("/api/diagnostics", s.diagnostics)
	s.mux.HandleFunc("/api/queue", s.queueHandler)
	s.mux.HandleFunc("/api/queue/", s.queueAction)
	s.mux.HandleFunc("/api/print", s.print)
	s.mux.Handle("/", http.FileServer(webFS()))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	s.mux.ServeHTTP(w, r)
}

func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) printersHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonOut(w, s.printers.List())
	case http.MethodDelete:
		id := r.URL.Query().Get("id")
		s.printers.Remove(id)
		_ = s.persist.Remove(id)
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPost:
		var p model.Printer
		if json.NewDecoder(r.Body).Decode(&p) != nil {
			http.Error(w, "bad printer", 400)
			return
		}
		if p.ID == "" {
			p.ID = fmt.Sprintf("%s:%d/%s", p.Host, p.Port, p.Protocol)
		}
		s.printers.Put(p)
		_ = s.persist.Upsert(p)
		jsonOut(w, p)
	default:
		w.WriteHeader(405)
	}
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	ps, e := discovery.Discover(r.Context())
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	for _, p := range ps {
		s.printers.Put(p)
		_ = s.persist.Upsert(p)
	}
	jsonOut(w, ps)
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := r.URL.Query().Get("id")
	p, ok := s.printers.Get(id)
	if !ok {
		http.Error(w, "unknown printer", http.StatusNotFound)
		return
	}
	d, _ := ipp.TestConnection(r.Context(), ipp.FromPrinter(p))
	jsonOut(w, map[string]any{
		"printer":     p,
		"diagnostics": d,
	})
}

func (s *Server) queueHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(405)
		return
	}
	jsonOut(w, s.queue.Jobs())
}

func (s *Server) queueAction(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/queue/")
	switch r.Method {
	case http.MethodDelete:
		jsonOut(w, map[string]any{"ok": s.queue.Cancel(id)})
	case http.MethodPost:
		action := r.URL.Query().Get("action")
		var ok bool
		if action == "pause" {
			ok = s.queue.Pause(id)
		} else if action == "resume" {
			ok = s.queue.Resume(id)
		}
		jsonOut(w, map[string]any{"ok": ok})
	default:
		w.WriteHeader(405)
	}
}

func (s *Server) print(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		http.Error(w, e.Error(), 400)
		return
	}
	defer f.Close()
	data, e := io.ReadAll(f)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	id := r.FormValue("printer_id")
	p, ok := s.printers.Get(id)
	if !ok {
		http.Error(w, "unknown printer", 404)
		return
	}
	copies, _ := strconv.Atoi(r.FormValue("copies"))
	if copies < 1 {
		copies = 1
	}
	color := r.FormValue("color") != "false"
	duplex := r.FormValue("duplex") == "true"
	media := r.FormValue("media")
	orientation := r.FormValue("orientation")
	mimeType := mime.TypeByExtension(filepath.Ext(h.Filename))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	j := s.queue.Submit(p, render.Document{Data: data, Name: h.Filename, MIME: mimeType}, model.PrintSettings{Copies: copies, Color: color, Duplex: duplex, Media: media, Orientation: orientation})
	jsonOut(w, j)
}
