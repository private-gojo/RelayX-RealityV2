package session

import (
	"errors"
	"net/http"
	"os"
	"strconv"
)

func MaxSessionsFromEnv() (int, error) {
	value := os.Getenv("MAX_SESSIONS")
	if value == "" { value = "256" }
	maxSessions, err := strconv.Atoi(value)
	if err != nil || maxSessions < 1 || maxSessions > 1000 {
		return 0, errors.New("MAX_SESSIONS must be an integer from 1 to 1000")
	}
	return maxSessions, nil
}

func LimitHandler(next http.Handler, maxSessions int) http.Handler {
	limit := make(chan struct{}, maxSessions)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case limit <- struct{}{}:
			defer func(){ <-limit }()
			next.ServeHTTP(w, r)
		default:
			http.Error(w, "session capacity reached", http.StatusServiceUnavailable)
		}
	})
}
