package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Controller watches MiddlewareRequest CRDs and provisions middleware.
type Controller struct {
	cfg           *Config
	dynamicClient dynamic.Interface
	kubeClient    kubernetes.Interface
	pg            *PGProvisioner
	redis         *RedisProvisioner
	nats          *NATSProvisioner
	secrets       *SecretManager
	ctx           context.Context
	cancel        context.CancelFunc
}

func NewController(cfg *Config) (*Controller, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("get in-cluster config: %w", err)
	}

	dynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}

	kubeClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Controller{
		cfg:           cfg,
		dynamicClient: dynamicClient,
		kubeClient:    kubeClient,
		pg:            NewPGProvisioner(cfg.PGHost, cfg.PGPort, cfg.PGAdminUser, cfg.PGAdminPassword),
		redis:         NewRedisProvisioner(cfg.RedisHost, cfg.RedisPort, cfg.RedisPassword),
		nats:          NewNATSProvisioner(cfg.NATSHost, cfg.NATSPort),
		secrets:       NewSecretManager(kubeClient),
		ctx:           ctx,
		cancel:        cancel,
	}, nil
}

// Run starts the controller's watch loop.
func (c *Controller) Run() error {
	log.Println("middleware controller starting, watching MiddlewareRequest resources")

	for {
		if err := c.watchLoop(); err != nil {
			log.Printf("watch error: %v, retrying in 5s", err)
			select {
			case <-c.ctx.Done():
				return c.ctx.Err()
			case <-time.After(5 * time.Second):
			}
		}
	}
}

func (c *Controller) watchLoop() error {
	resource := c.dynamicClient.Resource(MiddlewareRequestGVR)

	// First, list all existing resources and reconcile them.
	// Capture the resourceVersion so the subsequent watch picks up only
	// events that happen after the list.
	resourceVersion, err := c.reconcileExisting()
	if err != nil {
		log.Printf("reconcile existing resources: %v", err)
		// Fall back to watching from the beginning
		resourceVersion = ""
	}

	watchOpts := metav1.ListOptions{
		ResourceVersion: resourceVersion,
	}

	var watcher watch.Interface

	if c.cfg.WatchNamespace != "" {
		watcher, err = resource.Namespace(c.cfg.WatchNamespace).Watch(c.ctx, watchOpts)
	} else {
		watcher, err = resource.Watch(c.ctx, watchOpts)
	}
	if err != nil {
		return fmt.Errorf("watch middleware requests: %w", err)
	}
	defer watcher.Stop()

	log.Printf("watching MiddlewareRequests from resourceVersion=%s", resourceVersion)

	for {
		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("watch channel closed")
			}

			if event.Type == watch.Error {
				log.Printf("watch error event: %v", event.Object)
				return fmt.Errorf("watch error event received")
			}

			if event.Object == nil {
				continue
			}

			unObj, ok := event.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}

			req, err := c.parseMiddlewareRequest(unObj)
			if err != nil {
				log.Printf("parse middleware request: %v", err)
				continue
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				if err := c.handleCreateOrUpdate(req); err != nil {
					log.Printf("handle create/update %s/%s: %v", req.Namespace, req.Name, err)
				}
			case watch.Deleted:
				if err := c.handleDelete(req); err != nil {
					log.Printf("handle delete %s/%s: %v", req.Namespace, req.Name, err)
				}
			}
		}
	}
}

func (c *Controller) reconcileExisting() (string, error) {
	resource := c.dynamicClient.Resource(MiddlewareRequestGVR)

	var list *unstructured.UnstructuredList
	var err error

	if c.cfg.WatchNamespace != "" {
		list, err = resource.Namespace(c.cfg.WatchNamespace).List(c.ctx, metav1.ListOptions{})
	} else {
		list, err = resource.List(c.ctx, metav1.ListOptions{})
	}
	if err != nil {
		return "", fmt.Errorf("list middleware requests: %w", err)
	}

	log.Printf("reconciling %d existing MiddlewareRequest resources", len(list.Items))

	for i := range list.Items {
		req, err := c.parseMiddlewareRequest(&list.Items[i])
		if err != nil {
			log.Printf("parse existing middleware request: %v", err)
			continue
		}
		if err := c.handleCreateOrUpdate(req); err != nil {
			log.Printf("reconcile existing %s/%s: %v", req.Namespace, req.Name, err)
		}
	}

	return list.GetResourceVersion(), nil
}

func (c *Controller) parseMiddlewareRequest(obj *unstructured.Unstructured) (*MiddlewareRequest, error) {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, err
	}

	var req MiddlewareRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

