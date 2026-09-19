package skill

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func validName(name string) bool {
	return skillNamePattern.MatchString(name)
}

func descriptors(skills []archiveSkill) []Descriptor {
	result := make([]Descriptor, len(skills))
	for i := range skills {
		result[i] = skills[i].descriptor
	}
	return result
}

func manifestDescriptors(root string, tracked manifest) []Descriptor {
	result := make([]Descriptor, 0, len(tracked.Skills))
	for name, item := range tracked.Skills {
		result = append(result, Descriptor{
			Name:        name,
			Description: item.Description,
			Path:        filepath.Join(root, name),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func selectSkills(available []archiveSkill, names []string, all bool) ([]archiveSkill, error) {
	if err := validateSelectionRequest(names, all); err != nil {
		return nil, err
	}
	if all {
		return available, nil
	}
	byName := make(map[string]archiveSkill, len(available))
	for _, item := range available {
		byName[item.descriptor.Name] = item
	}
	selected := make([]archiveSkill, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		item, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("%w: skill %q is not available from %s", ErrValidation, name, DefaultRepo)
		}
		seen[name] = true
		selected = append(selected, item)
	}
	return selected, nil
}

func validateSelectionRequest(names []string, all bool) error {
	if all && len(names) > 0 {
		return fmt.Errorf("%w: --all cannot be combined with skill names", ErrValidation)
	}
	if !all && len(names) == 0 {
		return fmt.Errorf("%w: specify skill names or use --all", ErrValidation)
	}
	for _, name := range names {
		if !validName(name) {
			return fmt.Errorf("%w: invalid skill name %q", ErrValidation, name)
		}
	}
	return nil
}

func selectDescriptors(available []Descriptor, names []string, all bool) ([]Descriptor, error) {
	if err := validateSelectionRequest(names, all); err != nil {
		return nil, err
	}
	if all {
		return available, nil
	}
	byName := make(map[string]Descriptor, len(available))
	for _, item := range available {
		byName[item.Name] = item
	}
	selected := make([]Descriptor, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			continue
		}
		item, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("%w: skill %q is not installed", ErrValidation, name)
		}
		seen[name] = true
		selected = append(selected, item)
	}
	return selected, nil
}
