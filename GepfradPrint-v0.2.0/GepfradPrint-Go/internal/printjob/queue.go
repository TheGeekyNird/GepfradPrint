package printjob

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gepfrad/gepfradprint/internal/ipp"
	"github.com/gepfrad/gepfradprint/internal/model"
	"github.com/gepfrad/gepfradprint/internal/raw"
	"github.com/gepfrad/gepfradprint/internal/render"
)

type PrinterSource interface {
	Get(id string) (model.Printer, bool)
}

type Queue struct {
	mu       sync.RWMutex
	jobs     map[string]*model.PrintJob
	cancel   map[string]context.CancelFunc
	printers PrinterSource
}

func New(p PrinterSource) *Queue {
	return &Queue{jobs: map[string]*model.PrintJob{}, cancel: map[string]context.CancelFunc{}, printers: p}
}

func (q *Queue) Jobs() []model.PrintJob {
	q.mu.RLock()
	defer q.mu.RUnlock()
	out := make([]model.PrintJob, 0, len(q.jobs))
	for _, j := range q.jobs {
		c := *j
		c.Data = nil
		out = append(out, c)
	}
	return out
}

func (q *Queue) Submit(p model.Printer, d render.Document, s model.PrintSettings) *model.PrintJob {
	id := time.Now().Format("20060102150405.000000000")
	j := &model.PrintJob{ID: id, PrinterID: p.ID, FileName: d.Name, Data: d.Data, MediaType: d.MIME, Settings: s, State: model.Queued, CreatedAt: time.Now()}
	ctx, cancel := context.WithCancel(context.Background())
	q.mu.Lock()
	q.jobs[id] = j
	q.cancel[id] = cancel
	q.mu.Unlock()
	go q.run(ctx, j, p)
	return j
}

func (q *Queue) run(ctx context.Context, j *model.PrintJob, p model.Printer) {
	q.setState(j.ID, model.Printing, 5, "")
	data, mime, err := render.Prepare(render.Document{Data: j.Data, Name: j.FileName, MIME: j.MediaType})
	if err != nil {
		q.setState(j.ID, model.Failed, 0, "render: "+err.Error())
		return
	}
	q.setState(j.ID, model.Printing, 20, "")
	j.Data = nil

	var e error
	switch p.Protocol {
	case "raw":
		q.setState(j.ID, model.Printing, 35, "sending raw TCP data")
		e = raw.Send(ctx, p.Host, p.Port, data)
	case "lpr":
		q.setState(j.ID, model.Printing, 35, "sending LPR job")
		e = errors.New("LPR transport is discovered but not implemented in this build")
	case "ipps", "ipp":
		q.setState(j.ID, model.Printing, 35, "connecting to printer")
		resp, err := ipp.PrintJobEndpoint(ctx, ipp.FromPrinter(p), j.FileName, mime, data, j.Settings.Copies, j.Settings.Media, j.Settings.Orientation, j.Settings.Color, j.Settings.Duplex)
		if err != nil {
			e = err
		} else if resp.Status != 0x0000 {
			e = fmt.Errorf("IPP status 0x%04x", resp.Status)
		}
	default:
		e = fmt.Errorf("unsupported printer protocol %q", p.Protocol)
	}
	if e != nil {
		if errors.Is(e, context.Canceled) {
			q.setState(j.ID, model.Canceled, j.Progress, "")
		} else {
			q.setState(j.ID, model.Failed, j.Progress, e.Error())
		}
		return
	}
	q.setState(j.ID, model.Printing, 90, "printer accepted job")
	now := time.Now()
	q.mu.Lock()
	j.State = model.Completed
	j.Progress = 100
	j.Error = ""
	j.FinishedAt = &now
	delete(q.cancel, j.ID)
	q.mu.Unlock()
}

func (q *Queue) setState(id string, s model.JobState, p int, e string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if j := q.jobs[id]; j != nil {
		j.State = s
		j.Progress = p
		j.Error = e
	}
}

func (q *Queue) Cancel(id string) bool {
	q.mu.RLock()
	c, ok := q.cancel[id]
	q.mu.RUnlock()
	if ok {
		c()
		return true
	}
	return false
}

func (q *Queue) Pause(id string) bool  { return false }
func (q *Queue) Resume(id string) bool { return false }
