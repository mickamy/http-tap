package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

type user struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

var users = []user{
	{ID: 1, Name: "Alice"},
	{ID: 2, Name: "Bob"},
	{ID: 3, Name: "Charlie"},
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/users", handleListUsers)
	mux.HandleFunc("GET /api/users/{id}", handleGetUser)
	mux.HandleFunc("POST /api/users", handleCreateUser)
	mux.HandleFunc("DELETE /api/users/{id}", handleDeleteUser)
	mux.HandleFunc("GET /api/slow", handleSlow)
	mux.HandleFunc("GET /api/error", handleError)

	addr := ":9000"
	log.Printf("example server listening on %s", addr)
	//nolint:gosec // G114: example server
	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Printf("error: %v\n", err)
	}
}

func handleListUsers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, users)
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for _, u := range users {
		if fmt.Sprintf("%d", u.ID) == id {
			writeJSON(w, http.StatusOK, u)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	u := user{ID: len(users) + 1, Name: req.Name}
	users = append(users, u)
	writeJSON(w, http.StatusCreated, u)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	for i, u := range users {
		if fmt.Sprintf("%d", u.ID) == id {
			users = append(users[:i], users[i+1:]...)
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
}

func handleSlow(w http.ResponseWriter, _ *http.Request) {
	//nolint:gosec // G404: example code
	time.Sleep(time.Duration(500+rand.IntN(1500)) * time.Millisecond)
	writeJSON(w, http.StatusOK, map[string]string{"message": "done"})
}

func handleError(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "something went wrong"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
