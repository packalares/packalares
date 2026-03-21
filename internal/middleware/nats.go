package middleware

import (
	"context"
	"fmt"
	"log"

	"github.com/nats-io/nats.go"
)

// NATSProvisioner handles NATS user and stream provisioning.
type NATSProvisioner struct {
	host string
	port int
}

func NewNATSProvisioner(host string, port int) *NATSProvisioner {
	return &NATSProvisioner{
		host: host,
		port: port,
	}
}

// CreateStream creates a JetStream stream for an app's subjects.
func (n *NATSProvisioner) CreateStream(ctx context.Context, appNamespace, appName string, subjects []Subject) error {
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", n.host, n.port))
	if err != nil {
		return fmt.Errorf("connect to nats: %w", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("get jetstream context: %w", err)
	}

	streamName := fmt.Sprintf("%s_%s", appNamespace, appName)

	var subjectNames []string
	for _, s := range subjects {
		subjectNames = append(subjectNames, fmt.Sprintf("%s.%s", appNamespace, s.Name))
	}

	if len(subjectNames) == 0 {
		log.Printf("no subjects to create for stream %q", streamName)
		return nil
	}

	// Check if stream exists
	info, _ := js.StreamInfo(streamName)
	if info != nil {
		// Update existing stream
		_, err = js.UpdateStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: subjectNames,
		})
		if err != nil {
			return fmt.Errorf("update stream %q: %w", streamName, err)
		}
		log.Printf("updated NATS stream %q with subjects %v", streamName, subjectNames)
	} else {
		// Create new stream
		_, err = js.AddStream(&nats.StreamConfig{
			Name:     streamName,
			Subjects: subjectNames,
		})
		if err != nil {
			return fmt.Errorf("create stream %q: %w", streamName, err)
		}
		log.Printf("created NATS stream %q with subjects %v", streamName, subjectNames)
	}

	return nil
}

// DeleteStream removes the JetStream stream for an app.
func (n *NATSProvisioner) DeleteStream(ctx context.Context, appNamespace, appName string) error {
	nc, err := nats.Connect(fmt.Sprintf("nats://%s:%d", n.host, n.port))
	if err != nil {
		return fmt.Errorf("connect to nats: %w", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return fmt.Errorf("get jetstream context: %w", err)
	}

	streamName := fmt.Sprintf("%s_%s", appNamespace, appName)

	if err := js.DeleteStream(streamName); err != nil {
		if err == nats.ErrStreamNotFound {
			log.Printf("NATS stream %q not found, skipping delete", streamName)
			return nil
		}
		return fmt.Errorf("delete stream %q: %w", streamName, err)
	}

	log.Printf("deleted NATS stream %q", streamName)
	return nil
}
