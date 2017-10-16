package swgohapi

import (
	"fmt"
	"net/url"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
	"google.golang.org/appengine/urlfetch"
	"ronoaldo.gopkg.net/swgoh/swgohgg"
)

// Profile is an entity that saves user data from the website
type Profile struct {
	LastUpdate time.Time
	Collection swgohgg.Collection
	Ships      swgohgg.Ships
	Arena      []*swgohgg.CharacterStats
	Stats      []*swgohgg.CharacterStats
}

func (p *Profile) String() string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("<Profile: %d characters, %d ships>", len(p.Collection), len(p.Ships))
}

func GetProfile(c context.Context, user string, fullUpdate bool) (*Profile, error) {
	// Try to load from cache
	log.Infof(c, "Loading profile %v (fullUpdate=%v)", user, fullUpdate)
	playerData, err := GetPlayerData(c, user)
	profile := &Profile{}
	if err == nil {
		log.Infof(c, "Returning from cache!")
		// Found in cache, check if expired!
		profile, err = playerData.Decode()
		if err != nil {
			return nil, err
		}
		log.Infof(c, "Cached playerData found, updated %v ago", time.Since(profile.LastUpdate))
		if time.Since(profile.LastUpdate) < 24*time.Hour && !fullUpdate {
			log.Infof(c, "Not checking uptime, profile from cache is fresh.")
			return profile, nil
		}
	}
	if err != datastore.ErrNoSuchEntity && err != nil {
		return nil, err
	}

	// Profile not cached, let's fetch and save after some checking
	withTimeout, closer := context.WithTimeout(c, 120*time.Second)
	defer closer()
	hc := urlfetch.Client(withTimeout)
	gg := swgohgg.NewClient(user).UseHTTPClient(hc)

	log.Infof(c, "Loading arena team ...")
	arena, lastUpdate, err := gg.Arena()
	if err != nil {
		return profile, err
	}
	log.Infof(c, "Site last update was %v ago", time.Since(lastUpdate))
	// Check if we need a full reload. If website is lower than a day, and we
	// are not, let's reload. Otherwise, assume website is also outdated.
	if !profile.LastUpdate.IsZero() && time.Since(lastUpdate) > 24*time.Hour {
		log.Infof(c, "Site is probably as old as us, lets use what we have here.")
		return profile, err
	}

	profile.Arena = arena
	profile.LastUpdate = lastUpdate

	log.Infof(c, "Loading collection ...")
	if profile.Collection, err = gg.Collection(); err != nil {
		return profile, err
	}
	log.Infof(c, "Loading ships ...")
	if profile.Ships, err = gg.Ships(); err != nil {
		return profile, err
	}
	if fullUpdate {
		log.Infof(c, "Loading character stats ...")
		if err = fetchAllStats(c, gg, profile); err != nil {
			return profile, err
		}
	} else {
		log.Infof(c, "Scheduling full stats update ...")
		t := taskqueue.NewPOSTTask("/v1/profile/"+user, url.Values{
			"fullUpdate": {"true"},
		})
		if _, err := taskqueue.Add(c, t, "sync"); err != nil {
			log.Warningf(c, "Failed to schedule task: %v", err)
		}
	}
	if err = playerData.Encode(profile); err != nil {
		return profile, err
	}
	if err = SavePlayerData(c, user, playerData); err != nil {
		return profile, err
	}
	return profile, nil
}

// fetchAllStats run parallell code that fetches all user profiles.
func fetchAllStats(c context.Context, gg *swgohgg.Client, profile *Profile) error {
	// Split into two workers to half
	workCount := 10
	step := len(profile.Collection) / workCount

	buff := make(chan swgohgg.CharacterStats, workCount)
	done := make(chan bool)
	errors := make(chan error, 5)
	errorList := make([]error, 0)

	fetchBlock := func(worker, start, limit int, buff chan swgohgg.CharacterStats, done chan bool, errors chan error) {
		log.Infof(c, "Starting worker %d [%d:%d]", worker, start, limit)
		retryCount := 0
		for i := start; i < limit; i++ {
			char := profile.Collection[i]
			if char.Stars <= 0 {
				log.Infof(c, "[%d] Ignored inactive character %s", worker, char.Name)
				continue
			}
			log.Infof(c, "[%d] Loading %s ...", worker, char.Name)
			stat, err := gg.CharacterStats(char.Name)
			if err != nil {
				i--
				retryCount++
				time.Sleep(1 * time.Second)
				if retryCount > 2 {
					errors <- err
					break
				}
				continue
			}
			buff <- *stat
		}
		log.Infof(c, "[%d] Worker completed", worker)
		done <- true
	}

	aggregate := func(profile *Profile, buff chan swgohgg.CharacterStats) {
		for stat := range buff {
			statCopy := stat
			profile.Stats = append(profile.Stats, &statCopy)
			log.Infof(c, "Stats so far %d", len(profile.Stats))
		}
	}

	aggregateErr := func(out []error, errors chan error, done chan bool) {
		for err := range errors {
			log.Infof(c, "> Error: %v", err)
			out = append(out, err)
		}
		done <- true
	}

	// Star worker until buffer is empty
	go aggregate(profile, buff)
	go aggregateErr(errorList, errors, done)

	// Run and wait both parallell tasks to fetch all data
	log.Infof(c, "Starting all workers ...")
	start := 0
	for i := 0; i < workCount; i++ {
		if i == (workCount - 1) {
			go fetchBlock(i, start, len(profile.Collection), buff, done, errors)
		} else {
			go fetchBlock(i, start, start+step, buff, done, errors)
		}
		start += step
	}
	// Wait each worker to exit
	for i := 0; i < workCount; i++ {
		<-done
	}
	log.Infof(c, "All workers are done! Closing channels ... ")

	// Close buff to finish off aggregate
	close(buff)
	// Close error channel and wait final worker
	close(errors)
	<-done

	// Wait to consume all errors
	if len(errorList) > 0 {
		return fmt.Errorf("Several errors: %v", errorList)
	}

	return nil
}
