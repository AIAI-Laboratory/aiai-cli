package screens

// TemplateItem carries display metadata for one selectable setup pack or project template.
type TemplateItem struct {
	ID          string
	DisplayName string
	Description string
}

// TemplateSelect renders the setup pack chooser screen.
func TemplateSelect(items []TemplateItem, cursor int, t Theme) string {
	body := t.Heading("Choose a setup pack", "Start with Agent Setup for any repository, or scaffold a new project.")
	if len(items) == 0 {
		return body + t.Muted("No templates available. Esc to return.")
	}
	for i, item := range items {
		prefix := "  "
		name := t.Truncate(item.DisplayName, max(1, t.Width()-2))
		if i == cursor {
			prefix = "> "
			name = t.Accent(name)
		} else {
			name = t.Title(name)
		}
		body += prefix + name + "\n"
		if item.Description != "" {
			body += "  " + t.Muted(t.Truncate(item.Description, max(1, t.Width()-2))) + "\n"
		}
		if i < len(items)-1 {
			body += "\n"
		}
	}
	return body
}
