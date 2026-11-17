// Command devcerts writes a development CA and one mTLS certificate per
// workload into a directory (default infrastructure/certs). Run `make certs`.
//
// These files are for local use only and are git-ignored.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ViniciusMicarelli/digital-bank-platform/packages/go-platform/mtls"
)

func main() {
	out := flag.String("out", "infrastructure/certs", "output directory")
	names := flag.String("names", "api-gateway,identity-service,account-service,ledger-service,transaction-service,pix-service,statement-service,card-service,credit-service,kms-service,dev-client",
		"comma-separated workload identities")
	force := flag.Bool("force", false, "create a new CA and reissue every certificate")
	reissue := flag.Bool("reissue", false, "keep the CA, reissue every workload certificate (rotation; services reload them)")
	validity := flag.Duration("validity", 90*24*time.Hour, "workload certificate lifetime")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	caPath, caKeyPath := filepath.Join(*out, "ca.pem"), filepath.Join(*out, "ca-key.pem")
	ca, reused := loadCA(caPath, caKeyPath)
	if *force || !reused {
		var err error
		if ca, err = mtls.NewAuthority(365 * 24 * time.Hour); err != nil {
			fail(err)
		}
		key, err := ca.KeyPEM()
		if err != nil {
			fail(err)
		}
		write(caPath, ca.PEM)
		write(caKeyPath, key)
		reused = false
	}
	issued := 0
	for _, n := range strings.Split(*names, ",") {
		certPath := filepath.Join(*out, n+".pem")
		// With an existing CA, keep existing certificates: running services
		// keep working and only new workloads get one.
		if _, err := os.Stat(certPath); err == nil && reused && !*reissue {
			continue
		}
		cert, key, err := ca.Issue(n, *validity)
		if err != nil {
			fail(err)
		}
		// Key first, then certificate: a service that reloads in between sees
		// a mismatched pair, fails to load it and keeps its previous one.
		// World-readable on purpose: containers run as an unprivileged user
		// and mount these files. Never do this with real keys.
		write(filepath.Join(*out, n+"-key.pem"), key)
		write(certPath, cert)
		issued++
	}
	if reused {
		fmt.Printf("reused the CA in %s, issued %d new certificate(s)\n", *out, issued)
	} else {
		fmt.Printf("created a new CA and %d certificates in %s (restart running services)\n", issued, *out)
	}
}

func loadCA(certPath, keyPath string) (*mtls.Authority, bool) {
	certPEM, err1 := os.ReadFile(certPath)
	keyPEM, err2 := os.ReadFile(keyPath)
	if err1 != nil || err2 != nil {
		return nil, false
	}
	ca, err := mtls.LoadAuthority(certPEM, keyPEM)
	if err != nil {
		return nil, false
	}
	return ca, true
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "devcerts:", err)
	os.Exit(1)
}
