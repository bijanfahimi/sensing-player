// Package server provides the HTTP server that serves the video player page
// and a Server-Sent Events (SSE) endpoint for pushing ad-change commands to the browser.
package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"sensing-player/internal/config"
)

// AdEvent is sent over SSE to the browser when the active ad changes.
type AdEvent struct {
	AdKey string `json:"ad_key"`
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Server is the HTTP server.
type Server struct {
	cfg    *config.Config
	logger *slog.Logger

	mu          sync.RWMutex
	currentAd   AdEvent
	sseClients  map[chan AdEvent]struct{}
}

// New creates a new Server.
func New(cfg *config.Config, logger *slog.Logger) *Server {
	s := &Server{
		cfg:        cfg,
		logger:     logger,
		sseClients: make(map[chan AdEvent]struct{}),
	}

	// Set initial ad
	if cfg.DefaultAdKey != "" {
		if ad, ok := cfg.Ads[cfg.DefaultAdKey]; ok {
			s.currentAd = AdEvent{
				AdKey: cfg.DefaultAdKey,
				URL:   ad.URL,
				Label: ad.Label,
			}
		}
	}

	return s
}

// SetAd updates the currently displayed ad and broadcasts to all SSE clients.
func (s *Server) SetAd(key string) {
	ad, ok := s.cfg.Ads[key]
	if !ok {
		s.logger.Warn("SetAd: unknown ad key", "key", key)
		return
	}

	event := AdEvent{AdKey: key, URL: ad.URL, Label: ad.Label}

	s.mu.Lock()
	if s.currentAd.AdKey == key {
		s.mu.Unlock()
		return // no change
	}
	s.currentAd = event
	clients := make([]chan AdEvent, 0, len(s.sseClients))
	for ch := range s.sseClients {
		clients = append(clients, ch)
	}
	s.mu.Unlock()

	s.logger.Info("ad changed", "key", key, "label", ad.Label)

	for _, ch := range clients {
		select {
		case ch <- event:
		default:
			// client is slow; skip this event
		}
	}
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Serve static files (index.html, etc.)
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Root serves the player page
	mux.HandleFunc("/", s.handleIndex)

	// SSE endpoint: browser subscribes here for ad-change events
	mux.HandleFunc("/events", s.handleSSE)

	// Current ad state (for initial load)
	mux.HandleFunc("/current", s.handleCurrent)

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "static/index.html")
}

func (s *Server) handleCurrent(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	ev := s.currentAd
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ev)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan AdEvent, 4)

	s.mu.Lock()
	s.sseClients[ch] = struct{}{}
	current := s.currentAd
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.sseClients, ch)
		s.mu.Unlock()
	}()

	s.logger.Info("SSE client connected", "remote", r.RemoteAddr)

	// Send current ad immediately so the page can start playing right away
	writeSSE(w, flusher, current)

	// Heartbeat ticker to keep connection alive
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			s.logger.Info("SSE client disconnected", "remote", r.RemoteAddr)
			return
		case ev := <-ch:
			writeSSE(w, flusher, ev)
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, ev AdEvent) {
	data, _ := json.Marshal(ev)
	fmt.Fprintf(w, "event: ad\ndata: %s\n\n", data)
	flusher.Flush()
}
