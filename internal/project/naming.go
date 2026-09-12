package project

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	projectPattern = regexp.MustCompile(`^[A-Za-z0-9]+([._-][A-Za-z0-9]+)*$`)
	packagePattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)
	separators     = regexp.MustCompile(`[._-]+`)
)

func NormalizeName(name string) (string, error) {
	if !projectPattern.MatchString(name) || len(name) > 100 {
		return "", fmt.Errorf("project name must contain ASCII letters or digits separated by '.', '_' or '-' (maximum 100 characters)")
	}
	name = separators.ReplaceAllString(strings.ToLower(name), "-")
	if reservedWindowsName(name) {
		return "", fmt.Errorf("project name %q is reserved on Windows", name)
	}
	return name, nil
}

func PackageName(name string) (string, error) {
	name = strings.ReplaceAll(name, "-", "_")
	return name, ValidatePackage(name)
}

func ValidatePackage(name string) error {
	keywords := " False None True and as assert async await break class continue def del elif else except finally for from global if import in is lambda nonlocal not or pass raise return try while with yield "
	if !packagePattern.MatchString(name) || len(name) > 100 || strings.Contains(keywords, " "+name+" ") || reservedWindowsName(name) || name == "tests" {
		return fmt.Errorf("package %q must be a lowercase Python identifier, not a keyword, 'tests', or a reserved filename", name)
	}
	return nil
}

func reservedWindowsName(name string) bool {
	name = strings.ToUpper(name)
	if name == "CON" || name == "PRN" || name == "AUX" || name == "NUL" {
		return true
	}
	return len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9'
}
