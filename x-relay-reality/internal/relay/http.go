package relay

import (
	"net/http"

	bridgeserver "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/bridge/server"
	appconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/config"
	"github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/relay/session"
	xrayconfig "github.com/private-gojo/relayx-reality-v2/x-relay-reality/internal/xray/config"
)

func NewHTTPHandler(cfg *appconfig.Config, maxSessions int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/profile.json", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := xrayconfig.GenerateNetModProfileJSON(cfg.WithCloudRunHost(r.Host))
		if err != nil {
			http.Error(w, "invalid service host", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	})
	chainHandler := bridgeserver.NewWSProxyHandler(cfg.XrayWSAddr, cfg.ChainPath, cfg.RelayToken)
	legacyHandler := bridgeserver.NewServerHandler(cfg.XrayAddr, cfg.RelayToken)
	mux.Handle(cfg.ChainPath, session.LimitHandler(chainHandler, maxSessions))
	mux.Handle("/_ws", session.LimitHandler(legacyHandler, maxSessions))
	if cfg.WSPath != cfg.ChainPath && cfg.WSPath != "/_ws" {
		mux.Handle(cfg.WSPath, session.LimitHandler(legacyHandler, maxSessions))
	}
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}
