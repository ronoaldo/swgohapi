package swgohapi

import (
	"log"
	"testing"
	"time"

	"google.golang.org/appengine/aetest"
)

func TestLoadProfile(t *testing.T) {
	c, closer, err := aetest.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	defer closer()

	// Test if we can encode the whole shit
	start := time.Now()
	p, err := GetProfile(c, "ronoaldo", true)
	log.Printf("First non-cached request: %v", time.Since(start))
	if err != nil {
		t.Fatalf("Error loading and caching profile: %v", err)
	}
	log.Printf("Returned profile (no cache)  %s", p)

	start = time.Now()
	p, err = GetProfile(c, "ronoaldo", true)
	log.Printf("Second, cached request took %v", time.Since(start))
	if err != nil {
		t.Fatalf("Error loading an already cached profile: %v", err)
	}
	log.Printf("Returned profile (cached) %s", p)
}
