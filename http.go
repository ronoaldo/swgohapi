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
	fullUpdate := r.FormValue("fullUpdate") == "true"
	p, err := GetProfile(c, user, fullUpdate)
	if err != nil {
		log.Errorf(c, "Error loading profile: %v", err)
		if p == nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		} else {
			log.Infof(c, "Returning cached profile")
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err = json.NewEncoder(w).Encode(p); err != nil {
		log.Errorf(c, "Error encoding profile: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func ReloadAll(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	log.Infof(c, "Running schedule all")
	q := datastore.NewQuery(PlayerDataKind)
	tasks := make([]*taskqueue.Task, 0)
	for t := q.Run(c); ; {
		var p PlayerData
		key, err := t.Next(&p)
		if err == datastore.Done {
			break
		}
		if err != nil {
			log.Warningf(c, "Error loading player data: %v", err)
			return
		}
		if time.Since(p.LastUpdate) < 24*time.Hour {
			log.Debugf(c, "Profile skipped, recently updated.")
			continue
		}
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
}
