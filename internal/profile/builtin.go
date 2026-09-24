package profile

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

const (
	BuiltinTicketProfileID      = "default"
	BuiltinTicketProfileVersion = "v1"
)

//go:embed assets/ticket-profile.yaml assets/templates/*
var builtinFS embed.FS

func BuiltinTicketProfile() (Profile, error) {
	file, err := builtinFS.Open("assets/ticket-profile.yaml")
	if err != nil {
		return Profile{}, fmt.Errorf("open built-in ticket profile: %w", err)
	}
	defer file.Close()

	value, err := DecodeProfile(file)
	if err != nil {
		return Profile{}, fmt.Errorf("decode built-in ticket profile: %w", err)
	}
	return value, nil
}

func BuiltinTemplates() []Template {
	entries, err := fs.ReadDir(builtinFS, "assets/templates")
	if err != nil {
		panic(fmt.Sprintf("read embedded profile templates: %v", err))
	}
	result := make([]Template, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		content, readErr := fs.ReadFile(
			builtinFS,
			"assets/templates/"+entry.Name(),
		)
		if readErr != nil {
			panic(fmt.Sprintf(
				"read embedded profile template %s: %v",
				entry.Name(),
				readErr,
			))
		}
		result = append(result, Template{
			ID:          "ticket/" + trimTemplateExtension(entry.Name()),
			Version:     BuiltinTicketProfileVersion,
			SourceScope: "builtin",
			Content:     append([]byte(nil), content...),
		})
	}
	sort.Slice(result, func(left, right int) bool {
		return result[left].ID < result[right].ID
	})
	return result
}

func trimTemplateExtension(name string) string {
	const suffix = ".tmpl"
	if len(name) >= len(suffix) &&
		name[len(name)-len(suffix):] == suffix {
		return name[:len(name)-len(suffix)]
	}
	return name
}
