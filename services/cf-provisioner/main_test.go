package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInClusterKubeconfig(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.43.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("test-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), []byte("test-ca"), 0o600); err != nil {
		t.Fatal(err)
	}

	data, err := loadInClusterKubeconfig(dir)
	if err != nil {
		t.Fatalf("loadInClusterKubeconfig: %v", err)
	}
	kubeconfig := string(data)
	for _, want := range []string{
		`server: "https://10.43.0.1:443"`,
		`token: "test-token"`,
		`current-context: in-cluster`,
	} {
		if !strings.Contains(kubeconfig, want) {
			t.Fatalf("kubeconfig missing %q:\n%s", want, kubeconfig)
		}
	}
}
