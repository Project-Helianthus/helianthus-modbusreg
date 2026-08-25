package modbusreg_test

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vendorEvidenceLocatorDocument struct {
	PublicSources []struct {
		Locator string `json:"locator"`
	} `json:"public_sources"`
}

func TestVendorEvidenceLocatorsArePublicAndRedacted(t *testing.T) {
	paths, err := vendorEvidencePaths()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("vendor evidence corpus is empty")
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document vendorEvidenceLocatorDocument
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if len(document.PublicSources) == 0 {
			t.Fatalf("%s has no public sources", path)
		}
		for index, source := range document.PublicSources {
			if err := validatePublicVendorEvidenceLocator(source.Locator); err != nil {
				t.Fatalf("%s source %d: %v", path, index, err)
			}
		}
	}
}

func TestPublicVendorEvidenceLocatorRejectsPrivateOrSensitiveMaterial(t *testing.T) {
	for name, locator := range map[string]string{
		"private IPv4":       "https://192.168.1.1/evidence",
		"loopback IPv4":      "https://127.0.0.1/evidence",
		"link-local IPv4":    "https://169.254.1.1/evidence",
		"reserved IPv4":      "https://192.0.2.1/evidence",
		"private IPv6":       "https://[fd00::1]/evidence",
		"loopback IPv6":      "https://[::1]/evidence",
		"link-local IPv6":    "https://[fe80::1]/evidence",
		"loopback hostname":  "https://localhost/evidence",
		"local hostname":     "https://evidence.local/evidence",
		"internal hostname":  "https://evidence.internal/evidence",
		"reserved hostname":  "https://evidence.invalid/evidence",
		"single-label host":  "https://evidence/evidence",
		"userinfo":           "https://operator:secret@github.com/evidence",
		"explicit port":      "https://github.com:443/evidence",
		"fragment":           "https://github.com/evidence#private",
		"token query":        "https://github.com/evidence?access_token=value",
		"secret query value": "https://github.com/evidence?proof=contains-secret-material",
		"non-HTTPS":          "http://github.com/evidence",
	} {
		t.Run(name, func(t *testing.T) {
			if err := validatePublicVendorEvidenceLocator(locator); err == nil {
				t.Fatalf("locator was accepted: %q", locator)
			}
		})
	}
}

func TestPublicVendorEvidenceLocatorAllowsBenignPublicMetadata(t *testing.T) {
	if err := validatePublicVendorEvidenceLocator("https://github.com/Project-Helianthus/evidence?author=public"); err != nil {
		t.Fatalf("benign public locator was rejected: %v", err)
	}
}

func vendorEvidencePaths() ([]string, error) {
	var paths []string
	err := filepath.WalkDir("profiles/vendor", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Name() == "evidence.json" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func validatePublicVendorEvidenceLocator(locator string) error {
	parsed, err := url.Parse(locator)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.Hostname() == "" || parsed.User != nil || parsed.Port() != "" ||
		parsed.Fragment != "" || parsed.Path == "" || parsed.Path == "/" {
		return fmt.Errorf("locator is not a public HTTPS document URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if _, err := netip.ParseAddr(host); err == nil || strings.Contains(host, ":") {
		return fmt.Errorf("locator host is an IP literal")
	}
	if host == "localhost" || !strings.Contains(host, ".") ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".home") ||
		strings.HasSuffix(host, ".test") ||
		strings.HasSuffix(host, ".example") ||
		strings.HasSuffix(host, ".invalid") {
		return fmt.Errorf("locator host is not public")
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return fmt.Errorf("locator query is malformed")
	}
	for key, values := range query {
		if containsSensitiveLocatorMaterial(key) {
			return fmt.Errorf("locator query key is sensitive")
		}
		for _, value := range values {
			if containsSensitiveLocatorMaterial(value) {
				return fmt.Errorf("locator query value is sensitive")
			}
		}
	}
	return nil
}

func containsSensitiveLocatorMaterial(value string) bool {
	value = strings.ToLower(value)
	for _, marker := range []string{
		"token", "secret", "password", "credential", "apikey", "api_key",
		"authorization", "session", "signature", "signed", "privatekey", "private_key",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}