func (c *Controller) handleCreateOrUpdate(req *MiddlewareRequest) error {
	// Skip resources that are already provisioned
	if req.Status.State == "ready" {
		log.Printf("skipping %s/%s: already in ready state", req.Namespace, req.Name)
		return nil
	}

	log.Printf("handling create/update for %s/%s (middleware=%s, state=%s)",
		req.Namespace, req.Name, req.Spec.Middleware, req.Status.State)

	var provisionErr error
	switch req.Spec.Middleware {
	case TypePostgreSQL:
		provisionErr = c.handlePostgreSQL(req)
	case TypeRedis:
		provisionErr = c.handleRedis(req)
	case TypeNats:
		provisionErr = c.handleNATS(req)
	default:
		log.Printf("unsupported middleware type %q for %s/%s, skipping", req.Spec.Middleware, req.Namespace, req.Name)
		return nil
	}

	if provisionErr != nil {
		if statusErr := c.updateStatus(req, "failed", provisionErr.Error()); statusErr != nil {
			log.Printf("failed to update status for %s/%s: %v", req.Namespace, req.Name, statusErr)
		}
		return provisionErr
	}

	if err := c.updateStatus(req, "ready", ""); err != nil {
		log.Printf("provisioned %s/%s but failed to update status: %v", req.Namespace, req.Name, err)
		return err
	}

	log.Printf("successfully provisioned %s/%s (middleware=%s)", req.Namespace, req.Name, req.Spec.Middleware)
	return nil
}

func (c *Controller) handleDelete(req *MiddlewareRequest) error {
	log.Printf("handling delete for %s/%s (middleware=%s)", req.Namespace, req.Name, req.Spec.Middleware)

	switch req.Spec.Middleware {
	case TypePostgreSQL:
		return c.deletePostgreSQL(req)
	case TypeRedis:
		return c.deleteRedis(req)
	case TypeNats:
		return c.deleteNATS(req)
	default:
		return nil
	}
}

// updateStatus updates the MiddlewareRequest status subresource.
func (c *Controller) updateStatus(req *MiddlewareRequest, state, message string) error {
	resource := c.dynamicClient.Resource(MiddlewareRequestGVR).Namespace(req.Namespace)

	// Get the latest version of the object to avoid conflicts
	obj, err := resource.Get(c.ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get resource for status update: %w", err)
	}

	now := metav1.Now()
	status := map[string]interface{}{
		"state":      state,
		"updateTime": now.Format(time.RFC3339),
	}
	if message != "" {
		status["message"] = message
	}

	if err := unstructured.SetNestedField(obj.Object, status, "status"); err != nil {
		return fmt.Errorf("set status field: %w", err)
	}

	// Try status subresource first; fall back to regular update if the CRD
	// doesn't have a status subresource defined.
	_, err = resource.UpdateStatus(c.ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		// Fall back to a regular update (some CRD definitions may not have the
		// /status subresource enabled)
		_, err = resource.Update(c.ctx, obj, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update status: %w", err)
		}
	}

	log.Printf("updated status for %s/%s to %q", req.Namespace, req.Name, state)
	return nil
}

// PostgreSQL handling

func (c *Controller) handlePostgreSQL(req *MiddlewareRequest) error {
	if req.Spec.PostgreSQL == nil {
		return fmt.Errorf("postgreSQL spec is nil")
	}

	spec := req.Spec.PostgreSQL

	// Resolve password
	password, err := c.secrets.ResolvePassword(c.ctx, spec.Password, req.Namespace)
	if err != nil {
		return fmt.Errorf("resolve pg password: %w", err)
	}

	// Create/update user
	if err := c.pg.CreateOrUpdateUser(c.ctx, spec.User, password); err != nil {
		return fmt.Errorf("create pg user: %w", err)
	}

	// Create databases
	for _, db := range spec.Databases {
		if err := c.pg.CreateDatabase(c.ctx, req.Spec.AppNamespace, db.Name, spec.User); err != nil {
			return fmt.Errorf("create pg database %q: %w", db.Name, err)
		}

		if len(db.Extensions) > 0 {
			if err := c.pg.CreateExtensions(c.ctx, req.Spec.AppNamespace, db.Name, db.Extensions); err != nil {
				return fmt.Errorf("create extensions for %q: %w", db.Name, err)
			}
		}
	}

	// Store credentials in a Secret for the app to consume
	credSecretName := fmt.Sprintf("%s-pg-credentials", req.Spec.App)
	credData := map[string][]byte{
		"host":     []byte(c.cfg.PGHost),
		"port":     []byte(fmt.Sprintf("%d", c.cfg.PGPort)),
		"username": []byte(spec.User),
		"password": []byte(password),
	}
	for _, db := range spec.Databases {
		key := fmt.Sprintf("database_%s", db.Name)
		credData[key] = []byte(GetDatabaseName(req.Spec.AppNamespace, db.Name))
	}
	if len(spec.Databases) > 0 {
		credData["database"] = []byte(GetDatabaseName(req.Spec.AppNamespace, spec.Databases[0].Name))
	}

	return c.secrets.StoreCredentials(c.ctx, req.Namespace, credSecretName, credData)
}

