package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrPlanConflict  = errors.New("journal plan conflicts with existing plan")
	eventNamePattern = regexp.MustCompile(`^([0-9]{6})-(.+)\.json$`)
)

type Store struct {
	root                  string
	now                   func() time.Time
	newID                 func() string
	beforePublication     func(root *os.Root, sourceName string, targetName string) error
	publicationOperations rootedPublicationOperations
	mu                    sync.Mutex
}

func NewStore(
	root string,
	now func() time.Time,
	newID func() string,
) (*Store, error) {
	if root == "" {
		return nil, errors.New("journal root is required")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("journal root must be absolute: %q", root)
	}
	if now == nil {
		return nil, errors.New("journal clock is required")
	}
	if newID == nil {
		return nil, errors.New("journal id generator is required")
	}

	return &Store{
		root:  filepath.Clean(root),
		now:   now,
		newID: newID,
	}, nil
}

func (store *Store) ReadPlan(operationID string) (Plan, error) {
	if err := validateOperationID(operationID); err != nil {
		return Plan{}, err
	}
	journalRoot, err := store.openJournalRoot(false)
	if err != nil {
		return Plan{}, err
	}
	defer func() {
		_ = journalRoot.Close()
	}()
	operationRoot, err := openOperationRoot(
		journalRoot,
		operationID,
		false,
	)
	if err != nil {
		return Plan{}, err
	}
	defer func() {
		_ = operationRoot.Close()
	}()
	return readPlan(operationRoot, operationID)
}

