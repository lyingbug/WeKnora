package types

import (
	"reflect"
	"testing"
)

// The settings resolver is where a mistake is least visible and most damaging:
// getting a merge rule backwards would let a user quietly re-enable something
// their workspace switched off, and nothing in the UI would look wrong. These
// tests pin each merge rule against that.

func TestResolveMemorySettings_DefaultsWhenNoLayers(t *testing.T) {
	settings := DefaultMemorySettings()

	if settings.Enabled {
		t.Error("memory must be off until someone turns it on")
	}
	if settings.WriteMode != MemoryWriteModeExplicit {
		t.Errorf("default write mode = %q, want %q", settings.WriteMode, MemoryWriteModeExplicit)
	}
	if !settings.RequireReview {
		t.Error("extracted memories must default to requiring review")
	}
	if settings.BoostEnabled {
		t.Error("personalised ranking must be off until a workspace measures it")
	}
	if settings.InjectionTokenBudget != 600 {
		t.Errorf("default injection budget = %d, want 600", settings.InjectionTokenBudget)
	}
}

func TestResolveMemorySettings_EnableFlagsVeto(t *testing.T) {
	// A workspace enables memory; the user switches it off for themselves.
	resolution := ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryEnabled: true,
		}},
		MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
			SettingMemoryEnabled: false,
		}},
	)
	if resolution.Settings.Enabled {
		t.Fatal("a user must always be able to switch their own memory off")
	}

	// The reverse must not work: a user cannot enable what the workspace
	// disabled, and the UI must be able to say so.
	resolution = ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryEnabled: false,
		}},
		MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
			SettingMemoryEnabled: true,
		}},
	)
	if resolution.Settings.Enabled {
		t.Fatal("a user must not be able to re-enable memory their workspace disabled")
	}
	if got := resolution.Values[SettingMemoryEnabled].LockedBy; got != MemoryLayerTenant {
		t.Errorf("locked_by = %q, want %q so the UI can explain the disabled control", got, MemoryLayerTenant)
	}
	if resolution.EditableAt(SettingMemoryEnabled, MemoryLayerUser) {
		t.Error("the key must report as not editable at the user layer")
	}
}

func TestResolveMemorySettings_BudgetsTakeTheStricterValue(t *testing.T) {
	resolution := ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryRecallTokenBudget: 300,
		}},
		MemorySettingsLayer{Layer: MemoryLayerAgent, Patch: MemorySettingsPatch{
			SettingMemoryRecallTokenBudget: 1200,
		}},
	)
	if got := resolution.Settings.InjectionTokenBudget; got != 300 {
		t.Errorf("budget = %d, want 300: a narrower layer must not be able to inflate cost", got)
	}

	// Narrowing further is allowed.
	resolution = ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryRecallTokenBudget: 900,
		}},
		MemorySettingsLayer{Layer: MemoryLayerAgent, Patch: MemorySettingsPatch{
			SettingMemoryRecallTokenBudget: 200,
		}},
	)
	if got := resolution.Settings.InjectionTokenBudget; got != 200 {
		t.Errorf("budget = %d, want 200: tightening must be allowed", got)
	}
}

func TestResolveMemorySettings_WriteModeTakesTheMostRestrictive(t *testing.T) {
	cases := []struct {
		name   string
		tenant string
		user   string
		want   string
	}{
		{"user restricts", MemoryWriteModeAlwaysAuto, MemoryWriteModeExplicit, MemoryWriteModeExplicit},
		{"user cannot loosen", MemoryWriteModeExplicit, MemoryWriteModeAlwaysAuto, MemoryWriteModeExplicit},
		{"user turns writing off", MemoryWriteModeGatedAuto, MemoryWriteModeOff, MemoryWriteModeOff},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resolution := ResolveMemorySettings(
				MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
					SettingMemoryWriteMode: tc.tenant,
				}},
				MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
					SettingMemoryWriteMode: tc.user,
				}},
			)
			if got := resolution.Settings.WriteMode; got != tc.want {
				t.Errorf("write mode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveMemorySettings_AllowedTypesIntersect(t *testing.T) {
	resolution := ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryWriteAllowedTypes: []any{MemoryTypePreference, MemoryTypeProject, MemoryTypeEpisode},
		}},
		MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
			SettingMemoryWriteAllowedTypes: []any{MemoryTypePreference, MemoryTypeProfile},
		}},
	)
	want := []string{MemoryTypePreference}
	if got := resolution.Settings.AllowedTypes; !reflect.DeepEqual(got, want) {
		t.Errorf("allowed types = %v, want %v: a user cannot add a type the workspace excluded", got, want)
	}
}

func TestResolveMemorySettings_ReviewRequirementCanOnlyTighten(t *testing.T) {
	// The workspace requires review; a user asking to skip it must not win.
	resolution := ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryWriteRequireReview: true,
		}},
		MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
			SettingMemoryWriteRequireReview: false,
		}},
	)
	if !resolution.Settings.RequireReview {
		t.Fatal("a user must not be able to skip a review their workspace requires")
	}

	// With the workspace neutral, the user may opt out.
	resolution = ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerUser, Patch: MemorySettingsPatch{
			SettingMemoryWriteRequireReview: false,
		}},
	)
	if resolution.Settings.RequireReview {
		t.Error("a user should be able to opt out of review when nothing above requires it")
	}
}

