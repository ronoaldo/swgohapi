package swgohapi

import (
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
	"google.golang.org/appengine/memcache"
)

// PlayerDataKind is the Datastore Kind used to save the profile cache.
const PlayerDataKind = "ProfileCache"

// PlayerData is a datastore structure that caches the player
// data. LastUpdate will match the last time at which website
// parsed the data (fetched from the profile), and is indexed
// for use in the periodic reload.
type PlayerData struct {
	Key        *datastore.Key `datastore:"-"`
	LastUpdate time.Time
	Data       []byte
}

// Decode returns the Profile from the PlayerData.Data attribute.
// Returns an error if player data is an invalid JSON.
func (p *PlayerData) Decode() (*Profile, error) {
	if p == nil {
		return nil, fmt.Errorf("swgohapi: nil player data")
	}
	var profile Profile
	err := json.Unmarshal(p.Data, &profile)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

// Encode updates the PlayerData.Data attribute with a valid content.
func (p *PlayerData) Encode(profile *Profile) error {
	b, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	p.Data = b
	p.LastUpdate = profile.LastUpdate
	return nil
}

// Expired returns true if the profile has expired.
// Profile is assumed to be expired if the time since
// the last update is larger than 24 hours.
func (p *PlayerData) Expired() bool {
	return time.Since(p.LastUpdate) >= 24*time.Hour
}

// GetPlayerData fetches a PlayerData structure from the Cloud Datastore.
// Return both nil data and nil error if no value is cached.
func GetPlayerData(c context.Context, player string) (playerData *PlayerData, err error) {
	playerData = &PlayerData{}
	// Let's try from memcache first
	_, err = memcache.JSON.Get(c, player, playerData)
	// If an error, fetch from datastore. Otherwise it is filled in playerData.
	if err != nil {
		log.Debugf(c, "Not found on memcache, fetching from datastore (err=%v)", err)
		key := datastore.NewKey(c, PlayerDataKind, player, 0, nil)
		err = datastore.Get(c, key, playerData)
		if err == nil {
			if err := memcache.JSON.Set(c, &memcache.Item{Key: player, Object: playerData}); err != nil {
				log.Errorf(c, "Unable to save to cache: %v", err)
			}
		}
	}
	return playerData, err
}

// SavePlayerData updates the player data into Datastore.
func SavePlayerData(c context.Context, player string, playerData *PlayerData) (err error) {
	if playerData == nil {
		return fmt.Errorf("swgohapi: error saving: nil player data")
	}
	key := datastore.NewKey(c, PlayerDataKind, player, 0, nil)
	_, err = datastore.Put(c, key, playerData)
	if err == nil {
		if err := memcache.JSON.Set(c, &memcache.Item{Key: player, Object: playerData}); err != nil {
			log.Errorf(c, "Unable to save to cache: %v", err)
		}
	}
	return err
}

// PlayerStats represents system-wide player profile statistics
type PlayerStats struct {
	PlayerCount      int
	StalePlayerCount int

	OldestPlayerSync time.Time `datastore:"LastUpdate"`
}

// GetPlayerStats returns basic statistics about player profiles in the system.
func GetPlayerStats(c context.Context) (s PlayerStats, err error) {
	s = PlayerStats{}
	// Query how many player profiles we have in database.
	s.PlayerCount, err = datastore.NewQuery(PlayerDataKind).
		Count(c)
	if err != nil {
		return
	}
	// Query how many stale profiles we have currently.
	s.StalePlayerCount, err = datastore.NewQuery(PlayerDataKind).
		Filter("LastUpdate <= ", time.Now().AddDate(0, 0, -1)).
		Count(c)
	if err != nil {
		return
	}
	// Query the oldest data we have.
	_, err = datastore.NewQuery(PlayerDataKind).
		Order("LastUpdate").
		Project("LastUpdate").
		Limit(1).
		Run(c).
		Next(&s)
	if err == datastore.Done {
		err = nil
	}
	return
}

// ListStalePlayers returns a list of the oldest 100 profiles that needs to be sync'ed
func ListStalePlayers(c context.Context) (profiles []PlayerData, err error) {
	q := datastore.NewQuery(PlayerDataKind).
		Filter("LastUpdate <= ", time.Now().AddDate(0, 0, -1)).
		Order("LastUpdate").
		Limit(100)
	keys, err := q.GetAll(c, &profiles)
	if err != nil {
		return nil, err
	}
	for i := range keys {
		profiles[i].Key = keys[i]
	}
	return
}
