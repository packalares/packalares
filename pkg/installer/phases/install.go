package phases

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/packalares/packalares/pkg/installer/binaries"
	"github.com/packalares/packalares/pkg/installer/cni"
	"github.com/packalares/packalares/pkg/installer/etcd"
	"github.com/packalares/packalares/pkg/installer/helm"
	"github.com/packalares/packalares/pkg/installer/k3s"
	"github.com/packalares/packalares/pkg/installer/kernel"
	"github.com/packalares/packalares/pkg/installer/precheck"
	"github.com/packalares/packalares/pkg/installer/redis"
	"github.com/packalares/packalares/pkg/installer/storage"
)

type phase struct {
	Name string
	Fn   func(w io.Writer) error
}

// eventWriter wraps an event channel so writes are forwarded as EventPhaseLog events.
type eventWriter struct {
	ch       chan<- PhaseEvent
	phaseIdx int
	total    int
	phase    string
	buf      []byte // accumulate partial lines
}

func (ew *eventWriter) Write(p []byte) (int, error) {
	ew.buf = append(ew.buf, p...)
	for {
		idx := -1
		for i, b := range ew.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(ew.buf[:idx])
		ew.buf = ew.buf[idx+1:]
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		ew.ch <- PhaseEvent{
			Type:     EventPhaseLog,
			Phase:    ew.phase,
			PhaseIdx: ew.phaseIdx,
			Total:    ew.total,
			Message:  line,
		}
	}
	return len(p), nil
}

// flush sends any remaining partial line in the buffer.
func (ew *eventWriter) flush() {
	if len(ew.buf) > 0 {
		line := strings.TrimRight(string(ew.buf), "\r\n")
		if line != "" {
			ew.ch <- PhaseEvent{
				Type:     EventPhaseLog,
				Phase:    ew.phase,
				PhaseIdx: ew.phaseIdx,
				Total:    ew.total,
				Message:  line,
			}
		}
		ew.buf = nil
	}
}

// RunInstallWithEvents runs all install phases, sending events to the channel.
// The caller is responsible for consuming events (e.g. TUI or plain printer).
// The channel is closed when the function returns.
func RunInstallWithEvents(opts *InstallOptions, events chan<- PhaseEvent) error {
	defer close(events)

	// Check for a saved state file from a previous (interrupted) install.
	savedState, err := loadInstallState()
	if err != nil {
		return fmt.Errorf("load install state: %w", err)
	}

	resuming := savedState != nil
	var completedSet map[string]bool

	if resuming {
		*opts = savedState.Options
		completedSet = make(map[string]bool, len(savedState.CompletedPhases))
		for _, name := range savedState.CompletedPhases {
			completedSet[name] = true
		}
	}

	opts.applyDefaults()
	if err := opts.validate(); err != nil {
		return err
	}

	// Ensure base directories exist
	for _, dir := range []string{
		opts.BaseDir,
		filepath.Join(opts.BaseDir, "installer"),
		filepath.Join(opts.BaseDir, "installer", "wizard"),
		"/etc/packalares",
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}

	// Resolve username/password
	if opts.Username == "" {
		opts.Username = "admin"
	}
	if opts.Password == "" {
		plain, _, err := generatePassword(12)
		if err != nil {
			return fmt.Errorf("generate password: %w", err)
		}
		opts.Password = plain
		events <- PhaseEvent{Type: EventPhaseLog, Message: fmt.Sprintf("Generated admin password: %s", opts.Password)}
	}

	// Write config.yaml so all config.*() functions return correct values
	if err := writeConfigYAML(opts); err != nil {
		return fmt.Errorf("write config.yaml: %w", err)
	}

	arch := getArch()

	phases := buildPhases(opts, arch)

	// Build the running state that tracks progress through this run.
	state := &InstallState{
		Options: *opts,
	}
	if resuming {
		state.CompletedPhases = savedState.CompletedPhases
	}

	total := len(phases)
	for i, p := range phases {
		// Skip phases already completed in a prior run.
		if completedSet[p.Name] {
			events <- PhaseEvent{
				Type:     EventPhaseSkipped,
				Phase:    p.Name,
				PhaseIdx: i,
				Total:    total,
			}
			continue
		}

		events <- PhaseEvent{
			Type:     EventPhaseStart,
			Phase:    p.Name,
			PhaseIdx: i,
			Total:    total,
		}

		ew := &eventWriter{
			ch:       events,
			phaseIdx: i,
			total:    total,
			phase:    p.Name,
		}

		start := time.Now()
		if err := p.Fn(ew); err != nil {
			ew.flush()
			dur := time.Since(start)

			if errors.Is(err, ErrRebootRequired) {
				state.RebootReason = err.Error()
				if saveErr := saveInstallState(state); saveErr != nil {
					// best-effort
				}
				events <- PhaseEvent{
					Type:     EventRebootRequired,
					Phase:    p.Name,
					PhaseIdx: i,
					Total:    total,
					Duration: dur,
					Err:      err,
				}
				return ErrRebootRequired
			}

			// Save state so user can resume after fixing
			if saveErr := saveInstallState(state); saveErr != nil {
				// best-effort
			}
			events <- PhaseEvent{
				Type:     EventPhaseFailed,
				Phase:    p.Name,
				PhaseIdx: i,
				Total:    total,
				Duration: dur,
				Err:      err,
			}
			return fmt.Errorf("phase %q failed: %w", p.Name, err)
		}
		ew.flush()

		dur := time.Since(start)
		events <- PhaseEvent{
			Type:     EventPhaseComplete,
			Phase:    p.Name,
			PhaseIdx: i,
			Total:    total,
			Duration: dur,
		}

		// Record completion.
		state.CompletedPhases = append(state.CompletedPhases, p.Name)
		if saveErr := saveInstallState(state); saveErr != nil {
			// best-effort
		}
	}

	// All phases done — clean up state file.
	removeInstallState()

	events <- PhaseEvent{
		Type:  EventInstallComplete,
		Total: total,
	}
	return nil
}

