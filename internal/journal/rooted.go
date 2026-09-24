package journal

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/valter-silva-au/ai-dev-brain/internal/lockfile"
)

func (store *Store) openJournalRoot(create bool) (*os.Root, error) {
	root, err := openAbsoluteDirectoryNoSymlinks(store.root, create)
	if err != nil {
		return nil, fmt.Errorf("open journal root %q: %w", store.root, err)
	}
	return root, nil
}

func openAbsoluteDirectoryNoSymlinks(
	path string,
	create bool,
) (*os.Root, error) {
	start, components, err := rootedAbsolutePath(path)
	if err != nil {
		return nil, err
	}

	current, err := os.OpenRoot(start)
	if err != nil {
		return nil, fmt.Errorf("open path root %q: %w", start, err)
	}
	currentPath := start
	for _, component := range components {
		if create {
			err := current.Mkdir(component, 0o755)
			switch {
			case err == nil:
				if err := syncRootDirectory(current); err != nil {
					_ = current.Close()
					return nil, fmt.Errorf(
						"sync parent after creating directory %q: %w",
						filepath.Join(currentPath, component),
						err,
					)
				}
			case !errors.Is(err, fs.ErrExist):
				_ = current.Close()
				return nil, fmt.Errorf(
					"create directory %q: %w",
					filepath.Join(currentPath, component),
					err,
				)
			}
		}

		before, err := current.Lstat(component)
		if err != nil {
			_ = current.Close()
			return nil, fmt.Errorf(
				"inspect directory %q: %w",
				filepath.Join(currentPath, component),
				err,
			)
		}
		if before.Mode()&os.ModeSymlink != 0 {
			_ = current.Close()
			return nil, fmt.Errorf(
				"journal path component %q must not be a symlink",
				filepath.Join(currentPath, component),
			)
		}
		if !before.IsDir() {
			_ = current.Close()
			return nil, fmt.Errorf(
				"journal path component %q is not a directory",
				filepath.Join(currentPath, component),
			)
		}

		child, err := current.OpenRoot(component)
		if err != nil {
			_ = current.Close()
			return nil, fmt.Errorf(
				"open directory %q: %w",
				filepath.Join(currentPath, component),
				err,
			)
		}
		if err := verifyDirectoryChildBinding(
			current,
			component,
			child,
			before,
		); err != nil {
			_ = child.Close()
			_ = current.Close()
			return nil, err
		}
		_ = current.Close()
		current = child
		currentPath = filepath.Join(currentPath, component)
	}
	return current, nil
}

func rootedAbsolutePath(path string) (string, []string, error) {
	clean := filepath.Clean(path)
	volume := filepath.VolumeName(clean)
	start := volume + string(os.PathSeparator)
	relative, err := filepath.Rel(start, clean)
	if err != nil {
		return "", nil, fmt.Errorf("resolve journal path %q: %w", path, err)
	}
	if relative == "." {
		return start, nil, nil
	}
	if relative == ".." ||
		strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", nil, fmt.Errorf("journal path %q escapes its volume root", path)
	}

	components := strings.FieldsFunc(relative, func(r rune) bool {
		return r == os.PathSeparator
	})
	if runtime.GOOS != "windows" && len(components) > 0 {
		platformAlias := filepath.Join(start, components[0])
		info, err := os.Lstat(platformAlias)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(platformAlias)
			if err != nil {
				return "", nil, fmt.Errorf(
					"resolve platform root alias %q: %w",
					platformAlias,
					err,
				)
			}
			resolvedInfo, err := os.Stat(resolved)
			if err != nil {
				return "", nil, fmt.Errorf(
					"inspect platform root alias target %q: %w",
					resolved,
					err,
				)
			}
			if !resolvedInfo.IsDir() {
				return "", nil, fmt.Errorf(
					"platform root alias target %q is not a directory",
					resolved,
				)
			}
			start = resolved
			components = components[1:]
		}
	}
	return start, components, nil
}

