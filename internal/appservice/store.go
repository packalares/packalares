package appservice

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

// AppStore persists installed app metadata to a JSON file.
// In a full Olares-compatible deployment this would be backed by Application CRDs,
// but for Packalares we use a local file store that tracks all the same fields.
type AppStore struct {
	mu       sync.RWMutex
	apps     map[string]*AppRecord
	filePath string
}

// AppRecord is an installed app's persisted state.
type AppRecord struct {
	Name            string                  `json:"name"`
	AppID           string                  `json:"appID"`
	Namespace       string                  `json:"namespace"`
	Owner           string                  `json:"owner"`
	Icon            string                  `json:"icon,omitempty"`
	Title           string                  `json:"title,omitempty"`
	Description     string                  `json:"description,omitempty"`
	Version         string                  `json:"version,omitempty"`
	ChartRef        string                  `json:"chartRef,omitempty"`
	RepoURL         string                  `json:"repoURL,omitempty"`
	Source          string                  `json:"source"`
	State           ApplicationManagerState `json:"state"`
	OpType          OpType                  `json:"opType,omitempty"`
	OpID            string                  `json:"opID,omitempty"`
	ReleaseName     string                  `json:"releaseName"`
	Entrances       []Entrance              `json:"entrances,omitempty"`
	SharedEntrances []SharedEntrance        `json:"sharedEntrances,omitempty"`
	Permission      *Permission             `json:"permission,omitempty"`
	Values          map[string]string       `json:"values,omitempty"`
	CreatedAt       time.Time               `json:"createdAt"`
	UpdatedAt       time.Time               `json:"updatedAt"`
	IsSysApp        bool                    `json:"isSysApp"`
	InternetBlocked bool                    `json:"internetBlocked,omitempty"`
	RawAppName      string                  `json:"rawAppName,omitempty"`
}

// NewAppStore creates or loads the store from disk.
func NewAppStore(dir string) (*AppStore, error) {
	fp := filepath.Join(dir, "appstate.json")
	store := &AppStore{
		apps:     make(map[string]*AppRecord),
		filePath: fp,
	}

	data, err := os.ReadFile(fp)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read app store: %w", err)
	}

	var records []*AppRecord
	if err := json.Unmarshal(data, &records); err != nil {
		klog.Warningf("corrupt app store %s, starting fresh: %v", fp, err)
		return store, nil
	}

	for _, r := range records {
		store.apps[r.Name] = r
	}

	klog.Infof("loaded %d apps from %s", len(store.apps), fp)
	return store, nil
}

// save writes current state to disk.
func (s *AppStore) save() error {
	records := make([]*AppRecord, 0, len(s.apps))
	for _, r := range s.apps {
		records = append(records, r)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.filePath)
	_ = os.MkdirAll(dir, 0755)

	return os.WriteFile(s.filePath, data, 0644)
}

// Put upserts an app record.
func (s *AppStore) Put(_ context.Context, rec *AppRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	rec.UpdatedAt = time.Now()
	s.apps[rec.Name] = rec
	return s.save()
}

// Get returns an app record by name.
func (s *AppStore) Get(_ context.Context, name string) (*AppRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.apps[name]
	return r, ok
}

// Delete removes an app record.
func (s *AppStore) Delete(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.apps, name)
	return s.save()
}

// List returns all app records.
func (s *AppStore) List(_ context.Context) []*AppRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*AppRecord, 0, len(s.apps))
	for _, r := range s.apps {
		result = append(result, r)
	}
	return result
}

// SetState updates the state and opType for an app.
func (s *AppStore) SetState(_ context.Context, name string, state ApplicationManagerState, op OpType) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.apps[name]
	if !ok {
		return fmt.Errorf("app %q not found", name)
	}

	r.State = state
	r.OpType = op
	r.OpID = strconv.FormatInt(time.Now().Unix(), 10)
	r.UpdatedAt = time.Now()

	return s.save()
}
