package store

import (
	"github.com/gepfrad/gepfradprint/internal/model"
	"sync"
)

type Printers struct {
	mu sync.RWMutex
	m  map[string]model.Printer
}

func NewPrinters() *Printers            { return &Printers{m: map[string]model.Printer{}} }
func (s *Printers) Put(p model.Printer) { s.mu.Lock(); s.m[p.ID] = p; s.mu.Unlock() }
func (s *Printers) Get(id string) (model.Printer, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.m[id]
	return p, ok
}
func (s *Printers) List() []model.Printer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	o := make([]model.Printer, 0, len(s.m))
	for _, p := range s.m {
		o = append(o, p)
	}
	return o
}
func (s *Printers) Remove(id string) { s.mu.Lock(); delete(s.m, id); s.mu.Unlock() }
