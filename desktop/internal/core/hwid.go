package core

// Hardware-derived device identifier (HWID) for subscription device limits.
// Produces a deterministic 32-char lowercase hex SHA-256 fingerprint from
// stable OS/machine identifiers without requiring root/administrator privileges.

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var (
	hwidOnce   sync.Once
	cachedHWID string
)

// DeviceID returns a stable 32-character hex hardware fingerprint for this machine.
// If dataDir is non-empty and hardware IDs are unavailable (e.g. minimal containers),
// a random fallback is persisted in <dataDir>/device_id (0600).
func DeviceID(dataDir string) string {
	hwidOnce.Do(func() {
		raw := collectHardwareMaterial()
		if raw == "" && dataDir != "" {
			raw = loadOrCreateFallbackID(dataDir)
		}
		if raw == "" {
			host, _ := os.Hostname()
			raw = "fallback:" + runtime.GOOS + ":" + runtime.GOARCH + ":" + host
		}
		sum := sha256.Sum256([]byte("wisteria-hwid-v1:" + raw))
		cachedHWID = hex.EncodeToString(sum[:16])
	})
	return cachedHWID
}

// DeviceName returns a human-readable device label (hostname).
func DeviceName() string {
	h, err := os.Hostname()
	h = strings.TrimSpace(h)
	if err != nil || h == "" {
		return "Wisteria-" + runtime.GOOS
	}
	return h
}

// DeviceOS returns the normalized OS identifier ("linux", "windows", etc.).
func DeviceOS() string {
	return runtime.GOOS
}

func collectHardwareMaterial() string {
	var parts []string
	switch runtime.GOOS {
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if b, err := os.ReadFile(p); err == nil {
				if v := strings.TrimSpace(string(b)); v != "" {
					parts = append(parts, "mid:"+v)
					break
				}
			}
		}
		if b, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
			if v := strings.TrimSpace(string(b)); v != "" {
				parts = append(parts, "uuid:"+strings.ToLower(v))
			}
		}
	case "windows":
		if out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output(); err == nil {
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "MachineGuid") {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						parts = append(parts, "mguid:"+strings.ToLower(fields[len(fields)-1]))
						break
					}
				}
			}
		}
	}
	if len(parts) == 0 {
		if macs := physicalMACs(); len(macs) > 0 {
			parts = append(parts, "macs:"+strings.Join(macs, ","))
		}
	}
	return strings.Join(parts, "|")
}

func physicalMACs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || len(iface.HardwareAddr) == 0 {
			continue
		}
		name := strings.ToLower(iface.Name)
		if strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "veth") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") ||
			strings.HasPrefix(name, "wg") || strings.HasPrefix(name, "tun") ||
			strings.HasPrefix(name, "wisteria") {
			continue
		}
		out = append(out, strings.ToLower(iface.HardwareAddr.String()))
	}
	sort.Strings(out)
	return out
}

func loadOrCreateFallbackID(dataDir string) string {
	p := filepath.Join(dataDir, "device_id")
	if b, err := os.ReadFile(p); err == nil {
		if v := strings.TrimSpace(string(b)); v != "" {
			return "persisted:" + v
		}
	}
	id := newID() + newID()
	_ = os.MkdirAll(dataDir, 0o700)
	_ = os.WriteFile(p, []byte(id+"\n"), 0o600)
	return "persisted:" + id
}
