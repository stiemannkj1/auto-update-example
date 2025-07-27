package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Alternatively just do this after downloading:
// sudo xattr -dr com.apple.quarantine ~/Downloads/pokemon*
// chmod +x ~/Downloads/pokemon*

// Config holds the configuration for certificate and signing
type Config struct {
	CertDir    string
	KeyFile    string
	CertFile   string
	DaysValid  int
	CommonName string
	BinaryPath string
}

// runCommand executes a command and returns its combined output or an error
func runCommand(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("command %s %s failed: %w\nOutput: %s", name, strings.Join(args, " "), err, string(output))
	}
	return output, nil
}

// createCertificate creates a private key and a self-signed certificate
func createCertificate(cfg Config) error {
	// Create the certificate directory if it doesn't exist.
	if _, err := os.Stat(cfg.CertDir); os.IsNotExist(err) {
		fmt.Printf("Creating directory: %s\n", cfg.CertDir)
		if err := os.MkdirAll(cfg.CertDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", cfg.CertDir, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to check directory %s: %w", cfg.CertDir, err)
	} else {
		fmt.Printf("Directory %s already exists.\n", cfg.CertDir)
	}

	// Generate a private key
	fmt.Println("Generating private key...")
	_, err := runCommand("openssl", "genrsa", "-out", cfg.KeyFile, "2048")
	if err != nil {
		return fmt.Errorf("failed to generate private key: %w", err)
	}

	// Generate the self-signed certificate
	fmt.Println("Generating self-signed certificate...")
	subj := fmt.Sprintf("/CN=%s", cfg.CommonName)
	_, err = runCommand("openssl", "req", "-new", "-x509", "-days", fmt.Sprintf("%d", cfg.DaysValid), "-key", cfg.KeyFile, "-out", cfg.CertFile, "-subj", subj)
	if err != nil {
		return fmt.Errorf("failed to generate self-signed certificate: %w", err)
	}

	fmt.Printf("Certificate and key generated:\n")
	fmt.Printf("  Private Key: %s\n", cfg.KeyFile)
	fmt.Printf("  Certificate: %s\n", cfg.CertFile)
	fmt.Printf("  Common Name: %s\n", cfg.CommonName)
	fmt.Printf("  Validity: %d days\n", cfg.DaysValid)
	return nil
}

// signBinary signs a binary using the generated certificate
func signBinary(cfg Config) error {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter the path to the binary you want to sign: ")
	binaryPath, _ := reader.ReadString('\n')
	cfg.BinaryPath = strings.TrimSpace(binaryPath)

	// Check if the binary exists.
	if _, err := os.Stat(cfg.BinaryPath); os.IsNotExist(err) {
		return fmt.Errorf("binary not found at: %s", cfg.BinaryPath)
	}

	// Sign the binary
	fmt.Printf("Signing binary: %s\n", cfg.BinaryPath)
	_, err := runCommand("codesign", "--force", "--sign", cfg.CertFile, cfg.BinaryPath)
	if err != nil {
		return fmt.Errorf("failed to sign binary: %w", err)
	}
	fmt.Println("Binary signed successfully.")
	return nil
}

// verifySignature verifies the signature of a binary
func verifySignature(cfg Config) {
	fmt.Println("Verifying signature...")
	output, err := runCommand("codesign", "-v", cfg.BinaryPath)
	if err != nil {
		fmt.Printf("Signature verification failed. Please check the codesign output below:\n%s\nError: %s\n", string(output), err)
	} else {
		fmt.Printf("Signature verified successfully.\nOutput: %s\n", string(output))
	}
}

func main() {
	config := Config{
		CertDir:    "certs",
		KeyFile:    filepath.Join("certs", "cert.key"),
		CertFile:   filepath.Join("certs", "cert.crt"),
		DaysValid:  365,
		CommonName: "My Self-Signed Code Signing Certificate",
	}

	if err := createCertificate(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating certificate: %v\n", err)
		os.Exit(1)
	}

	if err := signBinary(config); err != nil {
		fmt.Fprintf(os.Stderr, "Error signing binary: %v\n", err)
		os.Exit(1)
	}

	verifySignature(config)

	fmt.Println("Script finished.")
}
