package swgohapi

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"regexp"

	"github.com/ronoaldo/swgoh/swgohgg"
	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/taskqueue"
	"google.golang.org/appengine/urlfetch"
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

var safeQueueNameRe = regexp.MustCompile("[^a-zA-Z0-9-]")

func safeQueueName(src string) string {
	return safeQueueNameRe.ReplaceAllString(src, "-")
}

// ReloadProfileAsync schedule the reload of a profile
func ReloadProfileAsync(c context.Context, user string, force bool) error {
	if unescaped, err := url.QueryUnescape(user); err == nil {
		// If we could escape first
		user = unescaped
	}
	user = url.QueryEscape(user)
	user = strings.Replace(user, "+", "%20", -1)
	log.Infof(c, "Scheduling full stats update for '%v' ...", user)
	t := taskqueue.NewPOSTTask("/v1/profile/"+user, url.Values{
		"fullUpdate": {"true"},
	})
	if !force {
		// Use a task name so we avoid reloading like a crazy bitch.
		t.Name = safeQueueName(user) + time.Now().Format("-20060102")
	}
	if _, err := taskqueue.Add(c, t, "sync"); err != nil {
		log.Warningf(c, "Failed to schedule task: %v", err)
	}
	return nil
}

// ReloadProfile fetches all data from the website and saves the
// resulting cache in the datastore/memcache.
func ReloadProfile(c context.Context, user string, fullUpdate bool) (*Profile, error) {
	profile, err := GetProfile(c, user)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		// We don't have the profile
		profile = &Profile{}
	}

	withTimeout, closer := context.WithTimeout(c, 120*time.Second)
	defer closer()
	hc := urlfetch.Client(withTimeout)
	gg := swgohgg.NewClient(user).UseHTTPClient(hc)

	// Temporary disable anything related to the character stats.
	// Arena() and fetchAllStats() are not going to work as the website
	// now requires login information to be provided
	/*log.Infof(c, "Loading arena team ...")
	arena, lastUpdate, err := gg.Arena()
	if err != nil {
		return nil, err
	}
	log.Infof(c, "Site last update was %v ago", time.Since(lastUpdate))
	profile.Arena = arena
	profile.LastUpdate = lastUpdate
	*/
	profile.LastUpdate = time.Now()

	log.Infof(c, "Loading collection ...")
	if profile.Collection, err = gg.Collection(); err != nil {
		return profile, err
	}
	log.Infof(c, "Loading ships ...")
	if profile.Ships, err = gg.Ships(); err != nil {
		return profile, err
	}
	/*log.Infof(c, "Loading character stats ...")
	if err = fetchAllStats(c, gg, profile); err != nil {
		log.Warningf(c, "Errors during fetchAllstats: %#v", err)
	}
	*/
	playerData := &PlayerData{}
	if err = playerData.Encode(profile); err != nil {
		return profile, err
	}
	if err = SavePlayerData(c, user, playerData); err != nil {
		return profile, err
	}
	return profile, nil
}

// GetProfile returns a cached profile from the Datastore/Memcache or nil if not found
func GetProfile(c context.Context, user string) (*Profile, error) {
	// Try to load from cache
	log.Infof(c, "Loading profile %v", user)
	playerData, err := GetPlayerData(c, user)
	profile := &Profile{}
	// Load from cache and decode profile
	if err == nil {
		log.Infof(c, "Returning from cache!")
		// Found in cache, check if expired!
		profile, err = playerData.Decode()
		if err != nil {
			return nil, err
		}
		log.Infof(c, "Cached playerData found, updated %v ago", time.Since(profile.LastUpdate))
		return profile, nil
	}
	// If error loading
	if err != datastore.ErrNoSuchEntity && err != nil {
		return nil, err
	}
	// Not found, (nil, nil)
	return nil, nil
}

// fetchAllStats run parallell code that fetches all user profiles.
func fetchAllStats(c context.Context, gg *swgohgg.Client, profile *Profile) error {
	// Split into two workers to half
	workCount := 2
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
