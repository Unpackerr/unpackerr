package unpackerr

import (
	"strings"
	"testing"
)

func TestAllPermissionsIncludeConfigSections(t *testing.T) {
	t.Parallel()

	perms := AllPermissions()

	if !KnownPermission(PermReadSystemStats) || !KnownPermission(PermAll) {
		t.Fatal("system permissions must be known")
	}

	if !KnownPermission(PermReadConfig(SectionSonarr)) || !KnownPermission(PermWriteConfig(SectionWebserver)) {
		t.Fatal("config section permissions must be known")
	}

	if KnownPermission("read:system:nope") {
		t.Fatal("unknown permission")
	}

	if len(perms) < systemPermCount+len(ConfigSections())*2 {
		t.Fatalf("too few permissions: %d", len(perms))
	}
}

func TestAPIKeyValidation(t *testing.T) {
	t.Parallel()

	web := &WebServer{
		Roles: map[string]Role{
			"stats": {Permissions: []string{PermReadSystemStats}},
		},
	}

	if err := (APIKey{Name: "x", Key: "short", Roles: []string{RoleAdmin}}).validate(web.Roles); err == nil {
		t.Fatal("short key")
	}

	long := strings.Repeat("k", apiKeyMinLen)
	if err := (APIKey{Name: "", Key: long, Roles: []string{RoleAdmin}}).validate(web.Roles); err == nil {
		t.Fatal("empty name")
	}

	if err := (APIKey{Name: "home", Key: long, Roles: []string{"missing"}}).validate(web.Roles); err == nil {
		t.Fatal("unknown role")
	}

	if err := (APIKey{Name: "home", Key: long, Roles: []string{"stats"}}).validate(web.Roles); err != nil {
		t.Fatal(err)
	}

	if err := (APIKey{Name: "boss", Key: long, Roles: []string{RoleAdmin}}).validate(nil); err != nil {
		t.Fatal(err)
	}
}

func TestValidateAuthDuplicateKeys(t *testing.T) {
	t.Parallel()

	key := strings.Repeat("a", apiKeyMinLen)
	web := &WebServer{
		APIKeys: []APIKey{
			{Name: "one", Key: key, Roles: []string{RoleAdmin}},
			{Name: "two", Key: key, Roles: []string{RoleAdmin}},
		},
	}

	if err := web.validateAuth(); err == nil {
		t.Fatal("duplicate keys must fail")
	}
}

func TestPermissionsForKeyAdminAndCustom(t *testing.T) {
	t.Parallel()

	adminKey := strings.Repeat("A", apiKeyMinLen)
	statKey := strings.Repeat("S", apiKeyMinLen)
	web := &WebServer{
		APIKeys: []APIKey{
			{Name: "boss", Key: adminKey, Roles: []string{RoleAdmin}},
			{Name: "home", Key: statKey, Roles: []string{"stats"}},
		},
		Roles: map[string]Role{
			"stats": {Permissions: []string{PermReadSystemStats}},
		},
	}

	if err := web.validateAuth(); err != nil {
		t.Fatal(err)
	}

	if !web.HasPermission(adminKey, PermWriteConfig(SectionFolders)) {
		t.Fatal("admin should have every permission")
	}

	if !web.HasPermission(statKey, PermReadSystemStats) {
		t.Fatal("stats role")
	}

	if web.HasPermission(statKey, PermReadSystemMetrics) {
		t.Fatal("stats role must not include metrics")
	}

	if web.HasPermission("nope", PermReadSystemStats) {
		t.Fatal("unknown key")
	}
}

func TestValidateAuthRejectsAdminRoleRedefine(t *testing.T) {
	t.Parallel()

	web := &WebServer{
		Roles: map[string]Role{
			RoleAdmin: {Permissions: []string{PermReadSystemStats}},
		},
	}

	if err := web.validateAuth(); err == nil {
		t.Fatal("redefining admin must fail")
	}
}

func TestValidateAuthRejectsStarAndEmptyRole(t *testing.T) {
	t.Parallel()

	if err := (&WebServer{Roles: map[string]Role{
		"gods": {Permissions: []string{PermAll}},
	}}).validateAuth(); err == nil {
		t.Fatal("custom role with * must fail")
	}

	if err := (&WebServer{Roles: map[string]Role{
		"empty": {Permissions: nil},
	}}).validateAuth(); err == nil {
		t.Fatal("empty role must fail")
	}
}

func TestValidateAuthRejectsDuplicateNamesAndNonASCII(t *testing.T) {
	t.Parallel()

	ascii := strings.Repeat("a", apiKeyMinLen)
	web := &WebServer{
		APIKeys: []APIKey{
			{Name: "home", Key: ascii, Roles: []string{RoleAdmin}},
			{Name: "home", Key: strings.Repeat("b", apiKeyMinLen), Roles: []string{RoleAdmin}},
		},
	}

	if err := web.validateAuth(); err == nil {
		t.Fatal("duplicate names must fail")
	}

	if err := (APIKey{Name: "home", Key: strings.Repeat("é", 60), Roles: []string{RoleAdmin}}).
		validate(nil); err == nil {
		t.Fatal("non-ASCII key must fail")
	}
}

func TestUnknownPermissionOnRole(t *testing.T) {
	t.Parallel()

	web := &WebServer{
		Roles: map[string]Role{"bad": {Permissions: []string{"eat:all:pies"}}},
	}

	if err := web.validateAuth(); err == nil {
		t.Fatal("unknown permission must fail")
	}
}
