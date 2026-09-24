package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/valter-silva-au/ai-dev-brain/pkg/models"
	"gopkg.in/yaml.v3"
)

// FileMetricStore persists product/PMF metric nodes (decision D11) as
// workspace-level YAML at metrics/index.yaml. One entry per (initiative, name):
// re-recording UPDATES the current value in place (manual-entry-first; a gate
// reads the current value). It mirrors the other registries' conventions: an
// RWMutex, seeded-empty when missing, and the shared readFileOrEmpty/writeYAML
// helpers. Metrics participate in the graph via app.go's graph source, which
// turns each into a node with a part_of edge toward its initiative.
type FileMetricStore struct {
	path string
	mu   sync.RWMutex
	now  func() time.Time
}

// NewFileMetricStore creates a metric store rooted at basePath.
func NewFileMetricStore(basePath string) *FileMetricStore {
	return &FileMetricStore{
		path: filepath.Join(basePath, "metrics", "index.yaml"),
		now:  func() time.Time { return time.Now().UTC() },
	}
}

func (s *FileMetricStore) loadUnsafe() (models.MetricIndex, error) {
	data, err := readFileOrEmpty(s.path)
	if err != nil {
		return models.MetricIndex{}, fmt.Errorf("read metric index: %w", err)
	}
	if len(data) == 0 {
		return models.MetricIndex{}, nil
	}
	var idx models.MetricIndex
	if err := yaml.Unmarshal(data, &idx); err != nil {
		return models.MetricIndex{}, fmt.Errorf("parse metric index: %w", err)
	}
	return idx, nil
}

// Record inserts or updates the metric for its (initiative, name). It stamps
// Recorded (UTC) and defaults Source to "manual" when unset.
func (s *FileMetricStore) Record(m models.Metric) (models.Metric, error) {
	if err := m.Validate(); err != nil {
		return models.Metric{}, err
	}
	if strings.TrimSpace(m.Source) == "" {
		m.Source = "manual"
	}
	if m.Recorded.IsZero() {
		m.Recorded = s.now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return models.Metric{}, err
	}
	replaced := false
	for i := range idx.Metrics {
		if idx.Metrics[i].Initiative == m.Initiative && idx.Metrics[i].Name == m.Name {
			idx.Metrics[i] = m
			replaced = true
			break
		}
	}
	if !replaced {
		idx.Metrics = append(idx.Metrics, m)
	}
	if err := writeYAML(s.path, idx); err != nil {
		return models.Metric{}, err
	}
	return m, nil
}

// List returns every recorded metric.
func (s *FileMetricStore) List() ([]models.Metric, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return nil, err
	}
	return idx.Metrics, nil
}

// Get returns the current metric for (initiative, name). found is false (no
// error) when it has not been recorded.
func (s *FileMetricStore) Get(initiative, name string) (models.Metric, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	idx, err := s.loadUnsafe()
	if err != nil {
		return models.Metric{}, false, err
	}
	for _, m := range idx.Metrics {
		if m.Initiative == initiative && m.Name == name {
			return m, true, nil
		}
	}
	return models.Metric{}, false, nil
}
