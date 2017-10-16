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

const PlayerDataKind = "ProfileCache"

type PlayerData struct {
	Key        *datastore.Key `datastore:"-"`
	LastUpdate time.Time
	Data       []byte
}

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

func (p *PlayerData) Encode(profile *Profile) error {
	b, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	p.Data = b
	p.LastUpdate = profile.LastUpdate
	return nil
}

func (p *PlayerData) Expired() bool {
	return time.Since(p.LastUpdate) < 24*time.Hour
}

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
