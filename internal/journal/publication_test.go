package journal

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPublicationStagesCompleteContentBeforeCanonicalRename(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		operation string
		publish   func(store *Store, operationID string) error
	}{
		{
			name:      "plan",
			operation: "operation-staged-plan",
			publish: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:" + operationID,
					Kind:           "test",
				})
				return err
			},
		},
		{
			name:      "event",
			operation: "operation-staged-event",
			publish: func(store *Store, operationID string) error {
				beginTestPlan(t, store, operationID, nil)
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			store.beforePublication = func(
				root *os.Root,
				sourceName string,
				targetName string,
			) error {
				if sourceName == "" {
					return errors.New("publication source name is empty")
				}
				if _, err := root.Lstat(targetName); !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf(
						"canonical journal file exists before rename: %v",
						err,
					)
				}
				content, err := root.ReadFile(sourceName)
				if err != nil {
					return fmt.Errorf("read staged journal file: %w", err)
				}
				var document map[string]any
				if err := json.Unmarshal(content, &document); err != nil {
					return fmt.Errorf("decode staged journal file: %w", err)
				}
				if len(document) == 0 {
					return errors.New("staged journal document is empty")
				}
				return nil
			}

			if err := test.publish(store, test.operation); err != nil {
				t.Fatalf("publish journal %s: %v", test.name, err)
			}
		})
	}
}

func TestFailedPublicationCleansOnlyItsCanonicalInode(t *testing.T) {
	t.Parallel()

	publications := []struct {
		name       string
		operation  string
		targetName string
		publish    func(store *Store, operationID string) error
		prepare    func(t *testing.T, store *Store, operationID string)
	}{
		{
			name:       "plan",
			operation:  "operation-failed-plan",
			targetName: "plan.json",
			publish: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:" + operationID,
					Kind:           "test",
				})
				return err
			},
		},
		{
			name:       "event",
			operation:  "operation-failed-event",
			targetName: eventFilename(1, "event-1"),
			prepare: func(t *testing.T, store *Store, operationID string) {
				t.Helper()
				beginTestPlan(t, store, operationID, nil)
			},
			publish: func(store *Store, operationID string) error {
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
	}

	failure := errors.New("injected journal publication failure")
	failures := []struct {
		name       string
		operations func() rootedPublicationOperations
		wantError  error
	}{
		{
			name: "write",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					write: func(*os.File, []byte) (int, error) {
						return 0, failure
					},
				}
			},
			wantError: failure,
		},
		{
			name: "short write",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					write: func(file *os.File, content []byte) (int, error) {
						return file.Write(content[:len(content)/2])
					},
				}
			},
			wantError: io.ErrShortWrite,
		},
		{
			name: "sync",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					sync: func(*os.File) error {
						return failure
					},
				}
			},
			wantError: failure,
		},
		{
			name: "directory sync",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					syncDirectory: func(*os.Root) error {
						return failure
					},
				}
			},
			wantError: failure,
		},
		{
			name: "binding verification",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					verifyBinding: func(
						*os.Root,
						string,
						*os.File,
					) error {
						return failure
					},
				}
			},
			wantError: failure,
		},
		{
			name: "close",
			operations: func() rootedPublicationOperations {
				return rootedPublicationOperations{
					close: func(file *os.File) error {
						if err := file.Close(); err != nil {
							return err
						}
						return failure
					},
				}
			},
			wantError: failure,
		},
	}

	replacement := []byte("{\"replacement\":true}\n")
	for _, publication := range publications {
		publication := publication
		for _, injectedFailure := range failures {
			injectedFailure := injectedFailure
			for _, preserveReplacement := range []bool{false, true} {
				preserveReplacement := preserveReplacement
				name := publication.name + "/" + injectedFailure.name
				if preserveReplacement {
					name += "/preserves replacement"
				} else {
					name += "/cleans partial"
				}
				t.Run(name, func(t *testing.T) {
					t.Parallel()

					store := newTestStore(t)
					if publication.prepare != nil {
						publication.prepare(t, store, publication.operation)
					}
					operations := injectedFailure.operations()
					if preserveReplacement {
						operations.beforeCleanup = replaceJournalPublicationTarget(
							replacement,
						)
					}
					store.publicationOperations = operations

					err := publication.publish(store, publication.operation)
					if !errors.Is(err, injectedFailure.wantError) {
						t.Fatalf(
							"publication error = %v, want %v",
							err,
							injectedFailure.wantError,
						)
					}

					targetPath := filepath.Join(
						store.root,
						publication.operation,
						publication.targetName,
					)
					content, readErr := os.ReadFile(targetPath)
					if preserveReplacement {
						if !errors.Is(readErr, fs.ErrNotExist) {
							t.Fatalf(
								"canonical content after quarantine = %q, error = %v; want absent",
								content,
								readErr,
							)
						}
						matches := findJournalFilesWithContent(
							t,
							filepath.Dir(targetPath),
							replacement,
						)
						if len(matches) != 1 ||
							!strings.Contains(matches[0], ".quarantine-") {
							t.Fatalf(
								"replacement evidence = %v, want one quarantine file",
								matches,
							)
						}
						return
					}
					if !errors.Is(readErr, fs.ErrNotExist) {
						t.Fatalf(
							"partial canonical read error = %v, want not exist; content = %q",
							readErr,
							content,
						)
					}
				})
			}
		}
	}
}

