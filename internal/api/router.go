package api

import (
	"net/http"

	"wireguard-ui/internal/auth"
)

// Mount registers all /api routes on the given mux.
func (h *Handlers) Mount(mux *http.ServeMux, mgr *auth.Manager) {
	// Public endpoints.
	mux.HandleFunc("GET /api/health", h.Health)
	mux.HandleFunc("POST /api/login", h.Login)
	mux.HandleFunc("POST /api/logout", h.Logout)
	mux.HandleFunc("GET /api/me", h.Me)

	// Authenticated endpoints.
	mux.HandleFunc("GET /api/overview", mgr.Require(h.Overview))
	mux.HandleFunc("GET /api/system", mgr.Require(h.System))
	mux.HandleFunc("GET /api/version", mgr.Require(h.Version))
	mux.HandleFunc("POST /api/update", mgr.Require(h.Update))
	mux.HandleFunc("POST /api/system/ip-forward", mgr.Require(h.EnableIPForward))
	mux.HandleFunc("POST /api/interface", mgr.Require(h.InterfaceControl))
	mux.HandleFunc("POST /api/interface/autostart", mgr.Require(h.SetAutostart))
	mux.HandleFunc("GET /api/clients", mgr.Require(h.Clients))
	mux.HandleFunc("POST /api/clients", mgr.Require(h.CreateClient))
	mux.HandleFunc("POST /api/clients/delete", mgr.Require(h.DeleteClient))
	mux.HandleFunc("POST /api/clients/rename", mgr.Require(h.RenameClient))
	mux.HandleFunc("POST /api/clients/subnets", mgr.Require(h.SetClientSubnets))
	mux.HandleFunc("POST /api/clients/config", mgr.Require(h.ClientConfig))
	mux.HandleFunc("GET /api/network", mgr.Require(h.Network))
	mux.HandleFunc("GET /api/public-ip", mgr.Require(h.DetectPublicIP))
	mux.HandleFunc("GET /api/ip-info", mgr.Require(h.IPInfo))
	mux.HandleFunc("GET /api/settings", mgr.Require(h.Settings))
	mux.HandleFunc("POST /api/settings", mgr.Require(h.UpdateSettings))
	mux.HandleFunc("POST /api/account/password", mgr.Require(h.ChangeCredentials))
}
