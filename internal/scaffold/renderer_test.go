package scaffold

import "testing"

func TestRendererHasNoIOHelpers(t *testing.T) {
	vars := Variables{ProjectName: "demo", PackageName: "demo"}
	for _, source := range []string{`{{ exec "touch /tmp/unsafe" }}`, `{{ readFile "/etc/passwd" }}`, `{{ env "HOME" }}`, `{{ .Missing }}`, `{{ call .ProjectName }}`} {
		if _, err := Render("test", []byte(source), vars); err == nil {
			t.Errorf("accepted %s", source)
		}
	}
	got, err := Render("test", []byte("{{ .ProjectName }}/{{ .PackageName }}"), vars)
	if err != nil || string(got) != "demo/demo" {
		t.Fatalf("%s: %v", got, err)
	}
}
