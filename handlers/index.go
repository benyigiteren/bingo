package handlers

import (
	"gotree/db"
	"gotree/middleware"
	"net/http"
)

// IndexHandler ana dizine (/) gelen istekleri yönlendirir veya ana sayfa biyografisini açar.
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	// Sadece tam eşleşme durumunda çalışsın
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// 1. Sistemde kullanıcı var mı?
	hasUsers, err := db.HasUsers()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !hasUsers {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}

	// 2. Bir varsayılan ana sayfa slug'ı ayarlanmış mı?
	primarySlug, err := db.GetSetting("primary_slug")
	if err == nil && primarySlug != "" {
		BioGetBySlug(w, r, primarySlug)
		return
	}

	// 3. Giriş yapmış kullanıcı var mı?
	user := middleware.GetUserContext(r)
	if user != nil {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return
	}

	// 4. Giriş yapmamışsa ve varsayılan slug yoksa giriş sayfasına yönlendir
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
