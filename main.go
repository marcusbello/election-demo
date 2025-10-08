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

)

var (
	totalPollingUnits = 1000
	totalVotes = 130000
	
	funcs = template.FuncMap{
		// exported html functions
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
	// Setting up routes
	mux := http.NewServeMux()
    mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
    mux.HandleFunc("/health", healthCheckHandler)
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
	// Setting up the HTTP server
	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	// Setting up signal handling
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		logger.Println("Server running on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Server error: %v", err)
		}
	}()

	for {
		sig := <-signalChan
		switch sig {
		case syscall.SIGHUP:
			logger.Println("Received SIGHUP (reload requested)")
		case syscall.SIGINT, syscall.SIGTERM:
			logger.Printf("Received %s — shutting down...", sig)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := server.Shutdown(ctx); err != nil {
				logger.Fatalf("Graceful shutdown failed: %v", err)
			}
			logger.Println("Server stopped cleanly.")
			return
		default:
			logger.Printf("Unhandled signal: %v", sig)
		}
	}

}

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
