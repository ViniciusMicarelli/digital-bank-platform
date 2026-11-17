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
	names := flag.String("names", "api-gateway,identity-service,account-service,ledger-service,transaction-service,pix-service,statement-service,dev-client",
		"comma-separated workload identities")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	ca, err := mtls.NewAuthority(365 * 24 * time.Hour)
	if err != nil {
		fail(err)
	}
	write(filepath.Join(*out, "ca.pem"), ca.PEM)
	for _, n := range strings.Split(*names, ",") {
		cert, key, err := ca.Issue(n, 90*24*time.Hour)
		if err != nil {
			fail(err)
		}
		write(filepath.Join(*out, n+".pem"), cert)
		// World-readable on purpose: containers run as an unprivileged user
		// and mount these files. Never do this with real keys.
		write(filepath.Join(*out, n+"-key.pem"), key)
	}
	fmt.Printf("wrote CA and %d certificates to %s\n", len(strings.Split(*names, ",")), *out)
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
