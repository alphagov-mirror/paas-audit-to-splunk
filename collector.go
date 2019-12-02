package main

import (
	// "context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	// "io"
	"log"
	"net/http"
	"time"

	"github.com/cenkalti/backoff"
)

type resource struct {
	// GUID is unique identifier for the event.
	GUID string `json:"guid"`
	// Type of the event.
	Type  string `json:"type"`
	Actor struct {
		// GUID is unique identifier for the actor (user or system resource that performed the action).
		GUID string `json:"guid"`
		// Type of the actor.
		Type string `json:"type"`
		// Name of the actor.
		Name string `json:"name"`
	} `json:"actor"`
	Target struct {
		// GUID nique identifier for the target (resource that the event acted upon).
		GUID string `json:"guid"`
		// Type of the target.
		Type string `json:"type"`
		// Name of the target.
		Name string `json:"name"`
	} `json:"target"`
	// Data poseses additional information about the event.
	Data interface{} `json:"data"`
	// Space the event is related to.
	Space struct {
		// GUID is unique identifier for the space where the event occurred. If the event did not occur within a space, the space field will be null.
		GUID string `json:"guid,omitempty"`
	} `json:"space,omitempty"`
	// Organization the event is related to.
	Organization struct {
		// GUID is unique identifier for the organization where the event occurred. If the event did not occur within an organization, the organization field will be null.
		GUID string `json:"guid"`
	} `json:"organization"`
	// CreatedAt is a timestamp representative of when did the event take place.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is a timestamp representative of when has the event been modified.
	UpdatedAt time.Time `json:"modified_at,omitempty"`
}

type eventsResponse struct {
	Resources  []resource `json:"resources"`
	Pagination struct {
		Next struct {
			Link string `json:"href"`
		} `json:"next"`
	} `json:"pagination"`
}

type Collector struct {
	Destination chan []byte

	AuthClient      Auth
	CloudFoundryAPI string

	ScrapeCadence time.Duration

	LastSeenGUID string

	client *http.Client
}

type CloudFoundryTransport struct {
	accessToken string
}

func (t *CloudFoundryTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", fmt.Sprintf("bearer %s", t.accessToken))
	return http.DefaultTransport.RoundTrip(r)
}

func (c *Collector) Collect() {
	ticker := time.NewTicker(c.ScrapeCadence)
	defer ticker.Stop()

	if err := c.AuthClient.Authenticate(); err != nil {
		log.Panicf("unable to authenticate: %s", err)
	}

	c.client = &http.Client{}
	c.client.Transport = &CloudFoundryTransport{accessToken: c.AuthClient.AccessToken()}

	log.Println("Collector: Start")

	for {
		select {
		case <-ticker.C:
			log.Println("Collector: Tick")

			collectEvents := func() error {
				firstPage := eventsResponse{}
				firstPage.Pagination.Next.Link = "/v3/audit_events?order_by=-created_at&per_page=500"

				data, err := c.recursivelyGather(firstPage)
				if err != nil {
					return fmt.Errorf("unable to recursively obtain resources: %s", err)
				}

				// FIXME: We need a more reliable way of creating a checkpoint for the future runs...
				// This is essentially responsible for not having to go through ALL of the events over and over again,
				// and should drop dead next time.
				// Ideally, this should be persisted in a database...
				if len(data) > 0 {
					c.LastSeenGUID = data[0].GUID
				}

				return c.SanitizeAndQueue(data)
			}

			err := backoff.Retry(
				collectEvents,
				backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 10),
			)

			if err != nil {
				log.Fatalf(
					"Collector: Fatal err encountered after 10 retries: %s\n", err,
				)
			}

		}
	}
}

func (c *Collector) SanitizeAndQueue(data []resource) error {
	for _, event := range data {
		event.Actor.Name = `¯\_(ツ)_/¯`

		msg, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("unable to marshal an event into json: %s", err)
		}

		c.Destination <- msg
	}

	return nil
}

func (c *Collector) recursivelyGather(response eventsResponse) ([]resource, error) {
	var data = response.Resources

	if response.Pagination.Next.Link == "" {
		return data, nil
	}

	res, err := c.client.Get(fmt.Sprintf("%s%s", c.CloudFoundryAPI, response.Pagination.Next.Link))
	if err != nil {
		return data, fmt.Errorf("unable to scrape the audit events: %s", err)
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return data, fmt.Errorf("unable to parse authentication response: %s", err)
	}

	var newResponse eventsResponse
	err = json.Unmarshal(body, &newResponse)
	if err != nil {
		return data, fmt.Errorf("unable to unmarshal authentication response: %s", err)
	}

	// FIXME: Remove this when there is a persistance for the LastSeenGUID...
	someHoursAgo := time.Now().Add(-1 * 6 * time.Hour)

	for i, event := range newResponse.Resources {
		// FIXME: The weird `CreatedAt` based check can go away as soon as there is a persistance for the LastSeenGUID...
		if event.GUID == c.LastSeenGUID || event.CreatedAt.Before(someHoursAgo) {
			log.Printf("Shipped until %s\n", event.CreatedAt)
			return append(data, newResponse.Resources[:i]...), nil
		}
	}
	newData, err := c.recursivelyGather(newResponse)

	return append(data, newData...), nil
}
