package swgohapi

import (
	"encoding/json"
	"net/http"
	"strings"

	"google.golang.org/appengine"
	"google.golang.org/appengine/log"
)

func init() {
	http.HandleFunc("/v1/profile/", ProfileHandler)
}

func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	user := strings.Replace(r.URL.Path, "/v1/profile/", "", -1)
	if user == "" {
		log.Debugf(c, "Invalid profile: %v", user)
		http.Error(w, "Invalid profile: "+user, http.StatusBadRequest)
		return
	}
	p, err := GetProfile(c, user)
	if err != nil {
		log.Errorf(c, "Error loading profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(p); err != nil {
		log.Errorf(c, "Error encoding profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
