package screens

import (
	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	"github.com/AIAI-Laboratory/aiai-cli/internal/scaffold"
)

func Result(r scaffold.Result, err error) string {
	title := "PROJECT READY"
	if err != nil {
		title = "GENERATION STOPPED\n\n" + err.Error()
	}
	return title + "\n\n" + output.ResultText(r) + "\nEnter to finish."
}
