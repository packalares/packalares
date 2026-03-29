package hfdownload

import (
	"encoding/json"
	"io"
	"os"
	"sync"
)

// ProgressStatus represents the current state of a download operation.
type ProgressStatus struct {
	Status    string `json:"status"`
	File      string `json:"file,omitempty"`
	Completed int64  `json:"completed,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Error     string `json:"error,omitempty"`
}

// ProgressReporter writes JSON-line progress messages to a writer.
type ProgressReporter struct {
	mu  sync.Mutex
	enc *json.Encoder
}

// NewProgressReporter creates a reporter that writes to stdout.
func NewProgressReporter() *ProgressReporter {
	return NewProgressReporterTo(os.Stdout)
}

// NewProgressReporterTo creates a reporter that writes to the given writer.
func NewProgressReporterTo(w io.Writer) *ProgressReporter {
	return &ProgressReporter{enc: json.NewEncoder(w)}
}

// Report writes a progress status as a JSON line.
func (p *ProgressReporter) Report(status ProgressStatus) {
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.enc.Encode(status)
}

// Downloading reports progress for a file download.
func (p *ProgressReporter) Downloading(file string, completed, total int64) {
	p.Report(ProgressStatus{
		Status:    "downloading",
		File:      file,
		Completed: completed,
		Total:     total,
	})
}

// Complete reports that all downloads finished.
func (p *ProgressReporter) Complete() {
	p.Report(ProgressStatus{Status: "complete"})
}

// Errorf reports an error.
func (p *ProgressReporter) Errorf(file, msg string) {
	p.Report(ProgressStatus{Status: "error", File: file, Error: msg})
}
