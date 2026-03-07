// Package sensor provides a client for polling a local WiFi sensing sensor.
// It expects the sensor to expose an HTTP endpoint returning JSON.
package sensor

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Reading is the parsed response from the sensor.
type Reading struct {
	// PeopleCount is the number of people detected.
	PeopleCount int `json:"people_count"`
	// Activity is a string describing detected motion/activity (e.g. "walking", "standing", "none").
	Activity string `json:"activity"`
	// Confidence is a 0-1 value indicating detection confidence.
	Confidence float64 `json:"confidence"`
	// Raw holds the full decoded JSON for any additional fields the sensor may return.
	Raw map[string]any `json:"-"`
}

// UnmarshalJSON decodes the sensor response, capturing both known fields and raw data.
func (r *Reading) UnmarshalJSON(data []byte) error {
	// decode into raw map first
	if err := json.Unmarshal(data, &r.Raw); err != nil {
		return err
	}

	// decode known fields using a type alias to avoid recursion
	type alias Reading
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	r.PeopleCount = a.PeopleCount
	r.Activity = a.Activity
	r.Confidence = a.Confidence
	return nil
}

// Client polls the sensor endpoint at the configured interval.
type Client struct {
	endpoint string
	interval time.Duration
	http     *http.Client
	logger   *slog.Logger
}

// New creates a new sensor Client.
func New(endpoint string, interval time.Duration, timeoutSecs int, logger *slog.Logger) *Client {
	return &Client{
		endpoint: endpoint,
		interval: interval,
		http: &http.Client{
			Timeout: time.Duration(timeoutSecs) * time.Second,
		},
		logger: logger,
	}
}

// Poll starts polling the sensor in a loop, sending readings on the returned channel.
// The channel is closed when ctx is cancelled.
func (c *Client) Poll(ctx context.Context) <-chan Reading {
	ch := make(chan Reading, 1)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// fetch immediately on start
		if r, err := c.fetch(ctx); err != nil {
			c.logger.Warn("sensor fetch error", "err", err)
		} else {
			select {
			case ch <- r:
			case <-ctx.Done():
				return
			}
		}

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r, err := c.fetch(ctx)
				if err != nil {
					c.logger.Warn("sensor fetch error", "err", err)
					continue
				}
				select {
				case ch <- r:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch
}

func (c *Client) fetch(ctx context.Context) (Reading, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return Reading{}, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return Reading{}, fmt.Errorf("GET %s: %w", c.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Reading{}, fmt.Errorf("sensor returned HTTP %d", resp.StatusCode)
	}

	var reading Reading
	if err := json.NewDecoder(resp.Body).Decode(&reading); err != nil {
		return Reading{}, fmt.Errorf("decoding sensor response: %w", err)
	}

	c.logger.Debug("sensor reading",
		"people_count", reading.PeopleCount,
		"activity", reading.Activity,
		"confidence", reading.Confidence,
	)

	return reading, nil
}