func verifyDirectoryChildBinding(
	parent *os.Root,
	name string,
	child *os.Root,
	before os.FileInfo,
) error {
	opened, err := child.Stat(".")
	if err != nil {
		return fmt.Errorf("inspect opened journal directory %q: %w", name, err)
	}
	after, err := parent.Lstat(name)
	if err != nil {
		return fmt.Errorf("reinspect journal directory %q: %w", name, err)
	}
	if after.Mode()&os.ModeSymlink != 0 ||
		!after.IsDir() ||
		!os.SameFile(before, opened) ||
		!os.SameFile(opened, after) {
		return fmt.Errorf("journal directory %q changed while opening", name)
	}
	return nil
}

func openOperationRoot(
	journalRoot *os.Root,
	operationID string,
	create bool,
) (*os.Root, error) {
	if create {
		err := journalRoot.Mkdir(operationID, 0o755)
		switch {
		case err == nil:
			if err := syncRootDirectory(journalRoot); err != nil {
				return nil, fmt.Errorf(
					"sync journal root after creating operation %q: %w",
					operationID,
					err,
				)
			}
		case !errors.Is(err, fs.ErrExist):
			return nil, fmt.Errorf(
				"create journal operation directory %q: %w",
				operationID,
				err,
			)
		}
	}

	before, err := journalRoot.Lstat(operationID)
	if err != nil {
		return nil, fmt.Errorf(
			"inspect journal operation directory %q: %w",
			operationID,
			err,
		)
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf(
			"journal operation directory %q must not be a symlink",
			operationID,
		)
	}
	if !before.IsDir() {
		return nil, fmt.Errorf(
			"journal operation path %q is not a directory",
			operationID,
		)
	}

	root, err := journalRoot.OpenRoot(operationID)
	if err != nil {
		return nil, fmt.Errorf(
			"open journal operation directory %q: %w",
			operationID,
			err,
		)
	}
	if err := verifyChildRootBinding(
		journalRoot,
		operationID,
		root,
		before,
	); err != nil {
		_ = root.Close()
		return nil, err
	}
	return root, nil
}

func verifyChildRootBinding(
	parent *os.Root,
	name string,
	child *os.Root,
	before os.FileInfo,
) error {
	opened, err := child.Stat(".")
	if err != nil {
		return fmt.Errorf(
			"inspect opened journal operation directory %q: %w",
			name,
			err,
		)
	}
	after, err := parent.Lstat(name)
	if err != nil {
		return fmt.Errorf(
			"reinspect journal operation directory %q: %w",
			name,
			err,
		)
	}
	if after.Mode()&os.ModeSymlink != 0 ||
		!after.IsDir() ||
		!os.SameFile(before, opened) ||
		!os.SameFile(opened, after) {
		return fmt.Errorf(
			"journal operation directory %q changed while opening",
			name,
		)
	}
	return nil
}

func openRootedRegular(
	root *os.Root,
	name string,
	flag int,
	mode fs.FileMode,
) (*os.File, error) {
	before, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("journal file %q must not be a symlink", name)
	}
	if !before.Mode().IsRegular() {
		return nil, fmt.Errorf("journal file %q is not a regular file", name)
	}

	file, err := root.OpenFile(name, flag, mode)
	if err != nil {
		return nil, err
	}
	if err := verifyRootedFileBinding(root, name, file, before); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func openOrCreateRootedRegular(
	root *os.Root,
	name string,
	flag int,
	mode fs.FileMode,
) (*os.File, error) {
	file, err := root.OpenFile(
		name,
		flag|os.O_CREATE|os.O_EXCL,
		mode.Perm(),
	)
	if err == nil {
		if chmodErr := file.Chmod(mode.Perm()); chmodErr != nil {
			_ = file.Close()
			return nil, removeAbandonedRootedFile(root, name, chmodErr)
		}
		created, statErr := file.Stat()
		if statErr != nil {
			_ = file.Close()
			return nil, removeAbandonedRootedFile(root, name, statErr)
		}
		if bindErr := verifyRootedFileBinding(
			root,
			name,
			file,
			created,
		); bindErr != nil {
			_ = file.Close()
			return nil, removeAbandonedRootedFile(root, name, bindErr)
		}
		return file, nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	return openRootedRegular(root, name, flag, mode)
}

// removeAbandonedRootedFile deletes a file this process had just created but
// cannot use, and reports a failed deletion alongside cause.
//
// The residue matters: the file was created O_EXCL, so it is empty and unverified,
// and the next openOrCreateRootedRegular for that name takes the "already exists"
// branch and adopts it. Leaving it silently would hand a later caller an empty
// journal artifact that no one wrote. The caller is failing either way, so the
// deletion error is joined onto the cause rather than replacing it.
func removeAbandonedRootedFile(root *os.Root, name string, cause error) error {
	if err := root.Remove(name); err != nil {
		return errors.Join(cause, fmt.Errorf(
			"remove abandoned journal file %q: %w",
			name,
			err,
		))
	}
	return cause
}

func verifyRootedFileBinding(
	root *os.Root,
	name string,
	file *os.File,
	before os.FileInfo,
) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened journal file %q: %w", name, err)
	}
	after, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("reinspect journal file %q: %w", name, err)
	}
	if after.Mode()&os.ModeSymlink != 0 ||
		!after.Mode().IsRegular() ||
		!os.SameFile(before, opened) ||
		!os.SameFile(opened, after) {
		return fmt.Errorf("journal file %q changed while opening", name)
	}
	return nil
}

