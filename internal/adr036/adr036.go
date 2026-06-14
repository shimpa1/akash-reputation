// Package adr036 implements Cosmos SDK ADR-036 off-chain arbitrary-message
// signing and verification for the Akash reputation service.
//
// A feedback message is canonicalised to deterministic JSON, wrapped in the
// well-known amino StdSignDoc with a single sign/MsgSignData message, hashed
// with SHA-256 and signed with secp256k1. This is byte-for-byte compatible with
// Keplr's signArbitrary, so browser wallets can produce signatures this package
// verifies, and vice versa.
//
// The same Sign/Verify pair is used by both the server (verify) and the
// repute-sign CLI (sign), so the two halves can never drift apart.
package adr036

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cosmos/btcutil/bech32"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck // cosmos address derivation requires ripemd160
)

// AddrHRP is the bech32 human-readable prefix for Akash account addresses.
const AddrHRP = "akash"

// MessageVersion is the canonical version tag embedded in every feedback message.
const MessageVersion = "akash-reputation/v1"

// Role identifies which party authored a feedback message.
type Role string

const (
	// RoleProvider means the provider operator is rating a deployer.
	RoleProvider Role = "provider"
	// RoleDeployer means a deployer is rating the provider.
	RoleDeployer Role = "deployer"
)

// Feedback is the canonical, signable reputation statement. The author is the
// party named by Role (provider rates deployer, or deployer rates provider) and
// must equal the address derived from the signing key.
type Feedback struct {
	Version  string `json:"v"`
	Role     Role   `json:"role"`
	Provider string `json:"provider"`
	Deployer string `json:"deployer"`
	DSeq     string `json:"dseq"`
	GSeq     string `json:"gseq"`
	OSeq     string `json:"oseq"`
	Score    int    `json:"score"` // +1 or -1
	Comment  string `json:"comment"`
	IssuedAt string `json:"issued_at"` // RFC3339 UTC
}

// Author returns the address that must have produced the signature for this
// feedback to be valid: the provider for provider-role, the deployer otherwise.
func (f Feedback) Author() string {
	if f.Role == RoleProvider {
		return f.Provider
	}
	return f.Deployer
}

// Subject returns the address the feedback is about (the counterparty).
func (f Feedback) Subject() string {
	if f.Role == RoleProvider {
		return f.Deployer
	}
	return f.Provider
}

// Validate checks structural invariants independent of any signature.
func (f Feedback) Validate() error {
	if f.Version != MessageVersion {
		return fmt.Errorf("unexpected version %q, want %q", f.Version, MessageVersion)
	}
	if f.Role != RoleProvider && f.Role != RoleDeployer {
		return fmt.Errorf("invalid role %q", f.Role)
	}
	if f.Score != 1 && f.Score != -1 {
		return fmt.Errorf("score must be +1 or -1, got %d", f.Score)
	}
	for name, addr := range map[string]string{"provider": f.Provider, "deployer": f.Deployer} {
		if _, err := DecodeAddress(addr); err != nil {
			return fmt.Errorf("invalid %s address: %w", name, err)
		}
	}
	if f.DSeq == "" || f.GSeq == "" || f.OSeq == "" {
		return fmt.Errorf("dseq/gseq/oseq must all be set")
	}
	if f.IssuedAt == "" {
		return fmt.Errorf("issued_at must be set")
	}
	return nil
}

// CanonicalBytes returns the deterministic JSON encoding of the feedback that is
// actually signed (as the ADR-036 message "data"). Keys are sorted, no
// whitespace, so independent implementations produce identical bytes.
func (f Feedback) CanonicalBytes() ([]byte, error) {
	m := map[string]any{
		"v":         f.Version,
		"role":      string(f.Role),
		"provider":  f.Provider,
		"deployer":  f.Deployer,
		"dseq":      f.DSeq,
		"gseq":      f.GSeq,
		"oseq":      f.OSeq,
		"score":     f.Score,
		"comment":   f.Comment,
		"issued_at": f.IssuedAt,
	}
	return marshalCanonical(m)
}

// SignDoc builds the ADR-036 amino StdSignDoc sign bytes for the given feedback
// and signer address. This is the exact byte string a wallet hashes and signs.
func SignDoc(f Feedback, signer string) ([]byte, error) {
	data, err := f.CanonicalBytes()
	if err != nil {
		return nil, err
	}
	doc := map[string]any{
		"account_number": "0",
		"chain_id":       "",
		"fee":            map[string]any{"amount": []any{}, "gas": "0"},
		"memo":           "",
		"msgs": []any{map[string]any{
			"type": "sign/MsgSignData",
			"value": map[string]any{
				"data":   base64.StdEncoding.EncodeToString(data),
				"signer": signer,
			},
		}},
		"sequence": "0",
	}
	return marshalCanonical(doc)
}

