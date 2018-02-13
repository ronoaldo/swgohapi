package swgohapi

import (
	"fmt"
	"html/template"
	"net/http"

	"golang.org/x/net/context"
	"google.golang.org/appengine/log"

	"google.golang.org/appengine"
)

func init() {
	http.HandleFunc("/_swgoh/admin", adminHandler)
}

func adminHandler(w http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	values := make(map[string]interface{})
	switch r.Method {
	case "GET":
		stats, err := GetPlayerStats(ctx)
		if err != nil {
			errorf(ctx, w, "Error loading player stats: %v", err)
			return
		}
		values["Stats"] = stats
		err = renderTemplate(w, "admin.html", values)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func renderTemplate(w http.ResponseWriter, name string, values map[string]interface{}) error {
	tpl, err := template.New("base.html").ParseFiles("templates/base.html", "templates/"+name)
	if err != nil {
		return err
	}
	return tpl.Execute(w, values)
}

func errorf(ctx context.Context, w http.ResponseWriter, msg string, args ...interface{}) {
	log.Warningf(ctx, msg, args...)
	http.Error(w, fmt.Sprintf(msg, args...), http.StatusInternalServerError)
}
