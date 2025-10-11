package main

import (
	"context"
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	rdb    *redis.Client
	funcs = template.FuncMap{
		"toJson": toJson,
	}
	logger = log.New(os.Stdout, "election-demo: ", log.LstdFlags|log.Lshortfile)
	templates = template.Must(template.New("").Funcs(funcs).ParseGlob("templates/*.html"))
)

type PollingUnit struct {
	ID    int     `json:"id"`
	Name  string  `json:"name"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Votes struct {
		CandidateA int `json:"candidate_a"`
		CandidateB int `json:"candidate_b"`
		CandidateC int `json:"candidate_c"`
	} `json:"votes"`
	Metrics struct {
		MachineUptime  string `json:"machine_uptime"`
		BallotsCast    int    `json:"ballots_cast"`
		SpoiledBallots int    `json:"spoiled_ballots"`
	} `json:"metrics"`
}

type Vote struct {
	PollingUnitID int `json:"polling_unit_id"`
	Votes         int `json:"votes"`
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	// get json file and send the data to the frontend
	jsonData := map[string]any{
		"polling_units": []PollingUnit{},
	}
	file, err := os.Open("data/votes.json")
	if err != nil {
		logger.Println("Error opening JSON file:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer file.Close()
	// Decode JSON data
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&jsonData); err != nil {
		logger.Println("Error decoding JSON:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// Prepare data for template
	data := map[string]any{
		"Title":         "Polling Units Dashboard",
		"PollingUnits":  jsonData["polling_units"],
	}
	// Render template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	// Pass data to template
    templates.ExecuteTemplate(w, "map.html", data)
}

func main() {
	logger.Println("Hello, Voters!.....")
	// Background Context
	ctx := context.Background()
	// Initialize Redis client
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	// Setting up routes
	mux := http.NewServeMux()
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
    mux.HandleFunc("/health", healthCheckHandler)
	// Voting stream endpoint
	mux.HandleFunc("/vote-events", handleVoteStream)
	// Main application routes
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/map", homeHandler) // Serve map on /map as well
	// Example stats handler
	mux.HandleFunc("/stats/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<h1>Stats Page - Under Construction</h1>"))
	})
	// Example recent activity handler
	mux.HandleFunc("/recent", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<h1>Recent Activity Page - Under Construction</h1>"))
	})
	server := &http.Server{
		Addr:    "0.0.0.0:8090",
		Handler: mux,
	}

	// Start Redis stream consumer
	go consumeVotes(ctx, "votes-stream")

	// Handle OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Println("Server running on :8090")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server error: %v", err)
		}
	}()

	sig := <-sigChan
	logger.Printf("Received %s — shutting down...", sig)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	server.Shutdown(ctx)
	logger.Println("Server stopped cleanly.")
}
// basic health check handler
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}
// toJson helper
func toJson(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Println("toJson error:", err)
		return "null"
	}
	return string(b)
}
// Vote SSE (Server-Sent Events) setup
func handleVoteStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Listen for Redis vote updates via channel
	pubsub := rdb.Subscribe(context.Background(), "votes-channel")
	defer pubsub.Close()

	for msg := range pubsub.Channel() {
		w.Write([]byte("data: " + msg.Payload + "\n\n"))
		flusher.Flush()
	}
}
// consumeVotes reads from Redis stream and publishes to a channel
func consumeVotes(ctx context.Context, streamName string) {
	lastID := "$" // start from latest
	for {
		streams, err := rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{streamName, lastID},
			Block:   0, // block until message
			Count:   1,
		}).Result()
		if err != nil {
			logger.Printf("Redis stream read error: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				lastID = msg.ID

				vote := Vote{}
				if val, ok := msg.Values["data"].(string); ok {
					json.Unmarshal([]byte(val), &vote)
				}

				jsonData, _ := json.Marshal(vote)
				rdb.Publish(ctx, "votes-channel", jsonData)
			}
		}
	}
}