func acquireRootedLock(root *os.Root) (func(), error) {
	file, err := openOrCreateRootedRegular(
		root,
		".lock",
		os.O_RDWR,
		0o644,
	)
	if err != nil {
		return nil, fmt.Errorf("open journal lock: %w", err)
	}

	unlock, err := lockfile.Lock(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire journal lock: %w", err)
	}
	if err := verifyExistingRootedFileBinding(root, ".lock", file); err != nil {
		unlock()
		_ = file.Close()
		return nil, err
	}

	return func() {
		unlock()
		_ = file.Close()
	}, nil
}

func verifyExistingRootedFileBinding(
	root *os.Root,
	name string,
	file *os.File,
) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("inspect opened journal file %q: %w", name, err)
	}
	return verifyRootedFileBinding(root, name, file, opened)
}

type rootedPublicationOperations struct {
	write         func(file *os.File, content []byte) (int, error)
	sync          func(file *os.File) error
	verifyBinding func(
		root *os.Root,
		name string,
		file *os.File,
	) error
	close         func(file *os.File) error
	beforeCleanup func(
		root *os.Root,
		name string,
		created os.FileInfo,
	) error
	renameNoReplace func(root *os.Root, oldName string, newName string) error
	syncDirectory   func(root *os.Root) error
	randomName      func(prefix string) (string, error)
}

func (operations rootedPublicationOperations) writeContent(
	file *os.File,
	content []byte,
) (int, error) {
	if operations.write != nil {
		return operations.write(file, content)
	}
	return file.Write(content)
}

func (operations rootedPublicationOperations) syncFile(file *os.File) error {
	if operations.sync != nil {
		return operations.sync(file)
	}
	return file.Sync()
}

func (operations rootedPublicationOperations) verifyFileBinding(
	root *os.Root,
	name string,
	file *os.File,
) error {
	if operations.verifyBinding != nil {
		return operations.verifyBinding(root, name, file)
	}
	return verifyExistingRootedFileBinding(root, name, file)
}

func (operations rootedPublicationOperations) closeFile(file *os.File) error {
	if operations.close != nil {
		return operations.close(file)
	}
	return file.Close()
}

func (operations rootedPublicationOperations) prepareCleanup(
	root *os.Root,
	name string,
	created os.FileInfo,
) error {
	if operations.beforeCleanup == nil {
		return nil
	}
	return operations.beforeCleanup(root, name, created)
}

func (operations rootedPublicationOperations) renameExclusive(
	root *os.Root,
	oldName string,
	newName string,
) error {
	if err := validateRootedPublicationName(oldName); err != nil {
		return err
	}
	if err := validateRootedPublicationName(newName); err != nil {
		return err
	}
	if operations.renameNoReplace != nil {
		return operations.renameNoReplace(root, oldName, newName)
	}
	return renameRootedNoReplace(root, oldName, newName)
}

