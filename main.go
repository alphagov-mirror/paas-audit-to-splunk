package main

import (
	"sync"

	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	uaaAPI = kingpin.Flag(
		"uaa-api-url",
		"UAA API URL to be used for authentication.",
	).Required().Envar("UAA_API_URL").String()

	uaaClientID = kingpin.Flag(
		"uaa-client-id",
		"UAA Client ID for initial client authentication.",
	).Required().Envar("UAA_CLIENT_ID").String()

	uaaClientSecret = kingpin.Flag(
		"uaa-client-secret",
		"UAA Secret ID for initial client authentication.",
	).Required().Envar("UAA_CLIENT_SECRET").String()

	cfAPI = kingpin.Flag(
		"cf-api-url",
		"CloudFoundry API URL to be used for scrapability.",
	).Required().Envar("CLOUDFOUNDRY_API_URL").String()

	scrapeCadence = kingpin.Flag(
		"scrape-cadence",
		"CloudFoundry Password for the User capable of scraping the audit logs.",
	).Default("15m").Envar("SCRAPE_CADENCE").Duration()

	splunkURL = kingpin.Flag(
		"splunk-url",
		"Splunk URL",
	).Required().Envar("SPLUNK_URL").String()

	splunkKey = kingpin.Flag(
		"splunk-key",
		"Splunk API Key",
	).Required().Envar("SPLUNK_KEY").String()
)

func main() {
	kingpin.Parse()

	collectLogs := make(chan []byte, 1024)
	shipLogs := make(chan []byte, 1024)

	uaa := Auth{
		UAAAPI:          *uaaAPI,
		UAAClientID:     *uaaClientID,
		UAAClientSecret: *uaaClientSecret,
	}

	collector := Collector{
		AuthClient:      uaa,
		CloudFoundryAPI: *cfAPI,

		ScrapeCadence: *scrapeCadence,
		Destination:   collectLogs,
	}

	deduplicator := Deduplicator{
		Source:      collectLogs,
		Destination: shipLogs,
	}

	shipper := Shipper{
		Source:    shipLogs,
		SplunkURL: *splunkURL,
		SplunkKey: *splunkKey,
	}

	waiter := sync.WaitGroup{}

	waiter.Add(1)
	go func() {
		deduplicator.Deduplicate()
		waiter.Done()
	}()

	waiter.Add(1)
	go func() {
		shipper.Ship()
		waiter.Done()
	}()

	waiter.Add(1)
	go func() {
		collector.Collect()
		waiter.Done()
	}()

	waiter.Wait()
}
