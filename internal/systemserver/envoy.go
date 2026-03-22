package systemserver

// injectEnvoySidecar is a placeholder for future Envoy sidecar injection.
// Currently disabled because the init container image needs to be built.
// Auth is enforced at the proxy level via nginx auth_request.
func (s *Server) injectEnvoySidecar(app *Application) error {
	// Will be re-enabled with a proper iptables init container image
	return nil
}
