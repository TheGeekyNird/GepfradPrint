package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/gepfrad/gepfradprint/internal/model"
)

type State struct {
	Printers       []model.Printer `json:"printers"`
	DefaultPrinter string          `json:"default_printer"`
}
type Store struct {
	mu    sync.RWMutex
	path  string
	state State
}

func New() (*Store, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, "GepfradPrint")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	s := &Store{path: filepath.Join(dir, "state.json")}
	_ = s.load()
	return s, nil
}
func (s *Store) load() error {
	b, e := os.ReadFile(s.path)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, &s.state)
}
func (s *Store) saveLocked() error {
	b, e := json.MarshalIndent(s.state, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path)
}
func (s *Store) Printers() []model.Printer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]model.Printer, len(s.state.Printers))
	copy(out, s.state.Printers)
	return out
}
func (s *Store) Upsert(p model.Printer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.state.Printers {
		if s.state.Printers[i].ID == p.ID {
			s.state.Printers[i] = p
			return s.saveLocked()
		}
	}
	s.state.Printers = append(s.state.Printers, p)
	return s.saveLocked()
}
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.state.Printers[:0]
	for _, p := range s.state.Printers {
		if p.ID != id {
			out = append(out, p)
		}
	}
	s.state.Printers = out
	if s.state.DefaultPrinter == id {
		s.state.DefaultPrinter = ""
	}
	return s.saveLocked()
}
func (s *Store) SetDefault(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.DefaultPrinter = id
	return s.saveLocked()
}
func (s *Store) Default() string { s.mu.RLock(); defer s.mu.RUnlock(); return s.state.DefaultPrinter }
