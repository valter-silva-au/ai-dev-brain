package scheduler

import (
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// stateStore persists per-job State to a YAML file. Writes are atomic
// (temp file + rename). Safe for concurrent use.
type stateStore struct {
	path   string
	mu     sync.Mutex
	states map[string]*State
}

func newStateStore(path string) *stateStore {
	return &stateStore{
		path:   path,
		states: make(map[string]*State),
	}
}

func (s *stateStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == "" {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file stateFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("parsing scheduler state: %w", err)
	}
	for i := range file.Jobs {
		st := file.Jobs[i]
		s.states[st.Name] = &st
	}
	return nil
}

func (s *stateStore) update(name string, mutator func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.states[name]
	if !ok {
		st = &State{Name: name}
		s.states[name] = st
	}
	mutator(st)
	_ = s.writeLocked()
}

// Snapshot returns a deep-ish copy of all current states. Useful for
// `adb scheduler list`.
func (s *stateStore) Snapshot() []State {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]State, 0, len(s.states))
	for _, v := range s.states {
		out = append(out, *v)
	}
	return out
}

type stateFile struct {
	Jobs []State `yaml:"jobs"`
}

func (s *stateStore) writeLocked() error {
	if s.path == "" {
		return nil
	}
	file := stateFile{Jobs: make([]State, 0, len(s.states))}
	for _, v := range s.states {
		file.Jobs = append(file.Jobs, *v)
	}
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// LoadStates reads a state file from path. Returns an empty slice if the
// file doesn't exist.
func LoadStates(path string) ([]State, error) {
	s := newStateStore(path)
	if err := s.load(); err != nil {
		return nil, err
	}
	return s.Snapshot(), nil
}