func replaceJournalPublicationTarget(
	replacement []byte,
) func(root *os.Root, name string, created os.FileInfo) error {
	return func(root *os.Root, name string, _ os.FileInfo) error {
		displacedName := "." + name + ".failed-attempt"
		if err := root.Rename(name, displacedName); err != nil {
			return fmt.Errorf("displace failed publication: %w", err)
		}

		file, err := root.OpenFile(
			name,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o644,
		)
		if err != nil {
			return fmt.Errorf("create concurrent replacement: %w", err)
		}
		if _, err := file.Write(replacement); err != nil {
			_ = file.Close()
			return fmt.Errorf("write concurrent replacement: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync concurrent replacement: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close concurrent replacement: %w", err)
		}
		if err := root.Remove(displacedName); err != nil {
			return fmt.Errorf("remove displaced failed publication: %w", err)
		}
		return nil
	}
}

func TestBeginAndAppendRejectSwappedStagingContent(t *testing.T) {
	t.Parallel()

	crafted := []byte("{\"crafted\":true}\n")
	tests := []struct {
		name      string
		operation string
		prepare   func(t *testing.T, store *Store, operationID string)
		target    string
		publish   func(store *Store, operationID string) error
	}{
		{
			name:      "begin",
			operation: "operation-swapped-plan",
			target:    "plan.json",
			publish: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:" + operationID,
					Kind:           "test",
				})
				return err
			},
		},
		{
			name:      "append",
			operation: "operation-swapped-event",
			target:    eventFilename(1, "event-1"),
			prepare: func(
				t *testing.T,
				store *Store,
				operationID string,
			) {
				t.Helper()
				beginTestPlan(t, store, operationID, nil)
			},
			publish: func(store *Store, operationID string) error {
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			if test.prepare != nil {
				test.prepare(t, store, test.operation)
			}
			store.beforePublication = swapJournalPublicationSource(t, crafted)

			if err := test.publish(store, test.operation); err == nil {
				t.Fatal("journal publication succeeded with swapped staging content")
			}
			targetPath := filepath.Join(
				store.root,
				test.operation,
				test.target,
			)
			if content, err := os.ReadFile(targetPath); !errors.Is(
				err,
				fs.ErrNotExist,
			) {
				t.Fatalf(
					"canonical swapped content = %q, error = %v; want absent",
					content,
					err,
				)
			}
			matches := findJournalFilesWithContent(
				t,
				filepath.Dir(targetPath),
				crafted,
			)
			if len(matches) != 1 ||
				!strings.Contains(matches[0], ".quarantine-") {
				t.Fatalf(
					"crafted replacement evidence = %v, want one quarantine file",
					matches,
				)
			}
		})
	}
}