// RunInstall is the legacy entry point that prints to stdout.
// It creates an event channel, launches a goroutine to print events,
// and blocks until installation completes.
func RunInstall(opts *InstallOptions) error {
	events := make(chan PhaseEvent, 64)

	var installErr error
	done := make(chan struct{})
	go func() {
		installErr = RunInstallWithEvents(opts, events)
		done <- struct{}{}
	}()

	// Consume events with plain-text output
	for ev := range events {
		switch ev.Type {
		case EventPhaseStart:
			fmt.Printf("\n[%d/%d] %s ...\n", ev.PhaseIdx+1, ev.Total, ev.Phase)
		case EventPhaseLog:
			fmt.Println(ev.Message)
		case EventPhaseComplete:
			fmt.Printf("[%d/%d] %s completed in %s\n", ev.PhaseIdx+1, ev.Total, ev.Phase, ev.Duration.Round(time.Second))
		case EventPhaseFailed:
			fmt.Printf("[%d/%d] %s FAILED after %s: %v\n", ev.PhaseIdx+1, ev.Total, ev.Phase, ev.Duration.Round(time.Second), ev.Err)
		case EventPhaseSkipped:
			fmt.Printf("\n[%d/%d] %s ... already completed\n", ev.PhaseIdx+1, ev.Total, ev.Phase)
		case EventRebootRequired:
			fmt.Println()
			fmt.Println("========================================")
			fmt.Println("  Reboot required to continue install.")
			fmt.Println("  After reboot, run: packalares install")
			fmt.Println("========================================")
		case EventInstallComplete:
			// handled by caller
		}
	}

	<-done
	return installErr
}

func buildPhases(opts *InstallOptions, arch string) []phase {
	return []phase{
		{"Precheck", func(w io.Writer) error {
			if opts.SkipPrecheck {
				fmt.Fprintln(w, "Skipping precheck (--skip-precheck)")
				return nil
			}
			result := precheck.RunPrecheck()
			precheck.PrintReport(result, w)
			if !result.Passed {
				return fmt.Errorf("precheck failed")
			}
			return nil
		}},
		{"Download binaries", func(w io.Writer) error {
			return binaries.DownloadAll(opts.BaseDir, arch, w)
		}},
		{"Configure kernel", func(w io.Writer) error {
			if err := kernel.LoadModules(w); err != nil {
				return err
			}
			return kernel.ApplySysctl(w)
		}},
		{"Install etcd", func(w io.Writer) error {
			return etcd.Install(opts.BaseDir, w)
		}},
		{"Install K3s", func(w io.Writer) error {
			return k3s.Install(opts.BaseDir, opts.Registry, w)
		}},
		{"Deploy Calico CNI", func(w io.Writer) error {
			return cni.DeployCalico(opts.Registry, w)
		}},
		{"Deploy OpenEBS", func(w io.Writer) error {
			return storage.DeployOpenEBS(opts.Registry, w)
		}},
		{"Setup Kubernetes management", func(w io.Writer) error {
			return deployCRDsAndNamespaces(opts, w)
		}},
		{"Generate secrets", func(w io.Writer) error {
			return GenerateSecrets(opts, w)
		}},
		{"Deploy KVRocks", func(w io.Writer) error {
			return redis.Install(opts.BaseDir, w)
		}},
		{"Install Helm", func(w io.Writer) error {
			return helm.Install(opts.BaseDir, arch, w)
		}},
		{"Deploy platform services", func(w io.Writer) error {
			return deployPlatformCharts(opts, w)
		}},
		{"Deploy framework services", func(w io.Writer) error {
			return deployFrameworkCharts(opts, w)
		}},
		{"Generate TLS certificate", func(w io.Writer) error {
			return generateTLSCert(opts, w)
		}},
		{"Seed Infisical", func(w io.Writer) error {
			return SeedInfisical(opts, w)
		}},
		{"Create LLDAP service account", func(w io.Writer) error {
			return createLLDAPServiceAccount(opts, w)
		}},
		{"Deploy user apps", func(w io.Writer) error {
			return deployAppCharts(opts, w)
		}},
		{"Deploy monitoring", func(w io.Writer) error {
			return deployMonitoring(opts, w)
		}},
		{"Setup GPU", func(w io.Writer) error {
			return InstallGPU(opts, w)
		}},
		{"Wait for pods", func(w io.Writer) error {
			return waitForAllPods(w)
		}},
		{"Write release info", func(w io.Writer) error {
			return writeReleaseFile(opts)
		}},
	}
}

// PhaseNames returns the names of all install phases in order.
// This is used by the TUI to pre-populate the phase list.
func PhaseNames() []string {
	// Use a dummy opts to build phases — we only need names.
	phases := buildPhases(&InstallOptions{}, "amd64")
	names := make([]string, len(phases))
	for i, p := range phases {
		names[i] = p.Name
	}
	return names
}

func writeReleaseFile(opts *InstallOptions) error {
	content := fmt.Sprintf(
		"PACKALARES_VERSION=1.0.0\nPACKALARES_BASE_DIR=%s\nPACKALARES_NAME=%s@%s\n",
		opts.BaseDir, opts.Username, opts.Domain,
	)
	return os.WriteFile(ReleaseFile, []byte(content), 0644)
}