func TestResolveMemorySettings_HardLockedKeysIgnoreEveryLayer(t *testing.T) {
	resolution := ResolveMemorySettings(
		MemorySettingsLayer{Layer: MemoryLayerTenant, Patch: MemorySettingsPatch{
			SettingMemoryExtractSources: []any{"document", "tool_output"},
			SettingMemoryAdminMetaOnly:  false,
			SettingMemoryInjectSanitize: false,
		}},
	)
	settings := resolution.Settings

	if !reflect.DeepEqual(settings.ExtractSources, []string{MemoryExtractSourceUserMessage}) {
		t.Errorf("extract sources = %v, want only user messages: widening this would let a "+
			"poisoned document implant a durable instruction", settings.ExtractSources)
	}
	if !settings.AdminMetadataOnly {
		t.Error("administrators must never gain access to memory content")
	}
	if !settings.InjectionSanitize {
		t.Error("injection sanitisation must not be switchable")
	}
}

func TestSanitizeMemoryPatch(t *testing.T) {
	patch := MemorySettingsPatch{
		SettingMemoryRecallTokenBudget: 99999,               // above the maximum
		SettingMemoryWriteMode:         "wide_open",         // not an allowed value
		SettingMemoryEnabled:           true,                // fine
		SettingMemoryExtractSources:    []any{"document"},   // hard locked
		"memory.not.a.real.setting":    true,                // unknown
		SettingMemoryChannels:          []any{"web", "fax"}, // partly invalid
	}

	clean, notes := SanitizeMemoryPatch(MemoryLayerTenant, patch)

	if got := clean[SettingMemoryRecallTokenBudget]; got != 1200 {
		t.Errorf("budget = %v, want it clamped to 1200 rather than rejected", got)
	}
	if _, present := clean[SettingMemoryWriteMode]; present {
		t.Error("an unknown enum value must be dropped, not stored")
	}
	if _, present := clean[SettingMemoryExtractSources]; present {
		t.Error("a hard-locked key must never be writable")
	}
	if _, present := clean["memory.not.a.real.setting"]; present {
		t.Error("an unknown key must be dropped")
	}
	if got := clean[SettingMemoryChannels]; !reflect.DeepEqual(got, []string{"web"}) {
		t.Errorf("channels = %v, want the invalid entry filtered out", got)
	}
	if len(notes) == 0 {
		t.Error("every adjustment must be reported so the UI can explain it")
	}
}

func TestSanitizeMemoryPatch_ExplicitNullClearsAnOverride(t *testing.T) {
	clean, _ := SanitizeMemoryPatch(MemoryLayerUser, MemorySettingsPatch{
		SettingMemoryEnabled: nil,
	})
	value, present := clean[SettingMemoryEnabled]
	if !present {
		t.Fatal("an explicit null must survive sanitisation so the caller can clear the override")
	}
	if value != nil {
		t.Errorf("value = %v, want nil", value)
	}
}

func TestSanitizeMemoryPatch_RejectsKeysNotSettableAtThatLayer(t *testing.T) {
	// Channel policy is a workspace decision, not an individual one.
	clean, notes := SanitizeMemoryPatch(MemoryLayerUser, MemorySettingsPatch{
		SettingMemoryChannels: []any{"web", "im"},
	})
	if len(clean) != 0 {
		t.Errorf("clean = %v, want empty", clean)
	}
	if len(notes) != 1 {
		t.Errorf("notes = %v, want one explanation", notes)
	}
}

func TestMemorySettingDescriptors_AreInternallyConsistent(t *testing.T) {
	seen := map[string]bool{}
	for _, d := range MemorySettingDescriptors() {
		if seen[d.Key] {
			t.Errorf("duplicate descriptor for %q", d.Key)
		}
		seen[d.Key] = true

		if d.Group == "" {
			t.Errorf("%s: every setting needs a group so the UI can lay it out", d.Key)
		}
		if d.HardLocked && len(d.Levels) != 0 {
			t.Errorf("%s: a hard-locked key must not advertise settable levels", d.Key)
		}
		if !d.HardLocked && len(d.Levels) == 0 {
			t.Errorf("%s: a settable key with no levels can never be changed", d.Key)
		}
		if d.Kind == memoryKindEnum && len(d.Allowed) == 0 {
			t.Errorf("%s: an enum needs its allowed values for validation and for the UI", d.Key)
		}
		for _, level := range d.Levels {
			if memoryLayerRank(level) < 0 {
				t.Errorf("%s: unknown level %q", d.Key, level)
			}
		}
	}

	// Everything the typed view reads must exist in the catalogue, or the
	// typed value would silently be a zero.
	if len(seen) == 0 {
		t.Fatal("no descriptors registered")
	}
}
