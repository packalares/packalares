package kubesphere

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// Server is a minimal API server that replaces KubeSphere's IAM API.
// It provides user CRUD endpoints compatible with what BFL and app-service expect.
type Server struct {
	dynClient dynamic.Interface
	addr      string
}

var userGVR = schema.GroupVersionResource{
	Group:    "iam.kubesphere.io",
	Version:  "v1alpha2",
	Resource: "users",
}

// NewServer creates a new KubeSphere replacement API server.
func NewServer(config *rest.Config, addr string) (*Server, error) {
	dynClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	return &Server{
		dynClient: dynClient,
		addr:      addr,
	}, nil
}

// Start begins serving HTTP requests. Blocks until context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// User CRUD endpoints matching KubeSphere API paths
	mux.HandleFunc("/kapis/iam.kubesphere.io/v1alpha2/users", s.handleUsers)
	mux.HandleFunc("/kapis/iam.kubesphere.io/v1alpha2/users/", s.handleUser)

	// Health check
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Version endpoint
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{
			"version":   "packalares-iam/1.0.0",
			"component": "kubesphere-replacement",
		})
	})

	srv := &http.Server{
		Addr:    s.addr,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Printf("[iam-server] Listening on %s", s.addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// handleUsers handles GET (list) and POST (create) for /users
func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	switch r.Method {
	case http.MethodGet:
		s.listUsers(ctx, w, r)
	case http.MethodPost:
		s.createUser(ctx, w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleUser handles GET, PUT, PATCH, DELETE for /users/{name}
func (s *Server) handleUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// Extract user name from path
	path := strings.TrimPrefix(r.URL.Path, "/kapis/iam.kubesphere.io/v1alpha2/users/")
	name := strings.Split(path, "/")[0]

	if name == "" {
		http.Error(w, "user name required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.getUser(ctx, w, name)
	case http.MethodPut:
		s.updateUser(ctx, w, r, name)
	case http.MethodPatch:
		s.patchUser(ctx, w, r, name)
	case http.MethodDelete:
		s.deleteUser(ctx, w, name)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) listUsers(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	list, err := s.dynClient.Resource(userGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, list)
}

func (s *Server) getUser(ctx context.Context, w http.ResponseWriter, name string) {
	user, err := s.dynClient.Resource(userGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (s *Server) createUser(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var obj unstructured.Unstructured
	if err := json.NewDecoder(r.Body).Decode(&obj.Object); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "iam.kubesphere.io",
		Version: "v1alpha2",
		Kind:    "User",
	})

	created, err := s.dynClient.Resource(userGVR).Create(ctx, &obj, metav1.CreateOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateUser(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	var obj unstructured.Unstructured
	if err := json.NewDecoder(r.Body).Decode(&obj.Object); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	obj.SetName(name)
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "iam.kubesphere.io",
		Version: "v1alpha2",
		Kind:    "User",
	})

	updated, err := s.dynClient.Resource(userGVR).Update(ctx, &obj, metav1.UpdateOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) patchUser(ctx context.Context, w http.ResponseWriter, r *http.Request, name string) {
	// Read the patch body
	var patchData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&patchData); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Get existing user
	existing, err := s.dynClient.Resource(userGVR).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	// Merge annotations
	if metadata, ok := patchData["metadata"].(map[string]interface{}); ok {
		if annotations, ok := metadata["annotations"].(map[string]interface{}); ok {
			existingAnnotations := existing.GetAnnotations()
			if existingAnnotations == nil {
				existingAnnotations = make(map[string]string)
			}
			for k, v := range annotations {
				if str, ok := v.(string); ok {
					existingAnnotations[k] = str
				}
			}
			existing.SetAnnotations(existingAnnotations)
		}
		if labels, ok := metadata["labels"].(map[string]interface{}); ok {
			existingLabels := existing.GetLabels()
			if existingLabels == nil {
				existingLabels = make(map[string]string)
			}
			for k, v := range labels {
				if str, ok := v.(string); ok {
					existingLabels[k] = str
				}
			}
			existing.SetLabels(existingLabels)
		}
	}

	// Merge spec if present
	if spec, ok := patchData["spec"].(map[string]interface{}); ok {
		existingSpec, _ := existing.Object["spec"].(map[string]interface{})
		if existingSpec == nil {
			existingSpec = make(map[string]interface{})
		}
		for k, v := range spec {
			existingSpec[k] = v
		}
		existing.Object["spec"] = existingSpec
	}

	// Merge status if present
	if status, ok := patchData["status"].(map[string]interface{}); ok {
		existingStatus, _ := existing.Object["status"].(map[string]interface{})
		if existingStatus == nil {
			existingStatus = make(map[string]interface{})
		}
		for k, v := range status {
			existingStatus[k] = v
		}
		existing.Object["status"] = existingStatus
	}

	updated, err := s.dynClient.Resource(userGVR).Update(ctx, existing, metav1.UpdateOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteUser(ctx context.Context, w http.ResponseWriter, name string) {
	err := s.dynClient.Resource(userGVR).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted", "name": name})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    status,
		"message": err.Error(),
	})
}
