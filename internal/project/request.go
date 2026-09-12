package project

type InitRequest struct {
	TemplateID  string
	TargetDir   string
	ProjectName string
	PackageName string
	Force       bool
}
