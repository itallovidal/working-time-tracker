package permission_test

import (
	"slices"
	"testing"

	"working-time-tracker/internal/domain/permission"
)

func TestPresets_AreMadeOfKnownPermissions(t *testing.T) {
	for _, p := range permission.Presets {
		if !permission.Valid(p.Permissions, permission.ProjectKeys) {
			t.Errorf("preset %s has a permission that is not in the project catalog: %v", p.ID, p.Permissions)
		}
	}
	if m, _ := permission.PresetByID(permission.PresetMember); len(m.Permissions) != 0 {
		t.Errorf("the member preset must give nothing, got %v", m.Permissions)
	}
	if a, _ := permission.PresetByID(permission.PresetAdmin); !slices.Equal(a.Permissions, permission.ProjectKeys) {
		t.Errorf("the admin preset must give every project permission, got %v", a.Permissions)
	}
	if _, ok := permission.PresetByID("nope"); ok {
		t.Error("an unknown preset was found")
	}
}

func TestPresetOf_RecognizesGroupsInAnyOrder(t *testing.T) {
	manager, _ := permission.PresetByID(permission.PresetManager)
	reversed := slices.Clone(manager.Permissions)
	slices.Reverse(reversed)
	for name, tc := range map[string]struct {
		keys []string
		want string
	}{
		"empty":        {nil, permission.PresetMember},
		"manager":      {manager.Permissions, permission.PresetManager},
		"other order":  {reversed, permission.PresetManager},
		"repeated":     {append(slices.Clone(manager.Permissions), permission.ProjectEdit), permission.PresetManager},
		"one more":     {append(slices.Clone(manager.Permissions), permission.BillingView), permission.PresetCustom},
		"all":          {permission.ProjectKeys, permission.PresetAdmin},
		"a single key": {[]string{permission.LabelsManage}, permission.PresetCustom},
	} {
		if got := permission.PresetOf(tc.keys); got != tc.want {
			t.Errorf("%s: PresetOf(%v) = %s, want %s", name, tc.keys, got, tc.want)
		}
	}
}

func TestNormalizeAndValid(t *testing.T) {
	got := permission.Normalize([]string{permission.LabelsManage, "bogus", permission.ProjectEdit, permission.LabelsManage}, permission.ProjectKeys)
	if want := []string{permission.ProjectEdit, permission.LabelsManage}; !slices.Equal(got, want) {
		t.Errorf("Normalize = %v, want %v (catalog order, no repeats, no unknown keys)", got, want)
	}
	if permission.Valid([]string{permission.ProjectsCreate}, permission.ProjectKeys) {
		t.Error("an organization permission is not valid at the project scope")
	}
	if !permission.Valid(nil, permission.ProjectKeys) {
		t.Error("an empty list is valid")
	}
}

func TestSet(t *testing.T) {
	none := permission.Set{}
	if none.Has(permission.RatesView) || none.Manages() {
		t.Error("an empty set has nothing")
	}
	some := permission.Set{Keys: []string{permission.RatesView}}
	if !some.Has(permission.RatesView) || some.Has(permission.BillingView) || !some.Manages() || !some.HasAny(permission.BillingView, permission.RatesView) {
		t.Errorf("set %+v answers wrongly", some)
	}
	all := permission.Set{All: true}
	if !all.Has(permission.BillingManage) || !all.Manages() {
		t.Error("the all set has everything")
	}
}