func validateRootedPublicationName(name string) error {
	if name == "" ||
		name == "." ||
		name == ".." ||
		filepath.IsAbs(name) ||
		filepath.Clean(name) != name ||
		filepath.Base(name) != name ||
		filepath.VolumeName(name) != "" ||
		strings.ContainsAny(name, `/\`) ||
		hasWindowsDriveVolume(name) {
		return fmt.Errorf(
			"journal publication name must be a single safe path component: %q",
			name,
		)
	}
	return nil
}

func (operations rootedPublicationOperations) syncRoot(root *os.Root) error {
	if operations.syncDirectory != nil {
		return operations.syncDirectory(root)
	}
	return syncRootDirectory(root)
}

func (operations rootedPublicationOperations) nextRandomName(
	prefix string,
) (string, error) {
	if operations.randomName != nil {
		return operations.randomName(prefix)
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate journal publication name: %w", err)
	}
	return prefix + hex.EncodeToString(token[:]), nil
}

func createRootedStagingFile(
	root *os.Root,
	targetName string,
	mode fs.FileMode,
	operations rootedPublicationOperations,
) (string, *os.File, os.FileInfo, error) {
	for attempts := 0; attempts < 32; attempts++ {
		name, err := operations.nextRandomName("." + targetName + ".stage-")
		if err != nil {
			return "", nil, nil, err
		}
		file, err := root.OpenFile(
			name,
			os.O_WRONLY|os.O_CREATE|os.O_EXCL,
			mode.Perm(),
		)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", nil, nil, fmt.Errorf(
				"create journal staging file %q: %w",
				name,
				err,
			)
		}
		created, err := file.Stat()
		if err != nil {
			_ = file.Close()
			return name, nil, nil, fmt.Errorf(
				"inspect journal staging file %q: %w",
				name,
				err,
			)
		}
		return name, file, created, nil
	}
	return "", nil, nil, fmt.Errorf(
		"allocate journal staging file for %q: too many collisions",
		targetName,
	)
}

func quarantineRootedPublication(
	root *os.Root,
	name string,
	attempt os.FileInfo,
	operations rootedPublicationOperations,
) (string, bool, error) {
	if err := operations.prepareCleanup(root, name, attempt); err != nil {
		return "", false, fmt.Errorf(
			"prepare journal publication quarantine for %q: %w",
			name,
			err,
		)
	}

	for tries := 0; tries < 32; tries++ {
		quarantineName, err := operations.nextRandomName(
			"." + filepath.Base(name) + ".quarantine-",
		)
		if err != nil {
			return "", false, err
		}
		err = operations.renameExclusive(root, name, quarantineName)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		if err != nil {
			return "", false, fmt.Errorf(
				"quarantine failed journal publication %q: %w",
				name,
				err,
			)
		}

		quarantined, err := root.Lstat(quarantineName)
		if err != nil {
			return quarantineName, false, fmt.Errorf(
				"inspect quarantined journal publication %q: %w",
				quarantineName,
				err,
			)
		}
		if err := operations.syncRoot(root); err != nil {
			return quarantineName, false, fmt.Errorf(
				"sync journal directory after quarantining %q: %w",
				name,
				err,
			)
		}
		if quarantined.Mode()&os.ModeSymlink != 0 ||
			!quarantined.Mode().IsRegular() ||
			!os.SameFile(attempt, quarantined) {
			return quarantineName, false, nil
		}
		if err := root.Remove(quarantineName); err != nil {
			return quarantineName, true, fmt.Errorf(
				"remove quarantined journal publication %q: %w",
				quarantineName,
				err,
			)
		}
		if err := operations.syncRoot(root); err != nil {
			return quarantineName, true, fmt.Errorf(
				"sync journal directory after removing quarantine %q: %w",
				quarantineName,
				err,
			)
		}
		return quarantineName, true, nil
	}
	return "", false, fmt.Errorf(
		"quarantine journal publication %q: too many name collisions",
		name,
	)
}

func writeRootedAtomic(
	root *os.Root,
	name string,
	mode fs.FileMode,
	beforePublication func(
		root *os.Root,
		sourceName string,
		targetName string,
	) error,
	operations rootedPublicationOperations,
	write func(io.Writer) error,
) error {
	if err := validateRootedPublicationName(name); err != nil {
		return err
	}

	var encoded bytes.Buffer
	if err := write(&encoded); err != nil {
		return fmt.Errorf("encode journal file %q: %w", name, err)
	}

	stagingName, staging, staged, err := createRootedStagingFile(
		root,
		name,
		mode,
		operations,
	)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = staging.Close()
		}
	}()

	failPublication := func(publicationName string, cause error) error {
		if !closed {
			closeErr := staging.Close()
			if closeErr == nil || errors.Is(closeErr, os.ErrClosed) {
				closed = true
			} else {
				cause = errors.Join(
					cause,
					fmt.Errorf(
						"close failed journal publication %q: %w",
						publicationName,
						closeErr,
					),
				)
			}
		}
		quarantineName, removed, err := quarantineRootedPublication(
			root,
			publicationName,
			staged,
			operations,
		)
		if err != nil {
			return errors.Join(cause, err)
		}
		if quarantineName != "" && !removed {
			return errors.Join(
				cause,
				fmt.Errorf(
					"journal publication replacement retained as %q",
					quarantineName,
				),
			)
		}
		return cause
	}

	if err := staging.Chmod(mode.Perm()); err != nil {
		return failPublication(
			stagingName,
			fmt.Errorf("set journal staging mode for %q: %w", name, err),
		)
	}
	if err := verifyRootedFileBinding(root, stagingName, staging, staged); err != nil {
		return failPublication(stagingName, err)
	}
	written, err := operations.writeContent(staging, encoded.Bytes())
	if err != nil {
		return failPublication(
			stagingName,
			fmt.Errorf("write journal file %q: %w", name, err),
		)
	}
	if written != encoded.Len() {
		return failPublication(
			stagingName,
			fmt.Errorf(
				"write journal file %q: wrote %d of %d bytes: %w",
				name,
				written,
				encoded.Len(),
				io.ErrShortWrite,
			),
		)
	}
	if err := operations.syncFile(staging); err != nil {
		return failPublication(
			stagingName,
			fmt.Errorf("sync journal file %q: %w", name, err),
		)
	}
	if err := operations.verifyFileBinding(root, stagingName, staging); err != nil {
		return failPublication(stagingName, err)
	}
	if err := operations.closeFile(staging); err != nil {
		return failPublication(
			stagingName,
			fmt.Errorf("close journal file %q: %w", name, err),
		)
	}
	closed = true

	if beforePublication != nil {
		if err := beforePublication(root, stagingName, name); err != nil {
			return failPublication(
				stagingName,
				fmt.Errorf(
					"run journal publication hook for %q: %w",
					name,
					err,
				),
			)
		}
	}

	if err := operations.renameExclusive(root, stagingName, name); err != nil {
		return failPublication(
			stagingName,
			fmt.Errorf(
				"publish journal file %q from staging: %w",
				name,
				err,
			),
		)
	}
	current, err := root.Lstat(name)
	if err != nil {
		return failPublication(
			name,
			fmt.Errorf("inspect published journal file %q: %w", name, err),
		)
	}
	if current.Mode()&os.ModeSymlink != 0 ||
		!current.Mode().IsRegular() ||
		!os.SameFile(staged, current) {
		return failPublication(
			name,
			fmt.Errorf("published journal file %q changed identity", name),
		)
	}
	if err := operations.syncRoot(root); err != nil {
		return failPublication(
			name,
			fmt.Errorf("sync journal operation directory: %w", err),
		)
	}
	return nil
}

func readRootedDirectory(root *os.Root) ([]fs.DirEntry, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = directory.Close()
	}()

	entries, err := directory.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(left int, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})
	return entries, nil
}

func syncRootDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close()
	}()

	if err := directory.Sync(); err != nil &&
		!errors.Is(err, syscall.EINVAL) &&
		!errors.Is(err, syscall.ENOTSUP) {
		return err
	}
	return nil
}
