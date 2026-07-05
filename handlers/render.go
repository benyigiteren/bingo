package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"
	"strings"
)

var templates = make(map[string]*template.Template)

// InitTemplates parses templates once at startup
func InitTemplates(dir string) error {
	pages, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return err
	}

	layoutPath := filepath.Join(dir, "layout.html")

	for _, pagePath := range pages {
		baseName := filepath.Base(pagePath)
		if baseName == "layout.html" {
			continue
		}

		// Parse the layout and the individual page together with custom function map
		tmpl, err := template.New(baseName).Funcs(template.FuncMap{
			"stringsHasSuffix": func(s string, suffixes ...string) bool {
				for _, suff := range suffixes {
					if strings.HasSuffix(strings.ToLower(s), suff) {
						return true
					}
				}
				return false
			},
		}).ParseFiles(layoutPath, pagePath)
		if err != nil {
			return err
		}
		templates[baseName] = tmpl
	}

	return nil
}

// RenderTemplate executes the parsed template with the base layout
func RenderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")

	tmpl, exists := templates[name]
	if !exists {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	// We execute the "layout" template defined in layout.html
	err := tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
	}
}
