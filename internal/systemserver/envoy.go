package systemserver

import "log"

// injectEnvoySidecar is intentionally disabled.
//
// The Envoy sidecar approach requires TWO pieces to work:
//   1. An init container (envoy-init) that sets up iptables rules to redirect
//      inbound traffic to port 15001 (the Envoy listener port).
//   2. An actual Envoy proxy sidecar container listening on port 15001 that
//      performs auth checks and forwards traffic to the app container.
//
// Without the Envoy sidecar container (#2), the init container's iptables
// rules would redirect traffic to a port with nothing listening, breaking the
// app entirely.
//
// For a single-user system (Olares One), proxy-level auth via the main reverse
// proxy (handleAppProxy + Authelia auth_request) is sufficient. Envoy sidecars
// would be needed for multi-user/multi-tenant isolation where apps need
// per-pod auth enforcement independent of the ingress proxy.
//
// To re-enable in the future:
//   - Build and push ghcr.io/packalares/envoy-init:latest (iptables init container)
//   - Build and push an Envoy sidecar image with auth filter config
//   - Inject BOTH the init container AND the Envoy sidecar container
//   - The Envoy config should use ext_authz to call Authelia for auth checks
func (s *Server) injectEnvoySidecar(app *Application) error {
	log.Printf("envoy sidecar injection skipped for %s (disabled — proxy-level auth is sufficient)", app.Spec.Name)
	return nil
}
