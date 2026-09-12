package screens

type TemplateItem struct {
	ID          string
	DisplayName string
	Description string
}

func TemplateSelect(items []TemplateItem, cursor int, t Theme) string {
	body := t.Heading("Choose a template", "A solid starting point, ready to make your own.")
	if len(items) == 0 {
		return body + t.Muted("No templates available. Esc to return.")
	}
	for i, item := range items {
		body += t.Choice(item.DisplayName, "", i == cursor) + "  " + t.Muted(item.Description) + "\n\n"
	}
	return body
}
