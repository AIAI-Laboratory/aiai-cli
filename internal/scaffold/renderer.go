package scaffold

import (
	"bytes"
	"text/template"
)

// Variables contains only validated scalar values; it exposes no methods or IO.
type Variables struct {
	ProjectName string
	PackageName string
}

func Render(name string, source []byte, vars Variables) ([]byte, error) {
	t, err := template.New(name).Option("missingkey=error").Parse(string(source))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, vars); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
