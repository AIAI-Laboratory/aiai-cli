package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func installedDescriptors(root string, tracked manifest) ([]Descriptor, error) {
	result := make([]Descriptor, 0, len(tracked.Skills))
	for _, item := range manifestDescriptors(root, tracked) {
		info, err := os.Lstat(item.Path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: inspect %s: %v", ErrIO, item.Path, err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: managed skill is not a regular directory: %s", ErrValidation, item.Path)
		}
		skillFile := filepath.Join(item.Path, "SKILL.md")
		fileInfo, err := os.Lstat(skillFile)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%w: inspect %s: %v", ErrIO, skillFile, err)
		}
		if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w: managed skill entry point is not a regular file: %s", ErrValidation, skillFile)
		}
		result = append(result, item)
	}
	return result, nil
}

func preflightInstall(root string, names []string, overwrite bool) error {
	for _, name := range names {
		destination := filepath.Join(root, name)
		info, err := os.Lstat(destination)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return fmt.Errorf("%w: inspect %s: %v", ErrIO, destination, err)
		case info.Mode()&os.ModeSymlink != 0:
			return fmt.Errorf("%w: symlink destination refused: %s", ErrValidation, destination)
		case !info.IsDir():
			return fmt.Errorf("%w: destination is not a directory: %s", ErrConflict, destination)
		case !overwrite:
			return fmt.Errorf("%w: %s already exists; use --force to replace it", ErrConflict, destination)
		}
	}
	return nil
}

func installSkills(root string, skills []archiveSkill, overwrite bool) error {
	if err := ensureDirectory(root); err != nil {
		return err
	}
	names := make([]string, len(skills))
	for i := range skills {
		names[i] = skills[i].descriptor.Name
	}
	if err := preflightInstall(root, names, overwrite); err != nil {
		return err
	}
	for _, item := range skills {
		if err := installOne(root, item); err != nil {
			return err
		}
	}
	return nil
}

func ensureDirectory(directory string) error {
	if info, err := os.Lstat(directory); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: skill root is not a regular directory: %s", ErrValidation, directory)
		}
		return nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%w: inspect skill root: %v", ErrIO, err)
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("%w: create skill root: %v", ErrIO, err)
	}
	return nil
}

func installOne(root string, item archiveSkill) error {
	staging, err := os.MkdirTemp(root, ".aiai-"+item.descriptor.Name+"-")
	if err != nil {
		return fmt.Errorf("%w: create skill staging directory: %v", ErrIO, err)
	}
	defer os.RemoveAll(staging)

	paths := make([]string, 0, len(item.files))
	for relative := range item.files {
		paths = append(paths, relative)
	}
	sort.Strings(paths)
	for _, relative := range paths {
		file := item.files[relative]
		if !validRelative(relative) {
			return fmt.Errorf("%w: unsafe skill path %q", ErrValidation, relative)
		}
		destination := filepath.Join(staging, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return fmt.Errorf("%w: create skill directory: %v", ErrIO, err)
		}
		mode := fs.FileMode(0o644)
		if file.mode&0o111 != 0 {
			mode = 0o755
		}
		if err := os.WriteFile(destination, file.data, mode); err != nil {
			return fmt.Errorf("%w: write skill file: %v", ErrIO, err)
		}
	}

	destination := filepath.Join(root, item.descriptor.Name)
	backup := ""
	if _, err := os.Lstat(destination); err == nil {
		backupDir, err := os.MkdirTemp(root, ".aiai-backup-"+item.descriptor.Name+"-")
		if err != nil {
			return fmt.Errorf("%w: reserve backup path: %v", ErrIO, err)
		}
		if err := os.Remove(backupDir); err != nil {
			return fmt.Errorf("%w: prepare backup path: %v", ErrIO, err)
		}
		if err := os.Rename(destination, backupDir); err != nil {
			return fmt.Errorf("%w: back up installed skill: %v", ErrIO, err)
		}
		backup = backupDir
	}
	if err := os.Rename(staging, destination); err != nil {
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("%w: commit skill installation: %v", ErrIO, err)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}

func validRelative(name string) bool {
	clean := filepath.Clean(filepath.FromSlash(name))
	return clean != "." && clean != ".." && !filepath.IsAbs(clean) && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
