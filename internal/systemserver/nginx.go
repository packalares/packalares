package systemserver

// Nginx config generation has been removed.
//
// The system-server's built-in Go reverse proxy (handleAppProxy in server.go)
// routes requests to app backends based on the Host header. This replaces the
// previous approach of generating per-app nginx server blocks and reloading
// nginx, which required nginx to be installed in the system-server pod.
//
// The main ingress nginx (deployed separately) forwards wildcard traffic to
// the system-server, which handles routing internally.
