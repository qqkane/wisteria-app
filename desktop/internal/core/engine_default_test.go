package core

import "testing"

func TestEngineDefaultsXray(t *testing.T) {
	// Fresh install: no saved choices — xray across the family.
	fresh := Settings{}
	fresh.defaults()
	for name, v := range map[string]string{
		"ss": fresh.CoreSS, "vmess": fresh.CoreVmess,
		"vless": fresh.CoreVless, "trojan": fresh.CoreTrojan,
	} {
		if v != "xray" {
			t.Errorf("fresh %s engine = %q, want xray", name, v)
		}
	}

	// Settings saved by pre-1.1.3 builds: explicit "sing-box" values that
	// were just the old default — migrate once to xray.
	old := Settings{CoreSS: "sing-box", CoreVmess: "sing-box",
		CoreVless: "sing-box", CoreTrojan: "sing-box", MPCore: "sing-box"}
	old.defaults()
	if old.CoreSS != "xray" || old.CoreVmess != "xray" ||
		old.CoreVless != "xray" || old.CoreTrojan != "xray" || old.MPCore != "xray" {
		t.Errorf("old-default settings must migrate to xray, got ss=%q vmess=%q vless=%q trojan=%q mp=%q",
			old.CoreSS, old.CoreVmess, old.CoreVless, old.CoreTrojan, old.MPCore)
	}
	if !old.EnginesXrayMigration {
		t.Error("migration must set the one-shot marker")
	}

	// After the marker, a deliberate sing-box choice is never touched again.
	kept := Settings{CoreSS: "sing-box", CoreVmess: "xray", EnginesXrayMigration: true}
	kept.defaults()
	if kept.CoreSS != "sing-box" {
		t.Errorf("post-migration sing-box choice must be kept, got %q", kept.CoreSS)
	}
	if kept.CoreVmess != "xray" {
		t.Errorf("post-migration xray choice must be kept, got %q", kept.CoreVmess)
	}
}
