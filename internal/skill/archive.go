package skill

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	maxArchiveSize = 64 << 20
	maxSkillFile   = 8 << 20
)

type archiveSkill struct {
	descriptor Descriptor
	files      map[string]archiveFile
}

type archiveFile struct {
	data []byte
	mode int64
}

type archiveOptions struct {
	names        map[string]struct{}
	metadataOnly bool
}

type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

func parseArchive(reader io.Reader, options archiveOptions) ([]archiveSkill, error) {
	gzr, err := gzip.NewReader(reader)
	if err != nil {
		return nil, fmt.Errorf("%w: open skill archive: %v", ErrIO, err)
	}
	defer gzr.Close()

	byName := make(map[string]*archiveSkill)
	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: read skill archive: %v", ErrIO, err)
		}

		skillName, relativePath, ok := archiveSkillPath(header.Name)
		if !ok {
			continue
		}
		if options.names != nil {
			if _, selected := options.names[skillName]; !selected {
				continue
			}
		}
		if options.metadataOnly && relativePath != "SKILL.md" {
			continue
		}
		if header.Typeflag == tar.TypeSymlink || header.Typeflag == tar.TypeLink {
			return nil, fmt.Errorf("%w: archive link refused: %s", ErrValidation, header.Name)
		}
		if header.Typeflag != tar.TypeReg || relativePath == "" {
			continue
		}
		if header.Size < 0 || header.Size > maxSkillFile {
			return nil, fmt.Errorf("%w: skill file is too large: %s", ErrValidation, header.Name)
		}
		body, err := io.ReadAll(io.LimitReader(tr, maxSkillFile+1))
		if err != nil {
			return nil, fmt.Errorf("%w: read %s: %v", ErrIO, header.Name, err)
		}
		if len(body) > maxSkillFile {
			return nil, fmt.Errorf("%w: skill file is too large: %s", ErrValidation, header.Name)
		}
		entry := byName[skillName]
		if entry == nil {
			entry = &archiveSkill{
				descriptor: Descriptor{Name: skillName},
				files:      make(map[string]archiveFile),
			}
			byName[skillName] = entry
		}
		entry.files[relativePath] = archiveFile{data: body, mode: header.Mode}
	}

	skills := make([]archiveSkill, 0, len(byName))
	for name, entry := range byName {
		skillMD, ok := entry.files["SKILL.md"]
		if !ok {
			continue
		}
		metadata, err := parseMetadata(skillMD.data)
		if err != nil {
			return nil, fmt.Errorf("%w: %s/SKILL.md: %v", ErrValidation, name, err)
		}
		if metadata.Name != "" && metadata.Name != name {
			return nil, fmt.Errorf("%w: skill folder %q declares name %q", ErrValidation, name, metadata.Name)
		}
		entry.descriptor.Description = metadata.Description
		skills = append(skills, *entry)
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].descriptor.Name < skills[j].descriptor.Name
	})
	return skills, nil
}

func archiveSkillPath(name string) (string, string, bool) {
	normalized := strings.ReplaceAll(name, `\`, "/")
	if strings.HasPrefix(normalized, "/") {
		return "", "", false
	}
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return "", "", false
		}
	}
	clean := path.Clean(normalized)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", "", false
	}
	parts := strings.Split(clean, "/")
	if len(parts) < 3 || parts[1] != "skills" || !validName(parts[2]) {
		return "", "", false
	}
	relative := strings.Join(parts[3:], "/")
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", "", false
	}
	return parts[2], relative, true
}

func parseMetadata(data []byte) (frontmatter, error) {
	var metadata frontmatter
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		return metadata, errors.New("missing YAML frontmatter")
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 && strings.HasSuffix(rest, "\n---") {
		end = len(rest) - len("\n---")
	}
	if end < 0 {
		return metadata, errors.New("unterminated YAML frontmatter")
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &metadata); err != nil {
		return metadata, fmt.Errorf("decode YAML frontmatter: %w", err)
	}
	if !validName(metadata.Name) {
		return metadata, fmt.Errorf("invalid name %q", metadata.Name)
	}
	if strings.TrimSpace(metadata.Description) == "" {
		return metadata, errors.New("missing description")
	}
	metadata.Description = strings.TrimSpace(metadata.Description)
	return metadata, nil
}
