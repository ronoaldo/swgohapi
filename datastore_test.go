package swgohapi

import (
	"testing"
	"time"

	"google.golang.org/appengine/aetest"
)

func TestSaveAndGetPlayerData(t *testing.T) {
	c, closer, err := aetest.NewContext()
	if err != nil {
		t.Fatalf("Error initializing test: %v", err)
	}
	defer closer()
	profile := Profile{
		LastUpdate: time.Now(),
	}
	player := "ronoaldo"
	playerData := PlayerData{}
	if err := playerData.Encode(&profile); err != nil {
		t.Fatalf("Error encoding profile: %v", err)
	}
	if err = SavePlayerData(c, player, &playerData); err != nil {
		t.Fatalf("Error saving player data: %v", err)
	}
	cached, err := GetPlayerData(c, player)
	if err != nil {
		t.Fatalf("Error loading saved player data: %v", err)
	}
	if cached.LastUpdate.Unix() != playerData.LastUpdate.Unix() {
		t.Fatalf("Cached data: %v !=  profile data:  %v", cached.LastUpdate, playerData.LastUpdate)
	}
}
