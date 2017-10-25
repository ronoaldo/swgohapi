package swgohapi

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"google.golang.org/appengine"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
)

func init() {
	http.HandleFunc("/v1/profile/", ProfileHandler)
	http.HandleFunc("/admin/reloadAll", ReloadAll)
}

func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	user := strings.Replace(r.URL.Path, "/v1/profile/", "", -1)
	if user == "" {
		log.Infof(c, "Invalid profile: %v", user)
		http.Error(w, "Invalid profile: "+user, http.StatusBadRequest)
		return
	}
	// TODO: use always lower case username - normalizes a lot of bugs.
	if unescaped, err := url.QueryUnescape(user); err == nil {
		user = unescaped
	}
	// For simplicity, if we are told to reaload, just parse the whole
	// data from site and save again.
	fullUpdate := r.FormValue("fullUpdate") == "true"
	if fullUpdate {
		ReloadProfile(c, user, fullUpdate)
	}

	// Lookup the cached profile...
	p, err := GetProfile(c, user)
	// ... if failure, report
	if err != nil {
		log.Errorf(c, "Error loading profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	// ... if not found, schedule
	if p == nil {
		if err = ReloadProfileAsync(c, user, false); err != nil {
			log.Errorf(c, "Error loading scheduling profile sync: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		http.Error(w, "{\"Status\": \"Reloading\"}", http.StatusAccepted)
		return
	}
	// ... render the API response
	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(p); err != nil {
		log.Errorf(c, "Error encoding profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// ReloadAll fore-reload all expired cache data.
func ReloadAll(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	log.Infof(c, "Running schedule all routine ... ")
	q := datastore.NewQuery(PlayerDataKind).
		Filter("LastUpdate <", time.Now().Add(-24*time.Hour)).
		KeysOnly()
	expired, err := q.GetAll(c, nil)
	if err != nil {
		log.Errorf(c, "Error loading expired profiles: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Infof(c, "Found %d expired profiles", len(expired))
	tasks := make([]*taskqueue.Task, 0)
	for _, key := range expired {
		escapedProfile := url.QueryEscape(key.StringID())
		escapedProfile = strings.Replace(escapedProfile, "+", "%20", -1)
		tasks = append(tasks, taskqueue.NewPOSTTask("/v1/profile/"+escapedProfile, url.Values{}))
		log.Debugf(c, "Added task for %s", escapedProfile)
		if len(tasks) > 10 {
			log.Infof(c, "Scheduling profiles %v", tasks)
			if _, err := taskqueue.AddMulti(c, tasks, "sync"); err != nil {
				log.Warningf(c, "Error scheduling: %v", err)
			}
			tasks = make([]*taskqueue.Task, 0)
		}
	}
	if len(tasks) > 0 {
		log.Infof(c, "Scheduling profiles %v", tasks)
		if _, err := taskqueue.AddMulti(c, tasks, "sync"); err != nil {
			log.Warningf(c, "Error scheduling: %v", err)
		}
	}
	log.Infof(c, "Done")
}
