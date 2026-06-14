// Command repute-sign builds, signs and (optionally) submits an Akash
// reputation feedback message using the same ADR-036 format the server verifies.
//
// The signing key is the rater's secp256k1 account key, supplied as unarmored
// hex. Export it from the standard keyring without touching this tool's flags:
//
//	provider-services keys export <name> --unarmored-hex --unsafe | \
//	  repute-sign --role provider --deployer akash1... --dseq 12345 --down \
//	    --comment "slow to pay" --api https://your-reputation-host
//
// With no --api the signed envelope is printed to stdout for inspection or
// out-of-band submission. The address derived from the key must match the party
// named by --role (provider for provider-role, deployer otherwise).
package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/shimpa1/akash-reputation/internal/adr036"
)

type envelope struct {
	Feedback  adr036.Feedback `json:"feedback"`
	PubKey    string          `json:"pubkey"`
	Signature string          `json:"signature"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	role := flag.String("role", "", "who is rating: provider|deployer")
	provider := flag.String("provider", "", "provider akash address")
	deployer := flag.String("deployer", "", "deployer akash address")
	dseq := flag.String("dseq", "", "deployment sequence")
	gseq := flag.String("gseq", "1", "group sequence")
	oseq := flag.String("oseq", "1", "order sequence")
	up := flag.Bool("up", false, "positive feedback (+1)")
	down := flag.Bool("down", false, "negative feedback (-1)")
	score := flag.Int("score", 0, "explicit score (+1 or -1); overrides --up/--down")
	comment := flag.String("comment", "", "optional comment (max 1000 chars)")
	keyHex := flag.String("key-hex", "", "unarmored hex private key (else read REPUTE_KEY_HEX or stdin)")
	api := flag.String("api", "", "reputation API base URL; if set, POST the feedback")
	flag.Parse()

	priv, err := loadKey(*keyHex)
	if err != nil {
		return err
	}

	sc, err := resolveScore(*score, *up, *down)
	if err != nil {
		return err
	}

	signer, err := adr036.AddressFromPubKey(priv.PubKey().SerializeCompressed())
	if err != nil {
		return err
	}

	f := adr036.Feedback{
		Version:  adr036.MessageVersion,
		Role:     adr036.Role(*role),
		Provider: *provider,
		Deployer: *deployer,
		DSeq:     *dseq,
		GSeq:     *gseq,
		OSeq:     *oseq,
		Score:    sc,
		Comment:  *comment,
		IssuedAt: time.Now().UTC().Format(time.RFC3339),
	}
	// Fill in the rater's own side from the key so the caller can't typo it.
	switch f.Role {
	case adr036.RoleProvider:
		if f.Provider == "" {
			f.Provider = signer
		}
	case adr036.RoleDeployer:
		if f.Deployer == "" {
			f.Deployer = signer
		}
	}
	if err := f.Validate(); err != nil {
		return err
	}
	if signer != f.Author() {
		return fmt.Errorf("key address %s does not match %s being rated as author (role=%s)", signer, f.Author(), f.Role)
	}

	sig, pub, _, err := adr036.Sign(f, priv)
	if err != nil {
		return err
	}
	env := envelope{
		Feedback:  f,
		PubKey:    base64.StdEncoding.EncodeToString(pub),
		Signature: base64.StdEncoding.EncodeToString(sig),
	}

	if *api == "" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}
	return submit(*api, env)
}

func resolveScore(score int, up, down bool) (int, error) {
	if score != 0 {
		return score, nil
	}
	switch {
	case up && down:
		return 0, fmt.Errorf("specify only one of --up or --down")
	case up:
		return 1, nil
	case down:
		return -1, nil
	default:
		return 0, fmt.Errorf("specify --up, --down, or --score")
	}
}

func loadKey(flagVal string) (*secp256k1.PrivateKey, error) {
	hexStr := flagVal
	if hexStr == "" {
		hexStr = os.Getenv("REPUTE_KEY_HEX")
	}
	if hexStr == "" {
		// Read piped stdin (e.g. provider-services keys export ... | repute-sign).
		info, _ := os.Stdin.Stat()
		if info.Mode()&os.ModeCharDevice == 0 {
			b, err := io.ReadAll(bufio.NewReader(os.Stdin))
			if err != nil {
				return nil, fmt.Errorf("read key from stdin: %w", err)
			}
			hexStr = string(b)
		}
	}
	if strings.TrimSpace(hexStr) == "" {
		return nil, fmt.Errorf("no private key provided (use --key-hex, REPUTE_KEY_HEX, or pipe via stdin)")
	}
	// Extract the 32-byte hex key, tolerating surrounding noise: `keys export`
	// interleaves a WARNING line and passphrase prompts with the key on the
	// same stream, so pull the standalone 64-hex token rather than decoding the
	// whole blob.
	m := hexKeyRE.FindStringSubmatch(hexStr)
	if m == nil {
		return nil, fmt.Errorf("no 64-char hex private key found in input")
	}
	raw, err := hex.DecodeString(strings.ToLower(m[1]))
	if err != nil {
		return nil, fmt.Errorf("decode key hex: %w", err)
	}
	return secp256k1.PrivKeyFromBytes(raw), nil
}

// hexKeyRE matches a standalone 64-hex-char (32-byte) private key, ignoring any
// surrounding non-hex text.
var hexKeyRE = regexp.MustCompile(`(?:^|[^0-9a-fA-F])([0-9a-fA-F]{64})(?:[^0-9a-fA-F]|$)`)

func submit(api string, env envelope) error {
	body, err := json.Marshal(env)
	if err != nil {
		return err
	}
	url := strings.TrimRight(api, "/") + "/feedback"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %s: %s", resp.Status, strings.TrimSpace(string(respBody)))
	}
	fmt.Println(strings.TrimSpace(string(respBody)))
	return nil
}
