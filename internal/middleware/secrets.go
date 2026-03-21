package middleware

import (
	"context"
	"fmt"
	"log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// SecretManager handles reading passwords from Kubernetes Secrets
// and storing provisioned credentials back into Secrets.
type SecretManager struct {
	client kubernetes.Interface
}

func NewSecretManager(client kubernetes.Interface) *SecretManager {
	return &SecretManager{client: client}
}

// ResolvePassword resolves a PasswordVar to a plaintext password.
func (m *SecretManager) ResolvePassword(ctx context.Context, pv PasswordVar, namespace string) (string, error) {
	if pv.Value != "" {
		return pv.Value, nil
	}

	if pv.ValueFrom != nil && pv.ValueFrom.SecretKeyRef != nil {
		ref := pv.ValueFrom.SecretKeyRef
		secret, err := m.client.CoreV1().Secrets(namespace).Get(ctx, ref.Name, metav1.GetOptions{})
		if err != nil {
			return "", fmt.Errorf("get secret %s/%s: %w", namespace, ref.Name, err)
		}

		data, ok := secret.Data[ref.Key]
		if !ok {
			return "", fmt.Errorf("key %q not found in secret %s/%s", ref.Key, namespace, ref.Name)
		}

		return string(data), nil
	}

	return "", fmt.Errorf("no password value or reference provided")
}

// StoreCredentials creates or updates a Secret with middleware credentials.
func (m *SecretManager) StoreCredentials(ctx context.Context, namespace, name string, data map[string][]byte) error {
	existing, err := m.client.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		// Update existing secret
		existing.Data = data
		_, err = m.client.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("update secret %s/%s: %w", namespace, name, err)
		}
		log.Printf("updated credentials secret %s/%s", namespace, name)
		return nil
	}

	// Create new secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "packalares-middleware",
			},
		},
		Data: data,
	}

	_, err = m.client.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create secret %s/%s: %w", namespace, name, err)
	}

	log.Printf("created credentials secret %s/%s", namespace, name)
	return nil
}

// DeleteCredentials removes a credentials Secret.
func (m *SecretManager) DeleteCredentials(ctx context.Context, namespace, name string) error {
	err := m.client.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("delete secret %s/%s: %w", namespace, name, err)
	}
	log.Printf("deleted credentials secret %s/%s", namespace, name)
	return nil
}
