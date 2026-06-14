package adr036

import (
	"strings"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// genKey returns a fresh secp256k1 key and its derived akash address. All test
// addresses are generated, never real on-chain identities.
func genKey(t *testing.T) (*secp256k1.PrivateKey, string) {
	t.Helper()
	priv, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	addr, err := AddressFromPubKey(priv.PubKey().SerializeCompressed())
	if err != nil {
		t.Fatal(err)
	}
	return priv, addr
}

func sampleFeedback(provider, deployer string, role Role) Feedback {
	return Feedback{
		Version:  MessageVersion,
		Role:     role,
		Provider: provider,
		Deployer: deployer,
		DSeq:     "12345",
		GSeq:     "1",
		OSeq:     "1",
		Score:    -1,
		Comment:  "example comment",
		IssuedAt: "2026-01-02T15:04:05Z",
	}
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, author := genKey(t)
	_, counterparty := genKey(t)

	for _, role := range []Role{RoleProvider, RoleDeployer} {
		var f Feedback
		if role == RoleProvider {
			f = sampleFeedback(author, counterparty, role)
		} else {
			f = sampleFeedback(counterparty, author, role)
		}
		if err := f.Validate(); err != nil {
			t.Fatalf("role=%s validate: %v", role, err)
		}
		sig, pub, signer, err := Sign(f, priv)
		if err != nil {
			t.Fatalf("role=%s sign: %v", role, err)
		}
		if signer != author {
			t.Fatalf("role=%s signer=%s want %s", role, signer, author)
		}
		got, err := Verify(f, pub, sig)
		if err != nil {
			t.Fatalf("role=%s verify: %v", role, err)
		}
		if got != author {
			t.Fatalf("role=%s verified author=%s want %s", role, got, author)
		}
	}
}

func TestVerifyRejectsTamper(t *testing.T) {
	priv, author := genKey(t)
	_, deployer := genKey(t)
	f := sampleFeedback(author, deployer, RoleProvider)
	sig, pub, _, err := Sign(f, priv)
	if err != nil {
		t.Fatal(err)
	}

	// Flipping the score must invalidate the signature.
	tampered := f
	tampered.Score = 1
	if _, err := Verify(tampered, pub, sig); err == nil {
		t.Fatal("expected verification to fail on tampered score")
	}

	// Corrupting a signature byte must fail.
	bad := append([]byte(nil), sig...)
	bad[0] ^= 0xff
	if _, err := Verify(f, pub, bad); err == nil {
		t.Fatal("expected verification to fail on corrupted signature")
	}
}

func TestVerifyRejectsWrongAuthor(t *testing.T) {
	priv, _ := genKey(t)
	_, otherProvider := genKey(t)
	_, deployer := genKey(t)
	// Provider-role feedback signed by priv, but the provider field is some
	// other address: the signer is not the claimed author.
	f := sampleFeedback(otherProvider, deployer, RoleProvider)
	sig, pub, _, err := Sign(f, priv)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(f, pub, sig); err == nil {
		t.Fatal("expected verification to fail when signer != author")
	}
}

func TestAddressRoundTrip(t *testing.T) {
	_, addr := genKey(t)
	if !strings.HasPrefix(addr, "akash1") {
		t.Fatalf("address %q lacks akash1 prefix", addr)
	}
	payload, err := DecodeAddress(addr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(payload) != 20 {
		t.Fatalf("payload len=%d want 20", len(payload))
	}
}

// TestSignDocFormat pins the ADR-036 sign-doc shape so we notice if the
// canonical encoding ever drifts from Keplr's signArbitrary format.
func TestSignDocFormat(t *testing.T) {
	_, provider := genKey(t)
	_, deployer := genKey(t)
	f := sampleFeedback(provider, deployer, RoleProvider)
	doc, err := SignDoc(f, f.Author())
	if err != nil {
		t.Fatal(err)
	}
	s := string(doc)
	wantPrefix := `{"account_number":"0","chain_id":"","fee":{"amount":[],"gas":"0"},"memo":"","msgs":[{"type":"sign/MsgSignData","value":{"data":"`
	if !strings.HasPrefix(s, wantPrefix) {
		t.Fatalf("sign doc prefix mismatch:\n got: %s", s)
	}
	if !strings.HasSuffix(s, `,"signer":"`+f.Author()+`"}}],"sequence":"0"}`) {
		t.Fatalf("sign doc suffix mismatch:\n got: %s", s)
	}
}

func TestValidate(t *testing.T) {
	_, provider := genKey(t)
	_, deployer := genKey(t)
	f := sampleFeedback(provider, deployer, RoleProvider)
	f.Score = 2
	if err := f.Validate(); err == nil {
		t.Fatal("expected invalid score to be rejected")
	}
}