func (c *Controller) deletePostgreSQL(req *MiddlewareRequest) error {
	if req.Spec.PostgreSQL == nil {
		return nil
	}

	spec := req.Spec.PostgreSQL

	// Drop databases
	for _, db := range spec.Databases {
		if err := c.pg.DropDatabase(c.ctx, req.Spec.AppNamespace, db.Name); err != nil {
			log.Printf("drop database %q: %v (continuing)", db.Name, err)
		}
	}

	// Drop any remaining databases owned by the user
	dbs, err := c.pg.ListDatabasesByOwner(c.ctx, spec.User)
	if err == nil {
		for _, db := range dbs {
			_ = c.pg.DropDatabase(c.ctx, "", db) // raw name, no prefix
		}
	}

	// Drop user
	if err := c.pg.DropUser(c.ctx, spec.User); err != nil {
		log.Printf("drop user %q: %v", spec.User, err)
	}

	// Delete credentials secret
	credSecretName := fmt.Sprintf("%s-pg-credentials", req.Spec.App)
	_ = c.secrets.DeleteCredentials(c.ctx, req.Namespace, credSecretName)

	return nil
}

// Redis handling

func (c *Controller) handleRedis(req *MiddlewareRequest) error {
	if req.Spec.Redis == nil {
		return fmt.Errorf("redis spec is nil")
	}

	spec := req.Spec.Redis

	password, err := c.secrets.ResolvePassword(c.ctx, spec.Password, req.Namespace)
	if err != nil {
		return fmt.Errorf("resolve redis password: %w", err)
	}

	namespace := fmt.Sprintf("%s_%s", req.Namespace, spec.Namespace)
	if err := c.redis.CreateNamespace(c.ctx, namespace, password); err != nil {
		return fmt.Errorf("create redis namespace: %w", err)
	}

	// Store credentials
	credSecretName := fmt.Sprintf("%s-redis-credentials", req.Spec.App)
	credData := map[string][]byte{
		"host":      []byte(c.cfg.RedisHost),
		"port":      []byte(fmt.Sprintf("%d", c.cfg.RedisPort)),
		"password":  []byte(password),
		"namespace": []byte(namespace),
	}

	return c.secrets.StoreCredentials(c.ctx, req.Namespace, credSecretName, credData)
}

func (c *Controller) deleteRedis(req *MiddlewareRequest) error {
	if req.Spec.Redis == nil {
		return nil
	}

	namespace := fmt.Sprintf("%s_%s", req.Namespace, req.Spec.Redis.Namespace)
	if err := c.redis.DeleteNamespace(c.ctx, namespace); err != nil {
		log.Printf("delete redis namespace %q: %v", namespace, err)
	}

	credSecretName := fmt.Sprintf("%s-redis-credentials", req.Spec.App)
	_ = c.secrets.DeleteCredentials(c.ctx, req.Namespace, credSecretName)

	return nil
}

// NATS handling

func (c *Controller) handleNATS(req *MiddlewareRequest) error {
	if req.Spec.Nats == nil {
		return fmt.Errorf("nats spec is nil")
	}

	spec := req.Spec.Nats

	if err := c.nats.CreateStream(c.ctx, req.Spec.AppNamespace, req.Spec.App, spec.Subjects); err != nil {
		return fmt.Errorf("create nats stream: %w", err)
	}

	// Resolve password for credentials secret
	password, err := c.secrets.ResolvePassword(c.ctx, spec.Password, req.Namespace)
	if err != nil {
		// NATS password is optional
		password = ""
	}

	// Store credentials
	credSecretName := fmt.Sprintf("%s-nats-credentials", req.Spec.App)
	credData := map[string][]byte{
		"host":     []byte(c.cfg.NATSHost),
		"port":     []byte(fmt.Sprintf("%d", c.cfg.NATSPort)),
		"user":     []byte(spec.User),
		"password": []byte(password),
	}

	return c.secrets.StoreCredentials(c.ctx, req.Namespace, credSecretName, credData)
}

func (c *Controller) deleteNATS(req *MiddlewareRequest) error {
	if req.Spec.Nats == nil {
		return nil
	}

	if err := c.nats.DeleteStream(c.ctx, req.Spec.AppNamespace, req.Spec.App); err != nil {
		log.Printf("delete nats stream: %v", err)
	}

	credSecretName := fmt.Sprintf("%s-nats-credentials", req.Spec.App)
	_ = c.secrets.DeleteCredentials(c.ctx, req.Namespace, credSecretName)

	return nil
}

// Stop gracefully stops the controller.
func (c *Controller) Stop() {
	c.cancel()
}

// Ensure MiddlewareRequest implements runtime.Object (for dynamic client)
var _ runtime.Object = &MiddlewareRequest{}
var _ runtime.Object = &MiddlewareRequestList{}
