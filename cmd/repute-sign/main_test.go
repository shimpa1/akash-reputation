package main

import "testing"

func TestHexKeyExtraction(t *testing.T) {
	const key = "caf51dc5f9e1831962565537fe4d347bde6acf7addd0c9824eb7a7d4be4a7c3c"
	cases := map[string]string{
		"bare":           key,
		"trailing space": key + "\n",
		"export warning": "WARNING: The private key will be exported as an unarmored hexadecimal string. USE AT YOUR OWN RISK. Continue? [y/N]: y\n" + key + "\n",
		"prompts around": "Enter keyring passphrase (attempt 1/3):\n" + key + "\nDone.\n",
		"0x prefix line": "0x" + key + "\n",
	}
	for name, in := range cases {
		m := hexKeyRE.FindStringSubmatch(in)
		if m == nil {
			t.Fatalf("%s: no key extracted", name)
		}
		if m[1] != key {
			t.Fatalf("%s: extracted %q want %q", name, m[1], key)
		}
	}
	// No false positive on a 63-char hex run.
	if hexKeyRE.FindStringSubmatch("abc "+key[:63]+" xyz") != nil {
		t.Fatal("matched a 63-char hex run")
	}
}
