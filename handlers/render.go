package handlers

import (
	"html/template"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
)

var templates = make(map[string]*template.Template)

func funcMap() template.FuncMap {
	return template.FuncMap{
		"stringsHasSuffix": func(s string, suffixes ...string) bool {
			for _, suff := range suffixes {
				if strings.HasSuffix(strings.ToLower(s), suff) {
					return true
				}
			}
			return false
		},
	}
}

// InitEmbeddedTemplates parses templates from an embedded filesystem (bulletproof, zero-disk dependency)
func InitEmbeddedTemplates(embedFS fs.FS, dir string) error {
	entries, err := fs.ReadDir(embedFS, dir)
	if err != nil {
		return err
	}

	layoutPath := filepath.ToSlash(filepath.Join(dir, "layout.html"))
	layoutContent, err := fs.ReadFile(embedFS, layoutPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == "layout.html" || !strings.HasSuffix(name, ".html") {
			continue
		}

		pagePath := filepath.ToSlash(filepath.Join(dir, name))
		pageContent, err := fs.ReadFile(embedFS, pagePath)
		if err != nil {
			return err
		}

		tmpl, err := template.New(name).Funcs(funcMap()).Parse(string(layoutContent))
		if err != nil {
			return err
		}

		tmpl, err = tmpl.Parse(string(pageContent))
		if err != nil {
			return err
		}

		templates[name] = tmpl
	}
	return nil
}

// InitTemplates fallback for local file loading
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

		tmpl, err := template.New(baseName).Funcs(funcMap()).ParseFiles(layoutPath, pagePath)
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

	err := tmpl.ExecuteTemplate(w, "layout", data)
	if err != nil {
		http.Error(w, "Error rendering template: "+err.Error(), http.StatusInternalServerError)
	}
}