// Sign produces a 64-byte [R||S] secp256k1 signature over the ADR-036 sign doc
// for the feedback, using priv. The returned signer address is derived from the
// key and must match feedback.Author() for the signature to be accepted.
func Sign(f Feedback, priv *secp256k1.PrivateKey) (sig []byte, pubKey []byte, signer string, err error) {
	pubKey = priv.PubKey().SerializeCompressed()
	signer, err = AddressFromPubKey(pubKey)
	if err != nil {
		return nil, nil, "", err
	}
	doc, err := SignDoc(f, signer)
	if err != nil {
		return nil, nil, "", err
	}
	hash := sha256.Sum256(doc)
	// SignCompact returns [V || R || S]; cosmos uses the low-S 64-byte R||S.
	compact := ecdsa.SignCompact(priv, hash[:], true)
	return compact[1:], pubKey, signer, nil
}

// Verify checks that sig is a valid secp256k1 signature by pubKey over the
// ADR-036 sign doc for f, and that the address derived from pubKey equals
// f.Author(). It returns the verified author address on success.
func Verify(f Feedback, pubKey, sig []byte) (author string, err error) {
	if len(sig) != 64 {
		return "", fmt.Errorf("signature must be 64 bytes, got %d", len(sig))
	}
	signer, err := AddressFromPubKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("derive signer address: %w", err)
	}
	if signer != f.Author() {
		return "", fmt.Errorf("signer %s is not the message author %s (role=%s)", signer, f.Author(), f.Role)
	}
	pk, err := secp256k1.ParsePubKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("parse pubkey: %w", err)
	}
	doc, err := SignDoc(f, signer)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(doc)
	var r, s secp256k1.ModNScalar
	if overflow := r.SetByteSlice(sig[:32]); overflow {
		return "", fmt.Errorf("signature R overflows curve order")
	}
	if overflow := s.SetByteSlice(sig[32:]); overflow {
		return "", fmt.Errorf("signature S overflows curve order")
	}
	if !ecdsa.NewSignature(&r, &s).Verify(hash[:], pk) {
		return "", fmt.Errorf("signature verification failed")
	}
	return signer, nil
}

// AddressFromPubKey derives the bech32 akash account address from a compressed
// secp256k1 public key, using the standard cosmos derivation
// (ripemd160(sha256(pubkey))).
func AddressFromPubKey(pubKey []byte) (string, error) {
	if _, err := secp256k1.ParsePubKey(pubKey); err != nil {
		return "", fmt.Errorf("invalid pubkey: %w", err)
	}
	sha := sha256.Sum256(pubKey)
	h := ripemd160.New()
	if _, err := h.Write(sha[:]); err != nil {
		return "", err
	}
	addr := h.Sum(nil) // 20 bytes
	conv, err := bech32.ConvertBits(addr, 8, 5, true)
	if err != nil {
		return "", err
	}
	return bech32.Encode(AddrHRP, conv)
}

// DecodeAddress validates an akash bech32 address and returns the 20-byte payload.
func DecodeAddress(addr string) ([]byte, error) {
	hrp, data, err := bech32.Decode(addr, 1023)
	if err != nil {
		return nil, err
	}
	if hrp != AddrHRP {
		return nil, fmt.Errorf("unexpected hrp %q, want %q", hrp, AddrHRP)
	}
	conv, err := bech32.ConvertBits(data, 5, 8, false)
	if err != nil {
		return nil, err
	}
	if len(conv) != 20 {
		return nil, fmt.Errorf("address payload is %d bytes, want 20", len(conv))
	}
	return conv, nil
}

// marshalCanonical marshals v to JSON with lexicographically sorted object keys
// and no insignificant whitespace, matching amino's signable JSON encoding.
func marshalCanonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	return sortedMarshal(generic)
}

func sortedMarshal(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := sortedMarshal(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		return append(buf, '}'), nil
	case []any:
		buf := []byte{'['}
		for i, e := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			eb, err := sortedMarshal(e)
			if err != nil {
				return nil, err
			}
			buf = append(buf, eb...)
		}
		return append(buf, ']'), nil
	default:
		return json.Marshal(v)
	}
}
