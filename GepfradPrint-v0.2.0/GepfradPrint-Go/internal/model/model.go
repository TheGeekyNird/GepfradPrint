package model

import "time"

type Printer struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Host         string            `json:"host"`
	Address      string            `json:"address,omitempty"`
	Port         int               `json:"port"`
	URI          string            `json:"uri"`
	Protocol     string            `json:"protocol"`
	RP           string            `json:"rp,omitempty"`
	DiscoveredAt time.Time         `json:"discovered_at"`
	TXT          map[string]string `json:"txt,omitempty"`
	Capabilities Capabilities      `json:"capabilities"`
}

type Capabilities struct {
	Color         bool     `json:"color"`
	Duplex        bool     `json:"duplex"`
	PDF           bool     `json:"pdf"`
	PostScript    bool     `json:"postscript"`
	PCL           bool     `json:"pcl"`
	Media         []string `json:"media,omitempty"`
	DocumentTypes []string `json:"document_types,omitempty"`
}

type PrintSettings struct {
	Copies      int    `json:"copies"`
	Media       string `json:"media"`
	Orientation string `json:"orientation"`
	Color       bool   `json:"color"`
	Duplex      bool   `json:"duplex"`
}

type JobState string

const (
	Queued    JobState = "queued"
	Printing  JobState = "printing"
	Paused    JobState = "paused"
	Completed JobState = "completed"
	Failed    JobState = "failed"
	Canceled  JobState = "canceled"
)

type PrintJob struct {
	ID         string        `json:"id"`
	PrinterID  string        `json:"printer_id"`
	FileName   string        `json:"file_name"`
	Data       []byte        `json:"-"`
	MediaType  string        `json:"media_type"`
	Settings   PrintSettings `json:"settings"`
	State      JobState      `json:"state"`
	Progress   int           `json:"progress"`
	Error      string        `json:"error,omitempty"`
	CreatedAt  time.Time     `json:"created_at"`
	FinishedAt *time.Time    `json:"finished_at,omitempty"`
}
