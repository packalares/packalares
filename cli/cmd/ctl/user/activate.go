package user

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/beclab/Olares/cli/pkg/wizard"
	"github.com/spf13/cobra"
)

type activateUserOptions struct {
	Mnemonic      string
	BflUrl        string
	VaultUrl      string
	Password      string
	OlaresId      string
	ResetPassword string

	Location     string
	Language     string
	EnableTunnel bool
	Host         string
	Jws          string
}

func NewCmdActivateUser() *cobra.Command {
	o := &activateUserOptions{}
	cmd := &cobra.Command{
		Use:   "activate {Olares ID (e.g., user@example.com)}",
		Short: "activate a new user",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			o.OlaresId = args[0]
			if err := o.Validate(); err != nil {
				log.Fatal(err)
			}
			if err := o.Run(); err != nil {
				log.Fatal(err)
			}
		},
	}
	o.AddFlags(cmd)
	return cmd
}

func (o *activateUserOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(&o.Mnemonic, "mnemonic", "", "12-word mnemonic phrase, required for activation")
	cmd.Flags().StringVar(&o.BflUrl, "bfl", "http://127.0.0.1:30180", "Bfl URL (e.g., https://example.com, default: http://127.0.0.1:30180)")
	cmd.Flags().StringVar(&o.VaultUrl, "vault", "http://127.0.0.1:30180", "Vault URL (e.g., https://example.com, default: http://127.0.0.1:30181)")
	cmd.Flags().StringVarP(&o.Password, "password", "p", "", "OS password for authentication, required for activation")
	cmd.Flags().StringVar(&o.Location, "location", "Asia/Shanghai", "Timezone location (default: Asia/Shanghai)")
	cmd.Flags().StringVar(&o.Language, "language", "en-US", "System language (default: en-US)")
	cmd.Flags().BoolVar(&o.EnableTunnel, "enable-tunnel", false, "Enable tunnel mode (default: false)")
	cmd.Flags().StringVar(&o.Host, "host", "", "FRP host (only used when tunnel is enabled)")
	cmd.Flags().StringVar(&o.Jws, "jws", "", "FRP JWS token (only used when tunnel is enabled)")
	cmd.Flags().StringVar(&o.ResetPassword, "reset-password", "", "New password for resetting (required for password reset)")
}

func (o *activateUserOptions) Validate() error {
	if o.OlaresId == "" {
		return fmt.Errorf("Olares ID is required")
	}
	if o.Password == "" {
		return fmt.Errorf("Password is required")
	}
	if o.ResetPassword == "" {
		return fmt.Errorf("Reset password is required")
	}
	// Mnemonic is only required when not in local cert mode
	if o.Mnemonic == "" && !isLocalMode() {
		return fmt.Errorf("Mnemonic is required (set OLARES_CERT_MODE=local to skip)")
	}
	return nil
}

func isLocalMode() bool {
	mode := strings.ToLower(strings.TrimSpace(
		func() string {
			if v, ok := lookupEnv("OLARES_CERT_MODE"); ok {
				return v
			}
			return ""
		}(),
	))
	if mode == "local" {
		return true
	}
	remote := strings.TrimSpace(func() string {
		if v, ok := lookupEnv("OLARES_SYSTEM_REMOTE_SERVICE"); ok {
			return v
		}
		return ""
	}())
	return remote == "" || strings.EqualFold(remote, "none") || strings.EqualFold(remote, "disabled")
}

var lookupEnv = os.LookupEnv

func (c *activateUserOptions) Run() error {
	localName := c.OlaresId
	if strings.Contains(c.OlaresId, "@") {
		localName = strings.Split(c.OlaresId, "@")[0]
	}

	log.Printf("Parameters:")
	log.Printf("  BflUrl: %s", c.BflUrl)
	log.Printf("  Terminus Name: %s", c.OlaresId)
	log.Printf("  Local Name: %s", localName)

	var accessToken string
	var err error

	if isLocalMode() {
		log.Println("=== Local Mode Activation (no LarePass/cloud required) ===")

		// In local mode: just do first-factor auth (password), skip DID/vault
		token, err := wizard.OnFirstFactor(c.BflUrl, c.OlaresId, localName, c.Password, false, false)
		if err != nil {
			return fmt.Errorf("authentication failed: %v", err)
		}
		accessToken = token.AccessToken

		// Bind user zone directly (no JWS/DID)
		err = wizard.BindUserZoneLocal(c.BflUrl, accessToken)
		if err != nil {
			return fmt.Errorf("bind user zone failed: %v", err)
		}
		log.Printf("User zone bound successfully")
	} else {
		log.Println("=== TermiPass CLI - User Bind Terminus ===")

		log.Printf("  VaultUrl: %s", c.VaultUrl)

		log.Printf("Initializing global stores with mnemonic...")
		err = wizard.InitializeGlobalStores(c.Mnemonic, c.OlaresId)
		if err != nil {
			return fmt.Errorf("failed to initialize global stores: %v", err)
		}

		accessToken, err = wizard.UserBindTerminus(c.Mnemonic, c.BflUrl, c.VaultUrl, c.Password, c.OlaresId, localName)
		if err != nil {
			return fmt.Errorf("user bind failed: %v", err)
		}
		log.Printf("Vault activation completed successfully!")
	}

	log.Printf("Starting system activation wizard...")

	wizardConfig := wizard.CustomWizardConfig(c.Location, c.Language, c.EnableTunnel, c.Host, c.Jws, c.Password, c.ResetPassword)

	log.Printf("Wizard configuration:")
	log.Printf("  Location: %s", wizardConfig.System.Location)
	log.Printf("  Language: %s", wizardConfig.System.Language)

	err = wizard.RunActivationWizard(c.BflUrl, accessToken, wizardConfig)
	if err != nil {
		return fmt.Errorf("activation wizard failed: %v", err)
	}

	log.Printf("Activation finished successfully!")
	return nil
}
