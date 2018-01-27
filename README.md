# swgohapi

Cached, parsed https://swgoh.gg/ player profile data

**WARNING** this is a beta software, under active development. Use at your own risk.

## Using the public version

Tired to scrape the website constantly for data consumption?
Getting too much HTTP 402 responses? Freat not, here is your solution.

This API is designed to be an easy escrape-to-JSON converter.
Using ronoaldo/swgoh as parsing library, this microservice
caches the parsed data for it's lifetime (around 24hs) and
serves responses in about 80ms.

Leaveraging memcache for ultra-fast responses, and with a on-demand,
background syncing engine that does not overloads the website, this
API can be used by apps and bots to enhance the player experience.

Please note that you may get blacklisted, graylisted and all sorts
of "listed" things if you use this to bad things.

As uncle Old Ben once told us: "with great power comes great responsibility"

## Hosting your own version

It requires a Google Cloud Platform account, but depending on your
usage, you should stay in the free quota. You also will need to
install Google Cloud SDK and Google App Engine Go SDK.

    appcfg.py update --application your-project-id --version beta .

Please refer to the Google Cloud Platform documentation on Google
App Engine and setting up `gcloud` and the SDK.
