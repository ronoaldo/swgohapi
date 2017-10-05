package swgohapi

import (
	"encoding/json"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
	"google.golang.org/appengine/log"
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

type ProfileCache struct {
	Data []byte
}

func Encode(profile *Profile) (*ProfileCache, error) {
	b, err := json.Marshal(profile)
	if err != nil {
		return nil, err
	}
	return &ProfileCache{b}, nil
}

func Decode(cache *ProfileCache) (*Profile, error) {
	var profile Profile
	err := json.Unmarshal(cache.Data, &profile)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func GetProfile(c context.Context, user string) (*Profile, error) {
	// Try to load from cache
	profile := &Profile{}
	cache := &ProfileCache{}
	key := datastore.NewKey(c, "ProfileCache", user, 0, nil)
	err := datastore.Get(c, key, cache)
	if err == nil {
		log.Debugf(c, "Returning from cache!")
		// Found in cache, return it
		return Decode(cache)
	}

	if err != datastore.ErrNoSuchEntity {
		return nil, err
	}

	// Profile not cached, let's fetch and save
	withTimeout, closer := context.WithTimeout(c, 60*time.Second)
	defer closer()
	hc := urlfetch.Client(withTimeout)
	gg := swgohgg.NewClient(user).UseHTTPClient(hc)
	log.Debugf(c, "Loading arena team ...")
	if profile.Arena, err = gg.Arena(); err != nil {
		return nil, err
	}
	log.Debugf(c, "Loading collection ...")
	if profile.Collection, err = gg.Collection(); err != nil {
		return nil, err
	}
	log.Debugf(c, "Loading ships ...")
	if profile.Ships, err = gg.Ships(); err != nil {
		return nil, err
	}
	// Todo: fetch all character stats and cache it up baby
	if cache, err = Encode(profile); err != nil {
		return nil, err
	}
	if key, err = datastore.Put(c, key, cache); err != nil {
		return nil, err
	}
	return profile, nil
}