func (store *Store) Begin(input Plan) (Plan, error) {
	if err := validatePlanInput(input); err != nil {
		return Plan{}, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	journalRoot, err := store.openJournalRoot(true)
	if err != nil {
		return Plan{}, err
	}
	defer func() {
		_ = journalRoot.Close()
	}()
	operationRoot, err := openOperationRoot(
		journalRoot,
		input.OperationID,
		true,
	)
	if err != nil {
		return Plan{}, err
	}
	defer func() {
		_ = operationRoot.Close()
	}()
	unlock, err := acquireRootedLock(operationRoot)
	if err != nil {
		return Plan{}, err
	}
	defer unlock()

	existing, err := readPlan(operationRoot, input.OperationID)
	if err == nil {
		if samePlanRequest(existing, input) {
			return existing, nil
		}
		return Plan{}, ErrPlanConflict
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Plan{}, err
	}

	created := input
	created.SchemaVersion = SchemaVersion
	if created.CreatedAt.IsZero() {
		created.CreatedAt = store.now().UTC()
	} else {
		created.CreatedAt = created.CreatedAt.UTC()
	}
	created.Hash = planHash(created)

	if err := writeRootedAtomic(
		operationRoot,
		"plan.json",
		0o644,
		store.beforePublication,
		store.publicationOperations,
		func(writer io.Writer) error {
			return encodeJSON(writer, created)
		},
	); err != nil {
		return Plan{}, fmt.Errorf("write journal plan: %w", err)
	}

	return created, nil
}

func (store *Store) Append(
	operationID string,
	input EventInput,
) (Event, error) {
	if err := validateOperationID(operationID); err != nil {
		return Event{}, err
	}
	if !validPhase(input.Phase) {
		return Event{}, fmt.Errorf("unsupported journal phase %q", input.Phase)
	}
	if (input.Phase == PhaseStepApplying ||
		input.Phase == PhaseStepApplied) &&
		input.Step < 1 {
		return Event{}, errors.New("journal step phase requires a positive step")
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	journalRoot, err := store.openJournalRoot(false)
	if err != nil {
		return Event{}, err
	}
	defer func() {
		_ = journalRoot.Close()
	}()
	operationRoot, err := openOperationRoot(
		journalRoot,
		operationID,
		false,
	)
	if err != nil {
		return Event{}, err
	}
	defer func() {
		_ = operationRoot.Close()
	}()
	unlock, err := acquireRootedLock(operationRoot)
	if err != nil {
		return Event{}, err
	}
	defer unlock()

	if _, err := readPlan(operationRoot, operationID); err != nil {
		return Event{}, err
	}

	events, issue, err := readEvents(operationRoot, operationID)
	if err != nil {
		return Event{}, err
	}
	if issue != "" {
		return Event{}, fmt.Errorf("journal cannot append: %s", issue)
	}

	sequence := len(events) + 1
	event := Event{
		SchemaVersion: EventSchemaVersion,
		OperationID:   operationID,
		Sequence:      sequence,
		ID:            store.newID(),
		Phase:         input.Phase,
		Timestamp:     store.now().UTC(),
		Step:          input.Step,
		Error:         input.Error,
	}
	if event.ID == "" {
		return Event{}, errors.New("journal id generator returned an empty id")
	}

	if err := writeRootedAtomic(
		operationRoot,
		eventFilename(event.Sequence, event.ID),
		0o644,
		store.beforePublication,
		store.publicationOperations,
		func(writer io.Writer) error {
			return encodeJSON(writer, event)
		},
	); err != nil {
		return Event{}, fmt.Errorf("write journal event: %w", err)
	}

	return event, nil
}

func validatePlanInput(plan Plan) error {
	if err := validateOperationID(plan.OperationID); err != nil {
		return err
	}
	if plan.IdempotencyKey == "" {
		return errors.New("journal plan idempotency key is required")
	}
	if plan.Kind == "" {
		return errors.New("journal plan kind is required")
	}
	for index, step := range plan.Steps {
		if step.Ordinal != index+1 {
			return fmt.Errorf(
				"journal step ordinal = %d, want %d",
				step.Ordinal,
				index+1,
			)
		}
		if step.Action == "" || step.Target == "" {
			return fmt.Errorf(
				"journal step %d requires action and target",
				step.Ordinal,
			)
		}
		if step.Action == "create-directory" {
			switch {
			case step.AppliedTarget != "":
				return fmt.Errorf(
					"journal create-directory step %d must not include an applied target",
					step.Ordinal,
				)
			case step.BeforeHash != "":
				return fmt.Errorf(
					"journal create-directory step %d must not include a before hash",
					step.Ordinal,
				)
			case step.AfterHash != "":
				return fmt.Errorf(
					"journal create-directory step %d must not include an after hash",
					step.Ordinal,
				)
			}
		}
		if step.AppliedTarget != "" {
			if filepath.Clean(step.AppliedTarget) ==
				filepath.Clean(step.Target) {
				return fmt.Errorf(
					"journal step %d applied target must differ from target",
					step.Ordinal,
				)
			}
			if step.BeforeHash == "" && step.AfterHash == "" {
				return fmt.Errorf(
					"journal step %d applied target requires an expected hash",
					step.Ordinal,
				)
			}
		}
	}
	return nil
}

func validateOperationID(operationID string) error {
	if operationID == "" {
		return errors.New("journal operation id is required")
	}
	if operationID == "." ||
		operationID == ".." ||
		filepath.IsAbs(operationID) ||
		filepath.Clean(operationID) != operationID ||
		filepath.Base(operationID) != operationID ||
		filepath.VolumeName(operationID) != "" ||
		strings.ContainsAny(operationID, `/\`) ||
		strings.IndexByte(operationID, 0) >= 0 ||
		hasWindowsDriveVolume(operationID) {
		return fmt.Errorf(
			"journal operation id must be a single safe path component: %q",
			operationID,
		)
	}
	return nil
}

func hasWindowsDriveVolume(path string) bool {
	if len(path) < 2 || path[1] != ':' {
		return false
	}
	drive := path[0]
	return drive >= 'A' && drive <= 'Z' ||
		drive >= 'a' && drive <= 'z'
}

func samePlanRequest(existing Plan, requested Plan) bool {
	return existing.OperationID == requested.OperationID &&
		existing.IdempotencyKey == requested.IdempotencyKey &&
		existing.Kind == requested.Kind &&
		reflect.DeepEqual(existing.Steps, requested.Steps)
}

func planHash(plan Plan) string {
	copy := plan
	copy.Hash = ""
	content, err := json.Marshal(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal journal plan for hashing: %v", err))
	}
	return Digest(content)
}

func readPlan(root *os.Root, operationID string) (Plan, error) {
	file, err := openRootedRegular(root, "plan.json", os.O_RDONLY, 0)
	if err != nil {
		return Plan{}, fmt.Errorf("open journal plan: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	var plan Plan
	if err := decodeJSON(file, &plan); err != nil {
		return Plan{}, fmt.Errorf("decode journal plan: %w", err)
	}
	if plan.SchemaVersion != SchemaVersion {
		return Plan{}, fmt.Errorf(
			"unsupported journal plan schema %q",
			plan.SchemaVersion,
		)
	}
	if plan.OperationID != operationID {
		return Plan{}, fmt.Errorf(
			"journal plan operation id = %q, want %q",
			plan.OperationID,
			operationID,
		)
	}
	if plan.Hash == "" || planHash(plan) != plan.Hash {
		return Plan{}, errors.New("journal plan hash mismatch")
	}
	if err := validatePlanInput(plan); err != nil {
		return Plan{}, fmt.Errorf("validate journal plan: %w", err)
	}
	return plan, nil
}

func readEvents(
	operationRoot *os.Root,
	operationID string,
) ([]Event, string, error) {
	entries, err := readRootedDirectory(operationRoot)
	if err != nil {
		return nil, "", fmt.Errorf("read journal operation directory: %w", err)
	}

	events := make([]Event, 0)
	expectedSequence := 1
	for _, entry := range entries {
		match := eventNamePattern.FindStringSubmatch(entry.Name())
		if len(match) == 0 {
			continue
		}

		sequence, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, "", fmt.Errorf(
				"parse journal event sequence %q: %w",
				entry.Name(),
				err,
			)
		}
		if sequence != expectedSequence {
			return events, "event_sequence_gap", nil
		}

		file, err := openRootedRegular(
			operationRoot,
			entry.Name(),
			os.O_RDONLY,
			0,
		)
		if err != nil {
			return nil, "", fmt.Errorf("open journal event: %w", err)
		}
		var event Event
		decodeErr := decodeJSON(file, &event)
		closeErr := file.Close()
		if decodeErr != nil {
			return nil, "", fmt.Errorf("decode journal event: %w", decodeErr)
		}
		if closeErr != nil {
			return nil, "", fmt.Errorf("close journal event: %w", closeErr)
		}
		if event.SchemaVersion != EventSchemaVersion ||
			event.Sequence != sequence ||
			event.ID != match[2] {
			return events, "event_identity_mismatch", nil
		}
		if event.OperationID != operationID {
			return events, "event_operation_mismatch", nil
		}

		events = append(events, event)
		expectedSequence++
	}

	return events, "", nil
}

func validPhase(phase Phase) bool {
	switch phase {
	case PhaseApplying,
		PhaseStepApplying,
		PhaseStepApplied,
		PhaseCommitted,
		PhaseFailed:
		return true
	default:
		return false
	}
}

func eventFilename(sequence int, id string) string {
	return fmt.Sprintf("%06d-%s.json", sequence, id)
}

func isEventFilename(name string) bool {
	return eventNamePattern.MatchString(name)
}

func encodeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}

func decodeJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}
