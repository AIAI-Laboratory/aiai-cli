package project

import "testing"

func TestNames(t *testing.T) {
	for input, want := range map[string]string{"My_Project": "my-project", "my.project": "my-project", "hello": "hello"} {
		got, err := NormalizeName(input)
		if err != nil || got != want {
			t.Fatalf("%q: got %q, %v", input, got, err)
		}
	}
	for _, input := range []string{"", "../escape", "/absolute", `C:\absolute`, "hello world", "--", "x\nfile", "a..b", "CON", "LPT1", "é"} {
		if _, err := NormalizeName(input); err == nil {
			t.Errorf("accepted project name %q", input)
		}
	}
	for _, input := range []string{"class", "await", "1demo", "tests", "con", "x/y", "hello-world", "Upper"} {
		if ValidatePackage(input) == nil {
			t.Errorf("accepted package name %q", input)
		}
	}
	if got, err := PackageName("my-project"); err != nil || got != "my_project" {
		t.Fatalf("%q: %v", got, err)
	}
}
