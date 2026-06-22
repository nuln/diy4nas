package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"version": "1.19.27", "meta": true})
	})

	mux.HandleFunc("/traffic", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"up": 1024.0, "down": 2048.0})
	})

	mux.HandleFunc("/proxies", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"proxies": map[string]any{
				"DIRECT": map[string]string{"type": "Direct", "now": "DIRECT"},
				"REJECT": map[string]string{"type": "Reject", "now": "REJECT"},
				"Proxy": map[string]any{
					"type": "Selector", "now": "DIRECT",
					"all": []string{"DIRECT", "REJECT"},
				},
			},
		})
	})

	mux.HandleFunc("/configs", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			w.WriteHeader(204)
			return
		}
		writeJSON(w, map[string]any{
			"mode": "rule", "log-level": "info",
			"mixed-port": 7890, "allow-lan": true,
			"external-controller": "127.0.0.1:19090",
		})
	})

	port := ":19090"
	log.Printf("Mock mihomo API on %s", port)
	log.Fatal(http.ListenAndServe(port, mux))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