func TestPublicationDoesNotReplaceConcurrentCanonicalFile(t *testing.T) {
	t.Parallel()

	replacement := []byte("{\"concurrent\":true}\n")
	tests := []struct {
		name      string
		operation string
		target    string
		prepare   func(t *testing.T, store *Store, operationID string)
		publish   func(store *Store, operationID string) error
	}{
		{
			name:      "plan",
			operation: "operation-concurrent-plan",
			target:    "plan.json",
			publish: func(store *Store, operationID string) error {
				_, err := store.Begin(Plan{
					OperationID:    operationID,
					IdempotencyKey: "test:" + operationID,
					Kind:           "test",
				})
				return err
			},
		},
		{
			name:      "event",
			operation: "operation-concurrent-event",
			target:    eventFilename(1, "event-1"),
			prepare: func(t *testing.T, store *Store, operationID string) {
				t.Helper()
				beginTestPlan(t, store, operationID, nil)
			},
			publish: func(store *Store, operationID string) error {
				_, err := store.Append(operationID, EventInput{
					Phase: PhaseApplying,
				})
				return err
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			if test.prepare != nil {
				test.prepare(t, store, test.operation)
			}
			store.beforePublication = func(
				root *os.Root,
				_ string,
				targetName string,
			) error {
				file, err := root.OpenFile(
					targetName,
					os.O_WRONLY|os.O_CREATE|os.O_EXCL,
					0o644,
				)
				if err != nil {
					return err
				}
				if _, err := file.Write(replacement); err != nil {
					_ = file.Close()
					return err
				}
				if err := file.Sync(); err != nil {
					_ = file.Close()
					return err
				}
				return file.Close()
			}

			if err := test.publish(store, test.operation); err == nil {
				t.Fatal("journal publication replaced a concurrent canonical file")
			}
			targetPath := filepath.Join(
				store.root,
				test.operation,
				test.target,
			)
			content, err := os.ReadFile(targetPath)
			if err != nil {
				t.Fatalf("read concurrent canonical file: %v", err)
			}
			if !bytes.Equal(content, replacement) {
				t.Fatalf(
					"concurrent canonical content = %q, want %q",
					content,
					replacement,
				)
			}
		})
	}
}

func TestAppendRejectsUnsafePublicationNamesWithoutMutation(t *testing.T) {
	t.Parallel()

	unsafeIDs := []string{
		"../outside",
		"subdirectory/event",
		`..\outside`,
		`C:\outside`,
	}
	for _, unsafeID := range unsafeIDs {
		unsafeID := unsafeID
		t.Run(unsafeID, func(t *testing.T) {
			t.Parallel()

			store := newTestStore(t)
			const operationID = "operation-unsafe-event-name"
			beginTestPlan(t, store, operationID, nil)
			operationRoot := filepath.Join(store.root, operationID)
			before := snapshotTree(t, operationRoot)
			store.newID = func() string { return unsafeID }

			if _, err := store.Append(operationID, EventInput{
				Phase: PhaseApplying,
			}); err == nil {
				t.Fatal("expected unsafe event publication name rejection")
			}
			after := snapshotTree(t, operationRoot)
			if !reflect.DeepEqual(after, before) {
				t.Fatalf(
					"journal operation changed for unsafe event id %q:\nbefore: %#v\nafter:  %#v",
					unsafeID,
					before,
					after,
				)
			}
		})
	}
}

func findJournalFilesWithContent(
	t *testing.T,
	root string,
	want []byte,
) []string {
	t.Helper()

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read journal operation directory: %v", err)
	}
	var matches []string
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			continue
		}
		content, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("read journal file %q: %v", entry.Name(), err)
		}
		if bytes.Equal(content, want) {
			matches = append(matches, entry.Name())
		}
	}
	return matches
}

func swapJournalPublicationSource(
	t *testing.T,
	crafted []byte,
) func(root *os.Root, sourceName string, targetName string) error {
	t.Helper()

	return func(
		root *os.Root,
		sourceName string,
		targetName string,
	) error {
		t.Helper()

		if sourceName == "" {
			sourceName = "." + targetName + ".tmp-swapped"
		}
		craftedName := sourceName + ".crafted"
		file, err := root.OpenFile(
			craftedName,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			0o644,
		)
		if err != nil {
			return fmt.Errorf("create crafted publication source: %w", err)
		}
		if _, err := file.Write(crafted); err != nil {
			_ = file.Close()
			return fmt.Errorf("write crafted publication source: %w", err)
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return fmt.Errorf("sync crafted publication source: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close crafted publication source: %w", err)
		}

		if err := root.Remove(sourceName); err != nil &&
			!errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove trusted publication source: %w", err)
		}
		if err := root.Rename(craftedName, sourceName); err != nil {
			return fmt.Errorf("swap crafted publication source: %w", err)
		}
		return nil
	}
}
