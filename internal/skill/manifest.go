package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const manifestFile = ".aiai-skills.json"

type manifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Repository    string                   `json:"repository"`
	Skills        map[string]manifestSkill `json:"skills"`
}

type manifestSkill struct {
	Description string `json:"description,omitempty"`
}

func readManifest(root string) (manifest, error) {
	result := manifest{SchemaVersion: 1, Repository: DefaultRepo, Skills: make(map[string]manifestSkill)}
	data, err := os.ReadFile(filepath.Join(root, manifestFile))
	if errors.Is(err, fs.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("%w: read skill manifest: %v", ErrIO, err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("%w: decode skill manifest: %v", ErrValidation, err)
	}
	if result.SchemaVersion != 1 || result.Repository != DefaultRepo {
		return result, fmt.Errorf("%w: unsupported skill manifest", ErrValidation)
	}
	if result.Skills == nil {
		result.Skills = make(map[string]manifestSkill)
	}
	for name := range result.Skills {
		if !validName(name) {
			return result, fmt.Errorf("%w: invalid skill name %q in manifest", ErrValidation, name)
		}
	}
	return result, nil
}

func writeManifest(root string, value manifest) error {
	value.SchemaVersion = 1
	value.Repository = DefaultRepo
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("%w: encode skill manifest: %v", ErrIO, err)
	}
	data = append(data, '\n')
	tempPath := filepath.Join(root, manifestFile+".tmp")
	if err := os.WriteFile(tempPath, data, 0o600); err != nil {
		return fmt.Errorf("%w: write skill manifest: %v", ErrIO, err)
	}
	if err := os.Rename(tempPath, filepath.Join(root, manifestFile)); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("%w: commit skill manifest: %v", ErrIO, err)
	}
	return nil
}
